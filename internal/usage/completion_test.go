package usage

import (
	"os"
	"testing"
	"time"
)

func TestCompletionTracker_Record_累積寫入(t *testing.T) {
	dir := t.TempDir()
	ct := NewCompletionTracker(dir)

	// Initially empty
	now := time.Now().UTC()
	entries, err := ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlyEntries empty: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0, got %d", len(entries))
	}

	// Record some completions
	if err := ct.Record("session-a", "/proj/a"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := ct.Record("session-b", "/proj/b"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := ct.Record("session-c", ""); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err = ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlyEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3, got %d", len(entries))
	}
}

func TestCompletionTracker_Record_寫入ProjectPath(t *testing.T) {
	dir := t.TempDir()
	ct := NewCompletionTracker(dir)

	if err := ct.Record("ragent-test-abc", "/home/user/my-app"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	now := time.Now().UTC()
	entries, err := ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlyEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ProjectPath != "/home/user/my-app" {
		t.Errorf("expected projectPath %q, got %q", "/home/user/my-app", entries[0].ProjectPath)
	}
}

func TestCompletionTracker_GetDailyCompletions_按時段分桶(t *testing.T) {
	dir := t.TempDir()
	ct := NewCompletionTracker(dir)

	// Entries spanning different time segments:
	// 10:00 → seg 2 (10-15), 14:00 → seg 2, 18:00 → seg 3 (15-20)
	// 09:00 → seg 1 (05-10), 12:00 → seg 2, 15:00 → seg 3
	content := `{"timestamp":"2026-03-05T10:00:00Z","sessionName":"s1"}
{"timestamp":"2026-03-05T14:00:00Z","sessionName":"s2"}
{"timestamp":"2026-03-05T18:00:00Z","sessionName":"s3"}
{"timestamp":"2026-03-10T09:00:00Z","sessionName":"s4"}
{"timestamp":"2026-03-18T12:00:00Z","sessionName":"s5"}
{"timestamp":"2026-03-18T15:00:00Z","sessionName":"s6"}
{"timestamp":"2026-03-18T02:00:00Z","sessionName":"s7"}
`
	if err := os.WriteFile(ct.filename(2026, time.March), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	daily, err := ct.GetDailyCompletions(2026, time.March)
	if err != nil {
		t.Fatalf("GetDailyCompletions: %v", err)
	}

	if len(daily) != 3 {
		t.Fatalf("expected 3 days, got %d", len(daily))
	}

	dayMap := make(map[string][SegmentCount]int)
	for _, d := range daily {
		dayMap[d.Date] = d.Segments
	}

	// Mar 5: seg2=2(10:00,14:00), seg3=1(18:00)
	if dayMap["2026-03-05"] != [5]int{0, 0, 2, 1, 0} {
		t.Errorf("2026-03-05 segments = %v, want [0 0 2 1 0]", dayMap["2026-03-05"])
	}
	// Mar 10: seg1=1(09:00)
	if dayMap["2026-03-10"] != [5]int{0, 1, 0, 0, 0} {
		t.Errorf("2026-03-10 segments = %v, want [0 1 0 0 0]", dayMap["2026-03-10"])
	}
	// Mar 18: seg0=1(02:00), seg2=1(12:00), seg3=1(15:00)
	if dayMap["2026-03-18"] != [5]int{1, 0, 1, 1, 0} {
		t.Errorf("2026-03-18 segments = %v, want [1 0 1 1 0]", dayMap["2026-03-18"])
	}
}

func TestHourToSegment_邊界值(t *testing.T) {
	tests := []struct {
		hour int
		want int
	}{
		{0, 0}, {4, 0},   // dawn
		{5, 1}, {9, 1},   // morning
		{10, 2}, {14, 2}, // midday
		{15, 3}, {19, 3}, // afternoon
		{20, 4}, {23, 4}, // night
	}
	for _, tt := range tests {
		got := hourToSegment(tt.hour)
		if got != tt.want {
			t.Errorf("hourToSegment(%d) = %d, want %d", tt.hour, got, tt.want)
		}
	}
}

func TestCompletionTracker_GetDailyCompletions_空月份回傳空陣列(t *testing.T) {
	dir := t.TempDir()
	ct := NewCompletionTracker(dir)

	daily, err := ct.GetDailyCompletions(2020, time.January)
	if err != nil {
		t.Fatalf("GetDailyCompletions: %v", err)
	}
	if daily == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(daily) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(daily))
	}
}

func TestCompletionTracker_Cleanup(t *testing.T) {
	dir := t.TempDir()
	ct := NewCompletionTracker(dir)

	// Create an old file
	oldFile := ct.filename(2020, time.January)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldFile, []byte(`{"timestamp":"2020-01-01T00:00:00Z","sessionName":"old"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ct.Cleanup(3); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Old file should be removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be removed")
	}
}
