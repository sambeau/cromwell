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
  cromwell cost [--ref <entity|milestone|roadmap>]   cost rollup (per entity, or --months)
  cromwell search <query>                      full-text search

  cromwell estimate [--ref <entity>]           roll-up: tokens, tier, unestimated work
  cromwell estimate set <ref> <tokens> [--rationale r] [--cite-corpus]
  cromwell estimate ai <ref>                   dispatch the estimator (considered/rough)
  cromwell milestone create <name> [--target-date YYYY-MM-DD]
  cromwell milestone add|remove <name> <member> [--reason r]   member: feature/initiative path or milestone
  cromwell milestone lock <name>               gate G4; snapshots the leaf set
  cromwell milestone list|show <name>
  cromwell roadmap create <name>
  cromwell roadmap add <roadmap> <milestone> [--position n]
  cromwell roadmap show <name>
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
		return costCmd(c, args)
	case "estimate":
		return estimateCmd(c, args)
	case "milestone":
		return milestoneCmd(c, args)
	case "roadmap":
		return roadmapCmd(c, args)
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

func costCmd(c *client.Client, args []string) error {
	fs := flag.NewFlagSet("cost", flag.ContinueOnError)
	ref := fs.String("ref", "", "roll cost up for an entity, milestone, or roadmap")
	months := fs.Bool("months", false, "roll cost up per calendar month")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *months {
		var rows []struct {
			Month   string  `json:"Month"`
			CostUSD float64 `json:"CostUSD"`
			Count   int     `json:"Count"`
		}
		if err := c.Call("GET", "/api/cost/months", nil, &rows); err != nil {
			return err
		}
		for _, m := range rows {
			fmt.Printf("%s  dispatches=%d  $%.4f\n", m.Month, m.Count, m.CostUSD)
		}
		return nil
	}
	if *ref != "" {
		var out struct {
			Ref     string  `json:"ref"`
			Kind    string  `json:"kind"`
			CostUSD float64 `json:"cost_usd"`
		}
		if err := c.Call("GET", "/api/cost/rollup?ref="+url.QueryEscape(*ref), nil, &out); err != nil {
			return err
		}
		fmt.Printf("%-10s %s  $%.4f\n", out.Kind, out.Ref, out.CostUSD)
		return nil
	}
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

// estimateCmd handles `estimate`, `estimate set`, and `estimate ai`.
func estimateCmd(c *client.Client, args []string) error {
	if len(args) >= 1 && (args[0] == "set" || args[0] == "ai") {
		return estimateWriteCmd(c, args)
	}
	fs := flag.NewFlagSet("estimate", flag.ContinueOnError)
	ref := fs.String("ref", "", "entity to roll up (feature/initiative path, or <feature>#<task>)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return fmt.Errorf("usage: cromwell estimate --ref <entity>")
	}
	var e estimateView
	if err := c.Call("GET", "/api/estimate?ref="+url.QueryEscape(*ref), nil, &e); err != nil {
		return err
	}
	printEstimate(*ref, e)
	return nil
}

type estimateView struct {
	RefType      string `json:"ref_type"`
	Tokens       int64  `json:"tokens"`
	Tier         string `json:"tier"`
	Estimated    bool   `json:"estimated"`
	Decomposed   bool   `json:"decomposed"`
	Complete     bool   `json:"complete"`
	ActualTokens int64  `json:"actual_tokens"`
	Delta        int64  `json:"delta"`
	Unestimated  []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"unestimated"`
	History []struct {
		Tokens    int64  `json:"tokens"`
		Tier      string `json:"tier"`
		Rationale string `json:"rationale"`
		CreatedAt string `json:"created_at"`
	} `json:"history"`
}

func printEstimate(ref string, e estimateView) {
	if !e.Estimated {
		fmt.Printf("%s (%s): unestimated\n", ref, e.RefType)
	} else {
		tokens := fmt.Sprintf("%d", e.Tokens)
		if !e.Complete {
			tokens += " + ?" // unestimated work remains
		}
		how := e.Tier
		if e.Decomposed {
			how += ", decomposed"
		}
		fmt.Printf("%s (%s): %s tokens [%s]\n", ref, e.RefType, tokens, how)
		if e.ActualTokens > 0 {
			fmt.Printf("  actual: %d tokens (delta %+d)\n", e.ActualTokens, e.Delta)
		}
	}
	if len(e.Unestimated) > 0 {
		fmt.Println("  unestimated:")
		for _, u := range e.Unestimated {
			fmt.Printf("    ? %s %s\n", u.Type, u.Name)
		}
	}
	if len(e.History) > 1 {
		fmt.Println("  history (current first):")
		for i, h := range e.History {
			marker := " "
			if i == 0 {
				marker = "*"
			}
			fmt.Printf("    %s %d tokens [%s] %s\n", marker, h.Tokens, h.Tier, h.CreatedAt)
		}
	}
}

func estimateWriteCmd(c *client.Client, args []string) error {
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "set":
		if len(rest) < 2 {
			return fmt.Errorf("usage: cromwell estimate set <ref> <tokens> [--rationale r] [--cite-corpus]")
		}
		ref := rest[0]
		var tokens int64
		if _, err := fmt.Sscan(rest[1], &tokens); err != nil {
			return fmt.Errorf("tokens must be a number: %w", err)
		}
		fs := flag.NewFlagSet("estimate set", flag.ContinueOnError)
		rationale := fs.String("rationale", "", "why this estimate")
		cite := fs.Bool("cite-corpus", false, "this estimate cites corpus reference points (considered tier)")
		if err := fs.Parse(rest[2:]); err != nil {
			return err
		}
		var out map[string]any
		if err := c.Call("POST", "/api/estimate/set", map[string]any{
			"ref": ref, "tokens": tokens, "rationale": *rationale, "cite_corpus": *cite,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("estimate recorded: %s = %v tokens [%v]\n", ref, out["tokens"], out["tier"])
		return nil
	case "ai":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell estimate ai <ref>")
		}
		var out map[string]any
		if err := c.Call("POST", "/api/estimate/ai", map[string]string{"ref": rest[0]}, &out); err != nil {
			return err
		}
		fmt.Printf("estimator dispatched for %s (watch `cromwell log`; the estimate lands on completion)\n", rest[0])
		return nil
	}
	return fmt.Errorf("unknown estimate subcommand %q", sub)
}

func milestoneCmd(c *client.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cromwell milestone create|add|remove|lock|list|show ...")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "create":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell milestone create <name> [--target-date YYYY-MM-DD]")
		}
		fs := flag.NewFlagSet("milestone create", flag.ContinueOnError)
		target := fs.String("target-date", "", "target date YYYY-MM-DD")
		if err := fs.Parse(rest[1:]); err != nil {
			return err
		}
		var out map[string]any
		if err := c.Call("POST", "/api/milestones", map[string]string{
			"name": rest[0], "target_date": *target,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("milestone %q created (open)\n", rest[0])
		return nil
	case "add", "remove":
		if len(rest) < 2 {
			return fmt.Errorf("usage: cromwell milestone %s <name> <member> [--reason r]", sub)
		}
		fs := flag.NewFlagSet("milestone member", flag.ContinueOnError)
		reason := fs.String("reason", "", "reason (required to remove — the descope record)")
		if err := fs.Parse(rest[2:]); err != nil {
			return err
		}
		var out map[string]any
		if err := c.Call("POST", "/api/milestones/members", map[string]string{
			"milestone": rest[0], "ref": rest[1], "action": sub, "reason": *reason,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("%s: %s %s\n", rest[0], sub, rest[1])
		return nil
	case "lock":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell milestone lock <name>")
		}
		var out map[string]any
		err := c.Call("POST", "/api/milestones/lock", map[string]string{"milestone": rest[0]}, &out)
		if err != nil {
			var se *client.StatusError
			if ok := errorsAs(err, &se); ok && se.Code == 409 {
				fmt.Printf("refused by G4: %v\n", out["reason"])
				return nil
			}
			return err
		}
		fmt.Printf("milestone %q locked — %v\n", rest[0], out["reason"])
		return nil
	case "list":
		var ms []struct {
			Name  string `json:"Name"`
			State string `json:"State"`
		}
		if err := c.Call("GET", "/api/milestones", nil, &ms); err != nil {
			return err
		}
		for _, m := range ms {
			fmt.Printf("%-10s %s\n", m.State, m.Name)
		}
		return nil
	case "show":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell milestone show <name>")
		}
		var out struct {
			Milestone struct {
				Name  string `json:"Name"`
				State string `json:"State"`
			} `json:"milestone"`
			Progress struct {
				Total int `json:"total"`
				Done  int `json:"done"`
			} `json:"progress"`
			CostUSD float64 `json:"cost_usd"`
		}
		if err := c.Call("GET", "/api/milestone?ref="+url.QueryEscape(rest[0]), nil, &out); err != nil {
			return err
		}
		fmt.Printf("%s [%s]  %d/%d done  $%.4f\n", out.Milestone.Name, out.Milestone.State,
			out.Progress.Done, out.Progress.Total, out.CostUSD)
		return nil
	}
	return fmt.Errorf("unknown milestone subcommand %q", sub)
}

func roadmapCmd(c *client.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cromwell roadmap create|add|show ...")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "create":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell roadmap create <name>")
		}
		var out map[string]any
		if err := c.Call("POST", "/api/roadmaps", map[string]string{"name": rest[0]}, &out); err != nil {
			return err
		}
		fmt.Printf("roadmap %q created\n", rest[0])
		return nil
	case "add":
		if len(rest) < 2 {
			return fmt.Errorf("usage: cromwell roadmap add <roadmap> <milestone> [--position n]")
		}
		fs := flag.NewFlagSet("roadmap add", flag.ContinueOnError)
		position := fs.Int("position", 0, "order position")
		if err := fs.Parse(rest[2:]); err != nil {
			return err
		}
		var out map[string]any
		if err := c.Call("POST", "/api/roadmaps/entries", map[string]any{
			"roadmap": rest[0], "milestone": rest[1], "position": *position,
		}, &out); err != nil {
			return err
		}
		fmt.Printf("%s: %s at position %d\n", rest[0], rest[1], *position)
		return nil
	case "show":
		if len(rest) < 1 {
			return fmt.Errorf("usage: cromwell roadmap show <name>")
		}
		var out struct {
			Roadmap string `json:"roadmap"`
			Entries []struct {
				Position  int    `json:"position"`
				Milestone string `json:"milestone"`
				State     string `json:"state"`
			} `json:"entries"`
		}
		if err := c.Call("GET", "/api/roadmap?ref="+url.QueryEscape(rest[0]), nil, &out); err != nil {
			return err
		}
		fmt.Printf("roadmap %s:\n", out.Roadmap)
		for _, e := range out.Entries {
			fmt.Printf("  %d. %-20s [%s]\n", e.Position, e.Milestone, e.State)
		}
		return nil
	}
	return fmt.Errorf("unknown roadmap subcommand %q", sub)
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
