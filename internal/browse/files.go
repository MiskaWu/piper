package browse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs contains directory names to skip when listing all files.
var skipDirs = map[string]bool{
	"node_modules": true,
	"__pycache__":  true,
	".git":         true,
	".venv":        true,
	"vendor":       true,
	".next":        true,
	"dist":         true,
}

// ListAll lists all entries (files and directories) under the given path.
// Directories are sorted first, then files, both alphabetically.
// Dotfiles, symlink-broken entries, and common noise directories are skipped.
func ListAll(dirPath string) (Result, error) {
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

	var dirs, files []Entry
	for _, de := range dirEntries {
		name := de.Name()
		// Skip hidden entries
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Follow symlinks
		resolved, err := os.Stat(filepath.Join(absPath, name))
		if err != nil {
			continue
		}
		isDir := resolved.IsDir()
		// Skip noise directories
		if isDir && skipDirs[name] {
			continue
		}
		entry := Entry{
			Name:  name,
			Path:  filepath.Join(absPath, name),
			IsDir: isDir,
		}
		if isDir {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	entries := make([]Entry, 0, len(dirs)+len(files))
	entries = append(entries, dirs...)
	entries = append(entries, files...)

	return Result{
		Current: absPath,
		Parent:  filepath.Dir(absPath),
		Entries: entries,
	}, nil
}
