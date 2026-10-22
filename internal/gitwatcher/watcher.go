package gitwatcher

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// GitWatcher watches .git/refs/ directories and broadcasts changes via a callback.
type GitWatcher struct {
	watcher        *fsnotify.Watcher
	broadcastFn    func([]byte)
	watched        map[string]bool
	dirToProject   map[string]string // watched directory → projectPath
	mu             sync.Mutex
	debounceTimers map[string]*time.Timer
}

type gitChangedMessage struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// New creates a GitWatcher that broadcasts git ref changes via broadcastFn.
func New(broadcastFn func([]byte)) (*GitWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	gw := &GitWatcher{
		watcher:        fsw,
		broadcastFn:    broadcastFn,
		watched:        make(map[string]bool),
		dirToProject:   make(map[string]string),
		debounceTimers: make(map[string]*time.Timer),
	}

	go gw.handleEvents()

	return gw, nil
}

// Watch registers a project path for git ref monitoring.
// It watches .git/refs/heads/, .git/refs/remotes/ (directories) and
// .git/HEAD + .git/index (files) — intentionally NOT the whole .git/
// directory, because lock/tmp/log churn in .git/ causes excessive events.
func (gw *GitWatcher) Watch(projectPath string) error {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	if gw.watched[projectPath] {
		return nil
	}

	// Watch ref directories
	refDirs := []string{
		filepath.Join(projectPath, ".git", "refs", "heads"),
		filepath.Join(projectPath, ".git", "refs", "remotes"),
	}
	for _, dir := range refDirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if err := gw.watcher.Add(dir); err != nil {
			log.Printf("[gitwatcher] failed to watch %s: %v", dir, err)
			continue
		}
		gw.dirToProject[dir] = projectPath
	}

	// Watch individual files:
	// - HEAD: branch switch / detached HEAD
	// - index: git add / reset / checkout (for file status updates)
	for _, name := range []string{"HEAD", "index"} {
		fpath := filepath.Join(projectPath, ".git", name)
		if _, err := os.Stat(fpath); err == nil {
			if err := gw.watcher.Add(fpath); err != nil {
				log.Printf("[gitwatcher] failed to watch %s: %v", fpath, err)
			} else {
				gw.dirToProject[fpath] = projectPath
			}
		}
	}

	gw.watched[projectPath] = true
	return nil
}

// Close cleans up the fsnotify watcher and stops all debounce timers.
func (gw *GitWatcher) Close() error {
	gw.mu.Lock()
	for _, t := range gw.debounceTimers {
		t.Stop()
	}
	gw.mu.Unlock()

	return gw.watcher.Close()
}

func (gw *GitWatcher) handleEvents() {
	for {
		select {
		case event, ok := <-gw.watcher.Events:
			if !ok {
				return
			}
			gw.onEvent(event)

		case err, ok := <-gw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[gitwatcher] error: %v", err)
		}
	}
}

func (gw *GitWatcher) onEvent(event fsnotify.Event) {
	if event.Op == 0 {
		return
	}

	projectPath := gw.resolveProject(event.Name)
	if projectPath == "" {
		return
	}

	gw.mu.Lock()
	defer gw.mu.Unlock()

	if t, ok := gw.debounceTimers[projectPath]; ok {
		t.Stop()
	}

	pp := projectPath // capture for closure
	gw.debounceTimers[projectPath] = time.AfterFunc(500*time.Millisecond, func() {
		msg := gitChangedMessage{
			Type: "git_changed",
			Path: pp,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[gitwatcher] marshal error: %v", err)
			return
		}
		gw.broadcastFn(data)
	})
}

// resolveProject finds which project path a file event belongs to by checking
// the reverse map from watched directory to project path.
func (gw *GitWatcher) resolveProject(eventPath string) string {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	dir := filepath.Dir(eventPath)

	// Check the event's directory and walk up to find a match.
	for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
		if pp, ok := gw.dirToProject[d]; ok {
			return pp
		}
	}

	// Fallback: scan all watched dirs to see if eventPath starts with one.
	for watchedDir, pp := range gw.dirToProject {
		if strings.HasPrefix(eventPath, watchedDir) {
			return pp
		}
	}

	return ""
}
