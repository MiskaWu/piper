package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// makeExitError creates a real *exec.ExitError with stderr content,
// matching the behavior of exec.Command().Output() on failure.
func makeExitError(stderr string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo -n %q >&2; exit 1", stderr))
	_, err := cmd.Output()
	return err
}

// mockCommander records calls and returns canned responses.
type mockCommander struct {
	outputFunc func(name string, args ...string) ([]byte, error)
	runFunc    func(name string, args ...string) error
	calls      []string
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, fmt.Sprintf("Output: %s %s", name, strings.Join(args, " ")))
	if m.outputFunc != nil {
		return m.outputFunc(name, args...)
	}
	return nil, nil
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.calls = append(m.calls, fmt.Sprintf("Run: %s %s", name, strings.Join(args, " ")))
	if m.runFunc != nil {
		return m.runFunc(name, args...)
	}
	return nil
}

func TestSanitizeProjectName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"一般basename", "piper", "piper"},
		{"大寫轉小寫", "MyProject", "myproject"},
		{"保留dash", "my-project", "my-project"},
		{"底線轉dash", "my_project", "my-project"},
		{"去除特殊字元", "my.project@v2", "myprojectv2"},
		{"空字串", "", ""},
		{"純特殊字元", "@#$", ""},
		{"截斷過長名稱", "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrst"},
		{"數字保留", "proj123", "proj123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeProjectName(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeProjectName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractGroupFromName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"新格式", "ragent-piper-a3f1b2c9d0", "piper"},
		{"多dash專案名", "ragent-my-project-a3f1b2c9d0", "my-project"},
		{"舊格式fallback", "ragent-a3f1b2c9d0", ""},
		{"非ragent前綴", "other-session", ""},
		{"uniqueName後綴-2", "ragent-piper-a3f1b2c9d0-2", "piper"},
		{"uniqueName後綴-10", "ragent-piper-a3f1b2c9d0-10", "piper"},
		{"空字串", "", ""},
		{"只有ragent", "ragent", ""},
		{"ragent-dash", "ragent-", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGroupFromName(tt.input)
			if got != tt.expected {
				t.Errorf("ExtractGroupFromName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGenerateShortID_產生10字元hex(t *testing.T) {
	id := GenerateShortID()
	if len(id) != 10 {
		t.Errorf("expected 10 chars, got %d: %q", len(id), id)
	}
	// Verify it's valid hex
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("expected hex char, got %c in %q", c, id)
		}
	}
}

func TestGenerateShortID_不重複(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateShortID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestCheck_Success(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("tmux 3.4"), nil
		},
	}
	client := NewClient(mock)
	if err := client.Check(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheck_NotAvailable(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exec: not found")
		},
	}
	client := NewClient(mock)
	if err := client.Check(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListSessions_ParsesCorrectly(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-project1\t3\t1710000000\t1\nragent-project2\t1\t1710000100\t0\n"), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "ragent-project1" {
		t.Errorf("expected name ragent-project1, got %s", sessions[0].Name)
	}
	if sessions[0].Windows != 3 {
		t.Errorf("expected 3 windows, got %d", sessions[0].Windows)
	}
	if !sessions[0].Attached {
		t.Error("expected session 0 to be attached")
	}
	if sessions[1].Attached {
		t.Error("expected session 1 to not be attached")
	}
}

func TestListSessions_FiltersOnlyRagent(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-work\t1\t1710000000\t0\nmy-server\t2\t1710000100\t1\nother-ragent-test\t1\t1710000200\t0\n"), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (ragent only), got %d", len(sessions))
	}
	if sessions[0].Name != "ragent-work" {
		t.Errorf("expected ragent-work, got %s", sessions[0].Name)
	}
	if sessions[1].Name != "other-ragent-test" {
		t.Errorf("expected other-ragent-test, got %s", sessions[1].Name)
	}
}

func TestListSessions_NoServerRunning_ErrorMessage(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exit status 1: no server running on /tmp/tmux-1000/default")
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("expected no error for 'no server running', got %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty slice, got %d sessions", len(sessions))
	}
}

func TestListSessions_NoServerRunning_ExitError(t *testing.T) {
	// Real exec.Command returns *exec.ExitError where err.Error() is just
	// "exit status 1" and the "no server running" text is only in Stderr.
	exitErr := makeExitError("no server running on /tmp/tmux-1000/default")
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, exitErr
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("expected no error for 'no server running' via ExitError.Stderr, got %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty slice, got %d sessions", len(sessions))
	}
}

func TestListSessions_SocketFileDeleted(t *testing.T) {
	// After WSL/system restart, the tmux socket in /tmp is deleted.
	// tmux returns "error connecting to ... (No such file or directory)".
	exitErr := makeExitError("error connecting to /tmp/tmux-1000/default (No such file or directory)")
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, exitErr
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("expected no error for deleted socket, got %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty slice, got %d sessions", len(sessions))
	}
}

func TestListSessions_EmptyOutput(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(""), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty slice, got %d sessions", len(sessions))
	}
}

func TestCreateSession_AutoPrepend(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateSession("myproject", "")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls (list-sessions + new-session + set-option + set-environment), got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux new-session -d -s ragent-myproject claude"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
	if actualName != "ragent-myproject" {
		t.Errorf("expected returned name %q, got %q", "ragent-myproject", actualName)
	}
}

func TestCreateSession_NoDuplicatePrepend(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateSession("ragent-myproject", "")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux new-session -d -s ragent-myproject claude"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
	if actualName != "ragent-myproject" {
		t.Errorf("expected returned name %q, got %q", "ragent-myproject", actualName)
	}
}

func TestCreateSession_RagentPrefix(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateSession("ragent", "")
	expected := "Run: tmux new-session -d -s ragent claude"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
	if actualName != "ragent" {
		t.Errorf("expected returned name %q, got %q", "ragent", actualName)
	}
}

func TestCreateSession_WithWorkDir(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSession("myproject", "/home/user/projects/foo")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux new-session -d -s ragent-myproject -c /home/user/projects/foo claude"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
}

func TestCreateSession_EmptyWorkDir_NoFlag(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSession("test", "")
	expected := "Run: tmux new-session -d -s ragent-test claude"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
}

func TestCreateSessionWithCommand_自訂command(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateSessionWithCommand("myproject", "/home/user/proj", "claude --resume abc123")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux new-session -d -s ragent-myproject -c /home/user/proj claude --resume abc123"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
	if actualName != "ragent-myproject" {
		t.Errorf("expected returned name %q, got %q", "ragent-myproject", actualName)
	}
}

func TestCreateSessionWithCommand_已有前綴不重複(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateSessionWithCommand("ragent-test", "", "claude --resume xyz")
	expected := "Run: tmux new-session -d -s ragent-test claude --resume xyz"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
	if actualName != "ragent-test" {
		t.Errorf("expected returned name %q, got %q", "ragent-test", actualName)
	}
}

func TestCreateSessionWithCommand_無workDir(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSessionWithCommand("test", "", "claude --resume abc")
	expected := "Run: tmux new-session -d -s ragent-test claude --resume abc"
	if mock.calls[1] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[1])
	}
}

func TestKillSession(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_ = client.KillSession("ragent-test")
	expected := "Run: tmux kill-session -t ragent-test"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestRenameSession(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_ = client.RenameSession("ragent-old", "ragent-new")
	expected := "Run: tmux rename-session -t ragent-old ragent-new"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestCapturePane(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("some output\nline 2\n"), nil
		},
	}
	client := NewClient(mock)
	out, err := client.CapturePane("ragent-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "some output\nline 2\n" {
		t.Errorf("unexpected output: %q", out)
	}
	expected := "Output: tmux capture-pane -t ragent-test -p -S -50"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestSendKeys(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_ = client.SendKeys("ragent-test", "hello world")
	expected := "Run: tmux send-keys -t ragent-test hello world Enter"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestCreateDevcontainerSession_產生正確的bash指令(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	projectPath := "/home/user/projects/acme"
	actualName, err := client.CreateDevcontainerSession("acme", projectPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actualName != "ragent-acme" {
		t.Errorf("expected name %q, got %q", "ragent-acme", actualName)
	}
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls (list-sessions + new-session + set-option + set-environment), got %d: %v", len(mock.calls), mock.calls)
	}
	call := mock.calls[1]
	// Should use bash -c with devcontainer up && exec, and fallback on failure
	if !strings.Contains(call, "bash") {
		t.Errorf("expected bash wrapper, got %q", call)
	}
	if !strings.Contains(call, "devcontainer up --workspace-folder "+projectPath) {
		t.Errorf("expected devcontainer up command, got %q", call)
	}
	if !strings.Contains(call, "devcontainer exec --remote-env PIPER_SESSION=ragent-acme --workspace-folder "+projectPath+" -- bash -c") {
		t.Errorf("expected devcontainer exec with bash -c wrapper, got %q", call)
	}
	if !strings.Contains(call, "exec claude") {
		t.Errorf("expected exec claude, got %q", call)
	}
}

func TestCreateDevcontainerSession_已有ragent前綴不重複加(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	actualName, _ := client.CreateDevcontainerSession("ragent-acme", "/home/user/projects/acme", false)
	if actualName != "ragent-acme" {
		t.Errorf("expected name %q, got %q", "ragent-acme", actualName)
	}
}

func TestCreateDevcontainerSession_失敗時保持session存活(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", false)
	call := mock.calls[1]
	// The bash command should have a fallback that keeps the session alive on failure
	if !strings.Contains(call, "read") || !strings.Contains(call, "||") {
		t.Errorf("expected failure fallback with read to keep session alive, got %q", call)
	}
}

func TestCreateDevcontainerSession_autoMode啟用時帶skipPermissions(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", true)
	call := mock.calls[1]
	if !strings.Contains(call, "exec claude --dangerously-skip-permissions") {
		t.Errorf("expected exec claude --dangerously-skip-permissions inside wrapper, got %q", call)
	}
}

func TestCreateDevcontainerSession_掛載剪貼簿目錄(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", false)
	call := mock.calls[1]
	expected := "--mount type=bind,source=/tmp/piper-clipboard,target=/tmp/piper-clipboard"
	if !strings.Contains(call, expected) {
		t.Errorf("expected clipboard mount %q, got %q", expected, call)
	}
}

func TestCreateDevcontainerSession_autoMode關閉時不帶skipPermissions(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", false)
	call := mock.calls[1]
	if strings.Contains(call, "--dangerously-skip-permissions") {
		t.Errorf("should not contain --dangerously-skip-permissions, got %q", call)
	}
	if !strings.Contains(call, "exec claude'") {
		t.Errorf("expected plain 'exec claude' in wrapper, got %q", call)
	}
}

// --- HasSession tests ---

func TestHasSession_存在(t *testing.T) {
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			return nil // exit 0 = exists
		},
	}
	client := NewClient(mock)
	exists, err := client.HasSession("pplan-my-epic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected session to exist")
	}
	expected := "Run: tmux has-session -t pplan-my-epic"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestHasSession_不存在(t *testing.T) {
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			return fmt.Errorf("exit status 1")
		},
	}
	client := NewClient(mock)
	exists, err := client.HasSession("pplan-nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected session to not exist")
	}
}

func TestHasSession_無server(t *testing.T) {
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			return fmt.Errorf("no server running")
		},
	}
	client := NewClient(mock)
	exists, err := client.HasSession("pplan-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected session to not exist when no server running")
	}
}

// --- ListPlanSessions tests ---

func TestListPlanSessions_篩選pplan前綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("pplan-epic1\t1\t1710000000\t0\nragent-work\t1\t1710000000\t0\npplan-epic2\t1\t1710000100\t0\n"), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListPlanSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 plan sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "pplan-epic1" {
		t.Errorf("expected pplan-epic1, got %s", sessions[0].Name)
	}
	if sessions[1].Name != "pplan-epic2" {
		t.Errorf("expected pplan-epic2, got %s", sessions[1].Name)
	}
}

// --- CleanOrphanPlanSessions tests ---

func TestCleanOrphanPlanSessions_清理孤兒保留有效(t *testing.T) {
	var killedSessions []string
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("pplan-valid\t1\t1710000000\t0\npplan-orphan\t1\t1710000100\t0\n"), nil
		},
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "kill-session" {
				killedSessions = append(killedSessions, args[2])
			}
			return nil
		},
	}
	client := NewClient(mock)
	epicIDs := map[string]bool{"valid": true}
	err := client.CleanOrphanPlanSessions(epicIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(killedSessions) != 1 || killedSessions[0] != "pplan-orphan" {
		t.Errorf("expected to kill [pplan-orphan], killed: %v", killedSessions)
	}
}

func TestListPlanSessions_無server回傳空(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("no server running")
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListPlanSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

// --- enableMouse tests ---

func TestCreateSession_建立後啟用mouse(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSession("test", "")
	// Expect: list-sessions, new-session, set-option mouse on, set-environment
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls (list-sessions + new-session + set-option + set-environment), got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux set-option -t ragent-test mouse on"
	if mock.calls[2] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[2])
	}
}

func TestCreateSessionWithCommand_建立後啟用mouse(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSessionWithCommand("test", "", "claude --resume abc")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux set-option -t ragent-test mouse on"
	if mock.calls[2] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[2])
	}
}

func TestCreateDevcontainerSession_建立後啟用mouse(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", false)
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux set-option -t ragent-test mouse on"
	if mock.calls[2] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[2])
	}
}

// --- PIPER_SESSION env tests ---

func TestCreateSession_注入PIPER_SESSION環境變數(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSession("test", "")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux set-environment -t ragent-test PIPER_SESSION ragent-test"
	if mock.calls[3] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[3])
	}
}

func TestCreateSessionWithCommand_注入PIPER_SESSION環境變數(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateSessionWithCommand("test", "", "claude --resume abc")
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	expected := "Run: tmux set-environment -t ragent-test PIPER_SESSION ragent-test"
	if mock.calls[3] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[3])
	}
}

func TestCreateDevcontainerSession_注入PIPER_SESSION環境變數(t *testing.T) {
	mock := &mockCommander{}
	client := NewClient(mock)
	_, _ = client.CreateDevcontainerSession("test", "/path/to/project", false)
	// DevContainer 透過 --remote-env 傳入容器，同時也透過 tmux set-environment 設定
	if len(mock.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	// 驗證 devcontainer exec 帶有 --remote-env
	call := mock.calls[1]
	if !strings.Contains(call, "--remote-env PIPER_SESSION=ragent-test") {
		t.Errorf("expected --remote-env PIPER_SESSION in devcontainer exec, got %q", call)
	}
	// 也驗證 tmux set-environment（host 端的 bash 也能用）
	expected := "Run: tmux set-environment -t ragent-test PIPER_SESSION ragent-test"
	if mock.calls[3] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[3])
	}
}

// --- uniqueName tests ---

func TestUniqueName_無衝突時直接回傳(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(""), nil // no existing sessions
		},
	}
	client := NewClient(mock)
	got, err := client.uniqueName("ragent-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ragent-test" {
		t.Errorf("expected %q, got %q", "ragent-test", got)
	}
}

func TestUniqueName_名稱衝突時加後綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-test\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	got, err := client.uniqueName("ragent-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ragent-test-2" {
		t.Errorf("expected %q, got %q", "ragent-test-2", got)
	}
}

func TestUniqueName_連續衝突時遞增後綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-test\t1\t1710000000\t0\nragent-test-2\t1\t1710000000\t0\nragent-test-3\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	got, err := client.uniqueName("ragent-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ragent-test-4" {
		t.Errorf("expected %q, got %q", "ragent-test-4", got)
	}
}

func TestCreateSession_名稱衝突時自動加後綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-myproject\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	actualName, err := client.CreateSession("myproject", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actualName != "ragent-myproject-2" {
		t.Errorf("expected %q, got %q", "ragent-myproject-2", actualName)
	}
}

func TestCreateSessionWithCommand_名稱衝突時自動加後綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-test\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	actualName, err := client.CreateSessionWithCommand("test", "", "claude --resume abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actualName != "ragent-test-2" {
		t.Errorf("expected %q, got %q", "ragent-test-2", actualName)
	}
}

func TestCreateDevcontainerSession_名稱衝突時自動加後綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("ragent-acme\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	actualName, err := client.CreateDevcontainerSession("acme", "/home/user/projects/acme", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actualName != "ragent-acme-2" {
		t.Errorf("expected %q, got %q", "ragent-acme-2", actualName)
	}
}

// --- ListAllPlanTypeSessions tests ---

func TestListAllPlanTypeSessions_掃描三種前綴(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("pplan-epic1\t1\t1710000000\t0\npdisp-epic2\t1\t1710000100\t0\npretro-epic3\t1\t1710000200\t0\nragent-work\t1\t1710000300\t0\nother-session\t1\t1710000400\t0\n"), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListAllPlanTypeSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 plan-type sessions, got %d", len(sessions))
	}
	// Verify names
	names := make(map[string]bool)
	for _, s := range sessions {
		names[s.Name] = true
	}
	for _, want := range []string{"pplan-epic1", "pdisp-epic2", "pretro-epic3"} {
		if !names[want] {
			t.Errorf("expected session %q in results", want)
		}
	}
}

func TestListAllPlanTypeSessions_無server回傳空(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("no server running")
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListAllPlanTypeSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListAllPlanTypeSessions_空輸出(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(""), nil
		},
	}
	client := NewClient(mock)
	sessions, err := client.ListAllPlanTypeSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListAllPlanTypeSessions_只呼叫一次tmux(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("pplan-a\t1\t1710000000\t0\n"), nil
		},
	}
	client := NewClient(mock)
	_, _ = client.ListAllPlanTypeSessions()
	// Should only have 1 Output call (one list-sessions)
	outputCalls := 0
	for _, c := range mock.calls {
		if strings.HasPrefix(c, "Output:") {
			outputCalls++
		}
	}
	if outputCalls != 1 {
		t.Errorf("expected 1 tmux call, got %d: %v", outputCalls, mock.calls)
	}
}

func TestGetSessionCreatedAt_正常解析Unix時戳(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("1710000000\n"), nil
		},
	}
	client := NewClient(mock)
	got, err := client.GetSessionCreatedAt("test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Unix(1710000000, 0)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestGetSessionCreatedAt_tmux指令失敗(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("session not found")
		},
	}
	client := NewClient(mock)
	_, err := client.GetSessionCreatedAt("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSessionCreatedAt_非數字輸出(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("not-a-number\n"), nil
		},
	}
	client := NewClient(mock)
	_, err := client.GetSessionCreatedAt("test-session")
	if err == nil {
		t.Fatal("expected error for non-numeric output, got nil")
	}
}

func TestGetPanePID_正常解析(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("12345\n"), nil
		},
	}
	client := NewClient(mock)
	got, err := client.GetPanePID("test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 12345 {
		t.Errorf("expected 12345, got %d", got)
	}
}

func TestGetPanePID_tmux失敗(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("session not found")
		},
	}
	client := NewClient(mock)
	_, err := client.GetPanePID("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPanePID_非數字(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("not-a-pid\n"), nil
		},
	}
	client := NewClient(mock)
	_, err := client.GetPanePID("test-session")
	if err == nil {
		t.Fatal("expected error for non-numeric output, got nil")
	}
}

func TestCreateNativeEpicSession_成功建立(t *testing.T) {
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "has-session" {
				return fmt.Errorf("exit status 1") // session not found
			}
			return nil
		},
	}
	client := NewClient(mock)
	client.SetLocalHost("127.0.0.1:8622")

	name, err := client.CreateNativeEpicSession("pplan-test", "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "pplan-test" {
		t.Errorf("expected name pplan-test, got %s", name)
	}

	// Verify tmux new-session was called with -c workDir and claude (no --dangerously-skip-permissions)
	found := false
	for _, call := range mock.calls {
		if strings.Contains(call, "new-session") && strings.Contains(call, "-c") && strings.Contains(call, "/tmp/workspace") && strings.Contains(call, "claude") {
			found = true
			if strings.Contains(call, "dangerously") {
				t.Error("native epic session should not use --dangerously-skip-permissions")
			}
		}
	}
	if !found {
		t.Errorf("expected new-session call with -c workDir, got calls: %v", mock.calls)
	}

	// Verify PIPER_SESSION and PIPER_HOST env vars were set
	envCalls := 0
	for _, call := range mock.calls {
		if strings.Contains(call, "set-environment") && strings.Contains(call, "pplan-test") {
			envCalls++
		}
	}
	if envCalls < 1 {
		t.Errorf("expected set-environment calls for PIPER_SESSION, got %d", envCalls)
	}
}

func TestCreateNativeEpicSession_已存在時直接回傳(t *testing.T) {
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			// has-session succeeds (session exists)
			return nil
		},
	}
	client := NewClient(mock)

	name, err := client.CreateNativeEpicSession("pplan-existing", "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "pplan-existing" {
		t.Errorf("expected name pplan-existing, got %s", name)
	}

	// Should only have the has-session call, no new-session
	for _, call := range mock.calls {
		if strings.Contains(call, "new-session") {
			t.Error("should not create new session when already exists")
		}
	}
}

func TestCreateNativeEpicSession_注入hooks(t *testing.T) {
	hookInjected := false
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "has-session" {
				return fmt.Errorf("exit status 1") // not found
			}
			return nil
		},
	}
	client := NewClient(mock)
	client.SetHookInjector(func(dir string) error {
		hookInjected = true
		if dir != "/tmp/epic-workspace" {
			t.Errorf("hook injected with wrong dir: %s", dir)
		}
		return nil
	})

	_, err := client.CreateNativeEpicSession("pplan-hook", "/tmp/epic-workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hookInjected {
		t.Error("expected hooks to be injected")
	}
}
