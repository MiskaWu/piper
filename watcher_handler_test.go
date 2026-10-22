package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/watcher"
	"github.com/stretchr/testify/assert"
)

func TestOverlayHookData(t *testing.T) {
	tests := []struct {
		name       string
		input      []watcher.Session
		seedStatus map[string]string
		seedMeta   map[string]*hookMetaEntry
		wantStatus []string
		wantMeta   []*watcher.HookMeta
	}{
		{
			name: "hook 有 working 時覆蓋 watcher 的 needs_input 預設值",
			input: []watcher.Session{
				{Name: "ragent-a", Status: "needs_input"},
			},
			seedStatus: map[string]string{"ragent-a": "working"},
			wantStatus: []string{"working"},
			wantMeta:   []*watcher.HookMeta{nil},
		},
		{
			name: "hook 沒資料時保留 watcher 原值（不覆蓋成空字串）",
			input: []watcher.Session{
				{Name: "ragent-unknown", Status: "needs_input"},
			},
			wantStatus: []string{"needs_input"},
			wantMeta:   []*watcher.HookMeta{nil},
		},
		{
			name: "多 session 各自獨立覆蓋",
			input: []watcher.Session{
				{Name: "ragent-a", Status: "needs_input"},
				{Name: "ragent-b", Status: "needs_input"},
				{Name: "ragent-c", Status: "needs_input"},
			},
			seedStatus: map[string]string{
				"ragent-a": "working",
				"ragent-c": "working",
			},
			wantStatus: []string{"working", "needs_input", "working"},
			wantMeta:   []*watcher.HookMeta{nil, nil, nil},
		},
		{
			name: "hook meta 覆蓋到 HookMeta 欄位",
			input: []watcher.Session{
				{Name: "ragent-meta", Status: "needs_input"},
			},
			seedMeta: map[string]*hookMetaEntry{
				"ragent-meta": {Model: "claude-opus-4-7", CostUSD: 1.23, ContextUsedPct: 42},
			},
			wantStatus: []string{"needs_input"},
			wantMeta: []*watcher.HookMeta{
				{Model: "claude-opus-4-7", CostUSD: 1.23, ContextUsedPct: 42, StatusSource: watcher.StatusSourceHook},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			hd := newHookDataStore()
			for name, status := range tt.seedStatus {
				hd.SetHookStatus(name, status, "test", time.Now().Unix())
			}
			for name, meta := range tt.seedMeta {
				hd.SetSessionMeta(name, meta)
			}

			// Act
			overlayHookData(tt.input, hd)

			// Assert
			for i, want := range tt.wantStatus {
				assert.Equal(t, want, tt.input[i].Status, "session[%d] status", i)
				assert.Equal(t, tt.wantMeta[i], tt.input[i].HookMeta, "session[%d] meta", i)
			}
		})
	}
}

func TestOnSessionClosed_ArchivesAndRecordsCompletion(t *testing.T) {
	dir := t.TempDir()

	ss := store.New(filepath.Join(dir, "sessions.json"))
	ss.SetMeta("ragent-test-abc", store.SessionMeta{
		Title:       "test session",
		Description: "doing stuff",
		ProjectPath: "/home/user/my-project",
	})

	ct := usage.NewCompletionTracker(dir)

	b := &StatusBroadcaster{
		sessionStore:      ss,
		completionTracker: ct,
		hookData:          newHookDataStore(),
	}

	b.onSessionClosed("ragent-test-abc")

	// Meta should be removed (archived internally)
	if meta := ss.GetMeta("ragent-test-abc"); meta != nil {
		t.Error("expected session meta to be removed after close")
	}

	// Completion should be recorded
	now := time.Now().UTC()
	entries, err := ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("unexpected error getting entries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 completion, got %d", len(entries))
	}
}

func TestOnSessionClosed_NoMeta_StillRecordsCompletion(t *testing.T) {
	dir := t.TempDir()

	ss := store.New(filepath.Join(dir, "sessions.json"))
	ct := usage.NewCompletionTracker(dir)

	b := &StatusBroadcaster{
		sessionStore:      ss,
		completionTracker: ct,
		hookData:          newHookDataStore(),
	}

	b.onSessionClosed("ragent-unknown-xyz")

	now := time.Now().UTC()
	entries, err := ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("unexpected error getting entries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 completion, got %d", len(entries))
	}
}

func TestOnSessionClosed_NilCompletionTracker(t *testing.T) {
	dir := t.TempDir()

	ss := store.New(filepath.Join(dir, "sessions.json"))
	ss.SetMeta("ragent-test-abc", store.SessionMeta{
		Title: "test",
	})

	b := &StatusBroadcaster{
		sessionStore:      ss,
		completionTracker: nil,
		hookData:          newHookDataStore(),
	}

	// Should not panic
	b.onSessionClosed("ragent-test-abc")

	if meta := ss.GetMeta("ragent-test-abc"); meta != nil {
		t.Error("expected session meta to be removed after close")
	}
}
