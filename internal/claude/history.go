package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// HistoryEntry represents a user prompt from history.jsonl.
type HistoryEntry struct {
	Display   string
	Timestamp time.Time
	Project   string
	SessionID string
}

// rawHistoryEntry maps to the JSON structure in history.jsonl.
type rawHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // Unix milliseconds
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// ReadDailyHistory reads a history.jsonl file and returns user prompts
// for the given date (format: "2006-01-02") grouped by project path.
// loc determines the timezone used to map UTC timestamps to calendar dates.
// If loc is nil, time.UTC is used.
// Filters out system events (display = "exit", "login", empty, or slash commands starting with "/").
// The historyPath parameter allows injecting a custom path for testing.
func ReadDailyHistory(historyPath string, date string, loc *time.Location) (map[string][]HistoryEntry, error) {
	if loc == nil {
		loc = time.UTC
	}
	targetDate, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return nil, fmt.Errorf("parse date %q: %w", date, err)
	}

	f, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("open history file %s: %w", historyPath, err)
	}
	defer f.Close()

	result := make(map[string][]HistoryEntry)
	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw rawHistoryEntry
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // skip malformed lines
		}

		// Filter system events
		if isSystemEvent(raw.Display) {
			continue
		}

		// Convert timestamp (milliseconds) to time.Time in the target timezone
		ts := time.UnixMilli(raw.Timestamp).In(loc)

		// Check if same date in the target timezone
		entryDate := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, loc)
		if !entryDate.Equal(targetDate) {
			continue
		}

		entry := HistoryEntry{
			Display:   raw.Display,
			Timestamp: ts,
			Project:   raw.Project,
			SessionID: raw.SessionID,
		}
		result[raw.Project] = append(result[raw.Project], entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file %s: %w", historyPath, err)
	}

	return result, nil
}

// isSystemEvent returns true if the display string is a system event that should be filtered out.
func isSystemEvent(display string) bool {
	if display == "" || display == "exit" || display == "login" {
		return true
	}
	if strings.HasPrefix(display, "/") {
		return true
	}
	return false
}
