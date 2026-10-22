package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CheckSSHAgent checks whether the SSH agent has loaded keys.
// Returns (true, nil) if keys are available, (false, nil) if agent is running
// but has no keys, and (false, error) if agent is unreachable.
func (g *GraphFetcher) CheckSSHAgent() (bool, error) {
	out, err := g.cmd.Output("ssh-add", "-l")
	if err == nil {
		return true, nil
	}
	// ssh-add -l returns exit 1 with "no identities" when agent runs but has no keys
	output := string(out)
	if strings.Contains(output, "no identities") {
		return false, nil
	}
	// exit 2 or other — agent itself is unreachable
	return false, fmt.Errorf("ssh agent unavailable: %w", err)
}

// Push runs git push for the given directory with a 30-second timeout.
// If branch is specified, that branch is pushed; otherwise the current branch is used.
// If the branch has no upstream, it automatically sets one with -u.
func (g *GraphFetcher) Push(dir, branch string) error {
	if dir == "" {
		return fmt.Errorf("git push: directory is required")
	}

	// Determine which branch to push
	targetBranch := branch
	if targetBranch == "" {
		// Use the currently checked-out branch
		branchOut, err := g.cmd.Output("git", "-C", dir, "branch", "--show-current")
		if err == nil {
			targetBranch = strings.TrimSpace(string(branchOut))
		}
	}

	// Check if the target branch has an upstream configured
	args := []string{"-C", dir, "push"}
	if targetBranch != "" {
		checkCmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", targetBranch+"@{upstream}")
		upstreamOut, err := checkCmd.Output()
		if err != nil {
			// No upstream — push with -u to origin
			args = []string{"-C", dir, "push", "-u", "origin", targetBranch}
		} else {
			// Parse remote name from upstream ref (e.g. "fork/feature-dungeon" → "fork")
			upstream := strings.TrimSpace(string(upstreamOut))
			remote := "origin"
			if idx := strings.Index(upstream, "/"); idx > 0 {
				remote = upstream[:idx]
			}
			args = []string{"-C", dir, "push", remote, targetBranch}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git push %s: timed out: %w", dir, ctx.Err())
		}
		return fmt.Errorf("git push %s: %s: %w", dir, strings.TrimSpace(string(out)), err)
	}
	return nil
}
