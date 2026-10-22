package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/miskawu/piper/internal/autostop"
	"github.com/miskawu/piper/internal/claude"
	"github.com/miskawu/piper/internal/git"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/summarizer"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/watcher"
	"github.com/miskawu/piper/internal/ws"
)

// statusMessage is the JSON structure broadcast via WebSocket on each poll.
type statusMessage struct {
	Sessions           []sessionResponse      `json:"sessions"`
	Usage              *usage.Usage           `json:"usage,omitempty"`
	ActivePlanSessions []tmux.PlanSessionInfo `json:"activePlanSessions,omitempty"`
}

// StatusBroadcaster wires watcher poll results to WebSocket broadcast,
// usage logging, work time tracking, auto-stop, and auto-summarizer.
type StatusBroadcaster struct {
	sessionStore      *store.Store
	tmuxClient        *tmux.Client
	hub               *ws.Hub
	hookData          *hookDataStore
	usageLogger       *usage.UsageLogger
	workTimeTracker   *usage.WorkTimeTracker
	completionTracker *usage.CompletionTracker
	autoStopper       *autostop.AutoStopper
	autoSummarizer    *summarizer.Summarizer
	gitChecker        *git.Checker
	prevSessionNames  map[string]bool // tracks previous scan's session names
}

// overlayHookData applies hookDataStore's status/meta onto sessions in place.
// Watcher's "needs_input" default masks real "working" state without this.
func overlayHookData(sessions []watcher.Session, hookData *hookDataStore) {
	for i := range sessions {
		if status, ok := hookData.ResolveStatus(sessions[i].Name); ok {
			sessions[i].Status = status
		}
		if meta := hookData.GetSessionMeta(sessions[i].Name); meta != nil {
			sessions[i].HookMeta = meta
		}
	}
}

// OnChange is the callback passed to watcher.New. It enriches session data,
// broadcasts status, and triggers side-effect subsystems.
func (b *StatusBroadcaster) OnChange(sessions []watcher.Session) {
	overlayHookData(sessions, b.hookData)

	enriched := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		sr := sessionResponse{Session: s}
		if m := b.sessionStore.GetMeta(s.Name); m != nil {
			sr.Title = m.Title
			sr.Description = m.Description
			sr.DescriptionAuto = m.DescriptionAuto
			sr.Order = m.Order
			if m.ProjectPath != "" {
				sr.ProjectPath = m.ProjectPath
				sr.ProjectName = filepath.Base(m.ProjectPath)
			}
		}
		enriched = append(enriched, sr)
	}

	msg := statusMessage{Sessions: enriched}

	// Scan plan-type sessions for sidebar indicators
	if planSessions, err := b.tmuxClient.ListAllPlanTypeSessions(); err == nil && len(planSessions) > 0 {
		aps := make([]tmux.PlanSessionInfo, 0, len(planSessions))
		for _, ps := range planSessions {
			if info, ok := tmux.ParsePlanSessionName(ps.Name); ok {
				aps = append(aps, info)
			}
		}
		msg.ActivePlanSessions = aps
	}

	// Use hook-pushed rate limits (account-level, from statusline)
	msg.Usage = b.hookData.GetLatestUsage()

	data, _ := json.Marshal(msg)
	b.hub.Broadcast(data)

	// Track session work time
	workStatuses := make([]usage.SessionStatus, len(sessions))
	for i, s := range sessions {
		ws := usage.SessionStatus{Name: s.Name, Status: s.Status}
		if meta := b.sessionStore.GetMeta(s.Name); meta != nil {
			ws.ProjectPath = meta.ProjectPath
		}
		workStatuses[i] = ws
	}
	b.workTimeTracker.Update(workStatuses)

	// Auto-stop containers when all sessions for a project end
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}
	b.autoStopper.Check(names)

	// Auto-summarize on status transitions
	statuses := make([]summarizer.SessionStatus, len(sessions))
	for i, s := range sessions {
		statuses[i] = summarizer.SessionStatus{Name: s.Name, Status: s.Status}
	}
	b.autoSummarizer.OnStatusUpdate(statuses)

	// Handle disappeared sessions through unified close flow
	currentNames := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		currentNames[s.Name] = true
	}
	if b.prevSessionNames != nil {
		for name := range b.prevSessionNames {
			if !currentNames[name] {
				b.onSessionClosed(name)
			}
		}
	}
	b.prevSessionNames = currentNames

	// Discover ClaudeSessionID for host (non-DevContainer) sessions.
	// DevContainer sessions get their sessionId via the session-meta API.
	b.discoverClaudeSessionIDs(sessions)

	// Snapshot active session metadata for recovery after unexpected restarts
	b.sessionStore.SnapshotRecoverable(b.sessionStore.AllSessions())
}

// onSessionClosed is the unified close flow for any session removal —
// whether triggered by DELETE API or detected by the watcher.
// It archives metadata to history and records a completion event.
func (b *StatusBroadcaster) onSessionClosed(name string) {
	// Read projectPath before archive deletes the meta
	var projPath string
	if meta := b.sessionStore.GetMeta(name); meta != nil {
		projPath = meta.ProjectPath
	}

	if err := b.sessionStore.ArchiveSession(name); err != nil {
		log.Printf("WARNING: failed to archive session %s: %v", name, err)
	}

	if b.completionTracker != nil {
		if err := b.completionTracker.Record(name, projPath); err != nil {
			log.Printf("WARNING: failed to record completion for %s: %v", name, err)
		}
	}

	// Clean up hook data for the closed session
	b.hookData.Cleanup(name)
}

// GitDirtyFilesFunc returns a watcher.GitDirtyFilesFunc that resolves dirty file paths
// for a session by looking up its project path.
func (b *StatusBroadcaster) GitDirtyFilesFunc() watcher.GitDirtyFilesFunc {
	return func(sessionName string) ([]string, bool) {
		dir := ""
		if m := b.sessionStore.GetMeta(sessionName); m != nil && m.ProjectPath != "" {
			dir = m.ProjectPath
		}
		if dir == "" {
			return nil, false
		}
		paths, err := b.gitChecker.GetDirtyFilePaths(dir)
		if err != nil {
			return nil, false
		}
		return paths, true
	}
}

// discoverClaudeSessionIDs resolves ClaudeSessionID for host (non-DevContainer) sessions
// using tmux pane PID. DevContainer sessions receive their sessionId directly via
// the POST /api/internal/session-meta endpoint.
func (b *StatusBroadcaster) discoverClaudeSessionIDs(sessions []watcher.Session) {
	for _, s := range sessions {
		m := b.sessionStore.GetMeta(s.Name)
		if m == nil || m.ProjectPath == "" || m.ClaudeSessionID != "" {
			continue
		}

		// Skip DevContainer sessions — they report sessionId via API
		if _, err := os.Stat(filepath.Join(m.ProjectPath, ".devcontainer", "devcontainer.json")); err == nil {
			continue
		}

		// Host session: use tmux pane PID
		if pid, err := b.tmuxClient.GetPanePID(s.Name); err == nil && pid > 0 {
			if claudeSessionID := claude.FindSessionIDByPanePID(pid); claudeSessionID != "" {
				b.sessionStore.SetClaudeSessionID(s.Name, claudeSessionID)
			}
		}
	}
}

// FileTrackerFunc returns a watcher.FileTrackerFunc that resolves file paths
// edited by a Claude session using JSONL history.
func (b *StatusBroadcaster) FileTrackerFunc() watcher.FileTrackerFunc {
	return func(sessionName string) []string {
		m := b.sessionStore.GetMeta(sessionName)
		if m == nil || m.ProjectPath == "" || m.ClaudeSessionID == "" {
			return nil
		}

		isDevContainer := false
		if _, err := os.Stat(filepath.Join(m.ProjectPath, ".devcontainer", "devcontainer.json")); err == nil {
			isDevContainer = true
		}

		var jsonlPath string
		if isDevContainer {
			jsonlPath = claude.SessionJSONLPathForDevcontainer(m.ClaudeSessionID, m.ProjectPath)
		} else {
			jsonlPath = claude.SessionJSONLPath(m.ClaudeSessionID, m.ProjectPath)
		}
		if jsonlPath == "" {
			return nil
		}
		return claude.CountTrackedFiles(jsonlPath).Files
	}
}
