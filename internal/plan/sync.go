package plan

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// SyncStatus represents the current state of epic git sync.
type SyncStatus struct {
	LastSync       *time.Time `json:"lastSync,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	PendingCommits int        `json:"pendingCommits"`
	Syncing        bool       `json:"syncing"`
}

// GitSyncer handles periodic git pull --rebase + push.
type GitSyncer struct {
	epic      SyncableRepo
	mu        sync.RWMutex
	status    SyncStatus
	interval  time.Duration
	enabled   bool
	stopCh    chan struct{}
	triggerCh chan struct{}
	running   bool
}

// NewGitSyncer creates a new syncer for the given SyncableRepo.
func NewGitSyncer(epic SyncableRepo) *GitSyncer {
	return &GitSyncer{
		epic:      epic,
		stopCh:    make(chan struct{}),
		triggerCh: make(chan struct{}, 1),
	}
}

// Configure updates the sync settings. If enabled, starts the sync loop; if disabled, stops it.
func (s *GitSyncer) Configure(enabled bool, intervalMin int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if intervalMin <= 0 {
		intervalMin = 5
	}
	s.interval = time.Duration(intervalMin) * time.Minute
	s.enabled = enabled

	if enabled && !s.running {
		s.stopCh = make(chan struct{})
		s.triggerCh = make(chan struct{}, 1)
		s.running = true
		go s.loop()
	} else if !enabled && s.running {
		close(s.stopCh)
		s.running = false
	}
}

// TriggerSync requests an immediate sync cycle.
func (s *GitSyncer) TriggerSync() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.running {
		// Do a one-shot sync even if loop isn't running
		go s.doSync()
		return
	}
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

// Status returns the current sync status.
func (s *GitSyncer) Status() SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Refresh pending count
	status := s.status
	if pending, err := s.epic.Git().PendingCommits(s.epic.BaseDir()); err == nil {
		status.PendingCommits = pending
	}
	return status
}

// Stop shuts down the sync loop.
func (s *GitSyncer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

func (s *GitSyncer) loop() {
	s.mu.RLock()
	interval := s.interval
	stopCh := s.stopCh
	triggerCh := s.triggerCh
	s.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.doSync()
		case <-triggerCh:
			s.doSync()
		}
	}
}

func (s *GitSyncer) doSync() {
	s.mu.Lock()
	s.status.Syncing = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.status.Syncing = false
		s.mu.Unlock()
	}()

	git := s.epic.Git()
	dir := s.epic.BaseDir()

	if !git.IsRepo(dir) {
		s.mu.Lock()
		s.status.LastError = fmt.Sprintf("vault 目錄不是 git repo: %s", dir)
		s.mu.Unlock()
		return
	}
	if !git.RemoteExists(dir, "origin") {
		s.mu.Lock()
		s.status.LastError = "尚未設定 origin remote，請先在設定中填寫 Git URL"
		s.mu.Unlock()
		return
	}

	// Commit any uncommitted changes before pushing
	if err := git.Add(dir, "-A"); err != nil {
		log.Printf("WARNING: epic git sync: stage failed: %v", err)
	}
	if err := git.Commit(dir, "auto-commit: sync pending changes"); err != nil {
		log.Printf("WARNING: epic git sync: commit failed: %v", err)
	}

	if err := git.Push(dir); err != nil {
		errMsg := fmt.Sprintf("push failed: %v", err)
		log.Printf("WARNING: epic git sync: %s", errMsg)
		s.mu.Lock()
		s.status.LastError = errMsg
		s.mu.Unlock()
		return
	}

	now := time.Now()
	s.mu.Lock()
	s.status.LastSync = &now
	s.status.LastError = ""
	s.mu.Unlock()
}
