package autostop

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"
)

// fakeStopper records Stop calls and can return errors.
type fakeStopper struct {
	stopped []string
	err     error
}

func (f *fakeStopper) Stop(projectPath string) error {
	f.stopped = append(f.stopped, projectPath)
	return f.err
}

// fakeStore returns pre-configured session-project mappings and project list.
type fakeStore struct {
	sessionsByProject map[string][]string
	projects          []string
}

func (f *fakeStore) GetSessionsByProject() map[string][]string {
	return f.sessionsByProject
}

func (f *fakeStore) GetProjects() []string {
	return f.projects
}

func newTestLogger() *log.Logger {
	return log.New(os.Stderr, "", 0)
}

func TestCheck_專案所有session消失時grace期滿後停止容器(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1", "sess-2"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: both sessions live
	as.Check([]string{"sess-1", "sess-2"})
	if len(stopper.stopped) != 0 {
		t.Fatalf("expected no stop on first tick, got %v", stopper.stopped)
	}

	// Second tick: all sessions gone — starts grace period, not immediate stop
	as.Check([]string{})
	if len(stopper.stopped) != 0 {
		t.Fatalf("expected no immediate stop during grace period, got %v", stopper.stopped)
	}

	// Wait for grace period to expire
	time.Sleep(120 * time.Millisecond)
	if len(stopper.stopped) != 1 {
		t.Fatalf("expected 1 stop after grace, got %d: %v", len(stopper.stopped), stopper.stopped)
	}
	if stopper.stopped[0] != "/proj/alpha" {
		t.Errorf("expected /proj/alpha, got %s", stopper.stopped[0])
	}
}

func TestCheck_grace期間session回來時取消停止(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(100*time.Millisecond))

	// Tick 1: session live
	as.Check([]string{"sess-1"})

	// Tick 2: session gone → grace starts
	as.Check([]string{})

	// Tick 3: session comes back → grace cancelled
	store.sessionsByProject = map[string][]string{
		"/proj/alpha": {"sess-new"},
	}
	as.Check([]string{"sess-new"})

	// Wait past original grace period
	time.Sleep(150 * time.Millisecond)
	if len(stopper.stopped) != 0 {
		t.Errorf("expected no stop after session resumed, got %v", stopper.stopped)
	}
}

func TestCheck_專案仍有session時不停止(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1", "sess-2"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: both live
	as.Check([]string{"sess-1", "sess-2"})

	// Second tick: one removed but one remains
	as.Check([]string{"sess-2"})
	time.Sleep(80 * time.Millisecond)
	if len(stopper.stopped) != 0 {
		t.Errorf("expected no stop when sessions remain, got %v", stopper.stopped)
	}
}

func TestCheck_專案從未有session時不停止(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: sess-1 is NOT in live list (project never had live sessions)
	as.Check([]string{"other-session"})
	time.Sleep(80 * time.Millisecond)
	if len(stopper.stopped) != 0 {
		t.Errorf("expected no stop for project that never had live sessions, got %v", stopper.stopped)
	}
}

func TestCheck_Stop失敗時記log不panic(t *testing.T) {
	stopper := &fakeStopper{err: fmt.Errorf("docker timeout")}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: session live
	as.Check([]string{"sess-1"})

	// Second tick: session gone, stop will fail after grace
	as.Check([]string{})
	time.Sleep(80 * time.Millisecond)
	if len(stopper.stopped) != 1 {
		t.Fatalf("expected stop attempt even though it fails, got %d", len(stopper.stopped))
	}
}

func TestCheck_多專案獨立判斷(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-a"},
			"/proj/beta":  {"sess-b"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: both live
	as.Check([]string{"sess-a", "sess-b"})

	// Second tick: only alpha's session gone
	as.Check([]string{"sess-b"})
	time.Sleep(80 * time.Millisecond)
	if len(stopper.stopped) != 1 {
		t.Fatalf("expected 1 stop, got %d: %v", len(stopper.stopped), stopper.stopped)
	}
	if stopper.stopped[0] != "/proj/alpha" {
		t.Errorf("expected /proj/alpha stopped, got %s", stopper.stopped[0])
	}
}

func TestCheck_kill後馬上重建不觸發stop(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-old"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(50*time.Millisecond))

	// First tick: old session live
	as.Check([]string{"sess-old"})

	// User kills old session and creates new one — store is updated
	store.sessionsByProject = map[string][]string{
		"/proj/alpha": {"sess-new"},
	}

	// Second tick: old session gone, new session live
	as.Check([]string{"sess-new"})
	time.Sleep(80 * time.Millisecond)
	if len(stopper.stopped) != 0 {
		t.Errorf("expected no stop when session was replaced, got %v", stopper.stopped)
	}
}

func TestCheck_grace期間重複Check不重複計時(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(80*time.Millisecond))

	// Tick 1: session live
	as.Check([]string{"sess-1"})

	// Tick 2: gone → grace starts
	as.Check([]string{})

	// Tick 3: still gone (30ms later) → should NOT reset timer
	time.Sleep(30 * time.Millisecond)
	as.Check([]string{})

	// Wait: original timer should fire at ~80ms from tick 2, not 80ms from tick 3
	time.Sleep(70 * time.Millisecond)
	if len(stopper.stopped) != 1 {
		t.Fatalf("expected 1 stop (timer not reset), got %d", len(stopper.stopped))
	}
}

func TestCancelAll_取消所有pending不停容器(t *testing.T) {
	stopper := &fakeStopper{}
	store := &fakeStore{
		sessionsByProject: map[string][]string{
			"/proj/alpha": {"sess-1"},
			"/proj/beta":  {"sess-2"},
		},
	}
	as := New(stopper, store, newTestLogger(), WithGrace(100*time.Millisecond))

	// Both live
	as.Check([]string{"sess-1", "sess-2"})

	// Both gone → grace starts
	as.Check([]string{})

	// Shutdown: cancel all pending
	as.CancelAll()

	// Wait past grace
	time.Sleep(150 * time.Millisecond)
	if len(stopper.stopped) != 0 {
		t.Errorf("expected no stop after CancelAll, got %v", stopper.stopped)
	}
}

type countingStopper struct {
	stopFunc func(string) error
}

func (c *countingStopper) Stop(projectPath string) error {
	return c.stopFunc(projectPath)
}
