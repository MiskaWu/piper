package plan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifest(t *testing.T) {
	t.Run("檔案不存在時回傳 ErrManifestNotFound", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()

		// Act
		_, err := ReadManifest(dir)

		// Assert
		if !errors.Is(err, ErrManifestNotFound) {
			t.Fatalf("expected ErrManifestNotFound, got %v", err)
		}
	})

	t.Run("讀取 .plan/manifest.yaml 回傳 raw bytes", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		planDir := filepath.Join(dir, ".plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("setup .plan dir: %v", err)
		}
		want := []byte("version: 1\nproject:\n  name: sample\n")
		if err := os.WriteFile(filepath.Join(planDir, "manifest.yaml"), want, 0o644); err != nil {
			t.Fatalf("setup manifest: %v", err)
		}

		// Act
		got, err := ReadManifest(dir)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("content mismatch: got %q, want %q", got, want)
		}
	})
}

func TestManifestPath(t *testing.T) {
	got := ManifestPath("/tmp/proj")
	want := filepath.Join("/tmp/proj", ".plan", "manifest.yaml")
	if got != want {
		t.Fatalf("ManifestPath = %q, want %q", got, want)
	}
}
