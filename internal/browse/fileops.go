package browse

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// CopyRequest describes sources to copy/move and the destination directory.
type CopyRequest struct {
	Sources     []string `json:"sources"`
	Destination string   `json:"destination"`
}

// FileOpEntry records a single source→destination mapping.
type FileOpEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// FileOpResult contains the list of entries that were copied or moved.
type FileOpResult struct {
	Entries []FileOpEntry `json:"entries"`
}

// execFileOp runs a file operation (copy or move) for each source.
func execFileOp(req CopyRequest, op func(src, destDir string) (string, error), verb string) (FileOpResult, error) {
	if err := validateRequest(req); err != nil {
		return FileOpResult{}, err
	}

	var result FileOpResult
	for _, src := range req.Sources {
		dest, err := op(src, req.Destination)
		if err != nil {
			return FileOpResult{}, fmt.Errorf("%s %s: %w", verb, src, err)
		}
		result.Entries = append(result.Entries, FileOpEntry{Source: src, Destination: dest})
	}
	return result, nil
}

// CopyFiles copies each source to the destination directory.
func CopyFiles(req CopyRequest) (FileOpResult, error) {
	return execFileOp(req, copySingle, "copy")
}

// MoveFiles moves each source to the destination directory.
// It tries os.Rename first, falling back to copy+remove on cross-device errors.
func MoveFiles(req CopyRequest) (FileOpResult, error) {
	return execFileOp(req, moveSingle, "move")
}

// ValidationError indicates a client input error (bad paths, missing sources).
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

// hasPathTraversal checks for ".." in the path string.
func hasPathTraversal(path string) bool {
	return strings.Contains(path, "..")
}

func validateRequest(req CopyRequest) error {
	if len(req.Sources) == 0 {
		return &ValidationError{"no sources specified"}
	}

	if hasPathTraversal(req.Destination) {
		return &ValidationError{fmt.Sprintf("path traversal not allowed: %s", req.Destination)}
	}

	info, err := os.Stat(req.Destination)
	if err != nil {
		return &ValidationError{fmt.Sprintf("destination %s: %s", req.Destination, err)}
	}
	if !info.IsDir() {
		return &ValidationError{fmt.Sprintf("destination is not a directory: %s", req.Destination)}
	}

	for _, src := range req.Sources {
		if hasPathTraversal(src) {
			return &ValidationError{fmt.Sprintf("path traversal not allowed: %s", src)}
		}
	}

	return nil
}

func copySingle(src, destDir string) (string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", err
	}

	name := resolveConflict(destDir, filepath.Base(src))
	destPath := filepath.Join(destDir, name)

	if srcInfo.IsDir() {
		return destPath, copyDir(src, destPath, srcInfo.Mode())
	}
	return destPath, copyFile(src, destPath, srcInfo.Mode())
}

func copyFile(src, dest string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(dest, mode); err != nil {
		return err
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func moveSingle(src, destDir string) (string, error) {
	name := resolveConflict(destDir, filepath.Base(src))
	destPath := filepath.Join(destDir, name)

	err := os.Rename(src, destPath)
	if err == nil {
		return destPath, nil
	}

	// Fall back to copy+remove on cross-device error.
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
		srcInfo, statErr := os.Stat(src)
		if statErr != nil {
			return "", statErr
		}
		if srcInfo.IsDir() {
			if copyErr := copyDir(src, destPath, srcInfo.Mode()); copyErr != nil {
				return "", copyErr
			}
		} else {
			if copyErr := copyFile(src, destPath, srcInfo.Mode()); copyErr != nil {
				return "", copyErr
			}
		}
		if removeErr := os.RemoveAll(src); removeErr != nil {
			return "", fmt.Errorf("remove source after copy: %w", removeErr)
		}
		return destPath, nil
	}

	return "", err
}
