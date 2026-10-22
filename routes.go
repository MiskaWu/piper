package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	gitPkg "github.com/miskawu/piper/internal/git"
	"github.com/miskawu/piper/internal/gitwatcher"
	"github.com/miskawu/piper/internal/clipboard"
	"github.com/miskawu/piper/internal/devcontainer"
	"github.com/miskawu/piper/internal/epiccontainer"
	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/summarizer"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/vault"
	"github.com/miskawu/piper/internal/watcher"
	"github.com/miskawu/piper/internal/ws"
)

// App holds all dependencies for HTTP route handlers.
type App struct {
	tmuxClient        *tmux.Client
	sessionStore      *store.Store
	dcManager         *devcontainer.Manager
	hub               *ws.Hub
	fileServer        http.Handler
	acker             SessionAcker
	costLogger        *summarizer.CostLogger
	usageLogger       *usage.UsageLogger
	workTimeTracker   *usage.WorkTimeTracker
	completionTracker *usage.CompletionTracker
	clipStore         *clipboard.Store
	activityProvider  ActivityProvider
	enrichCmd         tmux.Commander
	vaultManager      *vault.VaultManager
	projectRoot       string
	setupRunner       *devcontainer.SetupRunner
	upgrader          *devcontainer.Upgrader
	setupTracker      *setupTracker
	epicSyncer        *plan.GitSyncer
	gitFetcher        *gitPkg.GraphFetcher
	gitWatcher        *gitwatcher.GitWatcher
	gitChecker        *gitPkg.Checker
	broadcaster       *StatusBroadcaster
	hookData          *hookDataStore
	watcherTrigger    WatcherTrigger
	epicImageEnsurer  func() error // injectable; nil → calls epiccontainer.EnsureImage
	epicUpdater       *epiccontainer.Updater
	dockerAvailable   bool // set at startup from preflight; false → native epic sessions
}

// SetupRoutes registers all HTTP route handlers on the given mux.
func (app *App) SetupRoutes(mux *http.ServeMux) {
	if app.setupRunner == nil {
		gen := devcontainer.NewGenerator(&tmux.ExecCommander{}, devcontainer.TemplateFS())
		gen.ModelFunc = func() string {
			return app.sessionStore.GetModelConfig().DevcontainerGen
		}
		app.setupRunner = devcontainer.NewSetupRunner(app.dcManager, devcontainer.TemplateFS())
		app.upgrader = devcontainer.NewUpgrader(gen, devcontainer.TemplateFS())
	}
	if app.setupTracker == nil {
		app.setupTracker = newSetupTracker()
	}
	if app.gitFetcher == nil {
		app.gitFetcher = gitPkg.NewGraphFetcher(&tmux.ExecCommander{})
	}
	if app.gitChecker == nil {
		app.gitChecker = gitPkg.NewChecker(&tmux.ExecCommander{})
	}

	app.registerInternalRoutes(mux)
	app.registerSessionRoutes(mux)
	app.registerProjectRoutes(mux)
	app.registerAnalyticsRoutes(mux)
	app.registerSettingsRoutes(mux)
	app.registerIdeaRoutes(mux)
	app.registerEpicRoutes(mux)
	app.registerReferenceRoutes(mux)
	app.registerVaultRoutes(mux)
	app.registerGitRoutes(mux)
	app.registerFileRoutes(mux)
	app.registerWSRoutes(mux)
}

type sessionResponse struct {
	watcher.Session
	Title           string `json:"title"`
	Description     string `json:"description"`
	DescriptionAuto bool   `json:"descriptionAuto,omitempty"`
	Order           int    `json:"order"`
	ProjectPath     string `json:"projectPath,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
}

// SessionAcker allows routes to acknowledge a session has been viewed by the user.
type SessionAcker interface {
	AckSession(name string)
}

// WatcherTrigger allows triggering an immediate watcher scan cycle.
type WatcherTrigger interface {
	Trigger()
}

// ActivityProvider supplies recent activity events for idea context capture.
type ActivityProvider interface {
	GetRecentActivity() []watcher.ActivityEvent
}

// setupTracker tracks active setup processes.
type setupTracker struct {
	mu     sync.RWMutex
	setups map[string]*activeSetup
}

type activeSetup struct {
	ID     string
	Status devcontainer.SetupStatus
	Cancel context.CancelFunc
	Done   chan struct{}
}

func newSetupTracker() *setupTracker {
	return &setupTracker{setups: make(map[string]*activeSetup)}
}

func (t *setupTracker) get(id string) *activeSetup {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.setups[id]
}

func (t *setupTracker) set(id string, s *activeSetup) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setups[id] = s
}

func (t *setupTracker) delete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.setups, id)
}

// storeProjectResolver resolves project names to paths using the store.
type storeProjectResolver struct {
	store *store.Store
}

func (r *storeProjectResolver) ResolveProject(name string) (string, bool) {
	// Phase 1: slug match (new story maps)
	if entry, ok := r.store.ResolveBySlug(name); ok {
		return entry.Path, true
	}
	// Phase 2: basename fallback (old story maps)
	if entry, ok := r.store.ResolveByBasename(name); ok {
		return entry.Path, true
	}
	return "", false
}

// generateSetupID returns a short random hex string for setup tracking.
func generateSetupID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
