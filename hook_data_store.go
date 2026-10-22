package main

import (
	"sync"
	"time"

	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/watcher"
)

// hookRateLimits mirrors hookstatus.RateLimits for in-memory storage.
type hookRateLimits struct {
	FiveHour *hookRateLimitTier
	SevenDay *hookRateLimitTier
}

type hookRateLimitTier struct {
	UsedPercentage float64
	ResetsAt       float64 // unix epoch seconds
}

// hookStatusEntry stores the latest hook-reported status for a session.
type hookStatusEntry struct {
	Status    string
	HookEvent string
	Timestamp int64
}

// hookMetaEntry stores the latest statusline-reported metadata for a session.
type hookMetaEntry struct {
	Timestamp      int64
	SessionID      string
	Model          string
	CostUSD        float64
	ContextUsedPct float64
	DurationMs     int64
	RateLimits     *hookRateLimits
}

// hookDataStore holds hook-pushed session data in memory.
// Written by internal API handlers, read by the watcher's OnChange.
// Also manages the status state machine (working → completed → needs_input).
type hookDataStore struct {
	mu          sync.RWMutex
	statuses    map[string]*hookStatusEntry // session name → latest status
	metas       map[string]*hookMetaEntry   // session name → latest meta
	latestUsage *usage.Usage                // account-level rate limits (most recent)
	prevStatus  map[string]string           // session name → previous resolved status (for state machine)
	acked       map[string]bool             // session name → user has acknowledged completed state
}

func newHookDataStore() *hookDataStore {
	return &hookDataStore{
		statuses:   make(map[string]*hookStatusEntry),
		metas:      make(map[string]*hookMetaEntry),
		prevStatus: make(map[string]string),
		acked:      make(map[string]bool),
	}
}

// SetHookStatus stores hook-based status for a session.
func (s *hookDataStore) SetHookStatus(sessionName string, status, hookEvent string, timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[sessionName] = &hookStatusEntry{
		Status:    status,
		HookEvent: hookEvent,
		Timestamp: timestamp,
	}
}

// ResolveStatus returns the resolved status for a session, applying the state machine:
//   - raw "working" → "working", clear acked flag
//   - raw non-working, prev was "working" → "completed" (task just finished)
//   - "completed" + user acked → "needs_input"
//   - raw non-working, prev was not "working" → "needs_input"
//
// Returns ("", false) if no hook status has been received for this session.
func (s *hookDataStore) ResolveStatus(sessionName string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.statuses[sessionName]
	if !ok {
		return "", false
	}

	rawStatus := e.Status
	prev := s.prevStatus[sessionName]
	var resolved string

	if rawStatus == "working" {
		resolved = "working"
		s.acked[sessionName] = false
	} else if rawStatus == "needs_input" {
		resolved = "needs_input"
	} else if prev == "working" {
		resolved = "completed"
	} else if prev == "completed" && !s.acked[sessionName] {
		resolved = "completed"
	} else {
		resolved = "needs_input"
	}

	s.prevStatus[sessionName] = resolved
	return resolved, true
}

// AckSession marks a session as acknowledged by the user (degrading completed → needs_input).
func (s *hookDataStore) AckSession(sessionName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acked[sessionName] = true
}

// SetSessionMeta stores statusline metadata for a session and updates account-level rate limits.
func (s *hookDataStore) SetSessionMeta(sessionName string, meta *hookMetaEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metas[sessionName] = meta

	if meta.RateLimits != nil {
		s.latestUsage = convertRateLimitsToUsage(meta.RateLimits)
	}
}

// GetSessionMeta returns the hook metadata as a watcher.HookMeta for a session.
func (s *hookDataStore) GetSessionMeta(sessionName string) *watcher.HookMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.metas[sessionName]
	if !ok {
		return nil
	}
	return &watcher.HookMeta{
		Model:          m.Model,
		CostUSD:        m.CostUSD,
		ContextUsedPct: m.ContextUsedPct,
		DurationMs:     m.DurationMs,
		StatusSource:   watcher.StatusSourceHook,
	}
}

// GetSessionID returns the Claude session ID reported by the statusline, if any.
func (s *hookDataStore) GetSessionID(sessionName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.metas[sessionName]
	if !ok {
		return ""
	}
	return m.SessionID
}

// GetLatestUsage returns the most recently reported account-level rate limits.
func (s *hookDataStore) GetLatestUsage() *usage.Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestUsage
}

// Cleanup removes all data for a closed session.
func (s *hookDataStore) Cleanup(sessionName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.statuses, sessionName)
	delete(s.metas, sessionName)
	delete(s.prevStatus, sessionName)
	delete(s.acked, sessionName)
}

// convertRateLimitsToUsage converts hook rate limits to the usage.Usage shape
// expected by the UI (matching the old API fetcher's output).
func convertRateLimitsToUsage(rl *hookRateLimits) *usage.Usage {
	u := &usage.Usage{}
	if rl.FiveHour != nil {
		u.FiveHour = &usage.Quota{
			Utilization: rl.FiveHour.UsedPercentage,
			ResetsAt:    epochToISO8601(rl.FiveHour.ResetsAt),
		}
	}
	if rl.SevenDay != nil {
		u.SevenDay = &usage.Quota{
			Utilization: rl.SevenDay.UsedPercentage,
			ResetsAt:    epochToISO8601(rl.SevenDay.ResetsAt),
		}
	}
	return u
}

// epochToISO8601 converts a unix epoch seconds (float64) to an ISO8601 string.
func epochToISO8601(epoch float64) string {
	if epoch <= 0 {
		return ""
	}
	sec := int64(epoch)
	nsec := int64((epoch - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC().Format(time.RFC3339)
}
