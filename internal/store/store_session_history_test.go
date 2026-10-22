package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestArchiveSession_寫入歷史並刪除META(t *testing.T) {
	s := newTestStore(t)
	s.SetMeta("ragent-piper-aaa", SessionMeta{
		Description: "修復登入問題",
		ProjectPath: "/home/user/piper",
	})

	if err := s.ArchiveSession("ragent-piper-aaa"); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	// META should be gone
	if got := s.GetMeta("ragent-piper-aaa"); got != nil {
		t.Errorf("expected nil after archive, got %+v", got)
	}

	// History should have the entry
	entries, err := s.GetSessionHistory(
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].Name != "ragent-piper-aaa" {
		t.Errorf("expected name ragent-piper-aaa, got %s", entries[0].Name)
	}
	if entries[0].Description != "修復登入問題" {
		t.Errorf("expected description '修復登入問題', got %s", entries[0].Description)
	}
	if entries[0].ProjectPath != "/home/user/piper" {
		t.Errorf("expected projectPath /home/user/piper, got %s", entries[0].ProjectPath)
	}
}

func TestArchiveSession_無描述無路徑時只刪不存檔(t *testing.T) {
	s := newTestStore(t)
	s.SetMeta("ragent-empty", SessionMeta{Title: "EMPTY"})

	if err := s.ArchiveSession("ragent-empty"); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	// META should be gone
	if got := s.GetMeta("ragent-empty"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}

	// History should be empty
	entries, err := s.GetSessionHistory(
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 history entries, got %d", len(entries))
	}
}

func TestArchiveSession_不存在的session是noop(t *testing.T) {
	s := newTestStore(t)

	if err := s.ArchiveSession("nonexistent"); err != nil {
		t.Fatalf("ArchiveSession should be noop, got: %v", err)
	}
}

func TestGetSessionHistory_按日期範圍過濾(t *testing.T) {
	s := newTestStore(t)

	// Manually write history entries with different dates
	now := time.Now()
	entries := []SessionHistoryEntry{
		{Name: "old", Description: "old work", ClosedAt: now.Add(-48 * time.Hour)},
		{Name: "yesterday", Description: "yesterday work", ClosedAt: now.Add(-24 * time.Hour)},
		{Name: "today", Description: "today work", ClosedAt: now},
	}
	if err := s.writeHistory(entries); err != nil {
		t.Fatalf("writeHistory: %v", err)
	}

	// Query only last 25 hours
	got, err := s.GetSessionHistory(
		now.Add(-25*time.Hour),
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Name != "yesterday" {
		t.Errorf("expected 'yesterday', got %s", got[0].Name)
	}
	if got[1].Name != "today" {
		t.Errorf("expected 'today', got %s", got[1].Name)
	}
}

func TestPruneHistory_清理過期記錄(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()
	entries := []SessionHistoryEntry{
		{Name: "ancient", Description: "very old", ClosedAt: now.Add(-60 * 24 * time.Hour)},
		{Name: "old", Description: "old", ClosedAt: now.Add(-31 * 24 * time.Hour)},
		{Name: "recent", Description: "recent", ClosedAt: now.Add(-1 * time.Hour)},
	}
	if err := s.writeHistory(entries); err != nil {
		t.Fatalf("writeHistory: %v", err)
	}

	pruned, err := s.PruneHistory(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneHistory: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	// Only recent should remain
	remaining, err := s.loadHistory()
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
	if remaining[0].Name != "recent" {
		t.Errorf("expected 'recent', got %s", remaining[0].Name)
	}
}

func TestPruneHistory_無過期不寫檔(t *testing.T) {
	s := newTestStore(t)

	now := time.Now()
	entries := []SessionHistoryEntry{
		{Name: "fresh", Description: "just now", ClosedAt: now},
	}
	if err := s.writeHistory(entries); err != nil {
		t.Fatalf("writeHistory: %v", err)
	}

	pruned, err := s.PruneHistory(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneHistory: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned, got %d", pruned)
	}
}

func TestGetSessionHistory_檔案不存在回傳空(t *testing.T) {
	s := newTestStore(t)

	entries, err := s.GetSessionHistory(
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil, got %v", entries)
	}
}

func TestArchiveSession_多次歸檔累積(t *testing.T) {
	s := newTestStore(t)

	s.SetMeta("sess-1", SessionMeta{Description: "task 1", ProjectPath: "/proj"})
	s.SetMeta("sess-2", SessionMeta{Description: "task 2", ProjectPath: "/proj"})

	if err := s.ArchiveSession("sess-1"); err != nil {
		t.Fatalf("ArchiveSession sess-1: %v", err)
	}
	if err := s.ArchiveSession("sess-2"); err != nil {
		t.Fatalf("ArchiveSession sess-2: %v", err)
	}

	entries, err := s.GetSessionHistory(
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestSnapshotActiveSessions_快照活躍session到歷史(t *testing.T) {
	s := newTestStore(t)

	s.SetMeta("active-1", SessionMeta{Description: "working on feature", ProjectPath: "/proj1"})
	s.SetMeta("active-2", SessionMeta{Description: "debugging", ProjectPath: "/proj2"})
	s.SetMeta("no-desc", SessionMeta{Title: "EMPTY"}) // no description, no path

	activeNames := []string{"active-1", "active-2", "no-desc"}
	n, err := s.SnapshotActiveSessions(activeNames)
	if err != nil {
		t.Fatalf("SnapshotActiveSessions: %v", err)
	}
	// no-desc has no description/projectPath, should be skipped
	if n != 2 {
		t.Errorf("expected 2 snapshots, got %d", n)
	}

	entries, err := s.GetSessionHistory(
		time.Now().Add(-time.Minute),
		time.Now().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Sessions should NOT be removed from META (still active)
	if got := s.GetMeta("active-1"); got == nil {
		t.Error("active-1 should still be in META")
	}
	if got := s.GetMeta("active-2"); got == nil {
		t.Error("active-2 should still be in META")
	}
}
