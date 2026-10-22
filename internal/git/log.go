package git

import (
	"fmt"
	"strings"
)

// CommitEntry represents a single git commit.
type CommitEntry struct {
	Hash    string
	Subject string
}

// LogOptions configures the git log query.
type LogOptions struct {
	Dir   string // git repo directory (projectPath)
	Since string // date string "2006-01-02" (inclusive)
	Until string // date string "2006-01-02" (exclusive, usually Since + 1 day)
}

// Log returns git commits for the given options.
// Returns empty slice (not error) if the directory is not a git repo.
func (c *Checker) Log(opts LogOptions) ([]CommitEntry, error) {
	out, err := c.cmd.Output("git", "-C", opts.Dir,
		"log",
		"--after="+opts.Since,
		"--before="+opts.Until,
		"--oneline",
		"--no-merges",
	)
	if err != nil {
		errMsg := err.Error() + string(out)
		if strings.Contains(errMsg, "not a git repository") ||
			strings.Contains(errMsg, "No such file or directory") {
			return nil, nil
		}
		return nil, fmt.Errorf("git log %s: %w", opts.Dir, err)
	}

	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	entries := make([]CommitEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		hash, subject, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		entries = append(entries, CommitEntry{Hash: hash, Subject: subject})
	}
	return entries, nil
}
