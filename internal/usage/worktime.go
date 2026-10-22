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

// WorkTimeEntry records a single work period for a session.
type WorkTimeEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionName string    `json:"sessionName"`
	DurationSec int       `json:"durationSec"`
	ProjectPath string    `json:"projectPath,omitempty"`
}

// SessionStatus represents a session's current state.
type SessionStatus struct {
	Name        string
	Status      string
	ProjectPath string
}

// activeSession tracks a working session's start time and metadata.
type activeSession struct {
	start       time.Time
	projectPath string
}

// WorkTimeSummary aggregates work time for a month.
type WorkTimeSummary struct {
	Month            string                 `json:"month"`
	TotalDurationSec int                    `json:"totalDurationSec"`
	Projects         []ProjectWorkTimeStat  `json:"projects"`
}

// ProjectWorkTimeStat aggregates work time per project path.
type ProjectWorkTimeStat struct {
	ProjectPath      string `json:"projectPath"`
	TotalDurationSec int    `json:"totalDurationSec"`
	EntryCount       int    `json:"entryCount"`
}

// WorkTimeTracker tracks when sessions are working and records durations.
type WorkTimeTracker struct {
	dir    string
	mu     sync.Mutex
	active map[string]activeSession // session name → active info
}

// NewWorkTimeTracker creates a WorkTimeTracker that stores files in dir.
func NewWorkTimeTracker(dir string) *WorkTimeTracker {
	return &WorkTimeTracker{
		dir:    dir,
		active: make(map[string]activeSession),
	}
}

func (w *WorkTimeTracker) filename(year int, month time.Month) string {
	return filepath.Join(w.dir, fmt.Sprintf("worktime-%04d-%02d.jsonl", year, month))
}

// Update processes current session states. It starts tracking sessions that are
// "working" and records work duration when they transition away or disappear.
func (w *WorkTimeTracker) Update(sessions []SessionStatus) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Build a set of current session names
	current := map[string]string{}
	for _, s := range sessions {
		current[s.Name] = s.Status
	}

	// Check for sessions that stopped working or disappeared
	for name, as := range w.active {
		status, exists := current[name]
		if !exists || status != "working" {
			// Session stopped working — record duration
			duration := int(time.Since(as.start).Seconds())
			delete(w.active, name)

			// Skip very short durations (< 2 seconds)
			if duration < 2 {
				continue
			}

			if err := w.writeEntry(WorkTimeEntry{
				Timestamp:   time.Now().UTC(),
				SessionName: name,
				DurationSec: duration,
				ProjectPath: as.projectPath,
			}); err != nil {
				return err
			}
		}
	}

	// Track new working sessions
	for _, s := range sessions {
		if s.Status == "working" {
			if _, ok := w.active[s.Name]; !ok {
				w.active[s.Name] = activeSession{
					start:       time.Now(),
					projectPath: s.ProjectPath,
				}
			}
		}
	}

	return nil
}

func (w *WorkTimeTracker) writeEntry(entry WorkTimeEntry) error {
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("create worktime dir: %w", err)
	}

	year, month, _ := entry.Timestamp.Date()
	f, err := os.OpenFile(w.filename(year, month), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open worktime log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal worktime entry: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// GetMonthlyEntries reads all work time entries for a given month.
func (w *WorkTimeTracker) GetMonthlyEntries(year int, month time.Month) ([]WorkTimeEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.Open(w.filename(year, month))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open worktime log: %w", err)
	}
	defer f.Close()

	var entries []WorkTimeEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e WorkTimeEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// GetMonthlySummary returns aggregated work time stats for a given month.
func (w *WorkTimeTracker) GetMonthlySummary(year int, month time.Month) (*WorkTimeSummary, error) {
	entries, err := w.GetMonthlyEntries(year, month)
	if err != nil {
		return nil, err
	}

	summary := &WorkTimeSummary{
		Month: fmt.Sprintf("%04d-%02d", year, month),
	}

	projectStats := map[string]*ProjectWorkTimeStat{}
	var projectOrder []string

	for _, e := range entries {
		summary.TotalDurationSec += e.DurationSec
		key := e.ProjectPath
		if key == "" {
			key = "(unknown)"
		}
		stat, ok := projectStats[key]
		if !ok {
			stat = &ProjectWorkTimeStat{ProjectPath: key}
			projectStats[key] = stat
			projectOrder = append(projectOrder, key)
		}
		stat.TotalDurationSec += e.DurationSec
		stat.EntryCount++
	}

	for _, p := range projectOrder {
		summary.Projects = append(summary.Projects, *projectStats[p])
	}
	return summary, nil
}

// DailyWorkTime aggregates work time for a single day.
type DailyWorkTime struct {
	Date    string `json:"date"`
	WorkSec int    `json:"workSec"`
}

// GetDailyWorkTime computes per-day total work seconds for a given month.
func (w *WorkTimeTracker) GetDailyWorkTime(year int, month time.Month) ([]DailyWorkTime, error) {
	entries, err := w.GetMonthlyEntries(year, month)
	if err != nil {
		return nil, err
	}

	days := map[string]int{}
	var dayOrder []string
	for _, e := range entries {
		date := e.Timestamp.UTC().Format("2006-01-02")
		if _, ok := days[date]; !ok {
			dayOrder = append(dayOrder, date)
		}
		days[date] += e.DurationSec
	}

	result := make([]DailyWorkTime, 0, len(dayOrder))
	for _, date := range dayOrder {
		result = append(result, DailyWorkTime{Date: date, WorkSec: days[date]})
	}
	return result, nil
}

// DailySegmentedWorkTime holds per-time-segment work seconds for a single day.
type DailySegmentedWorkTime struct {
	Date     string            `json:"date"`
	Segments [SegmentCount]int `json:"segments"` // seconds per time segment
}

// GetDailySegmentedWorkTime computes per-day, per-segment work seconds for a given month.
func (w *WorkTimeTracker) GetDailySegmentedWorkTime(year int, month time.Month) ([]DailySegmentedWorkTime, error) {
	entries, err := w.GetMonthlyEntries(year, month)
	if err != nil {
		return nil, err
	}

	type segments = [SegmentCount]int
	days := make(map[string]*segments)
	var dayOrder []string
	for _, e := range entries {
		date := e.Timestamp.UTC().Format("2006-01-02")
		seg := hourToSegment(e.Timestamp.UTC().Hour())
		if days[date] == nil {
			days[date] = &segments{}
			dayOrder = append(dayOrder, date)
		}
		days[date][seg] += e.DurationSec
	}

	result := make([]DailySegmentedWorkTime, 0, len(dayOrder))
	for _, date := range dayOrder {
		result = append(result, DailySegmentedWorkTime{Date: date, Segments: *days[date]})
	}
	return result, nil
}

// ActiveSessions returns the names of sessions currently being tracked as working.
func (w *WorkTimeTracker) ActiveSessions() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	names := make([]string, 0, len(w.active))
	for name := range w.active {
		names = append(names, name)
	}
	return names
}

// FilePath returns the JSONL file path for a given year/month.
func (w *WorkTimeTracker) FilePath(year int, month time.Month) string {
	return w.filename(year, month)
}

// SetActiveForTest sets a session as active with a given start time (for testing).
func (w *WorkTimeTracker) SetActiveForTest(name string, start time.Time, projectPath ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	pp := ""
	if len(projectPath) > 0 {
		pp = projectPath[0]
	}
	w.active[name] = activeSession{start: start, projectPath: pp}
}

// Cleanup removes worktime JSONL files older than retention months.
func (w *WorkTimeTracker) Cleanup(retention int) error {
	return cleanupJSONL(w.dir, retention)
}
