package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionHistoryEntry records metadata of a closed session.
type SessionHistoryEntry struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ProjectPath string    `json:"projectPath,omitempty"`
	ClosedAt    time.Time `json:"closedAt"`
}

// ArchiveSession records a closed session to the history file and removes it
// from the active sessions map. If the session has no description and no
// project path, it is silently discarded (nothing worth archiving).
func (s *Store) ArchiveSession(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.sessions[name]
	if !ok {
		return nil // nothing to archive
	}

	// Only archive if there's meaningful data
	if meta.Description == "" && meta.ProjectPath == "" {
		delete(s.sessions, name)
		return s.Save()
	}

	entry := SessionHistoryEntry{
		Name:        name,
		Description: meta.Description,
		ProjectPath: meta.ProjectPath,
		ClosedAt:    time.Now(),
	}

	if err := s.appendHistory(entry); err != nil {
		return fmt.Errorf("archive session %s: %w", name, err)
	}

	delete(s.sessions, name)
	return s.Save()
}

// GetSessionHistory returns history entries whose ClosedAt falls within
// [from, to]. Both bounds are inclusive.
func (s *Store) GetSessionHistory(from, to time.Time) ([]SessionHistoryEntry, error) {
	entries, err := s.loadHistory()
	if err != nil {
		return nil, err
	}

	var result []SessionHistoryEntry
	for _, e := range entries {
		if !e.ClosedAt.Before(from) && !e.ClosedAt.After(to) {
			result = append(result, e)
		}
	}
	return result, nil
}

// PruneHistory removes entries older than the given duration and rewrites
// the history file.
func (s *Store) PruneHistory(maxAge time.Duration) (int, error) {
	entries, err := s.loadHistory()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var kept []SessionHistoryEntry
	pruned := 0
	for _, e := range entries {
		if e.ClosedAt.Before(cutoff) {
			pruned++
		} else {
			kept = append(kept, e)
		}
	}

	if pruned == 0 {
		return 0, nil
	}

	if err := s.writeHistory(kept); err != nil {
		return 0, err
	}
	return pruned, nil
}

func (s *Store) historyPath() string {
	return filepath.Join(filepath.Dir(s.path), "session-history.json")
}

func (s *Store) loadHistory() ([]SessionHistoryEntry, error) {
	data, err := os.ReadFile(s.historyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session history: %w", err)
	}

	var entries []SessionHistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse session history: %w", err)
	}
	return entries, nil
}

func (s *Store) appendHistory(entry SessionHistoryEntry) error {
	entries, err := s.loadHistory()
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	return s.writeHistory(entries)
}

// SnapshotActiveSessions writes a snapshot of the given active sessions to
// the history file without removing them from META. Used at digest generation
// time to capture sessions that are still running. Returns the number of
// entries written.
func (s *Store) SnapshotActiveSessions(activeNames []string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var toWrite []SessionHistoryEntry
	now := time.Now()
	for _, name := range activeNames {
		meta, ok := s.sessions[name]
		if !ok {
			continue
		}
		if meta.Description == "" && meta.ProjectPath == "" {
			continue
		}
		toWrite = append(toWrite, SessionHistoryEntry{
			Name:        name,
			Description: meta.Description,
			ProjectPath: meta.ProjectPath,
			ClosedAt:    now,
		})
	}

	if len(toWrite) == 0 {
		return 0, nil
	}

	entries, err := s.loadHistory()
	if err != nil {
		return 0, err
	}
	entries = append(entries, toWrite...)
	if err := s.writeHistory(entries); err != nil {
		return 0, err
	}
	return len(toWrite), nil
}

func (s *Store) writeHistory(entries []SessionHistoryEntry) error {
	if entries == nil {
		entries = []SessionHistoryEntry{}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session history: %w", err)
	}

	hp := s.historyPath()
	tmpPath := hp + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write session history temp: %w", err)
	}
	if err := os.Rename(tmpPath, hp); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename session history: %w", err)
	}
	return nil
}
