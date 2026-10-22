package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func TestExecGitRunner_Init(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()

	if g.IsRepo(dir) {
		t.Fatal("expected dir not to be a repo before init")
	}

	if err := g.Init(dir); err != nil {
		t.Fatalf("init: %v", err)
	}

	if !g.IsRepo(dir) {
		t.Fatal("expected dir to be a repo after init")
	}
}

func TestExecGitRunner_AddAndCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()

	g.Init(dir)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)

	if err := g.Add(dir, "test.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := g.Commit(dir, "initial"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Verify commit exists
	out, err := exec.Command("git", "-C", dir, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "initial") {
		t.Errorf("expected commit message in log, got: %s", out)
	}
}

func TestExecGitRunner_CommitNothingToCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()

	g.Init(dir)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)
	g.Add(dir, "test.txt")
	g.Commit(dir, "initial")

	// Second commit with no changes should be nil (not error)
	if err := g.Commit(dir, "empty"); err != nil {
		t.Fatalf("expected nil for nothing-to-commit, got: %v", err)
	}
}

func TestExecGitRunner_RemoteAddAndExists(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()
	g.Init(dir)

	if g.RemoteExists(dir, "origin") {
		t.Fatal("expected no remote before add")
	}

	if err := g.RemoteAdd(dir, "origin", "https://example.com/repo.git"); err != nil {
		t.Fatalf("remote add: %v", err)
	}

	if !g.RemoteExists(dir, "origin") {
		t.Fatal("expected remote to exist after add")
	}
}

func TestExecGitRunner_RemoteSetURL(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()
	g.Init(dir)
	g.RemoteAdd(dir, "origin", "https://example.com/old.git")

	if err := g.RemoteSetURL(dir, "origin", "https://example.com/new.git"); err != nil {
		t.Fatalf("remote set-url: %v", err)
	}

	out, _ := exec.Command("git", "-C", dir, "remote", "get-url", "origin").CombinedOutput()
	if got := strings.TrimSpace(string(out)); got != "https://example.com/new.git" {
		t.Errorf("expected new URL, got: %s", got)
	}
}

func TestExecGitRunner_PendingCommitsNoUpstream(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	g := NewExecGitRunner()
	g.Init(dir)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644)
	g.Add(dir, ".")
	g.Commit(dir, "init")

	count, err := g.PendingCommits(dir)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pending (no upstream), got %d", count)
	}
}
