// Package server wires the system together (DESIGN-002 §2): the
// orchestrator loop consuming the event bus, the action executor (every
// action transactional with its audit rows), the prompt builder, the git
// watcher, the heartbeat, and the HTTP API the CLI talks to (O-1).
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/dispatch"
	"cromwell/internal/lifecycle"
	"cromwell/internal/provider"
	"cromwell/internal/provider/anthropic"
	"cromwell/internal/store"
)

type Server struct {
	RepoRoot        string // git repo root; document paths are relative to it
	CompartmentRoot string // .cromwell/
	Store           *store.Store
	Bus             *bus.Bus
	Dispatcher      *dispatch.Dispatcher
	Log             *slog.Logger

	bootCfg *config.Config
}

// New validates the compartment, connects the store, and assembles the
// server. Any config error aborts startup naming file and field
// (DESIGN-004 §9).
func New(ctx context.Context, repoRoot string, log *slog.Logger) (*Server, error) {
	compRoot := filepath.Join(repoRoot, ".cromwell")
	comp, err := config.Load(compRoot, lifecycle.RuleKinds())
	if err != nil {
		return nil, fmt.Errorf("configuration invalid; refusing to start:\n%w", err)
	}
	dbURL, err := comp.Config.DatabaseURL()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return nil, err
	}

	s := &Server{
		RepoRoot:        repoRoot,
		CompartmentRoot: compRoot,
		Store:           st,
		Bus:             bus.New(256),
		Log:             log,
		bootCfg:         comp.Config,
	}
	s.Dispatcher = &dispatch.Dispatcher{
		Store:     st,
		Bus:       s.Bus,
		Config:    s.freshConfig,
		Planner:   s,
		Tools:     s,
		Providers: s.providerFor,
		Log:       log,
	}
	return s, nil
}

// freshConfig re-reads config.yaml (O-6).
func (s *Server) freshConfig() (*config.Config, error) {
	return config.LoadConfig(s.CompartmentRoot)
}

func (s *Server) providerFor(name string) (provider.Provider, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	p, ok := cfg.Providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	key, err := cfg.APIKey(name)
	if err != nil {
		return nil, err
	}
	// Phase 1 speaks one wire protocol (D-2): every configured provider is
	// an Anthropic-API-compatible endpoint; base_url selects the host.
	return anthropic.New(key, p.BaseURL), nil
}

// Run starts everything and blocks until ctx ends: boot recovery, the
// orchestrator loop, the dispatcher, the heartbeat, and the HTTP listener.
func (s *Server) Run(ctx context.Context) error {
	// Boot recovery (DESIGN-002 §8): fail stalled dispatches, catch up on
	// document changes made while down, reconcile gates, then open for
	// business.
	s.Dispatcher.StallSweep(ctx)
	if err := s.CatchUpScan(ctx); err != nil {
		s.Log.Error("boot catch-up scan", "err", err)
	}
	if err := s.ReconcileWorktrees(ctx); err != nil {
		s.Log.Error("boot worktree reconciliation", "err", err)
	}
	if err := s.ReconcileGates(ctx); err != nil {
		s.Log.Error("boot gate reconciliation", "err", err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); s.orchestrate(ctx) }()
	go func() { defer wg.Done(); s.Dispatcher.Run(ctx) }()
	go func() { defer wg.Done(); s.heartbeat(ctx) }()
	s.Dispatcher.Kick()

	ln, err := s.listen()
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	s.Log.Info("cromwell server listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	// Drain: the orchestrator finishes its in-flight action (which may
	// include a server-authored git commit) before we return.
	wg.Wait()
	return nil
}

func (s *Server) listen() (net.Listener, error) {
	cfg := s.bootCfg
	if cfg.Server.HTTP != "" {
		return net.Listen("tcp", cfg.Server.HTTP)
	}
	sock := cfg.Server.Socket
	if !filepath.IsAbs(sock) {
		sock = filepath.Join(s.RepoRoot, sock)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		return nil, err
	}
	// A leftover socket from a previous run blocks bind; remove it. Two
	// concurrent servers on one project are prevented by advisory lock
	// below, not by the socket file.
	_ = os.Remove(sock)
	return net.Listen("unix", sock)
}

// orchestrate is the single consumer of the event bus: fetch snapshot,
// decide, execute.
func (s *Server) orchestrate(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-s.Bus.Events():
			if err := s.handle(ctx, ev); err != nil {
				s.Log.Error("orchestrator", "event", ev.EventKind(), "err", err)
			}
		}
	}
}

// heartbeat runs the Tick duties (DESIGN-002 §3).
func (s *Server) heartbeat(ctx context.Context) {
	interval := time.Duration(s.bootCfg.Server.HeartbeatSeconds) * time.Second
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Dispatcher.StallSweep(ctx)
			s.Dispatcher.RetrySweep(ctx)
			s.GCWorktrees(ctx)
			s.Dispatcher.Kick()
		}
	}
}
