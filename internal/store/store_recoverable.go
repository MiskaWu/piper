package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RecoverableSession holds a snapshot of session metadata for recovery
// after unexpected restarts (e.g. WSL crash).
type RecoverableSession struct {
	Name            string    `json:"name"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	ProjectPath     string    `json:"projectPath,omitempty"`
	ClaudeSessionID string    `json:"claudeSessionId,omitempty"`
	LastSeenAt      time.Time `json:"lastSeenAt"`
}

func (s *Store) recoverablePath() string {
	return filepath.Join(filepath.Dir(s.path), "recoverable-sessions.json")
}

func (s *Store) loadRecoverable() ([]RecoverableSession, error) {
	data, err := os.ReadFile(s.recoverablePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read recoverable sessions: %w", err)
	}
	var entries []RecoverableSession
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse recoverable sessions: %w", err)
	}
	return entries, nil
}

func (s *Store) writeRecoverable(entries []RecoverableSession) error {
	if entries == nil {
		entries = []RecoverableSession{}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recoverable sessions: %w", err)
	}
	p := s.recoverablePath()
	tmpPath := p + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write recoverable temp: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename recoverable temp: %w", err)
	}
	return nil
}

// SnapshotRecoverable updates the recoverable snapshot with current active
// sessions. Active sessions get their LastSeenAt updated; entries for sessions
// no longer active are preserved (awaiting restore/dismiss/expiry).
func (s *Store) SnapshotRecoverable(activeSessions map[string]SessionMeta) {
	existing, err := s.loadRecoverable()
	if err != nil {
		return
	}

	// Index existing entries by name
	byName := make(map[string]RecoverableSession, len(existing))
	for _, e := range existing {
		byName[e.Name] = e
	}

	now := time.Now()

	// Upsert active sessions
	for name, meta := range activeSessions {
		byName[name] = RecoverableSession{
			Name:            name,
			Title:           meta.Title,
			Description:     meta.Description,
			ProjectPath:     meta.ProjectPath,
			ClaudeSessionID: meta.ClaudeSessionID,
			LastSeenAt:      now,
		}
	}

	// Collect and sort by LastSeenAt descending
	result := make([]RecoverableSession, 0, len(byName))
	for _, e := range byName {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeenAt.After(result[j].LastSeenAt)
	})

	if err := s.writeRecoverable(result); err != nil {
		return
	}
}

// ListRecoverable returns recoverable sessions that are within the retention
// period. Results are sorted by LastSeenAt descending (most recent first).
func (s *Store) ListRecoverable() ([]RecoverableSession, error) {
	entries, err := s.loadRecoverable()
	if err != nil {
		return nil, err
	}

	cfg := s.GetRecoveryConfig()
	cutoff := time.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)

	var result []RecoverableSession
	for _, e := range entries {
		if e.LastSeenAt.After(cutoff) {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeenAt.After(result[j].LastSeenAt)
	})
	return result, nil
}

// RemoveRecoverable removes a single entry by name (after restore or dismiss).
func (s *Store) RemoveRecoverable(name string) error {
	entries, err := s.loadRecoverable()
	if err != nil {
		return err
	}

	var kept []RecoverableSession
	for _, e := range entries {
		if e.Name != name {
			kept = append(kept, e)
		}
	}
	return s.writeRecoverable(kept)
}

// PruneRecoverable removes entries older than the configured retention period.
// Returns the number of entries pruned.
func (s *Store) PruneRecoverable() (int, error) {
	entries, err := s.loadRecoverable()
	if err != nil {
		return 0, err
	}

	cfg := s.GetRecoveryConfig()
	cutoff := time.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)

	var kept []RecoverableSession
	pruned := 0
	for _, e := range entries {
		if e.LastSeenAt.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, e)
		}
	}

	if pruned == 0 {
		return 0, nil
	}
	if err := s.writeRecoverable(kept); err != nil {
		return 0, err
	}
	return pruned, nil
}
