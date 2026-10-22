package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStoreForRecoverable(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestSnapshotRecoverable_活躍session寫入並更新LastSeenAt(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	active := map[string]SessionMeta{
		"ragent-piper-aaa": {Title: "PIPER", ProjectPath: "/home/user/piper", ClaudeSessionID: "uuid-1"},
		"ragent-hydro-bbb": {Title: "HYDRO", Description: "working on auth"},
	}
	s.SnapshotRecoverable(active)

	entries, err := s.ListRecoverable()
	if err != nil {
		t.Fatalf("ListRecoverable: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify fields are captured
	found := false
	for _, e := range entries {
		if e.Name == "ragent-piper-aaa" {
			found = true
			if e.Title != "PIPER" {
				t.Errorf("expected title PIPER, got %s", e.Title)
			}
			if e.ClaudeSessionID != "uuid-1" {
				t.Errorf("expected claudeSessionId uuid-1, got %s", e.ClaudeSessionID)
			}
			if e.ProjectPath != "/home/user/piper" {
				t.Errorf("expected projectPath /home/user/piper, got %s", e.ProjectPath)
			}
		}
	}
	if !found {
		t.Error("ragent-piper-aaa not found in recoverable entries")
	}
}

func TestSnapshotRecoverable_已關閉session保留不覆寫(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	// First snapshot with two sessions
	active1 := map[string]SessionMeta{
		"ragent-a": {Title: "A", ClaudeSessionID: "id-a"},
		"ragent-b": {Title: "B", ClaudeSessionID: "id-b"},
	}
	s.SnapshotRecoverable(active1)

	// Second snapshot — ragent-b is gone (closed)
	active2 := map[string]SessionMeta{
		"ragent-a": {Title: "A", ClaudeSessionID: "id-a"},
	}
	s.SnapshotRecoverable(active2)

	entries, err := s.ListRecoverable()
	if err != nil {
		t.Fatalf("ListRecoverable: %v", err)
	}

	// Both should still exist
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (closed session preserved), got %d", len(entries))
	}

	// ragent-a should have newer LastSeenAt than ragent-b
	var aTime, bTime time.Time
	for _, e := range entries {
		if e.Name == "ragent-a" {
			aTime = e.LastSeenAt
		}
		if e.Name == "ragent-b" {
			bTime = e.LastSeenAt
		}
	}
	if !aTime.After(bTime) {
		t.Error("expected ragent-a LastSeenAt to be after ragent-b")
	}
}

func TestListRecoverable_過濾過期條目(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	// Write entries with different ages
	entries := []RecoverableSession{
		{Name: "recent", Title: "Recent", LastSeenAt: time.Now().Add(-1 * 24 * time.Hour)},
		{Name: "expired", Title: "Expired", LastSeenAt: time.Now().Add(-30 * 24 * time.Hour)},
	}
	if err := s.writeRecoverable(entries); err != nil {
		t.Fatalf("writeRecoverable: %v", err)
	}

	// Default retention is 7 days
	result, err := s.ListRecoverable()
	if err != nil {
		t.Fatalf("ListRecoverable: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (filtered expired), got %d", len(result))
	}
	if result[0].Name != "recent" {
		t.Errorf("expected 'recent', got %s", result[0].Name)
	}
}

func TestRemoveRecoverable_移除單筆(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	entries := []RecoverableSession{
		{Name: "keep", Title: "Keep", LastSeenAt: time.Now()},
		{Name: "remove", Title: "Remove", LastSeenAt: time.Now()},
	}
	if err := s.writeRecoverable(entries); err != nil {
		t.Fatalf("writeRecoverable: %v", err)
	}

	if err := s.RemoveRecoverable("remove"); err != nil {
		t.Fatalf("RemoveRecoverable: %v", err)
	}

	remaining, err := s.loadRecoverable()
	if err != nil {
		t.Fatalf("loadRecoverable: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(remaining))
	}
	if remaining[0].Name != "keep" {
		t.Errorf("expected 'keep', got %s", remaining[0].Name)
	}
}

func TestPruneRecoverable_清除過期(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	entries := []RecoverableSession{
		{Name: "fresh", Title: "Fresh", LastSeenAt: time.Now()},
		{Name: "old", Title: "Old", LastSeenAt: time.Now().Add(-10 * 24 * time.Hour)},
	}
	if err := s.writeRecoverable(entries); err != nil {
		t.Fatalf("writeRecoverable: %v", err)
	}

	pruned, err := s.PruneRecoverable()
	if err != nil {
		t.Fatalf("PruneRecoverable: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	remaining, err := s.loadRecoverable()
	if err != nil {
		t.Fatalf("loadRecoverable: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
	if remaining[0].Name != "fresh" {
		t.Errorf("expected 'fresh', got %s", remaining[0].Name)
	}
}

func TestRecoverablePath_使用store同目錄(t *testing.T) {
	s := New("/tmp/test-piper/sessions.json")
	expected := "/tmp/test-piper/recoverable-sessions.json"
	if got := s.recoverablePath(); got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestLoadRecoverable_檔案不存在回傳空(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.json"))
	entries, err := s.loadRecoverable()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestWriteRecoverable_atomic寫入(t *testing.T) {
	s := newTestStoreForRecoverable(t)

	entries := []RecoverableSession{
		{Name: "test", Title: "Test", LastSeenAt: time.Now()},
	}
	if err := s.writeRecoverable(entries); err != nil {
		t.Fatalf("writeRecoverable: %v", err)
	}

	// Verify file exists and tmp doesn't
	if _, err := os.Stat(s.recoverablePath()); err != nil {
		t.Errorf("recoverable file should exist: %v", err)
	}
	if _, err := os.Stat(s.recoverablePath() + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after write")
	}
}
