package summarizer

import (
	"github.com/miskawu/piper/internal/store"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Commander abstracts command execution.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
}

// SessionStore abstracts session metadata access.
type SessionStore interface {
	GetMeta(name string) *store.SessionMeta
	SetMeta(name string, meta store.SessionMeta) error
	SetAutoDescription(name, description string) error
}

// PaneCapture abstracts tmux pane capture.
type PaneCapture interface {
	CapturePane(name string) (string, error)
}

// SessionStatus is a minimal session info passed from the watcher.
type SessionStatus struct {
	Name   string
	Status string
}

// Options configures the summarizer.
type Options struct {
	PollInterval time.Duration // How often to allow re-summarize for same session (default 30s)
}

// Summarizer automatically generates session descriptions using claude -p.
type Summarizer struct {
	cmd       Commander
	store     SessionStore
	pane      PaneCapture
	costLog   *CostLogger
	opts      Options
	ModelFunc  func() string // returns AI model to use; nil → "haiku"
	LocaleFunc func() string // returns locale for AI output (e.g. "zh-TW"); nil/empty → auto-detect

	mu         sync.Mutex
	prevStatus map[string]string    // session name → last known status
	prevHash   map[string]string    // session name → last pane content hash
	lastRun    map[string]time.Time // session name → last summarize time
	inflight   map[string]bool      // session name → is summarizing

	stopOnce sync.Once
	stopped  chan struct{}
	wg       sync.WaitGroup
}

// New creates a new Summarizer.
func New(cmd Commander, store SessionStore, pane PaneCapture, costLog *CostLogger, opts Options) *Summarizer {
	if opts.PollInterval == 0 {
		opts.PollInterval = 30 * time.Second
	}
	return &Summarizer{
		cmd:        cmd,
		store:      store,
		pane:       pane,
		costLog:    costLog,
		opts:       opts,
		prevStatus: make(map[string]string),
		prevHash:   make(map[string]string),
		lastRun:    make(map[string]time.Time),
		inflight:   make(map[string]bool),
		stopped:    make(chan struct{}),
	}
}

// Stop gracefully stops the summarizer and waits for inflight goroutines.
func (s *Summarizer) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopped)
	})
	s.wg.Wait()
}

// OnStatusUpdate is called by the watcher with current session states.
// It checks for status transitions and triggers summarization as needed.
func (s *Summarizer) OnStatusUpdate(sessions []SessionStatus) {
	select {
	case <-s.stopped:
		return
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	activeNames := make(map[string]bool, len(sessions))
	for _, sess := range sessions {
		activeNames[sess.Name] = true
		prev := s.prevStatus[sess.Name]

		shouldTrigger := false

		switch {
		case prev != "working" && sess.Status == "working":
			// Transition to working: immediate
			shouldTrigger = true
		case prev == "working" && sess.Status == "working":
			// Still working: check interval + content hash
			if time.Since(s.lastRun[sess.Name]) >= s.opts.PollInterval {
				shouldTrigger = true
			}
		case prev == "working" && sess.Status != "working":
			// Finished working: final summary
			shouldTrigger = true
		}

		s.prevStatus[sess.Name] = sess.Status

		if !shouldTrigger {
			continue
		}

		if s.inflight[sess.Name] {
			continue
		}

		if !s.shouldSummarize(sess.Name) {
			continue
		}

		s.triggerSummary(sess.Name)
	}

	// Clean up stale entries
	for name := range s.prevStatus {
		if !activeNames[name] {
			delete(s.prevStatus, name)
			delete(s.prevHash, name)
			delete(s.lastRun, name)
			delete(s.inflight, name)
		}
	}
}

// shouldSummarize checks store: skip if user manually set description.
func (s *Summarizer) shouldSummarize(name string) bool {
	meta := s.store.GetMeta(name)
	if meta == nil {
		return true
	}
	// Skip if description is non-empty and NOT auto-generated
	if meta.Description != "" && !meta.DescriptionAuto {
		return false
	}
	return true
}

// triggerSummary captures pane, checks hash, and launches goroutine.
// Must be called with s.mu held.
func (s *Summarizer) triggerSummary(name string) {
	content, err := s.pane.CapturePane(name)
	if err != nil || strings.TrimSpace(content) == "" {
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	if hash == s.prevHash[name] {
		return
	}

	s.inflight[name] = true
	s.prevHash[name] = hash
	s.lastRun[name] = time.Now()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("WARNING: summarizer panic for %s: %v", name, r)
			}
			s.mu.Lock()
			s.inflight[name] = false
			s.mu.Unlock()
		}()
		s.runClaude(name, content)
	}()
}

// claudeResponse represents the JSON output from claude -p --output-format json.
type claudeResponse struct {
	Result string `json:"result"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// runClaude executes claude -p and updates the store.
func (s *Summarizer) runClaude(sessionName, paneContent string) {
	var langInstruction string
	if s.LocaleFunc != nil {
		if locale := s.LocaleFunc(); locale != "" {
			langInstruction = fmt.Sprintf("Reply in %s.", localeDisplayName(locale))
		}
	}
	if langInstruction == "" {
		langInstruction = "Reply in the SAME language the user used (e.g. if they wrote in 繁體中文, reply in 繁體中文; if 简体中文, reply in 简体中文; if English, reply in English)."
	}
	prompt := fmt.Sprintf(
		"Extract the user's task from the terminal content below. Summarize in ONE short sentence (max 20 words). %s Output only the summary text, no quotes or prefixes:\n\n%s",
		langInstruction, paneContent,
	)

	model := "haiku"
	if s.ModelFunc != nil {
		if m := s.ModelFunc(); m != "" {
			model = m
		}
	}
	out, err := s.cmd.Output("claude", "-p", "--model", model, "--output-format", "json", "--no-session-persistence", prompt)
	if err != nil {
		log.Printf("WARNING: summarizer claude -p failed for %s: %v", sessionName, err)
		return
	}

	var resp claudeResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		// Try plain text fallback
		text := strings.TrimSpace(string(out))
		if text != "" {
			resp.Result = text
		} else {
			log.Printf("WARNING: summarizer failed to parse claude output for %s: %v", sessionName, err)
			return
		}
	}

	if resp.Result == "" {
		return
	}

	// Double check: user might have edited description while we were running
	meta := s.store.GetMeta(sessionName)
	if meta != nil && meta.Description != "" && !meta.DescriptionAuto {
		return
	}

	// Use SetAutoDescription to update only description fields,
	// preserving ProjectPath and other metadata.
	if err := s.store.SetAutoDescription(sessionName, resp.Result); err != nil {
		log.Printf("WARNING: summarizer failed to save description for %s: %v", sessionName, err)
	}

	// Log cost
	if s.costLog != nil {
		projectPath := ""
		if meta != nil {
			projectPath = meta.ProjectPath
		}
		s.costLog.Log(CostEntry{
			Timestamp:    time.Now(),
			SessionName:  sessionName,
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			ProjectPath:  projectPath,
		})
	}
}

// localeDisplayName maps locale codes to human-readable language names for prompts.
func localeDisplayName(locale string) string {
	switch locale {
	case "zh-TW":
		return "繁體中文"
	case "zh-CN":
		return "简体中文"
	case "ja":
		return "日本語"
	case "ko":
		return "한국어"
	default:
		return locale // e.g. "en" → "en", works fine in prompts
	}
}
