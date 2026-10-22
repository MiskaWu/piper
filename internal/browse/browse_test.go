package browse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDir_ReturnsSubdirectories(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "project-a"), 0755)
	os.MkdirAll(filepath.Join(tmp, "project-b"), 0755)
	os.WriteFile(filepath.Join(tmp, "readme.txt"), []byte("hi"), 0644)

	result, err := ListDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Current != tmp {
		t.Errorf("expected current %q, got %q", tmp, result.Current)
	}
	if result.Parent != filepath.Dir(tmp) {
		t.Errorf("expected parent %q, got %q", filepath.Dir(tmp), result.Parent)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(result.Entries))
	}
	names := map[string]bool{}
	for _, e := range result.Entries {
		names[e.Name] = true
		if !e.IsDir {
			t.Errorf("expected isDir for %s", e.Name)
		}
	}
	if !names["project-a"] || !names["project-b"] {
		t.Errorf("expected project-a and project-b, got %v", names)
	}
}

func TestListDir_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	result, err := ListDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result.Entries))
	}
}

func TestListDir_NonExistentPath(t *testing.T) {
	_, err := ListDir("/nonexistent/path/xyz")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestListDir_RejectsPathTraversal(t *testing.T) {
	_, err := ListDir("/tmp/../etc")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestListDir_SortedAlphabetically(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "zebra"), 0755)
	os.MkdirAll(filepath.Join(tmp, "alpha"), 0755)
	os.MkdirAll(filepath.Join(tmp, "beta"), 0755)

	result, err := ListDir(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "alpha" || result.Entries[1].Name != "beta" || result.Entries[2].Name != "zebra" {
		t.Errorf("expected sorted order, got %s, %s, %s",
			result.Entries[0].Name, result.Entries[1].Name, result.Entries[2].Name)
	}
}
