package claude

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

func TestReadDailyHistory_正常讀取按日期過濾及按project分組(t *testing.T) {
	result, err := ReadDailyHistory(testdataPath("history.jsonl"), "2026-03-11", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// project-a should have 2 entries (implement feature X, fix bug in module Y)
	entriesA, ok := result["/home/user/project-a"]
	if !ok {
		t.Fatal("expected entries for /home/user/project-a")
	}
	if len(entriesA) != 2 {
		t.Fatalf("expected 2 entries for project-a, got %d", len(entriesA))
	}
	if entriesA[0].Display != "implement feature X" {
		t.Errorf("expected 'implement feature X', got %q", entriesA[0].Display)
	}
	if entriesA[1].Display != "fix bug in module Y" {
		t.Errorf("expected 'fix bug in module Y', got %q", entriesA[1].Display)
	}
	if entriesA[0].SessionID != "aaa-111" {
		t.Errorf("expected sessionId 'aaa-111', got %q", entriesA[0].SessionID)
	}

	// project-b should have 1 entry (refactor service Z)
	entriesB, ok := result["/home/user/project-b"]
	if !ok {
		t.Fatal("expected entries for /home/user/project-b")
	}
	if len(entriesB) != 1 {
		t.Fatalf("expected 1 entry for project-b, got %d", len(entriesB))
	}
	if entriesB[0].Display != "refactor service Z" {
		t.Errorf("expected 'refactor service Z', got %q", entriesB[0].Display)
	}
}

func TestReadDailyHistory_過濾系統事件(t *testing.T) {
	result, err := ReadDailyHistory(testdataPath("history.jsonl"), "2026-03-11", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// project-a should NOT contain exit, login, empty, /help, /compact
	entriesA := result["/home/user/project-a"]
	for _, e := range entriesA {
		switch e.Display {
		case "exit", "login", "", "/help", "/compact":
			t.Errorf("system event %q should have been filtered out", e.Display)
		}
	}
}

func TestReadDailyHistory_不同日期不混入(t *testing.T) {
	result, err := ReadDailyHistory(testdataPath("history.jsonl"), "2026-03-10", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entriesA, ok := result["/home/user/project-a"]
	if !ok {
		t.Fatal("expected entries for /home/user/project-a on 2026-03-10")
	}
	if len(entriesA) != 1 {
		t.Fatalf("expected 1 entry for 2026-03-10, got %d", len(entriesA))
	}
	if entriesA[0].Display != "different day prompt" {
		t.Errorf("expected 'different day prompt', got %q", entriesA[0].Display)
	}

	// project-b should not exist on this date
	if _, ok := result["/home/user/project-b"]; ok {
		t.Error("project-b should not have entries on 2026-03-10")
	}
}

func TestReadDailyHistory_檔案不存在(t *testing.T) {
	_, err := ReadDailyHistory("/nonexistent/path/history.jsonl", "2026-03-11", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadDailyHistory_空檔案(t *testing.T) {
	result, err := ReadDailyHistory(testdataPath("empty.jsonl"), "2026-03-11", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestReadDailyHistory_時區感知_台北跨午夜(t *testing.T) {
	// history_tz.jsonl 有兩筆：
	// - UTC 2026-04-06 15:00 = 台北 2026-04-06 23:00（台北 4/6）
	// - UTC 2026-04-06 17:00 = 台北 2026-04-07 01:00（台北 4/7）
	taipei, _ := time.LoadLocation("Asia/Taipei")

	// 用台北時區查 4/6：應只有 "evening work"
	result, err := ReadDailyHistory(testdataPath("history_tz.jsonl"), "2026-04-06", taipei)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := result["/home/user/proj"]
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for 4/6 (Taipei), got %d", len(entries))
	}
	if entries[0].Display != "evening work" {
		t.Errorf("expected 'evening work', got %q", entries[0].Display)
	}

	// 用台北時區查 4/7：應只有 "after midnight work"
	result2, err := ReadDailyHistory(testdataPath("history_tz.jsonl"), "2026-04-07", taipei)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries2 := result2["/home/user/proj"]
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry for 4/7 (Taipei), got %d", len(entries2))
	}
	if entries2[0].Display != "after midnight work" {
		t.Errorf("expected 'after midnight work', got %q", entries2[0].Display)
	}

	// 用 UTC 查 4/6：兩筆都在 UTC 4/6
	result3, err := ReadDailyHistory(testdataPath("history_tz.jsonl"), "2026-04-06", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result3["/home/user/proj"]) != 2 {
		t.Errorf("expected 2 entries for 4/6 (UTC), got %d", len(result3["/home/user/proj"]))
	}
}
