package browse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFiles_單一檔案(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "hello.txt"), "content", 0644)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "hello.txt")},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 copied entry, got %d", len(result.Entries))
	}

	got := readFile(t, result.Entries[0].Destination)
	if got != "content" {
		t.Errorf("expected content, got %q", got)
	}
}

func TestCopyFiles_目錄遞迴(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	subDir := filepath.Join(src, "mydir", "sub")
	os.MkdirAll(subDir, 0755)
	writeFile(t, filepath.Join(src, "mydir", "a.txt"), "aaa", 0644)
	writeFile(t, filepath.Join(subDir, "b.txt"), "bbb", 0644)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "mydir")},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destDir := result.Entries[0].Destination
	if readFile(t, filepath.Join(destDir, "a.txt")) != "aaa" {
		t.Error("a.txt content mismatch")
	}
	if readFile(t, filepath.Join(destDir, "sub", "b.txt")) != "bbb" {
		t.Error("sub/b.txt content mismatch")
	}
}

func TestCopyFiles_名稱衝突_有副檔名(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "file.txt"), "original", 0644)
	writeFile(t, filepath.Join(dst, "file.txt"), "existing", 0644)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "file.txt")},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destName := filepath.Base(result.Entries[0].Destination)
	if destName != "file (1).txt" {
		t.Errorf("expected 'file (1).txt', got %q", destName)
	}

	// Original should be untouched.
	if readFile(t, filepath.Join(dst, "file.txt")) != "existing" {
		t.Error("original file was modified")
	}
}

func TestCopyFiles_名稱衝突_無副檔名(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "README"), "src", 0644)
	writeFile(t, filepath.Join(dst, "README"), "dst", 0644)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "README")},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destName := filepath.Base(result.Entries[0].Destination)
	if destName != "README (1)" {
		t.Errorf("expected 'README (1)', got %q", destName)
	}
}

func TestCopyFiles_路徑穿越_source(t *testing.T) {
	dst := t.TempDir()
	_, err := CopyFiles(CopyRequest{
		Sources:     []string{"../../../etc/passwd"},
		Destination: dst,
	})
	if err == nil {
		t.Fatal("expected error for path traversal in source")
	}
}

func TestCopyFiles_路徑穿越_destination(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "f.txt"), "x", 0644)

	_, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "f.txt")},
		Destination: "/tmp/../etc",
	})
	if err == nil {
		t.Fatal("expected error for path traversal in destination")
	}
}

func TestCopyFiles_目的地不存在(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "f.txt"), "x", 0644)

	_, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(src, "f.txt")},
		Destination: "/nonexistent-dir-12345",
	})
	if err == nil {
		t.Fatal("expected error for non-existent destination")
	}
}

func TestCopyFiles_複製到同目錄(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "file.txt"), "data", 0644)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{filepath.Join(dir, "file.txt")},
		Destination: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destName := filepath.Base(result.Entries[0].Destination)
	if destName != "file (1).txt" {
		t.Errorf("expected 'file (1).txt', got %q", destName)
	}
}

func TestMoveFiles_單一檔案(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	srcFile := filepath.Join(src, "move-me.txt")
	writeFile(t, srcFile, "movable", 0644)

	result, err := MoveFiles(CopyRequest{
		Sources:     []string{srcFile},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if readFile(t, result.Entries[0].Destination) != "movable" {
		t.Error("moved file content mismatch")
	}

	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Error("source file should have been removed")
	}
}

func TestMoveFiles_目錄(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	dirPath := filepath.Join(src, "movedir")
	os.MkdirAll(dirPath, 0755)
	writeFile(t, filepath.Join(dirPath, "inner.txt"), "inside", 0644)

	result, err := MoveFiles(CopyRequest{
		Sources:     []string{dirPath},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if readFile(t, filepath.Join(result.Entries[0].Destination, "inner.txt")) != "inside" {
		t.Error("moved dir content mismatch")
	}

	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Error("source dir should have been removed")
	}
}

func TestCopyFiles_保留權限(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	srcFile := filepath.Join(src, "exec.sh")
	writeFile(t, srcFile, "#!/bin/sh", 0755)

	result, err := CopyFiles(CopyRequest{
		Sources:     []string{srcFile},
		Destination: dst,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(result.Entries[0].Destination)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %o", info.Mode().Perm())
	}
}

// helpers

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
