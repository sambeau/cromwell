// Package client is the CLI's HTTP client. It talks only to the server's
// API over the unix socket (O-1, FR-2.2) — it must never import store or
// pgx; a test enforces that boundary.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"subutai/internal/config"
)

type Client struct {
	http  *http.Client
	base  string
	actor string
}

// FindRepoRoot walks up from dir to the directory containing .subutai/.
func FindRepoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".subutai")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no .subutai/ found from %s upward; run `subutai init` first", dir)
		}
		abs = parent
	}
}

// New builds a client for the project at repoRoot, reading the socket path
// from config.yaml (the CLI reads config files, never the database).
func New(repoRoot string) (*Client, error) {
	cfg, err := config.LoadConfig(filepath.Join(repoRoot, ".subutai"))
	if err != nil {
		return nil, err
	}
	c := &Client{actor: detectActor(repoRoot)}
	if cfg.Server.HTTP != "" {
		c.base = "http://" + cfg.Server.HTTP
		c.http = &http.Client{}
		return c, nil
	}
	sock := cfg.Server.Socket
	if !filepath.IsAbs(sock) {
		sock = filepath.Join(repoRoot, sock)
	}
	c.base = "http://subutai"
	c.http = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}}
	return c, nil
}

func detectActor(repoRoot string) string {
	cmd := exec.Command("git", "config", "user.name")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

// Call performs one API request. A connection error becomes the clear
// "server not running" message FR-2.2 requires.
func (c *Client) Call(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Subutai-Actor", c.actor)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("subutai server not running (start it with `subutai serve`): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			// 4xx carrying a structured report (e.g. validation failure)
			// still reaches out below when the caller wants it.
			if out != nil && resp.StatusCode == 422 {
				_ = json.Unmarshal(data, out)
			}
			return fmt.Errorf("%s", apiErr.Error)
		}
		if out != nil {
			if err := json.Unmarshal(data, out); err == nil {
				return &StatusError{Code: resp.StatusCode}
			}
		}
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// StatusError carries a non-2xx status whose body was still decodable into
// the caller's out value (validation reports, gate refusals).
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("status %d", e.Code) }
