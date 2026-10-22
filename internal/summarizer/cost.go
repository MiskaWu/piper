package summarizer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CostEntry records a single claude -p invocation's token usage.
type CostEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	SessionName  string    `json:"sessionName"`
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
	ProjectPath  string    `json:"projectPath,omitempty"`
}

// MonthlySummary aggregates cost data for a month.
type MonthlySummary struct {
	Month             string `json:"month"`
	Count             int    `json:"count"`
	TotalInputTokens  int    `json:"totalInputTokens"`
	TotalOutputTokens int    `json:"totalOutputTokens"`
}

// CostLogger writes and reads per-month JSONL cost files.
type CostLogger struct {
	dir string
	mu  sync.Mutex
}

// NewCostLogger creates a CostLogger that stores files in dir.
func NewCostLogger(dir string) *CostLogger {
	return &CostLogger{dir: dir}
}

func (c *CostLogger) filename(year int, month time.Month) string {
	return filepath.Join(c.dir, fmt.Sprintf("summary-cost-%04d-%02d.jsonl", year, month))
}

// Log appends an entry to the current month's JSONL file.
func (c *CostLogger) Log(entry CostEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("create cost log dir: %w", err)
	}

	year, month, _ := entry.Timestamp.Date()
	f, err := os.OpenFile(c.filename(year, month), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cost log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cost entry: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// GetMonthlyEntries reads all entries for a given month.
func (c *CostLogger) GetMonthlyEntries(year int, month time.Month) ([]CostEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.Open(c.filename(year, month))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open cost log: %w", err)
	}
	defer f.Close()

	var entries []CostEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e CostEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// GetMonthlySummary returns aggregated stats for a given month.
func (c *CostLogger) GetMonthlySummary(year int, month time.Month) (MonthlySummary, error) {
	entries, err := c.GetMonthlyEntries(year, month)
	if err != nil {
		return MonthlySummary{}, err
	}

	summary := MonthlySummary{
		Month: fmt.Sprintf("%04d-%02d", year, month),
	}
	for _, e := range entries {
		summary.Count++
		summary.TotalInputTokens += e.InputTokens
		summary.TotalOutputTokens += e.OutputTokens
	}
	return summary, nil
}

// FilePath returns the JSONL file path for a given year/month.
func (c *CostLogger) FilePath(year int, month time.Month) string {
	return c.filename(year, month)
}
