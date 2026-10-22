package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/miskawu/piper/internal/plan"
)

// fakeGitRunner for vault tests
type fakeGitRunner struct {
	inited  bool
	commits []string
	adds    [][]string
}

func (f *fakeGitRunner) IsRepo(dir string) bool              { return f.inited }
func (f *fakeGitRunner) Init(dir string) error               { f.inited = true; return nil }
func (f *fakeGitRunner) Add(dir string, p ...string) error   { f.adds = append(f.adds, p); return nil }
func (f *fakeGitRunner) Commit(dir string, m string) error   { f.commits = append(f.commits, m); return nil }
func (f *fakeGitRunner) RemoteExists(dir, name string) bool  { return false }
func (f *fakeGitRunner) RemoteAdd(dir, name, url string) error  { return nil }
func (f *fakeGitRunner) RemoteSetURL(dir, name, url string) error { return nil }
func (f *fakeGitRunner) LsRemote(dir, url string) error        { return nil }
func (f *fakeGitRunner) Pull(dir string) error                  { return nil }
func (f *fakeGitRunner) Push(dir string) error                  { return nil }
func (f *fakeGitRunner) PendingCommits(dir string) (int, error) { return 0, nil }

// Ensure fakeGitRunner satisfies plan.GitRunner
var _ plan.GitRunner = (*fakeGitRunner)(nil)

func newTestVault(t *testing.T) (*VaultManager, *fakeGitRunner) {
	t.Helper()
	fg := &fakeGitRunner{inited: true}
	v := New(filepath.Join(t.TempDir(), "vault"), WithGitRunner(fg))
	if err := v.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	return v, fg
}

func TestAddIdea_基本新增(t *testing.T) {
	v, _ := newTestVault(t)

	idea, err := v.AddIdea("my first idea", nil)
	if err != nil {
		t.Fatalf("AddIdea: %v", err)
	}
	if idea.ID != 1 {
		t.Errorf("expected ID=1, got %d", idea.ID)
	}
	if idea.Text != "my first idea" {
		t.Errorf("expected text 'my first idea', got %q", idea.Text)
	}
	if idea.Created.IsZero() {
		t.Error("expected non-zero Created")
	}

	// Verify file exists
	path := filepath.Join(v.baseDir, "inbox", "idea-00001.md")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected idea file to exist: %v", err)
	}
}

func TestAddIdea_遞增ID(t *testing.T) {
	v, _ := newTestVault(t)

	i1, _ := v.AddIdea("first", nil)
	i2, _ := v.AddIdea("second", nil)
	if i2.ID != i1.ID+1 {
		t.Errorf("expected ID %d, got %d", i1.ID+1, i2.ID)
	}
}

func TestAddIdea_含上下文(t *testing.T) {
	v, _ := newTestVault(t)

	ctx := []IdeaContext{
		{Session: "ragent-test", Kind: "output", Detail: "running tests", Time: 1000},
	}
	idea, err := v.AddIdea("idea with context", ctx)
	if err != nil {
		t.Fatalf("AddIdea: %v", err)
	}
	if len(idea.Context) != 1 {
		t.Fatalf("expected 1 context, got %d", len(idea.Context))
	}
}

func TestAddIdea_autoCommit(t *testing.T) {
	v, fg := newTestVault(t)

	_, err := v.AddIdea("test", nil)
	if err != nil {
		t.Fatalf("AddIdea: %v", err)
	}

	if len(fg.commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(fg.commits))
	}
	if fg.commits[0] != "add idea #1" {
		t.Errorf("expected commit msg 'add idea #1', got %q", fg.commits[0])
	}
}

func TestGetIdeas_最新在前(t *testing.T) {
	v, _ := newTestVault(t)

	v.AddIdea("first", nil)
	v.AddIdea("second", nil)
	v.AddIdea("third", nil)

	ideas, err := v.GetIdeas()
	if err != nil {
		t.Fatalf("GetIdeas: %v", err)
	}
	if len(ideas) != 3 {
		t.Fatalf("expected 3 ideas, got %d", len(ideas))
	}
	if ideas[0].Text != "third" {
		t.Errorf("expected newest first, got %q", ideas[0].Text)
	}
}

func TestGetIdeas_空inbox(t *testing.T) {
	v, _ := newTestVault(t)

	ideas, err := v.GetIdeas()
	if err != nil {
		t.Fatalf("GetIdeas: %v", err)
	}
	if len(ideas) != 0 {
		t.Errorf("expected 0 ideas, got %d", len(ideas))
	}
}

func TestGetIdea_找到(t *testing.T) {
	v, _ := newTestVault(t)

	created, _ := v.AddIdea("find me", nil)
	idea, err := v.GetIdea(created.ID)
	if err != nil {
		t.Fatalf("GetIdea: %v", err)
	}
	if idea == nil {
		t.Fatal("expected idea, got nil")
	}
	if idea.Text != "find me" {
		t.Errorf("expected 'find me', got %q", idea.Text)
	}
}

func TestGetIdea_不存在回傳nil(t *testing.T) {
	v, _ := newTestVault(t)

	idea, err := v.GetIdea(999)
	if err != nil {
		t.Fatalf("GetIdea: %v", err)
	}
	if idea != nil {
		t.Errorf("expected nil for missing idea, got %+v", idea)
	}
}

func TestDeleteIdea_刪除成功(t *testing.T) {
	v, _ := newTestVault(t)

	idea, _ := v.AddIdea("to delete", nil)
	if err := v.DeleteIdea(idea.ID); err != nil {
		t.Fatalf("DeleteIdea: %v", err)
	}

	got, _ := v.GetIdea(idea.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestDeleteIdea_不存在不報錯(t *testing.T) {
	v, _ := newTestVault(t)

	if err := v.DeleteIdea(999); err != nil {
		t.Errorf("DeleteIdea should not error for missing ID: %v", err)
	}
}

func TestUpdateIdeaEnrichedDesc_更新成功(t *testing.T) {
	v, _ := newTestVault(t)

	idea, _ := v.AddIdea("test idea", nil)
	if err := v.UpdateIdeaEnrichedDesc(idea.ID, "enriched description"); err != nil {
		t.Fatalf("UpdateIdeaEnrichedDesc: %v", err)
	}

	got, _ := v.GetIdea(idea.ID)
	if got == nil {
		t.Fatal("expected idea, got nil")
	}
	if got.EnrichedDesc != "enriched description" {
		t.Errorf("expected enriched desc, got %q", got.EnrichedDesc)
	}
}

func TestUpdateIdeaEnrichedDesc_不存在報錯(t *testing.T) {
	v, _ := newTestVault(t)

	err := v.UpdateIdeaEnrichedDesc(999, "desc")
	if err == nil {
		t.Error("expected error for missing idea")
	}
}

func TestAddIdea_meta持久化(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}

	// First vault instance: add ideas
	v1 := New(dir, WithGitRunner(fg))
	v1.Init()
	v1.AddIdea("first", nil)
	v1.AddIdea("second", nil)

	// Second vault instance: should continue from nextID=2
	v2 := New(dir, WithGitRunner(fg))
	v2.Init()
	idea, _ := v2.AddIdea("third", nil)
	if idea.ID != 3 {
		t.Errorf("expected ID=3 after reload, got %d", idea.ID)
	}
}
