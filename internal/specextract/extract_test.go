package specextract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract_不支援的格式(t *testing.T) {
	_, err := Extract("/tmp/test.txt", t.TempDir())
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format', got: %v", err)
	}
}

func TestExtract_檔案不存在(t *testing.T) {
	_, err := Extract("/tmp/nonexistent.xlsx", t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExtract_XLSX_真實檔案(t *testing.T) {
	sourcePath := "/tmp/piper-clipboard/6ea2cbc5d991c28427bb7f8762130fd1.xlsx"
	if _, err := os.Stat(sourcePath); err != nil {
		t.Skip("test xlsx file not available")
	}

	specsDir := filepath.Join(t.TempDir(), "specs")
	result, err := Extract(sourcePath, specsDir)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	t.Logf("ExtractName: %s", result.ExtractName)
	t.Logf("LineCount: %d", result.LineCount)
	t.Logf("ImageCount: %d", result.ImageCount)
	t.Logf("Content length: %d bytes", len(result.Content))

	// Verify file structure
	if _, err := os.Stat(filepath.Join(specsDir, "sources", "6ea2cbc5d991c28427bb7f8762130fd1.xlsx")); err != nil {
		t.Error("source file not copied")
	}
	extractPath := filepath.Join(specsDir, "extracts", "6ea2cbc5d991c28427bb7f8762130fd1.extract.md")
	if _, err := os.Stat(extractPath); err != nil {
		t.Error("extract file not created")
	}

	// Verify content has expected structure
	if !strings.Contains(result.Content, "# Extract:") {
		t.Error("missing extract header")
	}
	if !strings.Contains(result.Content, "## Sheet:") {
		t.Error("missing sheet headers")
	}
	if result.LineCount == 0 {
		t.Error("expected non-zero line count")
	}

	// Print first 50 lines for inspection
	contentLines := strings.Split(result.Content, "\n")
	limit := 50
	if len(contentLines) < limit {
		limit = len(contentLines)
	}
	t.Logf("--- First %d lines ---", limit)
	for i := 0; i < limit; i++ {
		t.Logf("  %s", contentLines[i])
	}
}
