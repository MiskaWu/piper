package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SubmoduleInfo represents a git submodule and its push status.
type SubmoduleInfo struct {
	Path  string `json:"path"`
	Ahead int    `json:"ahead"`
}

// ListSubmodules returns all submodules and how many commits each has ahead of upstream.
func (g *GraphFetcher) ListSubmodules(dir string) ([]SubmoduleInfo, error) {
	out, err := g.cmd.Output("git", "-C", dir, "submodule", "status")
	if err != nil {
		return nil, fmt.Errorf("git submodule status %s: %w", dir, err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var subs []SubmoduleInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: " abc1234 path (desc)" or "+abc1234 path (desc)"
		// First char may be ' ', '+', '-', 'U'; skip it if not alphanumeric
		if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ' || line[0] == 'U') {
			line = line[1:]
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		subPath := parts[1]

		// Check ahead count
		ahead := 0
		absPath := filepath.Join(dir, subPath)
		aheadOut, aheadErr := g.cmd.Output("git", "-C", absPath, "rev-list", "--count", "@{upstream}..HEAD")
		if aheadErr == nil {
			if n, parseErr := strconv.Atoi(strings.TrimSpace(string(aheadOut))); parseErr == nil {
				ahead = n
			}
		}

		subs = append(subs, SubmoduleInfo{
			Path:  subPath,
			Ahead: ahead,
		})
	}
	return subs, nil
}

// PushSubmodules pushes the specified submodule paths within the given parent directory.
func (g *GraphFetcher) PushSubmodules(dir string, paths []string) error {
	if dir == "" {
		return fmt.Errorf("git push submodules: directory is required")
	}
	if len(paths) == 0 {
		return nil
	}

	var errs []string
	for _, subPath := range paths {
		absPath := filepath.Join(dir, subPath)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "git", "-C", absPath, "push")
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				errs = append(errs, fmt.Sprintf("%s: timed out", subPath))
			} else {
				errs = append(errs, fmt.Sprintf("%s: %s", subPath, strings.TrimSpace(string(out))))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("push submodules failed:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}
