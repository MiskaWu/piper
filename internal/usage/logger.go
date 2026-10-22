package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// UsageSnapshot is a single point-in-time utilization record.
type UsageSnapshot struct {
	Timestamp      time.Time `json:"timestamp"`
	FiveHourUtil   *float64  `json:"fiveHourUtil,omitempty"`
	FiveHourResets string    `json:"fiveHourResets,omitempty"`
	SevenDayUtil   *float64  `json:"sevenDayUtil,omitempty"`
	SevenDayResets string    `json:"sevenDayResets,omitempty"`
	Stale          bool      `json:"stale,omitempty"`
}

// DailyAverage aggregates utilization data for a single day.
type DailyAverage struct {
	Date            string  `json:"date"`
	AvgFiveHourUtil float64 `json:"avgFiveHourUtil"`
	AvgSevenDayUtil float64 `json:"avgSevenDayUtil"`
	MaxFiveHourUtil float64 `json:"maxFiveHourUtil"`
	MaxSevenDayUtil float64 `json:"maxSevenDayUtil"`
	SnapshotCount   int     `json:"snapshotCount"`
}

// UsageLogger writes utilization snapshots to per-month JSONL files.
type UsageLogger struct {
	dir      string
	mu       sync.Mutex
	interval time.Duration
	lastLog  time.Time
}

// LoggerOption configures UsageLogger behavior.
type LoggerOption func(*UsageLogger)

// WithLogInterval sets the minimum interval between log writes.
func WithLogInterval(d time.Duration) LoggerOption {
	return func(l *UsageLogger) { l.interval = d }
}

// NewUsageLogger creates a UsageLogger that stores files in dir.
func NewUsageLogger(dir string, opts ...LoggerOption) *UsageLogger {
	l := &UsageLogger{
		dir:      dir,
		interval: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

func (l *UsageLogger) filename(year int, month time.Month) string {
	return filepath.Join(l.dir, fmt.Sprintf("usage-%04d-%02d.jsonl", year, month))
}

// LogIfDue writes a snapshot if enough time has passed since the last write.
// Returns true if a snapshot was written.
func (l *UsageLogger) LogIfDue(u *Usage) (bool, error) {
	if u == nil {
		return false, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if l.interval > 0 && !l.lastLog.IsZero() && now.Sub(l.lastLog) < l.interval {
		return false, nil
	}

	snap := UsageSnapshot{
		Timestamp: now.UTC(),
	}
	if u.FiveHour != nil {
		snap.FiveHourUtil = &u.FiveHour.Utilization
		snap.FiveHourResets = u.FiveHour.ResetsAt
	}
	if u.SevenDay != nil {
		snap.SevenDayUtil = &u.SevenDay.Utilization
		snap.SevenDayResets = u.SevenDay.ResetsAt
	}

	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return false, fmt.Errorf("create usage log dir: %w", err)
	}

	year, month, _ := now.Date()
	f, err := os.OpenFile(l.filename(year, month), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("open usage log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(snap)
	if err != nil {
		return false, fmt.Errorf("marshal snapshot: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return false, fmt.Errorf("write snapshot: %w", err)
	}

	l.lastLog = now
	return true, nil
}

// GetMonthlySnapshots reads all snapshots for a given month.
func (l *UsageLogger) GetMonthlySnapshots(year int, month time.Month) ([]UsageSnapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.Open(l.filename(year, month))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open usage log: %w", err)
	}
	defer f.Close()

	var snapshots []UsageSnapshot
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var s UsageSnapshot
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			continue
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, scanner.Err()
}

// GetDailyAverages computes per-day avg/max utilization for a given month.
func (l *UsageLogger) GetDailyAverages(year int, month time.Month) ([]DailyAverage, error) {
	snaps, err := l.GetMonthlySnapshots(year, month)
	if err != nil {
		return nil, err
	}

	type dayAccum struct {
		sumFiveH  float64
		sumSevenD float64
		maxFiveH  float64
		maxSevenD float64
		countFH   int
		countSD   int
		total     int
	}

	days := map[string]*dayAccum{}
	var dayOrder []string

	for _, s := range snaps {
		date := s.Timestamp.UTC().Format("2006-01-02")
		acc, ok := days[date]
		if !ok {
			acc = &dayAccum{}
			days[date] = acc
			dayOrder = append(dayOrder, date)
		}
		acc.total++
		if s.FiveHourUtil != nil {
			acc.sumFiveH += *s.FiveHourUtil
			acc.countFH++
			if *s.FiveHourUtil > acc.maxFiveH {
				acc.maxFiveH = *s.FiveHourUtil
			}
		}
		if s.SevenDayUtil != nil {
			acc.sumSevenD += *s.SevenDayUtil
			acc.countSD++
			if *s.SevenDayUtil > acc.maxSevenD {
				acc.maxSevenD = *s.SevenDayUtil
			}
		}
	}

	result := make([]DailyAverage, 0, len(dayOrder))
	for _, date := range dayOrder {
		acc := days[date]
		avg := DailyAverage{
			Date:            date,
			MaxFiveHourUtil: acc.maxFiveH,
			MaxSevenDayUtil: acc.maxSevenD,
			SnapshotCount:   acc.total,
		}
		if acc.countFH > 0 {
			avg.AvgFiveHourUtil = acc.sumFiveH / float64(acc.countFH)
		}
		if acc.countSD > 0 {
			avg.AvgSevenDayUtil = acc.sumSevenD / float64(acc.countSD)
		}
		result = append(result, avg)
	}
	return result, nil
}

// UsageMonthlySummary provides high-level monthly stats for dashboard display.
type UsageMonthlySummary struct {
	Month           string  `json:"month"`
	AvgFiveHourUtil float64 `json:"avgFiveHourUtil"`
	MaxFiveHourUtil float64 `json:"maxFiveHourUtil"`
	AvgSevenDayUtil float64 `json:"avgSevenDayUtil"`
	MaxSevenDayUtil float64 `json:"maxSevenDayUtil"`
	DaysAbove80Pct  int     `json:"daysAbove80Pct"`
	DaysAbove50Pct  int     `json:"daysAbove50Pct"`
	TotalDays       int     `json:"totalDays"`
}

// GetMonthlySummary computes a high-level monthly usage summary from daily averages.
func (l *UsageLogger) GetMonthlySummary(year int, month time.Month) (*UsageMonthlySummary, error) {
	daily, err := l.GetDailyAverages(year, month)
	if err != nil {
		return nil, err
	}

	summary := &UsageMonthlySummary{
		Month:     fmt.Sprintf("%04d-%02d", year, month),
		TotalDays: len(daily),
	}

	var sumFH, sumSD float64
	for _, d := range daily {
		sumFH += d.AvgFiveHourUtil
		sumSD += d.AvgSevenDayUtil
		if d.MaxFiveHourUtil > summary.MaxFiveHourUtil {
			summary.MaxFiveHourUtil = d.MaxFiveHourUtil
		}
		if d.MaxSevenDayUtil > summary.MaxSevenDayUtil {
			summary.MaxSevenDayUtil = d.MaxSevenDayUtil
		}
		if d.MaxFiveHourUtil >= 80 {
			summary.DaysAbove80Pct++
		}
		if d.MaxFiveHourUtil >= 50 {
			summary.DaysAbove50Pct++
		}
	}
	if len(daily) > 0 {
		summary.AvgFiveHourUtil = sumFH / float64(len(daily))
		summary.AvgSevenDayUtil = sumSD / float64(len(daily))
	}
	return summary, nil
}

// jsonlMonthPattern matches filenames like usage-YYYY-MM.jsonl and summary-cost-YYYY-MM.jsonl.
var jsonlMonthPattern = regexp.MustCompile(`^(?:usage|worktime|summary-cost|completions)-(\d{4})-(\d{2})\.jsonl$`)

// Cleanup removes JSONL files older than retention months.
func (l *UsageLogger) Cleanup(retention int) error {
	return cleanupJSONL(l.dir, retention)
}

// cleanupJSONL is shared logic for cleaning old JSONL files.
func cleanupJSONL(dir string, retention int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read usage dir: %w", err)
	}

	cutoff := time.Now().AddDate(0, -retention, 0)
	cutoffStr := fmt.Sprintf("%04d-%02d", cutoff.Year(), cutoff.Month())

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matches := jsonlMonthPattern.FindStringSubmatch(e.Name())
		if matches == nil {
			continue
		}
		fileMonth := matches[1] + "-" + matches[2]
		if fileMonth <= cutoffStr {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old file %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}
