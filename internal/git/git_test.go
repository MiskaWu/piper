package git

import (
	"errors"
	"testing"
)

func TestParseStatus_Clean(t *testing.T) {
	got := ParseStatus("")
	if got.Dirty {
		t.Error("expected clean repo, got dirty")
	}
	if got.ChangedFiles != 0 {
		t.Errorf("expected 0 changed files, got %d", got.ChangedFiles)
	}
}

func TestParseStatus_Modified(t *testing.T) {
	output := " M routes.go\n M routes_test.go\n"
	got := ParseStatus(output)
	if !got.Dirty {
		t.Error("expected dirty repo")
	}
	if got.ChangedFiles != 2 {
		t.Errorf("expected 2 changed files, got %d", got.ChangedFiles)
	}
}

func TestParseStatus_MixedChanges(t *testing.T) {
	output := "M  internal/store/store.go\n M internal/tmux/tmux.go\nA  new_file.go\n?? docs/setup.md\n?? refs/\n"
	got := ParseStatus(output)
	if !got.Dirty {
		t.Error("expected dirty repo")
	}
	if got.ChangedFiles != 5 {
		t.Errorf("expected 5 changed files, got %d", got.ChangedFiles)
	}
}

func TestParseStatus_EmptyLines(t *testing.T) {
	output := " M routes.go\n\n\n"
	got := ParseStatus(output)
	if got.ChangedFiles != 1 {
		t.Errorf("expected 1 changed file, got %d", got.ChangedFiles)
	}
}

type mockCommander struct {
	output []byte
	err    error
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	return m.output, m.err
}

func (m *mockCommander) Run(name string, args ...string) error {
	return m.err
}

func TestGetStatus_CleanRepo(t *testing.T) {
	cmd := &mockCommander{output: []byte("")}
	checker := NewChecker(cmd)
	got, err := checker.GetStatus("/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Dirty {
		t.Error("expected clean repo")
	}
}

func TestGetStatus_DirtyRepo(t *testing.T) {
	cmd := &mockCommander{output: []byte(" M file.go\n?? newfile.go\n")}
	checker := NewChecker(cmd)
	got, err := checker.GetStatus("/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Dirty {
		t.Error("expected dirty repo")
	}
	if got.ChangedFiles != 2 {
		t.Errorf("expected 2 changed files, got %d", got.ChangedFiles)
	}
}

func TestGetStatus_NotGitRepo(t *testing.T) {
	cmd := &mockCommander{
		output: []byte("fatal: not a git repository"),
		err:    errors.New("exit status 128"),
	}
	checker := NewChecker(cmd)
	got, err := checker.GetStatus("/not/a/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Dirty {
		t.Error("expected not dirty for non-git repo")
	}
	if got.ChangedFiles != 0 {
		t.Errorf("expected 0 changed files, got %d", got.ChangedFiles)
	}
}

func TestParseFileStatuses_各種狀態(t *testing.T) {
	output := " M file1.go\nA  new.go\n D deleted.go\n?? untracked.txt\n"
	result := ParseFileStatuses(output)
	if len(result) != 4 {
		t.Fatalf("expected 4 statuses, got %d", len(result))
	}
	tests := []struct {
		path   string
		status string
	}{
		{"file1.go", "M"},
		{"new.go", "A"},
		{"deleted.go", "D"},
		{"untracked.txt", "?"},
	}
	for i, tt := range tests {
		if result[i].Path != tt.path {
			t.Errorf("[%d] path = %q, want %q", i, result[i].Path, tt.path)
		}
		if result[i].Status != tt.status {
			t.Errorf("[%d] status = %q, want %q", i, result[i].Status, tt.status)
		}
	}
}

func TestParseFileStatuses_空輸出(t *testing.T) {
	result := ParseFileStatuses("")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestParseFileStatuses_Rename(t *testing.T) {
	output := "R  old.go -> new.go\n"
	result := ParseFileStatuses(output)
	if len(result) != 1 {
		t.Fatalf("expected 1 status, got %d", len(result))
	}
	if result[0].Status != "R" {
		t.Errorf("status = %q, want R", result[0].Status)
	}
	if result[0].Path != "new.go" {
		t.Errorf("path = %q, want new.go", result[0].Path)
	}
}

func TestGetFileStatuses_正常Repo(t *testing.T) {
	cmd := &mockCommander{output: []byte(" M file.go\n?? newfile.go\n")}
	checker := NewChecker(cmd)
	got, err := checker.GetFileStatuses("/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(got))
	}
	if got[0].Path != "file.go" || got[0].Status != "M" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Path != "newfile.go" || got[1].Status != "?" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestGetFileStatuses_非GitRepo(t *testing.T) {
	cmd := &mockCommander{
		output: []byte("fatal: not a git repository"),
		err:    errors.New("exit status 128"),
	}
	checker := NewChecker(cmd)
	got, err := checker.GetFileStatuses("/not/a/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

func TestGetDirtyFilePaths_正常Repo(t *testing.T) {
	cmd := &mockCommander{output: []byte(" M file.go\n?? newfile.go\n")}
	checker := NewChecker(cmd)
	paths, err := checker.GetDirtyFilePaths("/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "file.go" || paths[1] != "newfile.go" {
		t.Errorf("paths = %v", paths)
	}
}

func TestGetDirtyFilePaths_乾淨Repo(t *testing.T) {
	cmd := &mockCommander{output: []byte("")}
	checker := NewChecker(cmd)
	paths, err := checker.GetDirtyFilePaths("/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty paths, got %d", len(paths))
	}
}

func TestGetDirtyFilePaths_非GitRepo(t *testing.T) {
	cmd := &mockCommander{
		output: []byte("fatal: not a git repository"),
		err:    errors.New("exit status 128"),
	}
	checker := NewChecker(cmd)
	paths, err := checker.GetDirtyFilePaths("/not/a/repo")
	if !errors.Is(err, ErrNotRepo) {
		t.Errorf("expected ErrNotRepo, got %v", err)
	}
	if paths != nil {
		t.Errorf("expected nil paths, got %v", paths)
	}
}
