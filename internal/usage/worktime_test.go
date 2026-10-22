package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewWorkTimeTracker(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.dir != dir {
		t.Errorf("expected dir %q, got %q", dir, tracker.dir)
	}
}

func TestWorkTimeTracker_Update_開始工作(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Session starts working
	err := tracker.Update([]SessionStatus{
		{Name: "ragent-test", Status: "working"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Should track as active, no file written yet
	as, ok := tracker.active["ragent-test"]
	if !ok {
		t.Error("expected session to be tracked as active")
	}
	if as.projectPath != "" {
		t.Errorf("expected empty projectPath, got %q", as.projectPath)
	}
}

func TestWorkTimeTracker_Update_工作結束寫入記錄(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Start working
	tracker.active["ragent-test"] = activeSession{start: time.Now().Add(-30 * time.Second)}

	// Transition to idle
	err := tracker.Update([]SessionStatus{
		{Name: "ragent-test", Status: "idle"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Should no longer be active
	if _, ok := tracker.active["ragent-test"]; ok {
		t.Error("expected session to be removed from active")
	}

	// Verify file content
	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var entry WorkTimeEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.SessionName != "ragent-test" {
		t.Errorf("expected session name ragent-test, got %s", entry.SessionName)
	}
	if entry.DurationSec < 29 || entry.DurationSec > 31 {
		t.Errorf("expected ~30 sec duration, got %d", entry.DurationSec)
	}
}

func TestWorkTimeTracker_Update_Session消失視為結束(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Start working
	tracker.active["ragent-gone"] = activeSession{start: time.Now().Add(-60 * time.Second)}

	// Session disappears (empty list)
	err := tracker.Update([]SessionStatus{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if _, ok := tracker.active["ragent-gone"]; ok {
		t.Error("expected disappeared session to be removed from active")
	}

	// Should have written an entry
	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	if _, err := os.Stat(filename); err != nil {
		t.Error("expected worktime file to be written")
	}
}

func TestWorkTimeTracker_Update_寫入ProjectPath(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Start working with projectPath
	tracker.active["ragent-proj"] = activeSession{
		start:       time.Now().Add(-30 * time.Second),
		projectPath: "/home/user/my-app",
	}

	// Transition to idle → should write entry with projectPath
	err := tracker.Update([]SessionStatus{
		{Name: "ragent-proj", Status: "idle"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var entry WorkTimeEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.ProjectPath != "/home/user/my-app" {
		t.Errorf("expected projectPath %q, got %q", "/home/user/my-app", entry.ProjectPath)
	}
}

func TestWorkTimeTracker_Update_SessionStatus傳入ProjectPath(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Start working via Update with ProjectPath in SessionStatus
	err := tracker.Update([]SessionStatus{
		{Name: "ragent-new", Status: "working", ProjectPath: "/home/user/proj"},
	})
	if err != nil {
		t.Fatalf("Update start: %v", err)
	}

	as, ok := tracker.active["ragent-new"]
	if !ok {
		t.Fatal("expected session to be tracked")
	}
	if as.projectPath != "/home/user/proj" {
		t.Errorf("expected projectPath %q, got %q", "/home/user/proj", as.projectPath)
	}
}

func TestWorkTimeTracker_Update_非working不追蹤(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	err := tracker.Update([]SessionStatus{
		{Name: "ragent-idle", Status: "idle"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, ok := tracker.active["ragent-idle"]; ok {
		t.Error("idle session should not be tracked as active")
	}
}

func TestWorkTimeTracker_Update_短時間不記錄(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	// Working for only 0 seconds
	tracker.active["ragent-brief"] = activeSession{start: time.Now()}

	err := tracker.Update([]SessionStatus{
		{Name: "ragent-brief", Status: "idle"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// File should not be created for very short durations
	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Error("expected no file for very short work duration")
	}
}

func TestWorkTimeTracker_GetMonthlyEntries(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	f, _ := os.Create(filename)
	for i := 0; i < 3; i++ {
		entry := WorkTimeEntry{
			Timestamp:   now.Add(time.Duration(i) * time.Hour),
			SessionName: fmt.Sprintf("ragent-test-%d", i),
			DurationSec: (i + 1) * 60,
		}
		data, _ := json.Marshal(entry)
		f.Write(append(data, '\n'))
	}
	f.Close()

	tracker := NewWorkTimeTracker(dir)
	entries, err := tracker.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlyEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestWorkTimeTracker_GetMonthlyEntries_無檔案(t *testing.T) {
	dir := t.TempDir()
	tracker := NewWorkTimeTracker(dir)

	entries, err := tracker.GetMonthlyEntries(2025, time.January)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestWorkTimeTracker_GetMonthlySummary_按專案路徑聚合(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	f, _ := os.Create(filename)

	entries := []WorkTimeEntry{
		{Timestamp: now, SessionName: "ragent-piper-aaa", DurationSec: 120, ProjectPath: "/home/user/piper"},
		{Timestamp: now, SessionName: "ragent-piper-bbb", DurationSec: 180, ProjectPath: "/home/user/piper"},
		{Timestamp: now, SessionName: "ragent-chart-ccc", DurationSec: 60, ProjectPath: "/home/user/chartless"},
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(append(data, '\n'))
	}
	f.Close()

	tracker := NewWorkTimeTracker(dir)
	summary, err := tracker.GetMonthlySummary(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.TotalDurationSec != 360 {
		t.Errorf("expected total 360 sec, got %d", summary.TotalDurationSec)
	}
	if len(summary.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(summary.Projects))
	}

	// Check per-project totals
	projMap := map[string]int{}
	for _, p := range summary.Projects {
		projMap[p.ProjectPath] = p.TotalDurationSec
	}
	if projMap["/home/user/piper"] != 300 {
		t.Errorf("expected piper total 300, got %d", projMap["/home/user/piper"])
	}
	if projMap["/home/user/chartless"] != 60 {
		t.Errorf("expected chartless total 60, got %d", projMap["/home/user/chartless"])
	}
}

func TestWorkTimeTracker_GetMonthlySummary_空ProjectPath歸入unknown(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	f, _ := os.Create(filename)

	entries := []WorkTimeEntry{
		{Timestamp: now, SessionName: "ragent-old", DurationSec: 100},
		{Timestamp: now, SessionName: "ragent-old2", DurationSec: 50},
		{Timestamp: now, SessionName: "ragent-new", DurationSec: 200, ProjectPath: "/home/user/proj"},
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(append(data, '\n'))
	}
	f.Close()

	tracker := NewWorkTimeTracker(dir)
	summary, err := tracker.GetMonthlySummary(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if len(summary.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(summary.Projects))
	}

	projMap := map[string]int{}
	for _, p := range summary.Projects {
		projMap[p.ProjectPath] = p.TotalDurationSec
	}
	if projMap["(unknown)"] != 150 {
		t.Errorf("expected unknown total 150, got %d", projMap["(unknown)"])
	}
	if projMap["/home/user/proj"] != 200 {
		t.Errorf("expected proj total 200, got %d", projMap["/home/user/proj"])
	}
}

func TestWorkTimeTracker_GetDailyWorkTime(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month()))
	f, _ := os.Create(filename)

	day1 := time.Date(now.Year(), now.Month(), 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(now.Year(), now.Month(), 2, 14, 0, 0, 0, time.UTC)
	entries := []WorkTimeEntry{
		{Timestamp: day1, SessionName: "ragent-a", DurationSec: 3600},
		{Timestamp: day1.Add(time.Hour), SessionName: "ragent-b", DurationSec: 1800},
		{Timestamp: day2, SessionName: "ragent-a", DurationSec: 7200},
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(append(data, '\n'))
	}
	f.Close()

	tracker := NewWorkTimeTracker(dir)
	daily, err := tracker.GetDailyWorkTime(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetDailyWorkTime: %v", err)
	}
	if len(daily) != 2 {
		t.Fatalf("expected 2 days, got %d", len(daily))
	}
	if daily[0].WorkSec != 5400 {
		t.Errorf("expected day 1 = 5400, got %d", daily[0].WorkSec)
	}
	if daily[1].WorkSec != 7200 {
		t.Errorf("expected day 2 = 7200, got %d", daily[1].WorkSec)
	}
}

func TestWorkTimeTracker_GetDailySegmentedWorkTime_按時段分桶(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	filename := filepath.Join(dir, "worktime-2026-03.jsonl")
	f, _ := os.Create(filename)

	// Day 5: entries at 02:00 (seg 0), 10:00 (seg 2), 16:00 (seg 3)
	// Day 10: entry at 21:00 (seg 4)
	entries := []WorkTimeEntry{
		{Timestamp: time.Date(2026, 3, 5, 2, 0, 0, 0, time.UTC), SessionName: "a", DurationSec: 600},
		{Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC), SessionName: "b", DurationSec: 1800},
		{Timestamp: time.Date(2026, 3, 5, 16, 0, 0, 0, time.UTC), SessionName: "c", DurationSec: 900},
		{Timestamp: time.Date(2026, 3, 10, 21, 0, 0, 0, time.UTC), SessionName: "d", DurationSec: 3600},
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(append(data, '\n'))
	}
	f.Close()

	tracker := NewWorkTimeTracker(dir)
	daily, err := tracker.GetDailySegmentedWorkTime(2026, time.March)
	if err != nil {
		t.Fatalf("GetDailySegmentedWorkTime: %v", err)
	}

	if len(daily) != 2 {
		t.Fatalf("expected 2 days, got %d", len(daily))
	}

	dayMap := make(map[string][SegmentCount]int)
	for _, d := range daily {
		dayMap[d.Date] = d.Segments
	}

	// Day 5: seg0=600, seg2=1800, seg3=900
	if dayMap["2026-03-05"] != [5]int{600, 0, 1800, 900, 0} {
		t.Errorf("2026-03-05 segments = %v, want [600 0 1800 900 0]", dayMap["2026-03-05"])
	}
	// Day 10: seg4=3600
	if dayMap["2026-03-10"] != [5]int{0, 0, 0, 0, 3600} {
		t.Errorf("2026-03-10 segments = %v, want [0 0 0 0 3600]", dayMap["2026-03-10"])
	}
}

func TestWorkTimeTracker_Cleanup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	current := fmt.Sprintf("worktime-%04d-%02d.jsonl", now.Year(), now.Month())
	old5 := now.AddDate(0, -5, 0)
	remove := fmt.Sprintf("worktime-%04d-%02d.jsonl", old5.Year(), old5.Month())

	for _, name := range []string{current, remove} {
		os.WriteFile(filepath.Join(dir, name), []byte("test\n"), 0o644)
	}

	tracker := NewWorkTimeTracker(dir)
	if err := tracker.Cleanup(3); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, current)); err != nil {
		t.Errorf("expected current file to exist")
	}
	if _, err := os.Stat(filepath.Join(dir, remove)); !os.IsNotExist(err) {
		t.Errorf("expected old file to be removed")
	}
}
