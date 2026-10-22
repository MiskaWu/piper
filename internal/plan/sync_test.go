package plan

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// syncFakeGitRunner extends fakeGitRunner with remote/sync tracking.
type syncFakeGitRunner struct {
	mu           sync.Mutex
	inited       bool
	remoteAdded  bool
	pullCalled   atomic.Int32
	pushCalled   atomic.Int32
	pullErr      error
	pushErr      error
	pendingCount int
}

func (f *syncFakeGitRunner) IsRepo(dir string) bool        { return f.inited }
func (f *syncFakeGitRunner) Init(dir string) error          { f.inited = true; return nil }
func (f *syncFakeGitRunner) Add(dir string, p ...string) error { return nil }
func (f *syncFakeGitRunner) Commit(dir string, m string) error { return nil }
func (f *syncFakeGitRunner) RemoteExists(dir, name string) bool {
	return f.remoteAdded
}
func (f *syncFakeGitRunner) RemoteAdd(dir, name, url string) error {
	f.remoteAdded = true
	return nil
}
func (f *syncFakeGitRunner) RemoteSetURL(dir, name, url string) error { return nil }
func (f *syncFakeGitRunner) LsRemote(dir, url string) error            { return nil }
func (f *syncFakeGitRunner) Pull(dir string) error {
	f.pullCalled.Add(1)
	return f.pullErr
}
func (f *syncFakeGitRunner) Push(dir string) error {
	f.pushCalled.Add(1)
	return f.pushErr
}
func (f *syncFakeGitRunner) PendingCommits(dir string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pendingCount, nil
}

// fakeSyncableRepo satisfies SyncableRepo for testing GitSyncer.
type fakeSyncableRepo struct {
	dir string
	git GitRunner
}

func (f *fakeSyncableRepo) BaseDir() string  { return f.dir }
func (f *fakeSyncableRepo) Git() GitRunner   { return f.git }

func newFakeSyncableRepo(dir string, git GitRunner) *fakeSyncableRepo {
	return &fakeSyncableRepo{dir: dir, git: git}
}

func TestGitSyncer_TriggerSync成功(t *testing.T) {
	fg := &syncFakeGitRunner{inited: true, remoteAdded: true}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	syncer.TriggerSync()
	// Wait for goroutine to complete
	time.Sleep(200 * time.Millisecond)

	if fg.pushCalled.Load() != 1 {
		t.Errorf("expected 1 push, got %d", fg.pushCalled.Load())
	}

	status := syncer.Status()
	if status.LastSync == nil {
		t.Error("expected LastSync to be set after successful sync")
	}
	if status.LastError != "" {
		t.Errorf("expected no error, got %q", status.LastError)
	}
}

func TestGitSyncer_Push失敗設定LastError(t *testing.T) {
	fg := &syncFakeGitRunner{
		inited:      true,
		remoteAdded: true,
		pushErr:     fmt.Errorf("network error"),
	}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	syncer.TriggerSync()
	time.Sleep(200 * time.Millisecond)

	status := syncer.Status()
	if status.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

func TestGitSyncer_無Remote不sync且設定LastError(t *testing.T) {
	fg := &syncFakeGitRunner{inited: true, remoteAdded: false}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	syncer.TriggerSync()
	time.Sleep(200 * time.Millisecond)

	if fg.pullCalled.Load() != 0 {
		t.Errorf("expected 0 pull without remote, got %d", fg.pullCalled.Load())
	}
	status := syncer.Status()
	if status.LastError == "" {
		t.Error("expected LastError to be set when no remote configured")
	}
}

func TestGitSyncer_非GitRepo設定LastError(t *testing.T) {
	fg := &syncFakeGitRunner{inited: false, remoteAdded: false}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	syncer.TriggerSync()
	time.Sleep(200 * time.Millisecond)

	if fg.pullCalled.Load() != 0 {
		t.Errorf("expected 0 pull for non-repo, got %d", fg.pullCalled.Load())
	}
	status := syncer.Status()
	if status.LastError == "" {
		t.Error("expected LastError to be set when dir is not a git repo")
	}
}

func TestGitSyncer_Status回傳PendingCommits(t *testing.T) {
	fg := &syncFakeGitRunner{inited: true, pendingCount: 3}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	status := syncer.Status()
	if status.PendingCommits != 3 {
		t.Errorf("expected 3 pending, got %d", status.PendingCommits)
	}
}

func TestGitSyncer_ConfigureStartStop(t *testing.T) {
	fg := &syncFakeGitRunner{inited: true, remoteAdded: true}
	repo := newFakeSyncableRepo(t.TempDir(), fg)
	syncer := NewGitSyncer(repo)

	syncer.Configure(true, 1) // 1 min interval
	time.Sleep(50 * time.Millisecond)

	syncer.Stop()
	// Should not panic on double stop
	syncer.Stop()
}
