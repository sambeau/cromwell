package server

// Unit tests of the claim service's pure parts (SPEC-020): no database.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

func TestExecutorSentence(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	run, claimID := uuid.New(), uuid.New()
	agent := func(round int, done bool) store.Execution {
		e := store.Execution{Kind: "agent", Actor: "implementer", Model: "claude-sonnet-5", Round: round, Measured: true, DispatchID: &run}
		if done {
			t := now
			e.SubmittedAt = &t
		}
		return e
	}
	chat := func(round int, done bool) store.Execution {
		e := store.Execution{Kind: "chat", Actor: "chat-agent", Round: round, ClaimID: &claimID}
		if done {
			t := now
			e.SubmittedAt = &t
		}
		return e
	}
	person := store.Execution{Kind: "person", Actor: "sam", Round: 1, ClaimID: &claimID}
	task := func(s lifecycle.TaskState) *store.Task { return &store.Task{State: s} }
	cases := []struct {
		name  string
		task  *store.Task
		execs []store.Execution
		claim *store.Claim
		want  string
	}{
		{"nobody", task(lifecycle.TaskReady), nil, nil, "Nobody has started this task yet."},
		{"agent working", task(lifecycle.TaskActive), []store.Execution{agent(1, false)}, nil,
			"Being implemented by the implementer (claude-sonnet-5)."},
		{"chat done", task(lifecycle.TaskDone), []store.Execution{chat(1, true)}, nil, "Implemented by the chat agent."},
		{"person claimed", task(lifecycle.TaskActive), []store.Execution{person},
			&store.Claim{ID: claimID, State: lifecycle.ClaimOpen, ClaimedAt: now.Add(-3 * time.Hour)},
			"Being implemented by sam, who claimed it 3 hours ago."},
		{"released then finished by an agent", task(lifecycle.TaskReview), []store.Execution{chat(1, false), agent(1, true)}, nil,
			"Implemented by the implementer (claude-sonnet-5), from work the chat agent started."},
		{"reworked by the chat agent", task(lifecycle.TaskReview), []store.Execution{agent(1, true), chat(2, true)}, nil,
			"Implemented by the implementer (claude-sonnet-5), then reworked by the chat agent."},
		{"same executor again", task(lifecycle.TaskDone), []store.Execution{chat(1, true), chat(2, true)}, nil, "Implemented by the chat agent."},
	}
	for _, tc := range cases {
		got := executorSentence(tc.task, tc.execs, tc.claim, now)
		if got.Sentence != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got.Sentence, tc.want)
		}
	}
	if got := executorSentence(task(lifecycle.TaskDone), []store.Execution{agent(1, true)}, nil, now); got.RunID != run.String() || !got.Measured || got.Kind != "agent" {
		t.Errorf("fields = %+v", got)
	}
}

// While one hand is in a working copy the lock for its path is held, a second
// hand for that path waits for it, and another path is free. It uses channel
// handshakes and TryLock, so nothing depends on timing.
func TestWithWorkingCopyIsOneAtATimePerPath(t *testing.T) {
	s := &Server{}
	inside, release := make(chan struct{}), make(chan struct{})
	first := make(chan struct{})
	go func() {
		defer close(first)
		_ = s.withWorkingCopy("/work/a", func() error {
			close(inside)
			<-release
			return nil
		})
	}()
	<-inside

	m, ok := s.copyLocks.Load(filepath.Clean("/work/a"))
	if !ok {
		t.Fatal("the working copy's lock was not made")
	}
	if l := m.(*lockedCopy); !l.mu.TryLock() {
		// Held, as it should be.
	} else {
		l.mu.Unlock()
		t.Fatal("the lock is not held while a hand is in the working copy")
	}

	entered, secondDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(secondDone)
		_ = s.withWorkingCopy("/work/a", func() error { close(entered); return nil })
	}()
	other := make(chan struct{})
	go func() {
		_ = s.withWorkingCopy("/work/b", func() error { close(other); return nil })
	}()
	select {
	case <-other:
	case <-time.After(5 * time.Second):
		t.Fatal("another working copy was held up")
	}
	select {
	case <-entered:
		t.Fatal("a second hand got into a working copy that is held")
	default:
	}

	close(release)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the second hand never got in after the first left")
	}
	<-first
	<-secondDone
}

func TestWorktreeFingerprintChangesWithTheWorkingCopy(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	writeIn(t, dir, "a.txt", "one\n")
	run("add", "-A")
	run("commit", "-qm", "one")

	fp := func() string {
		t.Helper()
		f, err := worktreeFingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	clean := fp()
	if fp() != clean {
		t.Fatal("a fingerprint is stable")
	}
	writeIn(t, dir, "a.txt", "two!\n")
	edited := fp()
	if edited == clean {
		t.Fatal("an edit changes it")
	}
	writeIn(t, dir, "b.txt", "new\n")
	added := fp()
	if added == edited {
		t.Fatal("a new file changes it")
	}
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "two")
	if committed := fp(); committed == edited || committed == clean {
		t.Fatal("a commit changes it")
	}
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if fp() == "" {
		t.Fatal("a deletion still fingerprints")
	}
}
