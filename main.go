package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"path/filepath"

	"github.com/miskawu/piper/internal/autostop"
	"github.com/miskawu/piper/internal/clipboard"
	"github.com/miskawu/piper/internal/devcontainer"
	digestPkg "github.com/miskawu/piper/internal/digest"
	"github.com/miskawu/piper/internal/epiccontainer"
	"github.com/miskawu/piper/internal/git"
	"github.com/miskawu/piper/internal/hooksetup"
	"github.com/miskawu/piper/internal/gitwatcher"
	"github.com/miskawu/piper/internal/lockfile"
	epicplan "github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/preflight"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/summarizer"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/watcher"
	"github.com/miskawu/piper/internal/ws"
)

// version is set at build time via -ldflags.
var version = "dev"

//go:embed all:static
var staticFiles embed.FS

func main() {
	host := flag.String("host", "127.0.0.1", "bind addresses, comma-separated (e.g. 127.0.0.1,192.168.1.10)")
	port := flag.Int("port", 8622, "HTTP server port")
	pollInterval := flag.Duration("poll", 2*time.Second, "session poll interval")
	showVersion := flag.Bool("version", false, "print version and exit")
	projectRoot := flag.String("project-root", "", "piper project root directory (default: current working directory)")
	flag.Parse()

	if *showVersion {
		fmt.Println("piper", version)
		return
	}

	// Resolve project root
	resolvedRoot := *projectRoot
	if resolvedRoot == "" {
		var err error
		resolvedRoot, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get working directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Instance lock — prevent two pipers from corrupting the shared store
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	lf := lockfile.New(filepath.Join(dataDir, "piper", "piper.lock"))
	if err := lf.Acquire(*port); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	defer lf.Release()

	// Dependency check
	cmd := &tmux.ExecCommander{}
	results := preflight.RunAll(cmd, preflight.DefaultDeps())
	fmt.Fprint(os.Stderr, preflight.FormatResults(results))
	if preflight.HasRequiredFailures(results) {
		os.Exit(1)
	}

	// Detect Docker availability for epic session fallback
	dockerResult, _ := preflight.FindResult(results, preflight.DepDocker)
	dockerAvailable := dockerResult.OK
	if dockerAvailable {
		log.Println("Epic sessions: Docker mode (containerized)")
	} else {
		log.Println("Epic sessions: native mode (Docker unavailable)")
	}

	// Extract hook scripts to ~/.local/share/piper/ so local sessions can use them.
	if err := hooksetup.ExtractScripts(devcontainer.TemplateFS()); err != nil {
		log.Printf("WARNING: failed to extract hook scripts: %v", err)
	}

	// Resolve Piper host for DevContainer API access.
	// Find the first non-loopback bind address so containers can reach Piper.
	piperHost := resolvePiperHost(*host, *port)

	// Core infrastructure
	tmuxClient := tmux.NewClient(cmd)
	if piperHost != "" {
		tmuxClient.SetPiperHost(piperHost)
	}
	tmuxClient.SetLocalHost(fmt.Sprintf("127.0.0.1:%d", *port))
	tmuxClient.SetHookInjector(hooksetup.InjectSettings)
	sessionStore := store.New("")
	if err := sessionStore.Load(); err != nil {
		log.Printf("WARNING: failed to load session store: %v", err)
	}

	dcManager := devcontainer.NewManager(&tmux.ExecCommander{})
	if !dcManager.CLIAvailable() {
		log.Println("WARNING: devcontainer CLI not found — devcontainer features disabled. Install: npm install -g @devcontainers/cli")
	}

	hub := ws.NewHub()
	go hub.Run()

	gitWatcher, err := gitwatcher.New(hub.Broadcast)
	if err != nil {
		log.Printf("WARNING: failed to start git watcher: %v", err)
	}
	if gitWatcher != nil {
		defer gitWatcher.Close()
	}

	// Usage & tracking
	storeDir := filepath.Dir(sessionStore.Path())
	usageLogDir := filepath.Join(storeDir, "usage")
	usageLogger := usage.NewUsageLogger(usageLogDir)
	workTimeTracker := usage.NewWorkTimeTracker(usageLogDir)
	completionTracker := usage.NewCompletionTracker(usageLogDir)
	usageLogger.Cleanup(3)
	workTimeTracker.Cleanup(3)
	completionTracker.Cleanup(3)

	// Auto-summarizer
	costLogDir := filepath.Join(storeDir, "cost")
	costLogger := summarizer.NewCostLogger(costLogDir)
	autoSummarizer := summarizer.New(
		&tmux.ExecCommander{},
		sessionStore,
		tmuxClient,
		costLogger,
		summarizer.Options{PollInterval: 30 * time.Second},
	)
	autoSummarizer.ModelFunc = func() string {
		return sessionStore.GetModelConfig().Summarizer
	}
	autoSummarizer.LocaleFunc = func() string {
		return sessionStore.GetModelConfig().Locale
	}
	defer autoSummarizer.Stop()

	// Watcher + status broadcaster
	autoStopper := autostop.New(dcManager, sessionStore, log.Default())
	hookData := newHookDataStore()
	broadcaster := &StatusBroadcaster{
		sessionStore:      sessionStore,
		tmuxClient:        tmuxClient,
		hub:               hub,
		hookData:          hookData,
		usageLogger:       usageLogger,
		workTimeTracker:   workTimeTracker,
		completionTracker: completionTracker,
		autoStopper:       autoStopper,
		autoSummarizer:    autoSummarizer,
		gitChecker:        git.NewChecker(&tmux.ExecCommander{}),
	}
	w := watcher.New(tmuxClient, *pollInterval, broadcaster.OnChange)
	w.SetGitDirtyFilesFunc(broadcaster.GitDirtyFilesFunc())
	w.SetFileTrackerFunc(broadcaster.FileTrackerFunc())
	go w.Start()
	defer w.Stop()

	// Static files
	mux := http.NewServeMux()
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}

	// Clipboard
	clipCtx, clipCancel := context.WithCancel(context.Background())
	clipStore := clipboard.NewStore(filepath.Join(os.TempDir(), "piper-clipboard"), 30*time.Minute, 100<<20)
	clipStore.StartCleanup(clipCtx, 5*time.Minute)
	defer clipCancel()

	// Vault + migration + orphan cleanup
	vaultManager := initVault(sessionStore)
	cleanupOrphanSessions(tmuxClient, vaultManager, sessionStore)

	// Epic git syncer + file watcher
	epicGitCfg := sessionStore.GetEpicGitConfig()
	epicSyncer := epicplan.NewGitSyncer(vaultManager)
	if epicGitCfg.SyncEnabled {
		epicSyncer.Configure(true, epicGitCfg.SyncIntervalMin)
	}
	defer epicSyncer.Stop()

	// Digest scheduler
	digestScheduler := newDigestScheduler(sessionStore, workTimeTracker, completionTracker, costLogger)
	go digestScheduler.Start()
	defer digestScheduler.Stop()

	// Routes
	app := &App{
		tmuxClient:        tmuxClient,
		sessionStore:      sessionStore,
		dcManager:         dcManager,
		hub:               hub,
		fileServer:        http.FileServer(http.FS(staticFS)),
		acker:             hookData,
		costLogger:        costLogger,
		usageLogger:       usageLogger,
		workTimeTracker:   workTimeTracker,
		completionTracker: completionTracker,
		clipStore:         clipStore,
		activityProvider:  w,
		enrichCmd:         &tmux.ExecCommander{},
		vaultManager:      vaultManager,
		projectRoot:       resolvedRoot,
		epicSyncer:        epicSyncer,
		gitWatcher:        gitWatcher,
		broadcaster:       broadcaster,
		hookData:          hookData,
		watcherTrigger:    w,
		dockerAvailable:   dockerAvailable,
		epicUpdater:       epiccontainer.NewUpdater(),
	}
	app.SetupRoutes(mux)

	// Parse bind addresses
	hosts, err := parseHosts(*host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --host: %v\n", err)
		os.Exit(1)
	}

	// HTTP servers — one per bind address, sharing the same handler
	var servers []*http.Server
	for _, h := range hosts {
		addr := fmt.Sprintf("%s:%d", h, *port)
		servers = append(servers, &http.Server{
			Addr:    addr,
			Handler: mux,
		})
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU)
	go func() {
		for sig := range stopSignals {
			log.Printf("⚠️  received stop signal: %v, pid=%d, ppid=%d, pgid=%d",
				sig, os.Getpid(), os.Getppid(), syscall.Getpgrp())
			signal.Stop(stopSignals)
			syscall.Kill(os.Getpid(), sig.(syscall.Signal))
		}
	}()

	for _, srv := range servers {
		srv := srv
		go func() {
			log.Printf("piper listening on http://%s", srv.Addr)
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()
	}

	<-stop
	log.Println("shutting down...")
	autoStopper.CancelAll()
	for _, srv := range servers {
		srv.Close()
	}
}

// newDigestScheduler creates a digest scheduler with closures that
// gather data from the various trackers.
func newDigestScheduler(
	sessionStore *store.Store,
	workTimeTracker *usage.WorkTimeTracker,
	completionTracker *usage.CompletionTracker,
	costLogger *summarizer.CostLogger,
) *digestPkg.Scheduler {
	return digestPkg.NewScheduler(
		func(date string) *digestPkg.Digest {
			// Resolve timezone from config for date filtering
			loc := time.UTC
			cfg := sessionStore.GetDigestConfig()
			if cfg.Timezone != "" {
				if l, err := time.LoadLocation(cfg.Timezone); err == nil {
					loc = l
				}
			}

			targetDate, _ := time.Parse("2006-01-02", date)
			year, month, _ := targetDate.Date()

			src := &digestPkg.Sources{
				SessionDescriptions: map[string]digestPkg.SessionInfo{},
			}
			if entries, err := workTimeTracker.GetMonthlyEntries(year, month); err == nil {
				src.WorkTimeEntries = entries
			}
			// Read from session history (closed sessions for this period)
			dayStart, _ := time.Parse("2006-01-02", date)
			dayEnd := dayStart.AddDate(0, 0, 1).Add(-time.Second)
			if history, err := sessionStore.GetSessionHistory(dayStart, dayEnd); err == nil {
				for _, e := range history {
					src.SessionDescriptions[e.Name] = digestPkg.SessionInfo{
						Description: e.Description,
						ProjectPath: e.ProjectPath,
					}
				}
			}
			// Also include active sessions
			for name, meta := range sessionStore.AllSessions() {
				if _, exists := src.SessionDescriptions[name]; !exists {
					src.SessionDescriptions[name] = digestPkg.SessionInfo{
						Description: meta.Description,
						ProjectPath: meta.ProjectPath,
					}
				}
			}
			return digestPkg.Generate(date, src, loc)
		},
		func() (string, bool, int, string) {
			cfg := sessionStore.GetDigestConfig()
			return cfg.WebhookURL, cfg.Enabled, cfg.SendHour, cfg.Timezone
		},
	)
}
