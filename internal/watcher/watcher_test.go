package watcher

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/miskawu/piper/internal/tmux"
)

// mockTmuxClient implements TmuxClient for testing.
type mockTmuxClient struct {
	sessions []tmux.TmuxSession
	listErr  error
	panes    map[string]string // name -> captured content
	captErr  error
}

func (m *mockTmuxClient) ListSessions() ([]tmux.TmuxSession, error) {
	return m.sessions, m.listErr
}

func (m *mockTmuxClient) CapturePane(name string) (string, error) {
	if m.captErr != nil {
		return "", m.captErr
	}
	return m.panes[name], nil
}

func TestScan_EmptySessions(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{},
	}
	w := New(mock, time.Second, nil)

	sessions, err := w.Scan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestScan_EnrichesWithLastLine(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 2, Created: "1710000000", Attached: true},
			{Name: "ragent-2", Windows: 1, Created: "1710000100", Attached: false},
		},
		panes: map[string]string{
			"ragent-1": "Some output\n⏳ Thinking...\n",
			"ragent-2": "Done.\nType a message >",
		},
	}
	w := New(mock, time.Second, nil)

	sessions, err := w.Scan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Scan provides base info + lastLine; status defaults to needs_input
	if sessions[0].Name != "ragent-1" {
		t.Errorf("expected name ragent-1, got %s", sessions[0].Name)
	}
	if sessions[0].Status != "needs_input" {
		t.Errorf("expected default status needs_input, got %s", sessions[0].Status)
	}
	if sessions[0].Windows != 2 {
		t.Errorf("expected 2 windows, got %d", sessions[0].Windows)
	}
	if !sessions[0].Attached {
		t.Error("expected attached=true")
	}
	if sessions[1].LastLine == "" {
		t.Error("expected non-empty last line")
	}
}

func TestScan_ListSessionsError(t *testing.T) {
	mock := &mockTmuxClient{
		listErr: fmt.Errorf("tmux not running"),
	}
	w := New(mock, time.Second, nil)

	_, err := w.Scan()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScan_CapturePaneError(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000", Attached: false},
		},
		captErr: fmt.Errorf("pane error"),
	}
	w := New(mock, time.Second, nil)

	sessions, err := w.Scan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Status != "needs_input" {
		t.Errorf("expected status needs_input on capture error, got %s", sessions[0].Status)
	}
	if sessions[0].LastLine != "" {
		t.Errorf("expected empty last line on capture error, got %s", sessions[0].LastLine)
	}
}

func TestSession_JSONMarshalling(t *testing.T) {
	s := Session{
		Name:     "ragent-test",
		Windows:  3,
		Created:  "1710000000",
		Attached: true,
		Status:   "working",
		LastLine: "Processing...",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Session
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Name != s.Name || decoded.Status != s.Status {
		t.Errorf("round-trip mismatch: got %+v", decoded)
	}
}

func TestStartStop_CallsOnChange(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000", Attached: false},
		},
		panes: map[string]string{
			"ragent-1": "idle output\n",
		},
	}

	var mu sync.Mutex
	var calls int
	w := New(mock, 50*time.Millisecond, func(sessions []Session) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	go w.Start()
	time.Sleep(200 * time.Millisecond)
	w.Stop()

	mu.Lock()
	c := calls
	mu.Unlock()
	if c == 0 {
		t.Error("expected onChange to be called at least once")
	}
}

func TestTrigger_CausesImmediateScan(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{"ragent-1": "output\n"},
	}

	var mu sync.Mutex
	var calls int
	w := New(mock, 10*time.Second, func(sessions []Session) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	go w.Start()
	time.Sleep(50 * time.Millisecond) // initial scan
	mu.Lock()
	initial := calls
	mu.Unlock()

	w.Trigger()
	time.Sleep(100 * time.Millisecond)
	w.Stop()

	mu.Lock()
	final := calls
	mu.Unlock()
	if final <= initial {
		t.Errorf("expected trigger to cause additional scan, initial=%d final=%d", initial, final)
	}
}

func TestScan_清理已消失的session(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000", Attached: false},
		},
		panes: map[string]string{
			"ragent-1": "output\n",
		},
	}
	w := New(mock, time.Second, nil)
	w.Scan()

	// Session disappears
	mock.sessions = []tmux.TmuxSession{}
	mock.panes = map[string]string{}
	w.Scan()

	// Internal maps should be cleaned
	w.mu.Lock()
	if len(w.prevLastLine) != 0 {
		t.Errorf("expected prevLastLine to be empty, got %d entries", len(w.prevLastLine))
	}
	w.mu.Unlock()
}

func TestStop_Idempotent(t *testing.T) {
	mock := &mockTmuxClient{sessions: []tmux.TmuxSession{}}
	w := New(mock, time.Second, nil)
	go w.Start()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	w.Stop()
}

func TestWatcher_ActivityLog_記錄lastLine變化(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "line A\n",
		},
	}
	w := New(mock, time.Second, nil)

	w.Scan()

	mock.panes["ragent-1"] = "line B\n"
	w.Scan()

	events := w.GetRecentActivity()
	found := false
	for _, e := range events {
		if e.Kind == "output" && e.Detail == "line B" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected output event for 'line B', got %+v", events)
	}
}

func TestWatcher_ActivityLog_5分鐘後過期(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "output\n",
		},
	}
	w := New(mock, time.Second, nil)

	w.mu.Lock()
	w.activityLog = append(w.activityLog, ActivityEvent{
		Time:        time.Now().Add(-6 * time.Minute),
		SessionName: "ragent-old",
		Kind:        "status_change",
		Detail:      "old event",
	})
	w.mu.Unlock()

	w.Scan()

	events := w.GetRecentActivity()
	for _, e := range events {
		if e.Detail == "old event" {
			t.Error("expected old event to be trimmed, but it's still present")
		}
	}
}

func TestWatcher_GetRecentActivity_回傳副本(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "output\n",
		},
	}
	w := New(mock, time.Second, nil)
	w.Scan()

	events1 := w.GetRecentActivity()
	if len(events1) == 0 {
		t.Fatal("expected at least one event")
	}
	events1[0].Detail = "mutated"

	events2 := w.GetRecentActivity()
	if events2[0].Detail == "mutated" {
		t.Error("GetRecentActivity returned reference, not copy")
	}
}

func TestScan_GitStatus_Interval觸發(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "idle\n",
		},
	}

	gitCallCount := 0
	gitFn := func(name string) ([]string, bool) {
		gitCallCount++
		return []string{"a.go", "b.go", "c.go"}, true
	}
	fileFn := func(name string) []string {
		return []string{"a.go", "b.go"}
	}

	w := New(mock, time.Second, nil)
	w.SetGitDirtyFilesFunc(gitFn)
	w.SetFileTrackerFunc(fileFn)

	// First scan triggers git check
	sessions, _ := w.Scan()
	if gitCallCount != 1 {
		t.Errorf("expected 1 git call, got %d", gitCallCount)
	}
	if sessions[0].Git == nil {
		t.Fatal("expected git status")
	}
	if sessions[0].Git.SessionDirty != 2 {
		t.Errorf("expected 2 session dirty, got %d", sessions[0].Git.SessionDirty)
	}
	if sessions[0].SessionFileEdits != 2 {
		t.Errorf("expected 2 session file edits, got %d", sessions[0].SessionFileEdits)
	}
}

func TestScan_GitStatus_Fallback30秒(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "idle\n",
		},
	}

	gitCallCount := 0
	gitFn := func(name string) ([]string, bool) {
		gitCallCount++
		return []string{"a.go"}, true
	}
	fileFn := func(name string) []string {
		return []string{"a.go"}
	}

	w := New(mock, time.Second, nil)
	w.SetGitDirtyFilesFunc(gitFn)
	w.SetFileTrackerFunc(fileFn)

	w.Scan()
	if gitCallCount != 1 {
		t.Errorf("expected 1 call on initial load, got %d", gitCallCount)
	}

	// Simulate time passing
	w.mu.Lock()
	w.gitLastCheck["ragent-1"] = time.Now().Add(-31 * time.Second)
	w.mu.Unlock()

	sessions, _ := w.Scan()
	if gitCallCount != 2 {
		t.Errorf("expected 2 calls after fallback, got %d", gitCallCount)
	}
	if sessions[0].Git == nil || sessions[0].Git.SessionDirty != 1 {
		t.Error("expected refreshed git status with sessionDirty=1")
	}
}

func TestScan_GitStatus_無GitFunc時不crash(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "idle\n",
		},
	}

	w := New(mock, time.Second, nil)

	sessions, err := w.Scan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions[0].Git != nil {
		t.Error("expected nil git status when no func set")
	}
}

func TestScan_GitStatus_清理已消失session快取(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "idle\n",
		},
	}

	gitFn := func(name string) ([]string, bool) {
		return []string{"a.go"}, true
	}

	w := New(mock, time.Second, nil)
	w.SetGitDirtyFilesFunc(gitFn)
	w.Scan()

	mock.sessions = []tmux.TmuxSession{}
	mock.panes = map[string]string{}
	w.Scan()

	w.mu.Lock()
	if len(w.gitCache) != 0 {
		t.Errorf("expected gitCache to be empty, got %d entries", len(w.gitCache))
	}
	if len(w.gitLastCheck) != 0 {
		t.Errorf("expected gitLastCheck to be empty, got %d entries", len(w.gitLastCheck))
	}
	w.mu.Unlock()
}

func TestCountIntersection(t *testing.T) {
	tests := []struct {
		name         string
		sessionFiles []string
		dirtyPaths   []string
		want         int
	}{
		{"兩者皆空", nil, nil, 0},
		{"session有檔案但git乾淨", []string{"a.go"}, nil, 0},
		{"git有dirty但session沒編輯", nil, []string{"a.go"}, 0},
		{"完全交集", []string{"a.go", "b.go"}, []string{"a.go", "b.go"}, 2},
		{"部分交集", []string{"a.go", "b.go", "c.go"}, []string{"b.go", "d.go"}, 1},
		{"無交集", []string{"a.go"}, []string{"b.go"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countIntersection(tt.sessionFiles, tt.dirtyPaths)
			if got != tt.want {
				t.Errorf("countIntersection() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScan_無Git時只顯示SessionFileEdits(t *testing.T) {
	mock := &mockTmuxClient{
		sessions: []tmux.TmuxSession{
			{Name: "ragent-1", Windows: 1, Created: "1710000000"},
		},
		panes: map[string]string{
			"ragent-1": "idle\n",
		},
	}

	gitFn := func(name string) ([]string, bool) {
		return nil, false
	}
	fileFn := func(name string) []string {
		return []string{"a.go", "b.go", "c.go"}
	}

	w := New(mock, time.Second, nil)
	w.SetGitDirtyFilesFunc(gitFn)
	w.SetFileTrackerFunc(fileFn)

	sessions, _ := w.Scan()
	if sessions[0].Git != nil {
		t.Error("expected nil git when not a repo")
	}
	if sessions[0].SessionFileEdits != 3 {
		t.Errorf("expected 3 session file edits, got %d", sessions[0].SessionFileEdits)
	}
}
