package plan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GitRunner abstracts git operations for testability.
type GitRunner interface {
	// IsRepo checks whether dir is inside a git repository.
	IsRepo(dir string) bool
	// Init initialises a new git repository in dir.
	Init(dir string) error
	// Add stages paths relative to dir.
	Add(dir string, paths ...string) error
	// Commit creates a commit with the given message. Returns nil if nothing to commit.
	Commit(dir string, msg string) error
	// RemoteExists checks whether a named remote exists.
	RemoteExists(dir, name string) bool
	// RemoteAdd adds a remote.
	RemoteAdd(dir, name, url string) error
	// RemoteSetURL updates an existing remote URL.
	RemoteSetURL(dir, name, url string) error
	// LsRemote verifies that a remote URL is reachable (auth check).
	LsRemote(dir, url string) error
	// Pull does git pull --rebase.
	Pull(dir string) error
	// Push does git push.
	Push(dir string) error
	// PendingCommits returns the number of commits ahead of origin.
	PendingCommits(dir string) (int, error)
}

// execGitRunner shells out to the git binary.
type execGitRunner struct{}

// NewExecGitRunner returns a GitRunner that uses exec.Command("git", ...).
func NewExecGitRunner() GitRunner {
	return &execGitRunner{}
}

func (g *execGitRunner) run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *execGitRunner) runCtx(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("git %s: timed out: %w", args[0], ctx.Err())
		}
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *execGitRunner) IsRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func (g *execGitRunner) Init(dir string) error {
	_, err := g.run(dir, "init")
	if err != nil {
		return fmt.Errorf("init repo %s: %w", dir, err)
	}
	// Configure user for auto-commits
	g.run(dir, "config", "user.email", "piper@localhost")
	g.run(dir, "config", "user.name", "piper")
	return nil
}

func (g *execGitRunner) Add(dir string, paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := g.run(dir, args...)
	return err
}

func (g *execGitRunner) Commit(dir string, msg string) error {
	// Check if there are staged changes
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	if cmd.Run() == nil {
		// Also check for untracked—nothing to commit
		return nil
	}
	_, err := g.run(dir, "commit", "-m", msg)
	return err
}

func (g *execGitRunner) RemoteExists(dir, name string) bool {
	cmd := exec.Command("git", "remote", "get-url", name)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func (g *execGitRunner) RemoteAdd(dir, name, url string) error {
	_, err := g.run(dir, "remote", "add", name, url)
	return err
}

func (g *execGitRunner) RemoteSetURL(dir, name, url string) error {
	_, err := g.run(dir, "remote", "set-url", name, url)
	return err
}

func (g *execGitRunner) LsRemote(dir, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := g.runCtx(ctx, dir, "ls-remote", url)
	return err
}

func (g *execGitRunner) Pull(dir string) error {
	// Skip pull if no upstream tracking branch is configured
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "@{u}")
	cmd.Dir = dir
	if cmd.Run() != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := g.runCtx(ctx, dir, "pull", "--rebase")
	return err
}

func (g *execGitRunner) Push(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := g.runCtx(ctx, dir, "push", "-u", "origin", "HEAD")
	return err
}

func (g *execGitRunner) PendingCommits(dir string) (int, error) {
	// If no remote tracking branch, return 0
	out, err := g.run(dir, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return 0, nil // no upstream → 0 pending
	}
	var count int
	fmt.Sscanf(out, "%d", &count)
	return count, nil
}
