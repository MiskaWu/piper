package clipboard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSave_有效圖片類型回傳id和path(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	id, path, err := s.Save(strings.NewReader("fake-png-data"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if !strings.HasSuffix(path, ".png") {
		t.Errorf("expected path ending with .png, got %s", path)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("expected path under %s, got %s", dir, path)
	}

	// File should exist on disk
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist at %s", path)
	}

	// File content should match
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "fake-png-data" {
		t.Errorf("expected file content 'fake-png-data', got %q", string(data))
	}
}

func TestSave_各種圖片類型副檔名(t *testing.T) {
	tests := []struct {
		contentType string
		expectedExt string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "clipboard")
			s := NewStore(dir, 30*time.Minute, 10<<20)

			_, path, err := s.Save(strings.NewReader("data"), tt.contentType)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasSuffix(path, tt.expectedExt) {
				t.Errorf("expected ext %s, got path %s", tt.expectedExt, path)
			}
		})
	}
}

func TestSave_無效contentType回傳錯誤(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	_, _, err := s.Save(strings.NewReader("data"), "text/plain")
	if err == nil {
		t.Fatal("expected error for non-image content type")
	}
	if !strings.Contains(err.Error(), "image/") {
		t.Errorf("expected error mentioning image/, got: %v", err)
	}
}

func TestSave_超過maxSize回傳錯誤(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	maxSize := int64(10) // 10 bytes
	s := NewStore(dir, 30*time.Minute, maxSize)

	bigData := bytes.Repeat([]byte("x"), 100)
	_, _, err := s.Save(bytes.NewReader(bigData), "image/png")
	if err == nil {
		t.Fatal("expected error for oversized data")
	}
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("expected error mentioning size, got: %v", err)
	}
}

func TestPath_有效id回傳正確路徑(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	id, savedPath, err := s.Save(strings.NewReader("data"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotPath, err := s.Path(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != savedPath {
		t.Errorf("expected path %s, got %s", savedPath, gotPath)
	}
}

func TestPath_不存在的id回傳錯誤(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	_, err := s.Path("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent id")
	}
}

func TestSaveTo_存到指定目錄(t *testing.T) {
	defaultDir := filepath.Join(t.TempDir(), "default")
	s := NewStore(defaultDir, 30*time.Minute, 10<<20)

	customDir := filepath.Join(t.TempDir(), "project", ".piper", "clipboard")

	id, path, err := s.SaveTo(customDir, strings.NewReader("project-image"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if !strings.HasPrefix(path, customDir) {
		t.Errorf("expected path under %s, got %s", customDir, path)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Errorf("expected .png suffix, got %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "project-image" {
		t.Errorf("expected 'project-image', got %q", string(data))
	}

	// Should NOT exist in default dir
	entries, _ := os.ReadDir(defaultDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), id) {
			t.Errorf("file should not be in default dir")
		}
	}
}

func TestPathIn_從指定目錄查找(t *testing.T) {
	defaultDir := filepath.Join(t.TempDir(), "default")
	s := NewStore(defaultDir, 30*time.Minute, 10<<20)

	customDir := filepath.Join(t.TempDir(), "project", ".piper", "clipboard")

	id, savedPath, err := s.SaveTo(customDir, strings.NewReader("data"), "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotPath, err := s.PathIn(customDir, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != savedPath {
		t.Errorf("expected %s, got %s", savedPath, gotPath)
	}

	// PathIn on default dir should NOT find it
	_, err = s.PathIn(defaultDir, id)
	if err == nil {
		t.Error("expected error when looking in wrong directory")
	}
}

func TestSaveFile_各種檔案類型保留副檔名(t *testing.T) {
	tests := []struct {
		filename    string
		expectedExt string
	}{
		{"report.xlsx", ".xlsx"},
		{"document.pdf", ".pdf"},
		{"data.csv", ".csv"},
		{"archive.zip", ".zip"},
		{"photo.png", ".png"},
		{"notes.txt", ".txt"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "clipboard")
			s := NewStore(dir, 30*time.Minute, 10<<20)

			id, path, err := s.SaveFile(strings.NewReader("fake-data"), tt.filename)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id == "" {
				t.Fatal("expected non-empty id")
			}
			if !strings.HasSuffix(path, tt.expectedExt) {
				t.Errorf("expected ext %s, got path %s", tt.expectedExt, path)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			if string(data) != "fake-data" {
				t.Errorf("expected 'fake-data', got %q", string(data))
			}
		})
	}
}

func TestSaveFile_無副檔名回傳錯誤(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	_, _, err := s.SaveFile(strings.NewReader("data"), "noext")
	if err == nil {
		t.Fatal("expected error for filename without extension")
	}
	if !strings.Contains(err.Error(), "no extension") {
		t.Errorf("expected error mentioning 'no extension', got: %v", err)
	}
}

func TestSaveFile_超過maxSize回傳錯誤(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10) // 10 bytes max

	bigData := bytes.Repeat([]byte("x"), 100)
	_, _, err := s.SaveFile(bytes.NewReader(bigData), "big.xlsx")
	if err == nil {
		t.Fatal("expected error for oversized data")
	}
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("expected error mentioning size, got: %v", err)
	}
}

func TestSaveFileTo_存到指定目錄(t *testing.T) {
	defaultDir := filepath.Join(t.TempDir(), "default")
	s := NewStore(defaultDir, 30*time.Minute, 10<<20)

	customDir := filepath.Join(t.TempDir(), "project", ".piper", "clipboard")

	id, path, err := s.SaveFileTo(customDir, strings.NewReader("project-file"), "report.xlsx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if !strings.HasPrefix(path, customDir) {
		t.Errorf("expected path under %s, got %s", customDir, path)
	}
	if !strings.HasSuffix(path, ".xlsx") {
		t.Errorf("expected .xlsx suffix, got %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "project-file" {
		t.Errorf("expected 'project-file', got %q", string(data))
	}
}

func TestSaveFile_可用Path查找(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 30*time.Minute, 10<<20)

	id, savedPath, err := s.SaveFile(strings.NewReader("data"), "test.xlsx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotPath, err := s.Path(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != savedPath {
		t.Errorf("expected path %s, got %s", savedPath, gotPath)
	}
}

func TestCleanup_刪除過期檔案保留新檔案(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clipboard")
	s := NewStore(dir, 1*time.Second, 10<<20)

	// Save a file
	_, oldPath, err := s.Save(strings.NewReader("old"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Make the file appear old by changing its mod time
	oldTime := time.Now().Add(-2 * time.Second)
	os.Chtimes(oldPath, oldTime, oldTime)

	// Save another file (should be new)
	_, newPath, err := s.Save(strings.NewReader("new"), "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Run cleanup
	s.Cleanup()

	// Old file should be gone
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old file to be deleted: %s", oldPath)
	}

	// New file should remain
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Errorf("expected new file to still exist: %s", newPath)
	}
}
