// Command cromwell is the single binary (DEC-001): `serve` runs the server;
// every other subcommand is an API client over the unix socket (O-1), except
// `init`, which applies migrations before any server exists (DESIGN-002 §3).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"cromwell/internal/client"
	"cromwell/internal/server"
	"cromwell/internal/starter"
)

const usage = `cromwell — document-led, specification-centred workflow

Usage:
  cromwell init                                initialise this repo (once)
  cromwell serve                               run the server
  cromwell status                              server, queue, inbox summary

  cromwell initiative add <path> --name <n>    create an initiative ("auth" or "auth/basic")
  cromwell initiative archive <path>           archive (gate G5; may raise a checkpoint)
  cromwell feature add <init-path>/<slug> --name <n>
  cromwell feature abandon <path> --reason <r>
  cromwell feature start <path>                begin work (creates a worktree, dispatches tasks)
  cromwell task list <feature-path>            show a feature's tasks and states

  cromwell doc add <file> --type spec|dev_plan --owner <feature-path>|project|<init-path>
  cromwell doc comments <file>                 show the comment thread
  cromwell validate <file>                     run validation without submitting
  cromwell submit <file>                       validate + enter review
  cromwell revise <file>                       create a revision draft of an approved doc

  cromwell inbox                               pending checkpoints
  cromwell respond <id> <answer> [--reason r]  answer a checkpoint (approve | request_changes |
                                               override | deny | retry | cancel | proceed | continue | pause)
  cromwell log [--ref type:id] [--limit n]     audit stream
  cromwell cost                                cost rollup
  cromwell search <query>                      full-text search
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if err := run(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	switch cmd {
	case "init":
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if err := starter.Init(context.Background(), cwd, exe); err != nil {
			return err
		}
		fmt.Println("initialised .cromwell/ — set ANTHROPIC_API_KEY, then run `cromwell serve`")
		return nil

	case "serve":
		return serve()

	case "hook":
		// Internal: invoked by the git post-commit hook. Silent by design.
		fs := flag.NewFlagSet("hook", flag.ContinueOnError)
		repo := fs.String("repo", ".", "repo root")
		_ = fs.Parse(skipFirst(args)) // args[0] == "post-commit"
		c, err := clientFor(*repo)
		if err != nil {
			return nil
		}
		_ = c.Call("POST", "/api/hook/post-commit", map[string]any{}, nil)
		return nil
	}

	// Everything else is a pure API client.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := client.FindRepoRoot(cwd)
	if err != nil {
		return err
	}
	c, err := client.New(root)
	if err != nil {
		return err
	}

	switch cmd {
	case "status":
		return getJSON(c, "/api/status")
	case "initiative":
		return initiativeCmd(c, args)
	case "feature":
		return featureCmd(c, args)
	case "task":
		return taskCmd(c, args)
	case "doc":
		return docCmd(c, args)
	case "validate":
		return validateCmd(c, args, "/api/docs/validate")
	case "submit":
		return submitCmd(c, args)
	case "revise":
		if len(args) < 1 {
			return fmt.Errorf("usage: cromwell revise <file>")
		}
		var out map[string]any
		if err := c.Call("POST", "/api/docs/revise", map[string]string{"path": args[0]}, &out); err != nil {
			return err
		}
		fmt.Printf("revision draft created at %s (supersedes the approved doc on its own approval)\n", out["Path"])
		return nil
	case "inbox":
		return inboxCmd(c)
	case "respond":
		return respondCmd(c, args)
	case "log":
		return logCmd(c, args)
	case "cost":
		return costCmd(c)
	case "search":
		if len(args) < 1 {
			return fmt.Errorf("usage: cromwell search <query>")
		}
		return getJSON(c, "/api/search?q="+url.QueryEscape(strings.Join(args, " ")))
	default:
		fmt.Print(usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func serve() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := client.FindRepoRoot(cwd)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv, err := server.New(ctx, root, log)
	if err != nil {
		return err
	}
	defer srv.Store.Close()
	return srv.Run(ctx)
}

func clientFor(repoRoot string) (*client.Client, error) {
	root, err := client.FindRepoRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	return client.New(root)
}

func getJSON(c *client.Client, path string) error {
	var out any
	if err := c.Call("GET", path, nil, &out); err != nil {
		return err
	}
	return printJSON(out)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func initiativeCmd(c *client.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cromwell initiative add|archive <path> [flags]")
	}
	sub, path := args[0], args[1]
	fs := flag.NewFlagSet("initiative", flag.ContinueOnError)
	name := fs.String("name", "", "display name")
	desc := fs.String("description", "", "description")
	reason := fs.String("reason", "", "reason (archive)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	switch sub {
	case "add":
		parent, slug := splitParent(path)
		if *name == "" {
			*name = slug
		}
		var out map[string]any
		if err := c.Call("POST", "/api/initiatives", map[string]string{
			"parent_path": parent, "slug": slug, "name": *name, "description": *desc,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("initiative %s created\n", path)
		return nil
	case "archive":
		var out map[string]any
		err := c.Call("POST", "/api/initiatives/archive", map[string]string{
			"path": path, "reason": *reason,
		}, &out)
		if err != nil {
			var se *client.StatusError
			if ok := errorsAs(err, &se); ok && se.Code == 409 {
				fmt.Printf("blocked: %v — %v\n", out["reason"], out["note"])
				return nil
			}
			return err
		}
		fmt.Printf("initiative %s archived\n", path)
		return nil
	}
	return fmt.Errorf("unknown initiative subcommand %q", sub)
}

func featureCmd(c *client.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cromwell feature add|abandon <path> [flags]")
	}
	sub, path := args[0], args[1]
	fs := flag.NewFlagSet("feature", flag.ContinueOnError)
	name := fs.String("name", "", "display name")
	desc := fs.String("description", "", "description")
	reason := fs.String("reason", "", "reason (abandon)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	switch sub {
	case "add":
		initiativePath, slug := splitParent(path)
		if initiativePath == "" {
			return fmt.Errorf("feature path must be <initiative-path>/<slug>")
		}
		if *name == "" {
			*name = slug
		}
		var out map[string]any
		if err := c.Call("POST", "/api/features", map[string]string{
			"initiative_path": initiativePath, "slug": slug, "name": *name, "description": *desc,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("feature %s created (idea)\n", path)
		return nil
	case "abandon":
		var out map[string]any
		if err := c.Call("POST", "/api/features/abandon", map[string]string{
			"path": path, "reason": *reason,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("feature %s abandoned\n", path)
		return nil
	case "start":
		var out map[string]any
		if err := c.Call("POST", "/api/features/start", map[string]string{"path": path}, &out); err != nil {
			return err
		}
		fmt.Printf("feature %s started (active); worktree created, tasks dispatching\n", path)
		return nil
	}
	return fmt.Errorf("unknown feature subcommand %q", sub)
}

func taskCmd(c *client.Client, args []string) error {
	if len(args) < 2 || args[0] != "list" {
		return fmt.Errorf("usage: cromwell task list <feature-path>")
	}
	var tasks []struct {
		LocalID   string `json:"LocalID"`
		Title     string `json:"Title"`
		State     string `json:"State"`
		DependsOn []any  `json:"DependsOn"`
	}
	if err := c.Call("GET", "/api/tasks?path="+args[1], nil, &tasks); err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("no tasks (approve a dev-plan to decompose)")
		return nil
	}
	for _, t := range tasks {
		deps := ""
		if len(t.DependsOn) > 0 {
			deps = fmt.Sprintf("  (deps: %d)", len(t.DependsOn))
		}
		fmt.Printf("%-4s %-10s %s%s\n", t.LocalID, t.State, t.Title, deps)
	}
	return nil
}

func docCmd(c *client.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cromwell doc add|comments <file> [flags]")
	}
	sub, path := args[0], args[1]
	fs := flag.NewFlagSet("doc", flag.ContinueOnError)
	docType := fs.String("type", "spec", "document type")
	owner := fs.String("owner", "project", "owner: project, <initiative-path>, or <feature-path>")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	switch sub {
	case "add":
		ownerType, ownerRef := "project", ""
		if *owner != "project" {
			ownerRef = *owner
			// A path whose last segment names a feature is a feature owner;
			// the server resolves and errors precisely, so try feature first
			// only when the path has depth.
			if strings.Contains(*owner, "/") {
				ownerType = "feature"
			} else {
				ownerType = "initiative"
			}
		}
		var out map[string]any
		if err := c.Call("POST", "/api/docs", map[string]string{
			"path": path, "type": *docType, "owner_type": ownerType, "owner_ref": ownerRef,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("registered %s as %s (draft)\n", path, *docType)
		return nil
	case "comments":
		return getJSON(c, "/api/docs/comments?path="+path)
	}
	return fmt.Errorf("unknown doc subcommand %q", sub)
}

func validateCmd(c *client.Client, args []string, endpoint string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cromwell validate <file>")
	}
	var report struct {
		Valid  bool `json:"valid"`
		Issues []struct {
			Check  string `json:"check"`
			Detail string `json:"detail"`
		} `json:"issues"`
	}
	if err := c.Call("POST", endpoint, map[string]string{"path": args[0]}, &report); err != nil {
		return err
	}
	if report.Valid {
		fmt.Println("valid")
		return nil
	}
	fmt.Println("invalid:")
	for _, is := range report.Issues {
		fmt.Printf("  [%s] %s\n", is.Check, is.Detail)
	}
	os.Exit(1)
	return nil
}

func submitCmd(c *client.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cromwell submit <file>")
	}
	var out struct {
		Report struct {
			Valid  bool `json:"valid"`
			Issues []struct {
				Check  string `json:"check"`
				Detail string `json:"detail"`
			} `json:"issues"`
		} `json:"report"`
		State string `json:"state"`
	}
	err := c.Call("POST", "/api/docs/submit", map[string]string{"path": args[0]}, &out)
	if err != nil {
		var se *client.StatusError
		if !errorsAs(err, &se) {
			return err
		}
	}
	if !out.Report.Valid {
		fmt.Println("validation failed; not submitted:")
		for _, is := range out.Report.Issues {
			fmt.Printf("  [%s] %s\n", is.Check, is.Detail)
		}
		os.Exit(1)
	}
	fmt.Println("submitted: reviewing (agent review queued)")
	return nil
}

func inboxCmd(c *client.Client) error {
	var pending []struct {
		ID        string          `json:"ID"`
		Kind      string          `json:"Kind"`
		Question  string          `json:"Question"`
		Context   json.RawMessage `json:"Context"`
		CreatedAt string          `json:"CreatedAt"`
	}
	if err := c.Call("GET", "/api/inbox", nil, &pending); err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Println("inbox empty")
		return nil
	}
	for _, cp := range pending {
		fmt.Printf("%s  [%s]\n  %s\n  context: %s\n  since: %s\n\n",
			cp.ID, cp.Kind, cp.Question, cp.Context, cp.CreatedAt)
	}
	return nil
}

// respondCmd maps the human-friendly answer word to the response shape the
// rule engine expects for each checkpoint kind.
func respondCmd(c *client.Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cromwell respond <checkpoint-id> <answer> [--reason r]")
	}
	id, answer := args[0], args[1]
	fs := flag.NewFlagSet("respond", flag.ContinueOnError)
	reason := fs.String("reason", "", "reason for the decision")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	response := map[string]any{}
	switch answer {
	case "approve", "request_changes":
		response["decision"] = answer
	case "override":
		response["override"] = true
	case "deny":
		response["override"] = false
	case "retry":
		response["retry"] = true
	case "cancel":
		response["retry"] = false
	case "proceed":
		response["proceed"] = true
	case "continue":
		response["continue"] = true
	case "pause":
		response["continue"] = false
	default:
		response["answer"] = answer
	}
	if *reason != "" {
		response["reason"] = *reason
	}
	var out map[string]any
	if err := c.Call("POST", "/api/respond", map[string]any{"id": id, "response": response}, &out); err != nil {
		return err
	}
	fmt.Println("responded; the orchestrator resumes")
	return nil
}

func logCmd(c *client.Client, args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	ref := fs.String("ref", "", "filter: ref_type or ref_type:ref_id")
	limit := fs.Int("limit", 50, "max rows")
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := fmt.Sprintf("/api/log?limit=%d", *limit)
	if *ref != "" {
		refType, refID, _ := strings.Cut(*ref, ":")
		q += "&ref_type=" + refType
		if refID != "" {
			q += "&ref_id=" + refID
		}
	}
	var events []struct {
		OccurredAt string          `json:"OccurredAt"`
		Actor      string          `json:"Actor"`
		Kind       string          `json:"Kind"`
		RefType    string          `json:"RefType"`
		Payload    json.RawMessage `json:"Payload"`
	}
	if err := c.Call("GET", q, nil, &events); err != nil {
		return err
	}
	for _, e := range events {
		fmt.Printf("%s  %-22s %-10s %-14s %s\n", e.OccurredAt, e.Kind, e.RefType, e.Actor, e.Payload)
	}
	return nil
}

func costCmd(c *client.Client) error {
	var out struct {
		TotalUSD float64 `json:"total_usd"`
		ByEntity []struct {
			RefType string  `json:"RefType"`
			RefID   string  `json:"RefID"`
			Count   int     `json:"Count"`
			CostUSD float64 `json:"CostUSD"`
			Tokens  int64   `json:"Tokens"`
		} `json:"by_entity"`
	}
	if err := c.Call("GET", "/api/cost", nil, &out); err != nil {
		return err
	}
	for _, row := range out.ByEntity {
		fmt.Printf("%-10s %s  dispatches=%d  tokens=%d  $%.4f\n",
			row.RefType, row.RefID, row.Count, row.Tokens, row.CostUSD)
	}
	fmt.Printf("total: $%.4f\n", out.TotalUSD)
	return nil
}

func splitParent(path string) (parent, last string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

func skipFirst(args []string) []string {
	if len(args) > 0 {
		return args[1:]
	}
	return args
}

func errorsAs[T error](err error, target *T) bool {
	for err != nil {
		if t, ok := err.(T); ok {
			*target = t
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
