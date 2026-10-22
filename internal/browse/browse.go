package browse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry represents a directory entry.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// Result represents the result of listing a directory.
type Result struct {
	Current string  `json:"current"`
	Parent  string  `json:"parent"`
	Entries []Entry `json:"entries"`
}

// ListDir lists subdirectories under the given path.
// Only directories are returned, sorted alphabetically.
func ListDir(dirPath string) (Result, error) {
	if hasPathTraversal(dirPath) {
		return Result{}, fmt.Errorf("path traversal not allowed: %s", dirPath)
	}

	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve path %s: %w", dirPath, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("not a directory: %s", absPath)
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return Result{}, fmt.Errorf("read dir %s: %w", absPath, err)
	}

	var entries []Entry
	for _, de := range dirEntries {
		// Skip hidden directories
		if strings.HasPrefix(de.Name(), ".") {
			continue
		}
		// Follow symlinks
		info, err := os.Stat(filepath.Join(absPath, de.Name()))
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}
		entries = append(entries, Entry{
			Name:  de.Name(),
			Path:  filepath.Join(absPath, de.Name()),
			IsDir: true,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	if entries == nil {
		entries = []Entry{}
	}

	return Result{
		Current: absPath,
		Parent:  filepath.Dir(absPath),
		Entries: entries,
	}, nil
}
