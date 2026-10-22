package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CompletionEntry records a single session completion event.
type CompletionEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionName string    `json:"sessionName"`
	ProjectPath string    `json:"projectPath,omitempty"`
}

// CompletionTracker tracks how many sessions have been completed (closed).
type CompletionTracker struct {
	dir string
	mu  sync.Mutex
}

// NewCompletionTracker creates a CompletionTracker that stores files in dir.
func NewCompletionTracker(dir string) *CompletionTracker {
	return &CompletionTracker{dir: dir}
}

func (c *CompletionTracker) filename(year int, month time.Month) string {
	return filepath.Join(c.dir, fmt.Sprintf("completions-%04d-%02d.jsonl", year, month))
}

// Record writes a completion event for the given session.
func (c *CompletionTracker) Record(sessionName, projectPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("create completions dir: %w", err)
	}

	now := time.Now().UTC()
	year, month, _ := now.Date()

	f, err := os.OpenFile(c.filename(year, month), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open completions log: %w", err)
	}
	defer f.Close()

	entry := CompletionEntry{
		Timestamp:   now,
		SessionName: sessionName,
		ProjectPath: projectPath,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal completion entry: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// SegmentCount is the number of time segments per day.
const SegmentCount = 5

// segmentBounds defines hour boundaries for each segment:
// [0] 00-05 dawn, [1] 05-10 morning, [2] 10-15 midday, [3] 15-20 afternoon, [4] 20-24 night
var segmentBounds = [SegmentCount][2]int{{0, 5}, {5, 10}, {10, 15}, {15, 20}, {20, 24}}

// hourToSegment maps an hour (0-23) to its segment index (0-4).
func hourToSegment(hour int) int {
	for i, b := range segmentBounds {
		if hour >= b[0] && hour < b[1] {
			return i
		}
	}
	return SegmentCount - 1
}

// DailyCompletion holds per-time-segment completion counts for a single day.
type DailyCompletion struct {
	Date     string `json:"date"`               // "YYYY-MM-DD"
	Segments [SegmentCount]int `json:"segments"` // count per time segment
}

// GetDailyCompletions returns per-day, per-segment completion counts for the given month.
func (c *CompletionTracker) GetDailyCompletions(year int, month time.Month) ([]DailyCompletion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.Open(c.filename(year, month))
	if err != nil {
		if os.IsNotExist(err) {
			return []DailyCompletion{}, nil
		}
		return nil, fmt.Errorf("open completions log: %w", err)
	}
	defer f.Close()

	type segments = [SegmentCount]int
	days := make(map[string]*segments)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e CompletionEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		day := e.Timestamp.Format("2006-01-02")
		seg := hourToSegment(e.Timestamp.Hour())
		if days[day] == nil {
			days[day] = &segments{}
		}
		days[day][seg]++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan completions log: %w", err)
	}

	daily := make([]DailyCompletion, 0, len(days))
	for date, segs := range days {
		daily = append(daily, DailyCompletion{Date: date, Segments: *segs})
	}
	sortDailyCompletions(daily)
	return daily, nil
}

func sortDailyCompletions(d []DailyCompletion) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j].Date < d[j-1].Date; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

// GetMonthlyEntries reads all completion entries for a given month.
func (c *CompletionTracker) GetMonthlyEntries(year int, month time.Month) ([]CompletionEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.Open(c.filename(year, month))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open completions log: %w", err)
	}
	defer f.Close()

	var entries []CompletionEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e CompletionEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// FilePath returns the JSONL file path for a given year/month.
func (c *CompletionTracker) FilePath(year int, month time.Month) string {
	return c.filename(year, month)
}

// Cleanup removes completion JSONL files older than retention months.
func (c *CompletionTracker) Cleanup(retention int) error {
	return cleanupJSONL(c.dir, retention)
}
