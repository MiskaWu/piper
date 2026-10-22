package git

import "fmt"

// IsRepo checks if the given directory is a git repository.
func (g *GraphFetcher) IsRepo(dir string) bool {
	_, err := g.cmd.Output("git", "-C", dir, "rev-parse", "--git-dir")
	return err == nil
}

// InitRepo initializes a git repository in the given directory.
func (g *GraphFetcher) InitRepo(dir string) error {
	if err := g.cmd.Run("git", "-C", dir, "init", "-b", "main"); err != nil {
		return fmt.Errorf("git init %s: %w", dir, err)
	}
	return nil
}

// AddRemote adds a named remote to the git repository.
func (g *GraphFetcher) AddRemote(dir, name, url string) error {
	if err := g.cmd.Run("git", "-C", dir, "remote", "add", name, url); err != nil {
		return fmt.Errorf("git remote add %s %s: %w", name, url, err)
	}
	return nil
}
