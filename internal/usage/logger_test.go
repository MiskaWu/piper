package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewUsageLogger(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.dir != dir {
		t.Errorf("expected dir %q, got %q", dir, logger.dir)
	}
	if logger.interval != 5*time.Minute {
		t.Errorf("expected default interval 5m, got %v", logger.interval)
	}
}

func TestNewUsageLogger_WithInterval(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir, WithLogInterval(10*time.Minute))
	if logger.interval != 10*time.Minute {
		t.Errorf("expected interval 10m, got %v", logger.interval)
	}
}

func TestUsageLogger_LogIfDue_寫入快照(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir, WithLogInterval(0)) // no throttle

	fiveH := 0.42
	sevenD := 0.15
	u := &Usage{
		FiveHour: &Quota{Utilization: fiveH, ResetsAt: "2026-03-13T10:00:00Z"},
		SevenDay: &Quota{Utilization: sevenD, ResetsAt: "2026-03-20T00:00:00Z"},
	}

	logged, err := logger.LogIfDue(u)
	if err != nil {
		t.Fatalf("LogIfDue error: %v", err)
	}
	if !logged {
		t.Fatal("expected logged=true")
	}

	// Verify file content
	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("usage-%04d-%02d.jsonl", now.Year(), now.Month()))
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var snap UsageSnapshot
	if err := json.Unmarshal(data[:len(data)-1], &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.FiveHourUtil == nil || *snap.FiveHourUtil != fiveH {
		t.Errorf("expected FiveHourUtil=%v, got %v", fiveH, snap.FiveHourUtil)
	}
	if snap.SevenDayUtil == nil || *snap.SevenDayUtil != sevenD {
		t.Errorf("expected SevenDayUtil=%v, got %v", sevenD, snap.SevenDayUtil)
	}
	if snap.FiveHourResets != "2026-03-13T10:00:00Z" {
		t.Errorf("unexpected FiveHourResets: %s", snap.FiveHourResets)
	}
}

func TestUsageLogger_LogIfDue_節流(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir, WithLogInterval(1*time.Hour)) // long interval

	u := &Usage{
		FiveHour: &Quota{Utilization: 0.5, ResetsAt: "2026-03-13T10:00:00Z"},
	}

	logged, err := logger.LogIfDue(u)
	if err != nil {
		t.Fatalf("first LogIfDue: %v", err)
	}
	if !logged {
		t.Fatal("first call should log")
	}

	// Second call should be throttled
	logged, err = logger.LogIfDue(u)
	if err != nil {
		t.Fatalf("second LogIfDue: %v", err)
	}
	if logged {
		t.Fatal("second call should be throttled")
	}
}

func TestUsageLogger_LogIfDue_nil用量不寫入(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir, WithLogInterval(0))

	logged, err := logger.LogIfDue(nil)
	if err != nil {
		t.Fatalf("LogIfDue error: %v", err)
	}
	if logged {
		t.Fatal("nil usage should not log")
	}
}


func TestUsageLogger_GetMonthlySnapshots(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir, WithLogInterval(0))

	// Write 3 snapshots
	for i := 0; i < 3; i++ {
		fiveH := float64(i) * 0.1
		u := &Usage{
			FiveHour: &Quota{Utilization: fiveH, ResetsAt: "2026-03-13T10:00:00Z"},
		}
		if _, err := logger.LogIfDue(u); err != nil {
			t.Fatalf("LogIfDue %d: %v", i, err)
		}
	}

	now := time.Now()
	snaps, err := logger.GetMonthlySnapshots(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlySnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}
}

func TestUsageLogger_GetMonthlySnapshots_無檔案(t *testing.T) {
	dir := t.TempDir()
	logger := NewUsageLogger(dir)

	snaps, err := logger.GetMonthlySnapshots(2025, time.January)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if snaps != nil {
		t.Errorf("expected nil snapshots, got %v", snaps)
	}
}

func TestUsageLogger_GetDailyAverages(t *testing.T) {
	dir := t.TempDir()

	// Write test data directly to file
	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("usage-%04d-%02d.jsonl", now.Year(), now.Month()))
	os.MkdirAll(dir, 0o755)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	f, _ := os.Create(filename)
	for i := 0; i < 4; i++ {
		fiveH := float64(i+1) * 0.1 // 0.1, 0.2, 0.3, 0.4
		sevenD := float64(i+1) * 0.05
		snap := UsageSnapshot{
			Timestamp:    today.Add(time.Duration(i) * time.Hour),
			FiveHourUtil: &fiveH,
			SevenDayUtil: &sevenD,
		}
		data, _ := json.Marshal(snap)
		f.Write(append(data, '\n'))
	}
	f.Close()

	logger := NewUsageLogger(dir)
	avgs, err := logger.GetDailyAverages(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetDailyAverages: %v", err)
	}
	if len(avgs) != 1 {
		t.Fatalf("expected 1 day, got %d", len(avgs))
	}

	avg := avgs[0]
	if avg.SnapshotCount != 4 {
		t.Errorf("expected 4 snapshots, got %d", avg.SnapshotCount)
	}
	// avg = (0.1+0.2+0.3+0.4)/4 = 0.25
	if avg.AvgFiveHourUtil != 0.25 {
		t.Errorf("expected avg 0.25, got %f", avg.AvgFiveHourUtil)
	}
	// max = 0.4
	if avg.MaxFiveHourUtil != 0.4 {
		t.Errorf("expected max 0.4, got %f", avg.MaxFiveHourUtil)
	}
}

func TestUsageLogger_GetMonthlySummary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	filename := filepath.Join(dir, fmt.Sprintf("usage-%04d-%02d.jsonl", now.Year(), now.Month()))
	f, _ := os.Create(filename)

	// Day 1: max 85 (above 80)
	// Day 2: max 60 (above 50 but not 80)
	// Day 3: max 30 (below 50)
	days := []struct {
		day    int
		values []float64
	}{
		{1, []float64{40, 85, 50}},
		{2, []float64{30, 60, 45}},
		{3, []float64{10, 30, 20}},
	}
	for _, d := range days {
		for i, v := range d.values {
			ts := time.Date(now.Year(), now.Month(), d.day, i*2, 0, 0, 0, time.UTC)
			snap := UsageSnapshot{
				Timestamp:    ts,
				FiveHourUtil: &v,
			}
			data, _ := json.Marshal(snap)
			f.Write(append(data, '\n'))
		}
	}
	f.Close()

	logger := NewUsageLogger(dir)
	summary, err := logger.GetMonthlySummary(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if summary.TotalDays != 3 {
		t.Errorf("expected 3 days, got %d", summary.TotalDays)
	}
	if summary.DaysAbove80Pct != 1 {
		t.Errorf("expected 1 day above 80%%, got %d", summary.DaysAbove80Pct)
	}
	if summary.DaysAbove50Pct != 2 {
		t.Errorf("expected 2 days above 50%%, got %d", summary.DaysAbove50Pct)
	}
	if summary.MaxFiveHourUtil != 85 {
		t.Errorf("expected max 85, got %f", summary.MaxFiveHourUtil)
	}
}

func TestUsageLogger_Cleanup(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	// Create files: current month, 2 months ago (keep), 4 months ago (delete)
	now := time.Now()
	current := fmt.Sprintf("usage-%04d-%02d.jsonl", now.Year(), now.Month())
	old2 := now.AddDate(0, -2, 0)
	keep := fmt.Sprintf("usage-%04d-%02d.jsonl", old2.Year(), old2.Month())
	old4 := now.AddDate(0, -4, 0)
	remove := fmt.Sprintf("usage-%04d-%02d.jsonl", old4.Year(), old4.Month())

	for _, name := range []string{current, keep, remove} {
		os.WriteFile(filepath.Join(dir, name), []byte("test\n"), 0o644)
	}

	logger := NewUsageLogger(dir)
	if err := logger.Cleanup(3); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// current and keep should exist
	for _, name := range []string{current, keep} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist", name)
		}
	}
	// remove should be gone
	if _, err := os.Stat(filepath.Join(dir, remove)); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed", remove)
	}
}

func TestUsageLogger_Cleanup_也清理summaryCost(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)

	now := time.Now()
	old5 := now.AddDate(0, -5, 0)
	oldCost := fmt.Sprintf("summary-cost-%04d-%02d.jsonl", old5.Year(), old5.Month())
	os.WriteFile(filepath.Join(dir, oldCost), []byte("test\n"), 0o644)

	logger := NewUsageLogger(dir)
	if err := logger.Cleanup(3); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, oldCost)); !os.IsNotExist(err) {
		t.Errorf("expected old summary-cost file to be removed")
	}
}
