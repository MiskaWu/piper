package git

import (
	"fmt"
	"strings"
)

// ErrNotRepo indicates the directory is not a git repository or git is not available.
var ErrNotRepo = fmt.Errorf("not a git repository")

// Commander abstracts command execution for testability.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

// Status represents the git working tree status of a directory.
type Status struct {
	Dirty        bool `json:"dirty"`
	ChangedFiles int  `json:"changedFiles"`
}

// ParseStatus parses the output of `git status --porcelain` into a Status.
func ParseStatus(output string) Status {
	if output == "" {
		return Status{}
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return Status{
		Dirty:        count > 0,
		ChangedFiles: count,
	}
}

// FileStatus represents the git status of a single file.
type FileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"` // M, A, D, ?, R
}

// ParseFileStatuses parses the output of `git status --porcelain` into a list of FileStatus.
func ParseFileStatuses(output string) []FileStatus {
	if strings.TrimSpace(output) == "" {
		return []FileStatus{}
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var result []FileStatus
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// porcelain format: XY PATH or XY ORIG -> PATH
		xy := line[:2]
		path := strings.TrimSpace(line[3:])
		// Determine effective status: prefer index (X), fall back to worktree (Y)
		status := "M" // default
		x, y := xy[0], xy[1]
		switch {
		case x == '?' || y == '?':
			status = "?"
		case x == 'A' || y == 'A':
			status = "A"
		case x == 'D' || y == 'D':
			status = "D"
		case x == 'R' || y == 'R':
			status = "R"
			// Rename: "R  old -> new"
			if idx := strings.Index(path, " -> "); idx >= 0 {
				path = path[idx+4:]
			}
		case x == 'M' || y == 'M':
			status = "M"
		}
		result = append(result, FileStatus{Path: path, Status: status})
	}
	if result == nil {
		return []FileStatus{}
	}
	return result
}

// Checker runs git status for a given directory.
type Checker struct {
	cmd Commander
}

// NewChecker creates a Checker with the given Commander.
func NewChecker(cmd Commander) *Checker {
	return &Checker{cmd: cmd}
}

// GetStatus returns the git status for the given directory.
// Returns a zero Status (not dirty) if the directory is not a git repo.
func (c *Checker) GetStatus(dir string) (Status, error) {
	out, err := c.cmd.Output("git", "-C", dir, "status", "--porcelain")
	if err != nil {
		// Not a git repo or git not available — treat as clean
		errMsg := err.Error() + string(out)
		if strings.Contains(errMsg, "not a git repository") ||
			strings.Contains(errMsg, "No such file or directory") {
			return Status{}, nil
		}
		return Status{}, fmt.Errorf("git status %s: %w", dir, err)
	}
	return ParseStatus(string(out)), nil
}

// GetFileStatuses returns the per-file git status for the given directory.
// Returns an empty slice if the directory is not a git repo.
func (c *Checker) GetFileStatuses(dir string) ([]FileStatus, error) {
	out, err := c.cmd.Output("git", "-C", dir, "status", "--porcelain")
	if err != nil {
		errMsg := err.Error() + string(out)
		if strings.Contains(errMsg, "not a git repository") ||
			strings.Contains(errMsg, "No such file or directory") {
			return []FileStatus{}, nil
		}
		return nil, fmt.Errorf("git file statuses %s: %w", dir, err)
	}
	return ParseFileStatuses(string(out)), nil
}

// GetDirtyFilePaths returns dirty file paths for the given directory.
// Returns ErrNotRepo if the directory is not a git repository or git is not available.
func (c *Checker) GetDirtyFilePaths(dir string) ([]string, error) {
	out, err := c.cmd.Output("git", "-C", dir, "status", "--porcelain")
	if err != nil {
		return nil, ErrNotRepo
	}
	statuses := ParseFileStatuses(string(out))
	paths := make([]string, len(statuses))
	for i, s := range statuses {
		paths[i] = s.Path
	}
	return paths, nil
}
