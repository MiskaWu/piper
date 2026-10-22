package clipboard

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// Store manages temporary clipboard image files.
type Store struct {
	dir    string
	maxAge time.Duration
	maxSize int64
}

// NewStore creates a new clipboard Store. It creates the directory if it does not exist.
func NewStore(dir string, maxAge time.Duration, maxSize int64) *Store {
	os.MkdirAll(dir, 0o755)
	return &Store{
		dir:     dir,
		maxAge:  maxAge,
		maxSize: maxSize,
	}
}

// generateID returns a random hex string to use as a filename.
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// extFromContentType returns the file extension for the given content type.
func extFromContentType(contentType string) (string, error) {
	switch contentType {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	default:
		if strings.HasPrefix(contentType, "image/") {
			// Unknown image subtype — use the subtype as extension
			sub := strings.TrimPrefix(contentType, "image/")
			return "." + sub, nil
		}
		return "", fmt.Errorf("unsupported content type: expected image/* , got %s", contentType)
	}
}

// extFromFilename extracts the file extension from the original filename.
// Returns an error if the filename has no extension.
func extFromFilename(filename string) (string, error) {
	ext := filepath.Ext(filename)
	if ext == "" {
		return "", fmt.Errorf("filename has no extension: %s", filename)
	}
	return ext, nil
}

// SaveFile writes any file data from r to a new file, preserving the original
// filename's extension. Unlike Save, it is not restricted to image/* types.
func (s *Store) SaveFile(r io.Reader, filename string) (id string, path string, err error) {
	return s.SaveFileTo(s.dir, r, filename)
}

// SaveFileTo writes any file data to the specified directory, preserving the
// original filename's extension.
func (s *Store) SaveFileTo(dir string, r io.Reader, filename string) (id string, path string, err error) {
	ext, err := extFromFilename(filename)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	id = generateID()
	path = filepath.Join(dir, id+ext)

	limited := io.LimitReader(r, s.maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", "", fmt.Errorf("failed to read file data: %w", err)
	}
	if int64(len(data)) > s.maxSize {
		return "", "", fmt.Errorf("file size exceeds maximum allowed size of %d bytes", s.maxSize)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", fmt.Errorf("failed to write file: %w", err)
	}

	return id, path, nil
}

// Save writes image data from r to a new file in the default store directory.
func (s *Store) Save(r io.Reader, contentType string) (id string, path string, err error) {
	return s.SaveTo(s.dir, r, contentType)
}

// SaveTo writes image data from r to a new file in the specified directory.
// It validates the content type starts with "image/" and limits the read to maxSize bytes.
// Returns the generated id and the full file path.
func (s *Store) SaveTo(dir string, r io.Reader, contentType string) (id string, path string, err error) {
	ext, err := extFromContentType(contentType)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	id = generateID()
	path = filepath.Join(dir, id+ext)

	// Read up to maxSize+1 bytes to detect oversized data
	limited := io.LimitReader(r, s.maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", "", fmt.Errorf("failed to read image data: %w", err)
	}
	if int64(len(data)) > s.maxSize {
		return "", "", fmt.Errorf("image size exceeds maximum allowed size of %d bytes", s.maxSize)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", fmt.Errorf("failed to write image file: %w", err)
	}

	return id, path, nil
}

// Path returns the file path for a previously saved image by its id,
// searching the default store directory.
func (s *Store) Path(id string) (string, error) {
	return s.PathIn(s.dir, id)
}

// PathIn returns the file path for a previously saved image by its id,
// searching the specified directory.
func (s *Store) PathIn(dir, id string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read clipboard directory: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		nameWithoutExt := strings.TrimSuffix(name, filepath.Ext(name))
		if nameWithoutExt == id {
			return filepath.Join(dir, name), nil
		}
	}
	return "", fmt.Errorf("image not found: %s", id)
}

// Cleanup removes files older than maxAge from the store directory.
func (s *Store) Cleanup() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-s.maxAge)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

// StartCleanup runs Cleanup periodically until the context is cancelled.
func (s *Store) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Cleanup()
			}
		}
	}()
}
