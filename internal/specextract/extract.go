// Package specextract extracts structured markdown from spec source files
// (xlsx, docx, pptx, xmind). It replaces the Python extract-spec.py script.
package specextract

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Result holds the extraction output.
type Result struct {
	ExtractName string // stem of the source file (e.g. "spec-v1")
	Content     string // full extract markdown content
	ImageCount  int    // number of images extracted
	LineCount   int    // number of content lines
}

// extractor is a function that extracts lines from a source file.
type extractor func(sourcePath, imagesDir string) ([]string, error)

var extractors = map[string]extractor{
	".xlsx":  extractXLSX,
	".docx":  extractDOCX,
	".pptx":  extractPPTX,
	".xmind": extractXMind,
}

// SupportedExtensions returns the list of supported file extensions.
func SupportedExtensions() []string {
	return []string{".xlsx", ".docx", ".pptx", ".xmind"}
}

// Extract processes a source file and writes the extract to specsDir.
// It creates the directory structure:
//
//	specsDir/sources/<filename>       (copy of original)
//	specsDir/sources/images/          (extracted images)
//	specsDir/extracts/<stem>.extract.md
func Extract(sourcePath, specsDir string) (*Result, error) {
	ext := strings.ToLower(filepath.Ext(sourcePath))
	fn, ok := extractors[ext]
	if !ok {
		supported := strings.Join(SupportedExtensions(), ", ")
		return nil, fmt.Errorf("unsupported format %s (supported: %s)", ext, supported)
	}

	// Verify source exists
	if _, err := os.Stat(sourcePath); err != nil {
		return nil, fmt.Errorf("source file: %w", err)
	}

	// Create directory structure
	sourcesDir := filepath.Join(specsDir, "sources")
	imagesDir := filepath.Join(sourcesDir, "images")
	extractsDir := filepath.Join(specsDir, "extracts")
	for _, d := range []string{sourcesDir, imagesDir, extractsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Copy source to sources/
	basename := filepath.Base(sourcePath)
	if err := copyFile(sourcePath, filepath.Join(sourcesDir, basename)); err != nil {
		return nil, fmt.Errorf("copy source: %w", err)
	}

	// Extract content
	lines, err := fn(sourcePath, imagesDir)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", ext, err)
	}

	// Count images
	imageCount := 0
	if entries, err := os.ReadDir(imagesDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				imageCount++
			}
		}
	}

	// Build extract file
	stem := strings.TrimSuffix(basename, filepath.Ext(basename))
	today := time.Now().Format("2006-01-02")
	header := []string{
		fmt.Sprintf("# Extract: %s", basename),
		fmt.Sprintf("> source: `%s`", basename),
		fmt.Sprintf("> extracted: %s", today),
	}
	allLines := append(header, lines...)
	content := strings.Join(allLines, "\n") + "\n"

	extractPath := filepath.Join(extractsDir, stem+".extract.md")
	if err := os.WriteFile(extractPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write extract: %w", err)
	}

	return &Result{
		ExtractName: stem,
		Content:     content,
		ImageCount:  imageCount,
		LineCount:   len(lines),
	}, nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
