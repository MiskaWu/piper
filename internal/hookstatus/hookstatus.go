// Package hookstatus reads Claude Code hook and statusline output files
// written by Piper's in-container scripts (piper-hook.mjs, piper-statusline.mjs).
package hookstatus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HookStatus represents the status written by piper-hook.mjs on each Claude Code lifecycle event.
type HookStatus struct {
	Status    string `json:"status"`    // "working" | "idle" | "needs_input"
	Timestamp int64  `json:"timestamp"` // unix milliseconds
	HookEvent string `json:"hookEvent"`
	Tool      string `json:"tool,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Subagent  bool   `json:"subagent,omitempty"`
}

// SessionMeta represents the metadata written by piper-statusline.mjs after each assistant message.
type SessionMeta struct {
	Timestamp      int64       `json:"timestamp"` // unix milliseconds
	SessionID      string      `json:"sessionId"`
	Model          string      `json:"model"`
	CostUSD        float64     `json:"costUsd"`
	ContextUsedPct float64     `json:"contextUsedPct"`
	DurationMs     int64       `json:"durationMs"`
	RateLimits     *RateLimits `json:"rateLimits,omitempty"`
}

// RateLimits mirrors Claude Code's rate limit tiers.
type RateLimits struct {
	FiveHour *RateLimitTier `json:"five_hour,omitempty"`
	SevenDay *RateLimitTier `json:"seven_day,omitempty"`
}

// RateLimitTier represents a single rate limit window.
type RateLimitTier struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       float64 `json:"resets_at"` // unix epoch seconds
}

// ReadHookStatus reads and parses the hook status file for the given session.
// Returns nil and an error if the file does not exist or is malformed.
func ReadHookStatus(sessionsDir, sessionName string) (*HookStatus, error) {
	path := filepath.Join(sessionsDir, ".piper-status-"+sessionName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read hook status %s: %w", sessionName, err)
	}

	var hs HookStatus
	if err := json.Unmarshal(data, &hs); err != nil {
		return nil, fmt.Errorf("parse hook status %s: %w", sessionName, err)
	}
	return &hs, nil
}

// ReadSessionMeta reads and parses the statusline metadata file for the given session.
// Returns nil and an error if the file does not exist or is malformed.
func ReadSessionMeta(sessionsDir, sessionName string) (*SessionMeta, error) {
	path := filepath.Join(sessionsDir, ".piper-meta-"+sessionName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session meta %s: %w", sessionName, err)
	}

	var sm SessionMeta
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("parse session meta %s: %w", sessionName, err)
	}
	return &sm, nil
}

// CleanupFiles removes the hook status and metadata files for a session.
func CleanupFiles(sessionsDir, sessionName string) {
	os.Remove(filepath.Join(sessionsDir, ".piper-status-"+sessionName))
	os.Remove(filepath.Join(sessionsDir, ".piper-meta-"+sessionName))
}
