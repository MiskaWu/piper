package hookstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHookStatus_Valid(t *testing.T) {
	dir := t.TempDir()
	data := `{"status":"working","timestamp":1712000000000,"hookEvent":"PreToolUse","tool":"Bash"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".piper-status-test-session"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	hs, err := ReadHookStatus(dir, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.Status != "working" {
		t.Errorf("status = %q, want %q", hs.Status, "working")
	}
	if hs.HookEvent != "PreToolUse" {
		t.Errorf("hookEvent = %q, want %q", hs.HookEvent, "PreToolUse")
	}
	if hs.Tool != "Bash" {
		t.Errorf("tool = %q, want %q", hs.Tool, "Bash")
	}
	if hs.Timestamp != 1712000000000 {
		t.Errorf("timestamp = %d, want %d", hs.Timestamp, 1712000000000)
	}
}

func TestReadHookStatus_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadHookStatus(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadHookStatus_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".piper-status-bad"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadHookStatus(dir, "bad")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestReadSessionMeta_Valid(t *testing.T) {
	dir := t.TempDir()
	data := `{"timestamp":1712000000000,"sessionId":"abc123","model":"Opus","costUsd":0.05,"contextUsedPct":35.5,"durationMs":120000}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".piper-meta-test-session"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	sm, err := ReadSessionMeta(dir, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.SessionID != "abc123" {
		t.Errorf("sessionId = %q, want %q", sm.SessionID, "abc123")
	}
	if sm.Model != "Opus" {
		t.Errorf("model = %q, want %q", sm.Model, "Opus")
	}
	if sm.CostUSD != 0.05 {
		t.Errorf("costUsd = %f, want %f", sm.CostUSD, 0.05)
	}
	if sm.ContextUsedPct != 35.5 {
		t.Errorf("contextUsedPct = %f, want %f", sm.ContextUsedPct, 35.5)
	}
}

func TestReadSessionMeta_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadSessionMeta(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadSessionMeta_WithRateLimits(t *testing.T) {
	dir := t.TempDir()
	data := `{"timestamp":1712000000000,"sessionId":"x","model":"Opus","costUsd":0,"contextUsedPct":0,"durationMs":0,"rateLimits":{"five_hour":{"used_percentage":23.5,"resets_at":1738425600}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".piper-meta-rl"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	sm, err := ReadSessionMeta(dir, "rl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.RateLimits == nil || sm.RateLimits.FiveHour == nil {
		t.Fatal("expected rate limits with five_hour tier")
	}
	if sm.RateLimits.FiveHour.UsedPercentage != 23.5 {
		t.Errorf("five_hour.used_percentage = %f, want %f", sm.RateLimits.FiveHour.UsedPercentage, 23.5)
	}
}

func TestCleanupFiles(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, ".piper-status-cleanup")
	metaPath := filepath.Join(dir, ".piper-meta-cleanup")

	os.WriteFile(statusPath, []byte("{}"), 0644)
	os.WriteFile(metaPath, []byte("{}"), 0644)

	CleanupFiles(dir, "cleanup")

	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Error("status file should be removed")
	}
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Error("meta file should be removed")
	}
}
