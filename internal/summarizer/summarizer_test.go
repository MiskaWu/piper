package summarizer

import (
	"github.com/miskawu/piper/internal/store"
	"sync"
	"testing"
	"time"
)

// mockCommander records calls and returns preset output.
type mockCommander struct {
	mu      sync.Mutex
	calls   [][]string
	output  []byte
	err     error
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, append([]string{name}, args...))
	return m.output, m.err
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, append([]string{name}, args...))
	return m.err
}

func (m *mockCommander) getCalls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]string, len(m.calls))
	copy(result, m.calls)
	return result
}

// mockStore implements the SessionStore interface for testing.
type mockStore struct {
	mu   sync.Mutex
	data map[string]*store.SessionMeta
}

func newMockStore() *mockStore {
	return &mockStore{data: make(map[string]*store.SessionMeta)}
}

func (m *mockStore) GetMeta(name string) *store.SessionMeta {
	m.mu.Lock()
	defer m.mu.Unlock()
	if meta, ok := m.data[name]; ok {
		cp := *meta
		return &cp
	}
	return nil
}

func (m *mockStore) SetMeta(name string, meta store.SessionMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = &meta
	return nil
}

func (m *mockStore) SetAutoDescription(name, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.data[name]
	if !ok {
		return nil
	}
	meta.Description = description
	meta.DescriptionAuto = true
	return nil
}

// mockPaneCapture returns preset pane content.
type mockPaneCapture struct {
	content map[string]string
}

func (m *mockPaneCapture) CapturePane(name string) (string, error) {
	if c, ok := m.content[name]; ok {
		return c, nil
	}
	return "", nil
}

func TestSummarizer_WorkingTransitionTriggers(t *testing.T) {
	cmd := &mockCommander{output: []byte(`{"result":"修復登入 bug","usage":{"input_tokens":800,"output_tokens":20}}`)}
	st := newMockStore()
	// Session must have meta (created by POST /api/sessions in real flow)
	st.SetMeta("ragent-test", store.SessionMeta{ProjectPath: "/test/project"})
	pane := &mockPaneCapture{content: map[string]string{
		"ragent-test": "User: fix the login bug\n⏳ Thinking...",
	}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{
		PollInterval: 30 * time.Second,
	})
	defer s.Stop()

	// First call: session not working yet → no trigger
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "needs_input"}})
	time.Sleep(50 * time.Millisecond)
	if len(cmd.getCalls()) != 0 {
		t.Fatalf("expected no calls for needs_input, got %d", len(cmd.getCalls()))
	}

	// Second call: transition to working → should trigger
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	// Wait for goroutine
	time.Sleep(200 * time.Millisecond)

	calls := cmd.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected claude -p call on working transition")
	}
	// Verify claude was called
	if calls[0][0] != "claude" {
		t.Errorf("expected claude command, got %q", calls[0][0])
	}

	// Verify store was updated
	meta := st.GetMeta("ragent-test")
	if meta == nil {
		t.Fatal("expected meta to be set")
	}
	if meta.Description != "修復登入 bug" {
		t.Errorf("description = %q, want %q", meta.Description, "修復登入 bug")
	}
	if !meta.DescriptionAuto {
		t.Error("expected DescriptionAuto = true")
	}
	if meta.ProjectPath != "/test/project" {
		t.Errorf("projectPath = %q, want preserved %q", meta.ProjectPath, "/test/project")
	}
}

func TestSummarizer_ManualDescriptionSkips(t *testing.T) {
	cmd := &mockCommander{output: []byte(`{"result":"test","usage":{"input_tokens":100,"output_tokens":10}}`)}
	st := newMockStore()
	st.SetMeta("ragent-test", store.SessionMeta{
		Description:     "my manual desc",
		DescriptionAuto: false,
	})
	pane := &mockPaneCapture{content: map[string]string{"ragent-test": "working..."}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{PollInterval: 30 * time.Second})
	defer s.Stop()

	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(100 * time.Millisecond)

	if len(cmd.getCalls()) != 0 {
		t.Fatal("should not call claude when description is manually set")
	}
}

func TestSummarizer_SameContentHashSkips(t *testing.T) {
	cmd := &mockCommander{output: []byte(`{"result":"doing stuff","usage":{"input_tokens":500,"output_tokens":20}}`)}
	st := newMockStore()
	st.SetMeta("ragent-test", store.SessionMeta{})
	pane := &mockPaneCapture{content: map[string]string{
		"ragent-test": "same content",
	}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{PollInterval: 1 * time.Second})
	defer s.Stop()

	// First: working transition triggers
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(200 * time.Millisecond)
	initialCalls := len(cmd.getCalls())
	if initialCalls == 0 {
		t.Fatal("expected initial call")
	}

	// Second: same status, same content → should skip
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(100 * time.Millisecond)
	if len(cmd.getCalls()) != initialCalls {
		t.Errorf("expected no additional calls for same content, got %d total", len(cmd.getCalls()))
	}
}

func TestSummarizer_IdleTransitionTriggers(t *testing.T) {
	cmd := &mockCommander{output: []byte(`{"result":"完成登入修復","usage":{"input_tokens":900,"output_tokens":25}}`)}
	st := newMockStore()
	st.SetMeta("ragent-test", store.SessionMeta{})
	pane := &mockPaneCapture{content: map[string]string{
		"ragent-test": "Done! Changes applied.",
	}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{PollInterval: 30 * time.Second})
	defer s.Stop()

	// Set previous status to working
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(200 * time.Millisecond)
	callsBefore := len(cmd.getCalls())

	// Change pane content so hash differs
	pane.content["ragent-test"] = "All done! Task completed."

	// Transition working → idle/completed
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "completed"}})
	time.Sleep(200 * time.Millisecond)

	if len(cmd.getCalls()) <= callsBefore {
		t.Fatal("expected final summary call on idle transition")
	}
}

func TestSummarizer_InflightPreventsDouble(t *testing.T) {
	// Use a slow commander to simulate inflight
	cmd := &mockCommander{output: []byte(`{"result":"test","usage":{"input_tokens":100,"output_tokens":10}}`)}
	st := newMockStore()
	pane := &mockPaneCapture{content: map[string]string{"ragent-test": "content"}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{PollInterval: 30 * time.Second})
	defer s.Stop()

	// Manually set inflight
	s.mu.Lock()
	s.inflight["ragent-test"] = true
	s.mu.Unlock()

	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(100 * time.Millisecond)

	if len(cmd.getCalls()) != 0 {
		t.Fatal("should not call claude when inflight")
	}
}

func TestSummarizer_PeriodicResummarize(t *testing.T) {
	callCount := 0
	cmd := &mockCommander{output: []byte(`{"result":"working on it","usage":{"input_tokens":500,"output_tokens":20}}`)}
	st := newMockStore()
	st.SetMeta("ragent-test", store.SessionMeta{})
	pane := &mockPaneCapture{content: map[string]string{"ragent-test": "content v1"}}
	costDir := t.TempDir()

	s := New(cmd, st, pane, NewCostLogger(costDir), Options{PollInterval: 100 * time.Millisecond})
	defer s.Stop()

	// Initial working transition
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(200 * time.Millisecond)
	callCount = len(cmd.getCalls())
	if callCount == 0 {
		t.Fatal("expected initial call")
	}

	// Change content + enough time passes → should re-summarize
	pane.content["ragent-test"] = "content v2"
	s.OnStatusUpdate([]SessionStatus{{Name: "ragent-test", Status: "working"}})
	time.Sleep(200 * time.Millisecond)

	if len(cmd.getCalls()) <= callCount {
		t.Error("expected re-summarize after content change + interval passed")
	}
}
