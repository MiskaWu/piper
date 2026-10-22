package epiccontainer

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"time"
)

// UpdateState captures the current status of an epic image update job.
type UpdateState struct {
	InProgress     bool      `json:"inProgress"`
	StartedAt      time.Time `json:"startedAt,omitempty"`
	FinishedAt     time.Time `json:"finishedAt,omitempty"`
	TargetVersion  string    `json:"targetVersion,omitempty"`
	PreviousVersion string   `json:"previousVersion,omitempty"`
	CurrentVersion string    `json:"currentVersion,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// ErrUpdateInProgress is returned when Start is called while another update is running.
var ErrUpdateInProgress = errors.New("epic image update already in progress")

// ImageRebuilder abstracts the image rebuild operation for testability.
type ImageRebuilder func(templateFS fs.FS, targetVersion string) error

// VersionProbe reads the currently-bundled Claude Code version from the image.
type VersionProbe func(ctx context.Context) (string, error)

// Updater coordinates epic image rebuilds with a single-flight state machine.
// Only one update can be running at a time; a successful run replaces image state.
type Updater struct {
	mu    sync.Mutex
	state UpdateState

	rebuild ImageRebuilder
	probe   VersionProbe
}

// NewUpdater returns an Updater using the package's default rebuild/probe functions.
func NewUpdater() *Updater {
	return &Updater{
		rebuild: RebuildImage,
		probe:   GetImageClaudeVersion,
	}
}

// Snapshot returns a copy of the current state.
func (u *Updater) Snapshot() UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// InProgress reports whether an update is currently running.
func (u *Updater) InProgress() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.InProgress
}

// Start launches a background rebuild targeting targetVersion. Returns
// ErrUpdateInProgress if another update is still running.
func (u *Updater) Start(ctx context.Context, templateFS fs.FS, targetVersion string) error {
	u.mu.Lock()
	if u.state.InProgress {
		u.mu.Unlock()
		return ErrUpdateInProgress
	}
	previous, _ := u.probe(ctx)
	u.state = UpdateState{
		InProgress:      true,
		StartedAt:       time.Now(),
		TargetVersion:   targetVersion,
		PreviousVersion: previous,
	}
	u.mu.Unlock()

	go u.run(templateFS, targetVersion, previous)
	return nil
}

func (u *Updater) run(templateFS fs.FS, targetVersion, previous string) {
	rebuildErr := u.rebuild(templateFS, targetVersion)

	current := ""
	if rebuildErr == nil {
		probeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if v, err := u.probe(probeCtx); err == nil {
			current = v
		}
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.InProgress = false
	u.state.FinishedAt = time.Now()
	u.state.CurrentVersion = current
	u.state.PreviousVersion = previous
	if rebuildErr != nil {
		u.state.Error = rebuildErr.Error()
	} else {
		u.state.Error = ""
	}
}
