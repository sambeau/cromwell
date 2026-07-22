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
	"cromwell/internal/notify"
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
	Hub             *notify.Hub // realtime fan-out to browsers (DESIGN-007 §6)
	Log             *slog.Logger

	ui      *uiTemplates
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

	tmpl, err := loadUITemplates()
	if err != nil {
		return nil, err
	}
	s := &Server{
		RepoRoot:        repoRoot,
		CompartmentRoot: compRoot,
		Store:           st,
		Bus:             bus.New(256),
		Hub:             notify.NewHub(),
		Log:             log,
		ui:              tmpl,
		bootCfg:         comp.Config,
	}
	s.Dispatcher = &dispatch.Dispatcher{
		Store:        st,
		Bus:          s.Bus,
		Config:       s.freshConfig,
		Planner:      s,
		Tools:        s,
		Providers:    s.providerFor,
		Log:          log,
		OnCheckpoint: s.notifyCheckpointRaised,
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

	// The socket is always the CLI's transport; the TCP listener, when
	// configured (server.http), additionally serves the JSON API and the web
	// command centre (DESIGN-007 §3, FR-1.1). Both carry the same handler —
	// /ui/* is simply unreachable when no TCP listener is open.
	listeners, err := s.listeners()
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
	var serveWG sync.WaitGroup
	serveErr := make(chan error, len(listeners))
	for _, ln := range listeners {
		s.Log.Info("cromwell server listening", "net", ln.Addr().Network(), "addr", ln.Addr().String())
		serveWG.Add(1)
		go func(ln net.Listener) {
			defer serveWG.Done()
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				serveErr <- err
			}
		}(ln)
	}
	serveWG.Wait()
	// Drain: the orchestrator finishes its in-flight action (which may
	// include a server-authored git commit) before we return.
	wg.Wait()
	select {
	case err := <-serveErr:
		return err
	default:
		return nil
	}
}

// listeners opens the transports the server accepts on: always the unix socket
// (the CLI), and — when server.http is configured — a TCP listener for the JSON
// API and the web UI (DESIGN-007 §3). With server.http unset the UI port is
// simply closed and the CLI/socket is unaffected (FR-1.1).
func (s *Server) listeners() ([]net.Listener, error) {
	cfg := s.bootCfg
	sock := cfg.Server.Socket
	if !filepath.IsAbs(sock) {
		sock = filepath.Join(s.RepoRoot, sock)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		return nil, err
	}
	// A leftover socket from a previous run blocks bind; remove it. Two
	// concurrent servers on one project are prevented by advisory lock, not by
	// the socket file.
	_ = os.Remove(sock)
	unixLn, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	out := []net.Listener{unixLn}
	if cfg.Server.HTTP != "" {
		tcpLn, err := net.Listen("tcp", cfg.Server.HTTP)
		if err != nil {
			_ = unixLn.Close()
			return nil, err
		}
		out = append(out, tcpLn)
	}
	return out, nil
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
			// Forward every processed event to the SSE hub (DESIGN-007 §6):
			// the orchestrator is the bus's sole consumer, so the hub is fed
			// here rather than by a second reader that would steal events. This
			// is presentation-only and non-blocking (SD-5) — a broadcast never
			// affects whether or how the event was handled.
			s.Hub.Broadcast(ev)
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
