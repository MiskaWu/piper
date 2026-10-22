package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestJSONL(t *testing.T, lines []string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "session-*.jsonl")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer f.Close()

	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("failed to write line: %v", err)
		}
	}
	return f.Name()
}

func TestCountTrackedFiles(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		wantCount int
		wantFiles []string
	}{
		{
			name:      "empty file",
			lines:     []string{},
			wantCount: 0,
			wantFiles: nil,
		},
		{
			name: "no snapshot entries",
			lines: []string{
				`{"type":"user","messageId":"m1","message":"hello"}`,
				`{"type":"assistant","messageId":"m2","message":"hi"}`,
			},
			wantCount: 0,
			wantFiles: nil,
		},
		{
			name: "single snapshot with 2 files",
			lines: []string{
				`{"type":"user","messageId":"m1","message":"hello"}`,
				`{"type":"file-history-snapshot","messageId":"m2","snapshot":{"messageId":"m2","trackedFileBackups":{"src/main.go":{"backupFileName":"abc@v1","version":1,"backupTime":"2026-01-01T00:00:00Z"},"src/util.go":{"backupFileName":"def@v1","version":1,"backupTime":"2026-01-01T00:00:00Z"}},"timestamp":"2026-01-01T00:00:00Z"},"isSnapshotUpdate":false}`,
			},
			wantCount: 2,
			wantFiles: []string{"src/main.go", "src/util.go"},
		},
		{
			name: "multiple snapshots uses last one",
			lines: []string{
				`{"type":"file-history-snapshot","messageId":"m1","snapshot":{"messageId":"m1","trackedFileBackups":{"file1.go":{"backupFileName":"a@v1","version":1,"backupTime":"2026-01-01T00:00:00Z"}},"timestamp":"2026-01-01T00:00:00Z"},"isSnapshotUpdate":false}`,
				`{"type":"user","messageId":"m2","message":"more work"}`,
				`{"type":"file-history-snapshot","messageId":"m3","snapshot":{"messageId":"m3","trackedFileBackups":{"file1.go":{"backupFileName":"a@v2","version":2,"backupTime":"2026-01-01T01:00:00Z"},"file2.go":{"backupFileName":"b@v1","version":1,"backupTime":"2026-01-01T01:00:00Z"},"file3.go":{"backupFileName":"c@v1","version":1,"backupTime":"2026-01-01T01:00:00Z"}},"timestamp":"2026-01-01T01:00:00Z"},"isSnapshotUpdate":false}`,
			},
			wantCount: 3,
			wantFiles: []string{"file1.go", "file2.go", "file3.go"},
		},
		{
			name: "snapshot with empty trackedFileBackups",
			lines: []string{
				`{"type":"file-history-snapshot","messageId":"m1","snapshot":{"messageId":"m1","trackedFileBackups":{},"timestamp":"2026-01-01T00:00:00Z"},"isSnapshotUpdate":false}`,
			},
			wantCount: 0,
			wantFiles: nil,
		},
		{
			name: "malformed JSON lines mixed in",
			lines: []string{
				`not json at all`,
				`{"broken":`,
				`{"type":"file-history-snapshot","messageId":"m1","snapshot":{"messageId":"m1","trackedFileBackups":{"good.go":{"backupFileName":"a@v1","version":1,"backupTime":"2026-01-01T00:00:00Z"}},"timestamp":"2026-01-01T00:00:00Z"},"isSnapshotUpdate":false}`,
				`{also broken}`,
			},
			wantCount: 1,
			wantFiles: []string{"good.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestJSONL(t, tt.lines)
			got := CountTrackedFiles(path)

			if got.Count != tt.wantCount {
				t.Errorf("Count = %d, want %d", got.Count, tt.wantCount)
			}

			if tt.wantFiles == nil {
				if got.Files != nil {
					t.Errorf("Files = %v, want nil", got.Files)
				}
				return
			}

			if len(got.Files) != len(tt.wantFiles) {
				t.Fatalf("Files length = %d, want %d; got %v", len(got.Files), len(tt.wantFiles), got.Files)
			}
			for i, f := range got.Files {
				if f != tt.wantFiles[i] {
					t.Errorf("Files[%d] = %q, want %q", i, f, tt.wantFiles[i])
				}
			}
		})
	}
}

func TestCountTrackedFiles_NonexistentFile(t *testing.T) {
	got := CountTrackedFiles(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	if got.Count != 0 {
		t.Errorf("Count = %d, want 0 for nonexistent file", got.Count)
	}
}

func TestCountTrackedFiles_VeryLongLines(t *testing.T) {
	// Build a snapshot with many files to create a long line.
	var backups []string
	for i := 0; i < 500; i++ {
		padding := strings.Repeat("x", 200)
		backups = append(backups, `"path/to/`+padding+`/file`+fmt.Sprintf("%04d", i)+`.go":{"backupFileName":"bk","version":1,"backupTime":"t"}`)
	}
	snapshotLine := `{"type":"file-history-snapshot","messageId":"m1","snapshot":{"messageId":"m1","trackedFileBackups":{` + strings.Join(backups, ",") + `},"timestamp":"t"},"isSnapshotUpdate":false}`

	// Verify the line is reasonably long (should be well over 100KB).
	if len(snapshotLine) < 100000 {
		t.Fatalf("test line too short: %d bytes", len(snapshotLine))
	}

	path := writeTestJSONL(t, []string{snapshotLine})
	got := CountTrackedFiles(path)

	if got.Count != 500 {
		t.Errorf("Count = %d, want 500", got.Count)
	}
}

// setupSessionsDir creates a temp sessions directory with the given entries
// and returns the path to the sessions directory.
func setupSessionsDir(t *testing.T, entries []claudeSessionEntry) string {
	t.Helper()
	sessionsDir := filepath.Join(t.TempDir(), ".claude", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("failed to create sessions dir: %v", err)
	}
	for i, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("failed to marshal entry %d: %v", i, err)
		}
		filename := filepath.Join(sessionsDir, fmt.Sprintf("%d.json", entry.PID))
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			t.Fatalf("failed to write session file: %v", err)
		}
	}
	return sessionsDir
}

func TestFindSessionIDByPID(t *testing.T) {
	t.Run("存在的 PID 回傳 sessionId", func(t *testing.T) {
		sessionsDir := setupSessionsDir(t, []claudeSessionEntry{
			{PID: 741, SessionID: "fb8c0a95-d916-4272-be48-7be8a5fa182a", Cwd: "/workspace"},
		})
		got := findSessionIDByPID(sessionsDir, 741)
		if got != "fb8c0a95-d916-4272-be48-7be8a5fa182a" {
			t.Errorf("got %q, want fb8c0a95-...", got)
		}
	})

	t.Run("不存在的 PID 回傳空", func(t *testing.T) {
		sessionsDir := setupSessionsDir(t, []claudeSessionEntry{
			{PID: 100, SessionID: "sess-aaa", Cwd: "/workspace"},
		})
		got := findSessionIDByPID(sessionsDir, 999)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("JSON 格式錯誤回傳空", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "123.json"), []byte("{broken"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := findSessionIDByPID(dir, 123)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}


