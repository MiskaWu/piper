package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TagInfo represents a git tag.
type TagInfo struct {
	Name   string `json:"name"`
	Hash   string `json:"hash"`
	Pushed bool   `json:"pushed"`
}

// CreateTag creates a lightweight tag at the given commit hash.
func (g *GraphFetcher) CreateTag(dir, name, hash string) error {
	if dir == "" || name == "" || hash == "" {
		return fmt.Errorf("git tag: dir, name, and hash are required")
	}
	out, err := g.cmd.Output("git", "-C", dir, "tag", name, hash)
	if err != nil {
		return fmt.Errorf("git tag %s at %s: %s: %w", name, hash, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DeleteTag deletes a local tag.
func (g *GraphFetcher) DeleteTag(dir, name string) error {
	if dir == "" || name == "" {
		return fmt.Errorf("git tag -d: dir and name are required")
	}
	out, err := g.cmd.Output("git", "-C", dir, "tag", "-d", name)
	if err != nil {
		return fmt.Errorf("git tag -d %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// PushTag pushes a single tag to the remote with a 30-second timeout.
func (g *GraphFetcher) PushTag(dir, remote, name string) error {
	if dir == "" || name == "" {
		return fmt.Errorf("git push tag: dir and name are required")
	}
	if remote == "" {
		remote = "origin"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "push", remote, "refs/tags/"+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git push tag %s: timed out: %w", name, ctx.Err())
		}
		return fmt.Errorf("git push tag %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListTags returns all tags in the given directory, marking each as pushed or not.
func (g *GraphFetcher) ListTags(dir string) ([]TagInfo, error) {
	out, err := g.cmd.Output("git", "-C", dir, "tag", "--format=%(refname:short)%00%(objectname:short)")
	if err != nil {
		return nil, fmt.Errorf("git tag list %s: %w", dir, err)
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return []TagInfo{}, nil
	}
	lines := strings.Split(text, "\n")
	tags := make([]TagInfo, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		tags = append(tags, TagInfo{
			Name: parts[0],
			Hash: parts[1],
		})
	}

	// Check which tags exist on remote
	remoteTags := g.fetchRemoteTags(dir)
	for i := range tags {
		tags[i].Pushed = remoteTags[tags[i].Name]
	}
	return tags, nil
}

// fetchRemoteTags returns a set of tag names that exist on origin.
// On failure (no remote, network error, etc.), returns empty map — tags default to unpushed.
func (g *GraphFetcher) fetchRemoteTags(dir string) map[string]bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "ls-remote", "--tags", "origin")
	out, err := cmd.Output()
	if err != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		// Format: "<hash>\trefs/tags/<name>" (skip ^{} dereferenced entries)
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := parts[1]
		if strings.HasSuffix(ref, "^{}") {
			continue
		}
		name := strings.TrimPrefix(ref, "refs/tags/")
		result[name] = true
	}
	return result
}
