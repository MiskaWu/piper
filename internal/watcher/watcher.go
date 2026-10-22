package watcher

import (
	"fmt"
	"github.com/miskawu/piper/internal/tmux"
	"sync"
	"time"
)

const activityLogTTL = 5 * time.Minute
const gitFallbackInterval = 30 * time.Second

// ActivityEvent records a session state change or output change.
type ActivityEvent struct {
	Time        time.Time `json:"time"`
	SessionName string    `json:"sessionName"`
	Kind        string    `json:"kind"`   // "status_change" | "output"
	Detail      string    `json:"detail"` // "working→completed" or lastLine content
}

// GitStatus represents the per-session git status.
// A non-nil GitStatus means git is available for the project.
type GitStatus struct {
	SessionDirty int `json:"sessionDirty"` // session-edited files that are uncommitted
}

// GitDirtyFilesFunc resolves dirty file paths for a session.
// Returns (nil, false) if git is not available for the session's project.
type GitDirtyFilesFunc func(sessionName string) (dirtyPaths []string, isRepo bool)

// FileTrackerFunc returns the file paths edited by a Claude session.
type FileTrackerFunc func(sessionName string) []string

// gitCacheEntry stores the resolved git + file tracker result for a session.
type gitCacheEntry struct {
	git              *GitStatus // nil = no git
	sessionFileEdits int
}

// countIntersection returns how many items in sessionFiles appear in dirtyPaths.
// sessionFiles are absolute or repo-relative paths from Claude's trackedFileBackups.
// dirtyPaths are repo-relative paths from git status --porcelain.
func countIntersection(sessionFiles []string, dirtyPaths []string) int {
	if len(sessionFiles) == 0 || len(dirtyPaths) == 0 {
		return 0
	}
	dirty := make(map[string]struct{}, len(dirtyPaths))
	for _, p := range dirtyPaths {
		dirty[p] = struct{}{}
	}
	count := 0
	for _, sf := range sessionFiles {
		if _, ok := dirty[sf]; ok {
			count++
		}
	}
	return count
}

const (
	StatusSourceHook        = "hook"
	StatusSourceCapturePANE = "capture-pane"
)

// HookMeta holds metadata from Claude Code hooks/statusline (cost, context, model).
type HookMeta struct {
	Model          string  `json:"model,omitempty"`
	CostUSD        float64 `json:"costUsd,omitempty"`
	ContextUsedPct float64 `json:"contextUsedPct,omitempty"`
	DurationMs     int64   `json:"durationMs,omitempty"`
	StatusSource   string  `json:"statusSource"` // StatusSourceHook | StatusSourceCapturePANE
}

// Session represents an enriched tmux session with status information.
type Session struct {
	Name             string     `json:"name"`
	Windows          int        `json:"windows"`
	Created          string     `json:"created"`
	Attached         bool       `json:"attached"`
	Status           string     `json:"status"`
	LastLine         string     `json:"lastLine"`
	Git              *GitStatus `json:"git,omitempty"`
	SessionFileEdits int        `json:"sessionFileEdits,omitempty"` // files edited by this Claude session
	HookMeta         *HookMeta  `json:"hookMeta,omitempty"`
}

// TmuxClient defines the interface for interacting with tmux.
// *tmux.Client satisfies this interface.
type TmuxClient interface {
	ListSessions() ([]tmux.TmuxSession, error)
	CapturePane(name string) (string, error)
}

// Watcher periodically scans tmux sessions and calls onChange when results are available.
type Watcher struct {
	client   TmuxClient
	interval time.Duration
	onChange func([]Session)
	stop     chan struct{}
	trigger  chan struct{} // external trigger for immediate scan
	once     sync.Once

	mu           sync.Mutex
	activityLog  []ActivityEvent
	prevLastLine map[string]string

	gitDirtyFilesFn   GitDirtyFilesFunc
	gitCache          map[string]*gitCacheEntry // session name → cached result
	gitLastCheck      map[string]time.Time      // session name → last check time
	fileTrackerFn     FileTrackerFunc
}

// New creates a new Watcher.
func New(client TmuxClient, interval time.Duration, onChange func([]Session)) *Watcher {
	return &Watcher{
		client:       client,
		interval:     interval,
		onChange:      onChange,
		stop:         make(chan struct{}),
		trigger:      make(chan struct{}, 1),
		prevLastLine: make(map[string]string),
		gitCache:     make(map[string]*gitCacheEntry),
		gitLastCheck: make(map[string]time.Time),
	}
}

// SetGitDirtyFilesFunc sets the function used to resolve dirty file paths for sessions.
func (w *Watcher) SetGitDirtyFilesFunc(fn GitDirtyFilesFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gitDirtyFilesFn = fn
}

// SetFileTrackerFunc sets the function used to count files edited by a Claude session.
func (w *Watcher) SetFileTrackerFunc(fn FileTrackerFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fileTrackerFn = fn
}


// Scan performs a single scan of all tmux sessions, enriching each with lastLine and git info.
// Status is set to "needs_input" as default; the actual status comes from hookDataStore
// and is overlaid by OnChange.
func (w *Watcher) Scan() ([]Session, error) {
	tmuxSessions, err := w.client.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	sessions := make([]Session, 0, len(tmuxSessions))
	activeNames := make(map[string]bool, len(tmuxSessions))

	for _, ts := range tmuxSessions {
		activeNames[ts.Name] = true

		s := Session{
			Name:     ts.Name,
			Windows:  ts.Windows,
			Created:  ts.Created,
			Attached: ts.Attached,
			Status:   "needs_input",
		}

		// Capture pane: always read for lastLine display.
		content, err := w.client.CapturePane(ts.Name)
		if err == nil {
			s.LastLine = tmux.GetLastSignificantLine(content)
		}

		// Status and HookMeta are overlaid by OnChange from hookDataStore.
		// Watcher only provides the base session info + lastLine.

		// Record activity: lastLine change
		if s.LastLine != "" && s.LastLine != w.prevLastLine[ts.Name] {
			w.activityLog = append(w.activityLog, ActivityEvent{
				Time:        time.Now(),
				SessionName: ts.Name,
				Kind:        "output",
				Detail:      s.LastLine,
			})
		}
		w.prevLastLine[ts.Name] = s.LastLine

		// Git + file edit check on interval
		if lastCheck, ok := w.gitLastCheck[ts.Name]; !ok || time.Since(lastCheck) >= gitFallbackInterval {
			entry := &gitCacheEntry{}

			var sessionFiles []string
			if w.fileTrackerFn != nil {
				sessionFiles = w.fileTrackerFn(ts.Name)
				entry.sessionFileEdits = len(sessionFiles)
			}

			if w.gitDirtyFilesFn != nil {
				dirtyPaths, isRepo := w.gitDirtyFilesFn(ts.Name)
				if isRepo {
					entry.git = &GitStatus{
						SessionDirty: countIntersection(sessionFiles, dirtyPaths),
					}
				}
			}

			w.gitCache[ts.Name] = entry
			w.gitLastCheck[ts.Name] = time.Now()
		}
		if cached := w.gitCache[ts.Name]; cached != nil {
			s.Git = cached.git
			s.SessionFileEdits = cached.sessionFileEdits
		}

		sessions = append(sessions, s)
	}

	// Trim expired activity events
	cutoff := time.Now().Add(-activityLogTTL)
	trimIdx := 0
	for trimIdx < len(w.activityLog) && w.activityLog[trimIdx].Time.Before(cutoff) {
		trimIdx++
	}
	if trimIdx > 0 {
		w.activityLog = w.activityLog[trimIdx:]
	}

	// Clean up stale entries for sessions that no longer exist
	for name := range w.prevLastLine {
		if !activeNames[name] {
			delete(w.prevLastLine, name)
			delete(w.gitCache, name)
			delete(w.gitLastCheck, name)
		}
	}

	return sessions, nil
}

// GetRecentActivity returns a copy of the activity log.
func (w *Watcher) GetRecentActivity() []ActivityEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	result := make([]ActivityEvent, len(w.activityLog))
	copy(result, w.activityLog)
	return result
}

// Trigger requests an immediate scan cycle from outside the polling loop.
// Non-blocking: if a trigger is already pending, this is a no-op.
func (w *Watcher) Trigger() {
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

// Start begins the polling loop. It blocks until Stop is called.
func (w *Watcher) Start() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Do an immediate scan on start.
	w.scanAndNotify()

	for {
		select {
		case <-ticker.C:
			w.scanAndNotify()
		case <-w.trigger:
			w.scanAndNotify()
		case <-w.stop:
			return
		}
	}
}

// Stop stops the polling loop. It is safe to call multiple times.
func (w *Watcher) Stop() {
	w.once.Do(func() {
		close(w.stop)
	})
}

func (w *Watcher) scanAndNotify() {
	sessions, err := w.Scan()
	if err != nil {
		return
	}
	if w.onChange != nil {
		w.onChange(sessions)
	}
}
