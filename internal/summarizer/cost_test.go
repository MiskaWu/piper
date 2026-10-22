package summarizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCostLogger_LogAndRead(t *testing.T) {
	dir := t.TempDir()
	logger := NewCostLogger(dir)

	ts := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	entry := CostEntry{
		Timestamp:    ts,
		SessionName:  "claude-test",
		InputTokens:  500,
		OutputTokens: 30,
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Verify file exists
	expectedFile := filepath.Join(dir, "summary-cost-2026-03.jsonl")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Fatalf("expected file %s to exist: %v", expectedFile, err)
	}

	entries, err := logger.GetMonthlyEntries(2026, time.March)
	if err != nil {
		t.Fatalf("GetMonthlyEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].SessionName != "claude-test" {
		t.Errorf("session name = %q, want %q", entries[0].SessionName, "claude-test")
	}
	if entries[0].InputTokens != 500 {
		t.Errorf("input tokens = %d, want 500", entries[0].InputTokens)
	}
}

func TestCostLogger_GetMonthlySummary(t *testing.T) {
	dir := t.TempDir()
	logger := NewCostLogger(dir)

	ts := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		logger.Log(CostEntry{
			Timestamp:    ts.Add(time.Duration(i) * time.Minute),
			SessionName:  "claude-test",
			InputTokens:  1000,
			OutputTokens: 50,
		})
	}

	summary, err := logger.GetMonthlySummary(2026, time.March)
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if summary.Count != 5 {
		t.Errorf("count = %d, want 5", summary.Count)
	}
	if summary.TotalInputTokens != 5000 {
		t.Errorf("total input = %d, want 5000", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 250 {
		t.Errorf("total output = %d, want 250", summary.TotalOutputTokens)
	}
}

func TestCostLogger_EmptyMonth(t *testing.T) {
	dir := t.TempDir()
	logger := NewCostLogger(dir)

	summary, err := logger.GetMonthlySummary(2026, time.March)
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if summary.Count != 0 {
		t.Errorf("count = %d, want 0", summary.Count)
	}
}
