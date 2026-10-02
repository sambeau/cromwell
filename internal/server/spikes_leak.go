package server

// The leak check (SPEC-021 FR-6.3, SD-2 fact 5, NFR-4): before a spike's
// worktree is discarded, find out whether the run kept its code anywhere else.
// The commits made in the worktree come from its own HEAD reflog, which lives
// in the repository's administrative copy, so a damaged .git file in the
// worktree can't hide them. A ref is a leak when it is new or moved since the
// spike started and holds one of those commits, or is a tag or the stash.
// Anything that stops the check from running is treated as a leak whose ref
// is unknown: a run that tries to keep its code must not pass by breaking git.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// snapshotRefs is every ref in the repository and the commit it names, for
// StartSpike to record.
func (s *Server) snapshotRefs() (map[string]string, error) {
	out, err := gitIn(s.RepoRoot, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if name, sha, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			refs[name] = sha
		}
	}
	return refs, nil
}

// shortRef is how a leaked ref is named to a person: a branch by its name,
// anything else (a tag, the stash) by its full name.
func shortRef(ref string) string {
	if b, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return b
	}
	return ref
}

// backtickJoin names refs, or anything else, in code formatting, joined as
// words.
func backtickJoin(refs []string) string {
	quoted := make([]string, len(refs))
	for i, r := range refs {
		quoted[i] = "`" + r + "`"
	}
	return joinWords(quoted)
}

// spikeKept says what the leak check found for a spike whose run is ending.
// What it finds is audited once, with a checkpoint that asks a person; a later
// call, after the worktree is gone, reads it back from the audit row, so the
// findings still say so. A zero result means nothing was kept and the check
// ran.
func (s *Server) spikeKept(ctx context.Context, sp *store.Spike) (store.SpikeKept, error) {
	prior, err := store.SpikeKeptRecord(ctx, s.Store.Pool, sp.ID)
	if err == nil {
		return *prior, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.SpikeKept{}, err
	}
	kept := s.checkSpikeLeak(sp)
	if len(kept.Refs) == 0 && kept.CouldntCheck == "" {
		return kept, nil
	}
	text := fmt.Sprintf("Code from %s was kept on %s, outside its working copy. A spike's code is never merged. "+
		"Delete the branch, or keep it knowing it won't be built from.", sp.PublicID, backtickJoin(kept.Refs))
	if kept.CouldntCheck != "" {
		text = fmt.Sprintf("Subutai couldn't check whether code from %s was kept outside its working copy: %s. "+
			"A spike's code is never merged. Look for a branch, tag or stash that holds it, and delete it, "+
			"or keep it knowing it won't be built from.", sp.PublicID, kept.CouldntCheck)
	}
	if kept.Refs == nil {
		kept.Refs = []string{}
	}
	payload := map[string]any{"spike_id": sp.ID.String(), "refs": kept.Refs}
	if kept.CouldntCheck != "" {
		payload["couldnt_check"] = kept.CouldntCheck
	}
	var cp *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.RecordSpikeCodeKept(ctx, tx, sp.ID, kept); err != nil {
			return err
		}
		var err error
		// Idempotent: one already pending for the spike returns nil.
		cp, err = store.CreateCheckpoint(ctx, tx, "spike-code-kept", "spike", sp.ID, text, payload)
		return err
	})
	if err != nil {
		return store.SpikeKept{}, err
	}
	s.notifyCheckpointRaised(cp)
	return kept, nil
}

// checkSpikeLeak runs the check and turns a failure into a result.
func (s *Server) checkSpikeLeak(sp *store.Spike) store.SpikeKept {
	refs, err := s.leakedRefs(sp)
	if err != nil {
		s.Log.Warn("spike leak check couldn't run", "spike", sp.PublicID, "err", err)
		return store.SpikeKept{CouldntCheck: err.Error()}
	}
	return store.SpikeKept{Refs: refs}
}

// leakedRefs is FR-6.3. An error is a reason, as a fragment that follows
// "Subutai couldn't check whether code from this spike was kept: ".
func (s *Server) leakedRefs(sp *store.Spike) ([]string, error) {
	if sp.WorktreePath == "" || sp.BaseCommit == "" {
		return nil, nil
	}
	abs := s.worktreeAbs(sp.WorktreePath)
	admin, err := s.worktreeAdminDir(abs)
	if err != nil {
		return nil, err
	}
	if admin == "" {
		// The planner makes an agent spike's working copy before the run's
		// first call, and a chat or person spike's is made at the start, so a
		// spike that had one (spikeHadWorkingCopy) lost it. Only one that
		// never did, and has no directory, was never made.
		if _, err := os.Stat(abs); errors.Is(err, fs.ErrNotExist) {
			if spikeHadWorkingCopy(sp) {
				return nil, errors.New("the working copy was removed, so git's record of it is gone")
			}
			return nil, nil
		}
		return nil, errors.New("git has no record of the working copy")
	}
	made, last, err := worktreeCommits(admin)
	if err != nil {
		return nil, err
	}
	head, err := s.worktreeHead(admin)
	if err != nil {
		return nil, err
	}
	if head != last {
		return nil, errors.New("the working copy's HEAD reflog doesn't end where HEAD is")
	}
	if sp.RefsAtStart == nil {
		return nil, errors.New("the refs from when the spike started weren't recorded")
	}
	var commits map[string]bool
	if len(made) > 0 {
		// Less anything that was already on a ref when the spike started.
		// On stdin: a repository with many refs would overflow the command line.
		lines := append(slices.Clone(made), "^"+sp.BaseCommit)
		for _, sha := range sp.RefsAtStart {
			lines = append(lines, "^"+sha)
		}
		out, err := gitInWithStdin(s.RepoRoot, strings.Join(lines, "\n")+"\n", "rev-list", "--stdin")
		if err != nil {
			return nil, fmt.Errorf("git couldn't list the commits made there (%v)", err)
		}
		commits = setOf(strings.Fields(out))
	}
	now, err := s.snapshotRefs()
	if err != nil {
		return nil, fmt.Errorf("git couldn't list the refs (%v)", err)
	}
	var leaked []string
	for name, sha := range now {
		if sp.RefsAtStart[name] == sha {
			continue // not new, and not moved
		}
		switch {
		case name == "refs/stash" || strings.HasPrefix(name, "refs/tags/"):
			// Always a leak when changed: nothing to look up, so it falls
			// through to be reported.
		case len(commits) == 0:
			continue
		default:
			out, err := gitIn(s.RepoRoot, "rev-list", name, "--not", sp.BaseCommit)
			if err != nil {
				return nil, fmt.Errorf("git couldn't list what %s holds (%v)", name, err)
			}
			if !slices.ContainsFunc(strings.Fields(out), func(c string) bool { return commits[c] }) {
				continue
			}
		}
		leaked = append(leaked, shortRef(name))
	}
	slices.Sort(leaked)
	return leaked, nil
}

func setOf(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

// worktreeAdminDir finds git's administrative directory for a worktree, by the
// gitdir file each one holds, rather than by trusting the worktree's own .git
// file. It is "" when git has no worktree there.
func (s *Server) worktreeAdminDir(abs string) (string, error) {
	out, err := gitIn(s.RepoRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("git couldn't find its directory (%v)", err)
	}
	common := strings.TrimSpace(out)
	if !filepath.IsAbs(common) {
		common = filepath.Join(s.RepoRoot, common)
	}
	entries, err := os.ReadDir(filepath.Join(common, "worktrees"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("git's list of working copies couldn't be read (%s)", reason(err))
	}
	want := canonPath(filepath.Join(abs, ".git"))
	for _, e := range entries {
		dir := filepath.Join(common, "worktrees", e.Name())
		if raw, err := os.ReadFile(filepath.Join(dir, "gitdir")); err == nil &&
			canonPath(strings.TrimSpace(string(raw))) == want {
			return dir, nil
		}
	}
	return "", nil
}

// canonPath is a path with its symbolic links resolved as far as they exist,
// so two spellings of the same place compare equal.
func canonPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(r, filepath.Base(p))
	}
	return filepath.Clean(p)
}

// reason is an error without the path in it.
func reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// worktreeHead is the commit the worktree's HEAD names now, read from the
// administrative directory.
func (s *Server) worktreeHead(admin string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(admin, "HEAD"))
	if err != nil {
		return "", fmt.Errorf("the working copy's HEAD couldn't be read (%s)", reason(err))
	}
	head := strings.TrimSpace(string(raw))
	if ref, ok := strings.CutPrefix(head, "ref: "); ok {
		out, err := gitIn(s.RepoRoot, "rev-parse", "--verify", "-q", ref)
		if err != nil {
			return "", fmt.Errorf("git couldn't read where the working copy's branch is (%v)", err)
		}
		head = strings.TrimSpace(out)
	}
	if !isSHA(head) {
		return "", errors.New("the working copy's HEAD isn't a commit")
	}
	return head, nil
}

// worktreeCommits is every commit the worktree's HEAD has been at, from its
// reflog in the administrative directory, and the last of them. The reflog
// must hang together: it has an entry and every line reads, and the caller
// checks that its last entry is where HEAD is now. One that was truncated,
// deleted or switched off fails that, and the check fails closed.
func worktreeCommits(admin string) (shas []string, last string, err error) {
	raw, err := os.ReadFile(filepath.Join(admin, "logs", "HEAD"))
	if err != nil {
		return nil, "", fmt.Errorf("the working copy's HEAD reflog couldn't be read (%s)", reason(err))
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := strings.SplitN(line, "\t", 2)
		f := strings.Fields(entry[0])
		if len(f) < 2 || !isSHA(f[0]) || !isSHA(f[1]) {
			return nil, "", errors.New("the working copy's HEAD reflog has a line that can't be read")
		}
		last = f[1]
		// A checkout or a reset only moves HEAD to a commit made elsewhere,
		// such as a newer main; every other entry made the commit it names.
		if len(entry) == 2 && (strings.HasPrefix(entry[1], "checkout:") || strings.HasPrefix(entry[1], "reset:")) {
			continue
		}
		if strings.Trim(last, "0") != "" && !slices.Contains(shas, last) {
			shas = append(shas, last)
		}
	}
	if last == "" {
		return nil, "", errors.New("the working copy's HEAD reflog is empty")
	}
	return shas, last, nil
}

func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
