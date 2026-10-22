package autostop

import (
	"log"
	"sync"
	"time"
)

const defaultGrace = 5 * time.Minute

// ContainerStopper abstracts the ability to stop a container by project path.
type ContainerStopper interface {
	Stop(projectPath string) error
}

// SessionStore provides session-to-project mapping and project listing.
type SessionStore interface {
	GetSessionsByProject() map[string][]string
	GetProjects() []string
}

// Option configures AutoStopper.
type Option func(*AutoStopper)

// WithGrace sets the grace period before stopping a container.
func WithGrace(d time.Duration) Option {
	return func(a *AutoStopper) {
		a.grace = d
	}
}

// AutoStopper monitors live sessions and stops devcontainers when all sessions
// for a project have ended, after a grace period.
type AutoStopper struct {
	stopper  ContainerStopper
	store    SessionStore
	previous map[string]bool        // projectPaths that had live sessions last tick
	pending  map[string]*time.Timer // grace period timers
	grace    time.Duration
	mu       sync.Mutex
	logger   *log.Logger
}

// New creates a new AutoStopper.
func New(stopper ContainerStopper, store SessionStore, logger *log.Logger, opts ...Option) *AutoStopper {
	a := &AutoStopper{
		stopper:  stopper,
		store:    store,
		previous: make(map[string]bool),
		pending:  make(map[string]*time.Timer),
		grace:    defaultGrace,
		logger:   logger,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Check compares live tmux session names against the store's session-to-project
// mapping. If a project had sessions in the previous tick but now has zero live
// sessions, it starts a grace period. If sessions return during grace, the stop
// is cancelled.
func (a *AutoStopper) Check(liveSessionNames []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Build set of live session names
	liveSet := make(map[string]bool, len(liveSessionNames))
	for _, name := range liveSessionNames {
		liveSet[name] = true
	}

	// Get project -> session mapping from store
	projectSessions := a.store.GetSessionsByProject()

	// For each project, check if any of its sessions are still live
	current := make(map[string]bool)
	for projectPath, sessionNames := range projectSessions {
		for _, name := range sessionNames {
			if liveSet[name] {
				current[projectPath] = true
				break
			}
		}
	}

	// Projects that regained sessions: cancel pending grace timer
	for pp, timer := range a.pending {
		if current[pp] {
			timer.Stop()
			delete(a.pending, pp)
			a.logger.Printf("auto-stop: cancelled for %s (sessions resumed)", pp)
		}
	}

	// Detect projects that had sessions before but now have zero — start grace
	for projectPath := range a.previous {
		if !current[projectPath] {
			if _, ok := a.pending[projectPath]; ok {
				continue // already pending, don't reset timer
			}
			pp := projectPath
			a.logger.Printf("auto-stop: grace period started for %s (%v)", pp, a.grace)
			a.pending[pp] = time.AfterFunc(a.grace, func() {
				a.mu.Lock()
				_, still := a.pending[pp]
				if still {
					delete(a.pending, pp)
				}
				a.mu.Unlock()

				if still {
					a.logger.Printf("auto-stop: grace expired, stopping container for %s", pp)
					if err := a.stopper.Stop(pp); err != nil {
						a.logger.Printf("auto-stop: failed to stop container for %s: %v", pp, err)
					}
				}
			})
		}
	}

	a.previous = current
}

// CancelAll cancels all pending grace timers without stopping any containers.
// Intended for graceful shutdown — let containers keep running.
func (a *AutoStopper) CancelAll() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for pp, timer := range a.pending {
		timer.Stop()
		delete(a.pending, pp)
	}
}
