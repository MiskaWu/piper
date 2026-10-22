package browse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAll_回傳檔案與目錄(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	os.MkdirAll(filepath.Join(tmp, "docs"), 0755)
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# hi"), 0644)

	result, err := ListAll(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Current != tmp {
		t.Errorf("current = %q, want %q", result.Current, tmp)
	}
	// 2 dirs + 2 files = 4
	if len(result.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result.Entries))
	}
	// Dirs come first, sorted
	if result.Entries[0].Name != "docs" || !result.Entries[0].IsDir {
		t.Errorf("entries[0] = %+v, want docs dir", result.Entries[0])
	}
	if result.Entries[1].Name != "src" || !result.Entries[1].IsDir {
		t.Errorf("entries[1] = %+v, want src dir", result.Entries[1])
	}
	// Files after dirs, sorted
	if result.Entries[2].Name != "README.md" || result.Entries[2].IsDir {
		t.Errorf("entries[2] = %+v, want README.md file", result.Entries[2])
	}
	if result.Entries[3].Name != "main.go" || result.Entries[3].IsDir {
		t.Errorf("entries[3] = %+v, want main.go file", result.Entries[3])
	}
}

func TestListAll_跳過Dotfiles(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmp, "visible.txt"), []byte("hi"), 0644)

	result, err := ListAll(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (visible.txt), got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "visible.txt" {
		t.Errorf("entry = %q, want visible.txt", result.Entries[0].Name)
	}
}

func TestListAll_跳過噪音目錄(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "node_modules"), 0755)
	os.MkdirAll(filepath.Join(tmp, "__pycache__"), 0755)
	os.MkdirAll(filepath.Join(tmp, "src"), 0755)

	result, err := ListAll(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry (src), got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "src" {
		t.Errorf("entry = %q, want src", result.Entries[0].Name)
	}
}

func TestListAll_空目錄(t *testing.T) {
	tmp := t.TempDir()
	result, err := ListAll(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result.Entries))
	}
}

func TestListAll_不存在路徑(t *testing.T) {
	_, err := ListAll("/nonexistent/path/xyz")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestListAll_拒絕路徑穿越(t *testing.T) {
	_, err := ListAll("/tmp/../etc")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestListAll_目錄優先排序(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "aaa.txt"), []byte(""), 0644)
	os.MkdirAll(filepath.Join(tmp, "zzz"), 0755)

	result, err := ListAll(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	// Dir should come first even though 'z' > 'a'
	if result.Entries[0].Name != "zzz" || !result.Entries[0].IsDir {
		t.Errorf("entries[0] = %+v, want zzz dir first", result.Entries[0])
	}
	if result.Entries[1].Name != "aaa.txt" || result.Entries[1].IsDir {
		t.Errorf("entries[1] = %+v, want aaa.txt file", result.Entries[1])
	}
}
