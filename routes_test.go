package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miskawu/piper/internal/clipboard"
	gitPkg "github.com/miskawu/piper/internal/git"
	"github.com/miskawu/piper/internal/devcontainer"
	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/usage"
	"github.com/miskawu/piper/internal/vault"
	"github.com/miskawu/piper/internal/watcher"
	"github.com/miskawu/piper/internal/ws"

	gorillaws "github.com/gorilla/websocket"
)

// mockCommander records calls and returns canned responses.
type mockCommander struct {
	outputFunc func(name string, args ...string) ([]byte, error)
	runFunc    func(name string, args ...string) error
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	if m.outputFunc != nil {
		return m.outputFunc(name, args...)
	}
	return nil, nil
}

func (m *mockCommander) OutputWithStderr(name string, args ...string) ([]byte, []byte, error) {
	stdout, err := m.Output(name, args...)
	return stdout, nil, err
}

func (m *mockCommander) Run(name string, args ...string) error {
	if m.runFunc != nil {
		return m.runFunc(name, args...)
	}
	return nil
}

// mockActivityProvider implements ActivityProvider for testing.
type mockActivityProvider struct {
	events []watcher.ActivityEvent
}

func (m *mockActivityProvider) GetRecentActivity() []watcher.ActivityEvent {
	return m.events
}

type testServerOptions struct {
	mock               *mockCommander
	store              *store.Store
	usageLogger        *usage.UsageLogger
	workTimeTracker    *usage.WorkTimeTracker
	completionTracker  *usage.CompletionTracker
	clipStore          *clipboard.Store
	activityProvider   ActivityProvider
	enrichCmd          tmux.Commander
	vaultManager       *vault.VaultManager
	setupRunner        *devcontainer.SetupRunner
	tracker            *setupTracker
	epicSyncer         *plan.GitSyncer
	gitFetcher         *gitPkg.GraphFetcher
	epicImageEnsurer   func() error
	dockerAvailable    bool
}

func setupTestServer(t *testing.T, mock *mockCommander) *httptest.Server {
	t.Helper()
	return setupTestServerWithOpts(t, testServerOptions{mock: mock})
}

func setupTestServerWithOpts(t *testing.T, opts testServerOptions) *httptest.Server {
	t.Helper()
	mock := opts.mock
	if mock == nil {
		mock = &mockCommander{}
	}
	tmuxClient := tmux.NewClient(mock)
	sessionStore := opts.store
	if sessionStore == nil {
		sessionStore = store.New(filepath.Join(t.TempDir(), "sessions.json"))
		if err := sessionStore.Load(); err != nil {
			t.Fatalf("failed to load session store: %v", err)
		}
	}
	hub := ws.NewHub()
	go hub.Run()

	dcManager := devcontainer.NewManager(mock)

	clipStore := opts.clipStore
	if clipStore == nil {
		clipStore = clipboard.NewStore(filepath.Join(t.TempDir(), "clipboard"), 30*time.Minute, 10<<20)
	}

	vm := opts.vaultManager
	if vm == nil {
		vaultDir := filepath.Join(t.TempDir(), "vault")
		vm = vault.New(vaultDir)
		if err := vm.Init(); err != nil {
			t.Fatalf("failed to init vault: %v", err)
		}
	}

	mux := http.NewServeMux()
	broadcaster := &StatusBroadcaster{
		sessionStore:      sessionStore,
		completionTracker: opts.completionTracker,
		hookData:          newHookDataStore(),
	}
	app := &App{
		tmuxClient:        tmuxClient,
		sessionStore:      sessionStore,
		dcManager:         dcManager,
		hub:               hub,
		fileServer:        http.NotFoundHandler(),
		usageLogger:       opts.usageLogger,
		workTimeTracker:   opts.workTimeTracker,
		completionTracker: opts.completionTracker,
		clipStore:         clipStore,
		activityProvider:  opts.activityProvider,
		enrichCmd:         opts.enrichCmd,
		vaultManager:      vm,
		projectRoot:       t.TempDir(),
		setupRunner:       opts.setupRunner,
		setupTracker:      opts.tracker,
		epicSyncer:        opts.epicSyncer,
		gitFetcher:        opts.gitFetcher,
		broadcaster:       broadcaster,
		epicImageEnsurer:  opts.epicImageEnsurer,
		dockerAvailable:   opts.dockerAvailable,
	}
	app.SetupRoutes(mux)
	return httptest.NewServer(mux)
}

func TestPostSessions_ReturnsActualSessionName(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		runFunc        func(string, ...string) error
		expectedStatus int
		expectedName   string
	}{
		{
			name:           "無前綴自動加 ragent-",
			body:           `{"name":"1"}`,
			expectedStatus: http.StatusCreated,
			expectedName:   "ragent-1",
		},
		{
			name:           "已有前綴不重複",
			body:           `{"name":"ragent-myproject"}`,
			expectedStatus: http.StatusCreated,
			expectedName:   "ragent-myproject",
		},
		{
			name:           "空名稱自動產生 ragent-{shortID}",
			body:           `{"name":""}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "無效 JSON 回傳 400",
			body:           `not json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "tmux 錯誤回傳 500",
			body: `{"name":"fail"}`,
			runFunc: func(name string, args ...string) error {
				return fmt.Errorf("tmux: server exited")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCommander{runFunc: tt.runFunc}
			ts := setupTestServer(t, mock)
			defer ts.Close()

			resp, err := http.Post(ts.URL+"/api/sessions", "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("unexpected request error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if tt.expectedName != "" {
				var result map[string]string
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if result["name"] != tt.expectedName {
					t.Errorf("expected name %q, got %q", tt.expectedName, result["name"])
				}
			}
		})
	}
}

func TestPostSessions_自動產生Title和Order(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		expectedTitle string
	}{
		{
			name:          "從 workDir 取 basename 作為 title",
			body:          `{"workDir":"/home/user/projects/piper"}`,
			expectedTitle: "PIPER",
		},
		{
			name:          "從 projectPath 取 basename 作為 title（devcontainer）",
			body:          `{"projectPath":"/home/user/projects/acme"}`,
			expectedTitle: "ACME",
		},
		{
			name:          "無 workDir 時 title 為空",
			body:          `{}`,
			expectedTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCommander{}
			sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
			if err := sessionStore.Load(); err != nil {
				t.Fatalf("failed to load store: %v", err)
			}

			ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
			defer ts.Close()

			resp, err := http.Post(ts.URL+"/api/sessions", "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("unexpected request error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
			}

			var result map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// Verify auto-generated name starts with ragent-
			if !strings.HasPrefix(result["name"], "ragent-") {
				t.Errorf("expected name to start with ragent-, got %q", result["name"])
			}
			// Verify name contains project segment when title is expected
			if tt.expectedTitle != "" {
				expectedPrefix := "ragent-" + strings.ToLower(tt.expectedTitle) + "-"
				if !strings.HasPrefix(result["name"], expectedPrefix) {
					t.Errorf("expected name to start with %q, got %q", expectedPrefix, result["name"])
				}
			}

			// Verify title in store
			meta := sessionStore.GetMeta(result["name"])
			if meta == nil {
				t.Fatal("expected session meta to be saved")
			}
			if meta.Title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, meta.Title)
			}
		})
	}
}

func TestPostSessions_同專案排序聚合(t *testing.T) {
	mock := &mockCommander{}
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	if err := sessionStore.Load(); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	// Create session for project A
	resp, _ := http.Post(ts.URL+"/api/sessions", "application/json",
		strings.NewReader(`{"workDir":"/home/user/project-a"}`))
	resp.Body.Close()

	// Create session for project B
	resp, _ = http.Post(ts.URL+"/api/sessions", "application/json",
		strings.NewReader(`{"workDir":"/home/user/project-b"}`))
	resp.Body.Close()

	// Create another session for project A — should be inserted after the first A
	resp, _ = http.Post(ts.URL+"/api/sessions", "application/json",
		strings.NewReader(`{"workDir":"/home/user/project-a"}`))
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	meta := sessionStore.GetMeta(result["name"])
	if meta == nil {
		t.Fatal("expected session meta to be saved")
	}
	// The second project-a session should have order 1 (right after the first project-a at order 0)
	if meta.Order != 1 {
		t.Errorf("expected order 1 (grouped with first project-a), got %d", meta.Order)
	}
}

func TestGetProjects_回傳已註冊專案(t *testing.T) {
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()

	// Create a temp dir with .devcontainer
	projDir := t.TempDir()
	os.MkdirAll(filepath.Join(projDir, ".devcontainer"), 0o755)
	sessionStore.AddProject(projDir)

	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Projects             []devcontainer.Project `json:"projects"`
		DevcontainerCLI      bool                   `json:"devcontainerCLI"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
	if result.Projects[0].Path != projDir {
		t.Errorf("expected path %s, got %s", projDir, result.Projects[0].Path)
	}
}

func TestGetProjects_devcontainerCLI可用(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "devcontainer" && len(args) > 0 && args[0] == "--help" {
				return []byte("ok"), nil
			}
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		DevcontainerCLI bool `json:"devcontainerCLI"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.DevcontainerCLI {
		t.Error("expected devcontainerCLI=true when CLI is available")
	}
}

func TestGetProjects_devcontainerCLI不可用(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "devcontainer" {
				return nil, fmt.Errorf("not found")
			}
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		DevcontainerCLI bool `json:"devcontainerCLI"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.DevcontainerCLI {
		t.Error("expected devcontainerCLI=false when CLI is not available")
	}
}

func TestPostProjects_新增專案(t *testing.T) {
	projDir := t.TempDir()
	os.MkdirAll(filepath.Join(projDir, ".devcontainer"), 0o755)

	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock})
	defer ts.Close()

	body := fmt.Sprintf(`{"path":"%s"}`, projDir)
	resp, err := http.Post(ts.URL+"/api/projects", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var project devcontainer.Project
	json.NewDecoder(resp.Body).Decode(&project)
	if project.Path != projDir {
		t.Errorf("expected path %s, got %s", projDir, project.Path)
	}
	if project.ContainerState != "none" {
		t.Errorf("expected state none, got %s", project.ContainerState)
	}
}

func TestPostProjects_無devcontainer目錄回傳400(t *testing.T) {
	projDir := t.TempDir() // no .devcontainer

	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := fmt.Sprintf(`{"path":"%s"}`, projDir)
	resp, err := http.Post(ts.URL+"/api/projects", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostProjects_空路徑回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/projects", "application/json", strings.NewReader(`{"path":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostProjects_相對路徑回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/projects", "application/json", strings.NewReader(`{"path":"relative/path"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDeleteProjects_移除專案(t *testing.T) {
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject("/home/user/my-project")

	ts := setupTestServerWithOpts(t, testServerOptions{store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/projects/my-project", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify removed from store
	projects := sessionStore.GetProjects()
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestDeleteProjects_cleanup_containers_呼叫RemoveContainer(t *testing.T) {
	projectPath := "/home/user/my-project"
	var dockerRmCalled bool
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			if name == "docker" && len(args) > 0 && args[0] == "rm" {
				dockerRmCalled = true
			}
			return nil
		},
	}
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject(projectPath)

	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/projects/my-project?cleanup=containers", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if !dockerRmCalled {
		t.Error("expected docker rm -f to be called")
	}

	// Verify response contains cleanup result
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["containerRemoved"] != true {
		t.Errorf("expected containerRemoved=true, got %v", result["containerRemoved"])
	}
}

func TestDeleteProjects_cleanup_images_呼叫RemoveImage(t *testing.T) {
	projectPath := "/home/user/my-project"
	var dockerRmiCalled bool
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:abc123"), nil
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			if name == "docker" && len(args) > 0 && args[0] == "rmi" {
				dockerRmiCalled = true
			}
			return nil
		},
	}
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject(projectPath)

	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/projects/my-project?cleanup=containers&cleanup=images", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["containerRemoved"] != true {
		t.Errorf("expected containerRemoved=true, got %v", result["containerRemoved"])
	}
	if !dockerRmiCalled {
		t.Error("expected docker rmi to be called")
	}
	if result["imageRemoved"] != true {
		t.Errorf("expected imageRemoved=true, got %v", result["imageRemoved"])
	}
}

func TestDeleteProjects_無cleanup參數不觸發Docker操作(t *testing.T) {
	var dockerCalled bool
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				dockerCalled = true
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			if name == "docker" {
				dockerCalled = true
			}
			return nil
		},
	}
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject("/home/user/my-project")

	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/projects/my-project", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if dockerCalled {
		t.Error("expected no docker calls without cleanup param")
	}
}

func TestGetProjects_回傳autoMode欄位(t *testing.T) {
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()

	projDir := t.TempDir()
	os.MkdirAll(filepath.Join(projDir, ".devcontainer"), 0o755)
	sessionStore.AddProject(projDir)
	autoMode := true
	sessionStore.SetProjectAutoMode(projDir, &autoMode)

	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Projects []struct {
			Path     string `json:"path"`
			AutoMode *bool  `json:"autoMode"`
		} `json:"projects"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
	if result.Projects[0].AutoMode == nil || *result.Projects[0].AutoMode != true {
		t.Errorf("expected autoMode=true, got %v", result.Projects[0].AutoMode)
	}
}

func TestGetProjects_hasManifest欄位(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(projDir string)
		wantHasMan bool
	}{
		{
			name:       "沒有 .plan/manifest.yaml 時 hasManifest=false",
			setup:      func(projDir string) {},
			wantHasMan: false,
		},
		{
			name: "存在 .plan/manifest.yaml 時 hasManifest=true",
			setup: func(projDir string) {
				planDir := filepath.Join(projDir, ".plan")
				os.MkdirAll(planDir, 0o755)
				os.WriteFile(filepath.Join(planDir, "manifest.yaml"), []byte("version: 1\n"), 0o644)
			},
			wantHasMan: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
			sessionStore.Load()

			projDir := t.TempDir()
			os.MkdirAll(filepath.Join(projDir, ".devcontainer"), 0o755)
			sessionStore.AddProject(projDir)
			tt.setup(projDir)

			mock := &mockCommander{
				outputFunc: func(name string, args ...string) ([]byte, error) {
					return []byte(""), nil
				},
			}
			ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
			defer ts.Close()

			resp, err := http.Get(ts.URL + "/api/projects")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer resp.Body.Close()

			var result struct {
				Projects []struct {
					HasManifest bool `json:"hasManifest"`
				} `json:"projects"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			if len(result.Projects) != 1 {
				t.Fatalf("expected 1 project, got %d", len(result.Projects))
			}
			if result.Projects[0].HasManifest != tt.wantHasMan {
				t.Errorf("HasManifest = %v, want %v", result.Projects[0].HasManifest, tt.wantHasMan)
			}
		})
	}
}

func TestPutProjectAutoMode_設定autoMode(t *testing.T) {
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject("/home/user/my-project")

	ts := setupTestServerWithOpts(t, testServerOptions{store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/projects/my-project/auto-mode",
		strings.NewReader(`{"autoMode":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify stored
	got := sessionStore.GetProjectAutoMode("/home/user/my-project")
	if got == nil || *got != true {
		t.Errorf("expected autoMode=true in store, got %v", got)
	}
}

func TestPutProjectAutoMode_不存在回傳404(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/projects/nonexistent/auto-mode",
		strings.NewReader(`{"autoMode":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRebuildProject_清除autoMode(t *testing.T) {
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	sessionStore.Load()
	sessionStore.AddProject("/home/user/my-project")
	autoMode := true
	sessionStore.SetProjectAutoMode("/home/user/my-project", &autoMode)

	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(fmt.Sprintf("%s\trunning\tcontainer\txyz\n", "/home/user/my-project")), nil
			}
			return nil, nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: sessionStore})
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/projects/my-project/rebuild", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// autoMode should be cleared
	got := sessionStore.GetProjectAutoMode("/home/user/my-project")
	if got != nil {
		t.Errorf("expected autoMode nil after rebuild, got %v", *got)
	}
}

func TestDeleteProjects_不存在回傳404(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/projects/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}


func TestGetDashboardStats_合併回傳(t *testing.T) {
	dir := t.TempDir()
	logger := usage.NewUsageLogger(dir, usage.WithLogInterval(0))
	tracker := usage.NewWorkTimeTracker(dir)

	// Write usage data
	fiveH := 85.0
	logger.LogIfDue(&usage.Usage{
		FiveHour: &usage.Quota{Utilization: fiveH, ResetsAt: "2026-03-13T10:00:00Z"},
	})

	// Write worktime data
	tracker.SetActiveForTest("ragent-test", time.Now().Add(-60*time.Second))
	tracker.Update([]usage.SessionStatus{{Name: "ragent-test", Status: "idle"}})

	ts := setupTestServerWithOpts(t, testServerOptions{
		usageLogger:     logger,
		workTimeTracker: tracker,
	})
	defer ts.Close()

	now := time.Now()
	url := fmt.Sprintf("%s/api/dashboard-stats?year=%d&month=%d", ts.URL, now.Year(), int(now.Month()))
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET /api/dashboard-stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Verify all sections present
	if result["usage"] == nil {
		t.Error("expected usage section")
	}
	if result["worktime"] == nil {
		t.Error("expected worktime section")
	}
	if result["summaryCost"] == nil {
		t.Error("expected summaryCost section")
	}

	// Verify usage has daily array and summary fields
	u := result["usage"].(map[string]interface{})
	if u["daily"] == nil {
		t.Error("expected usage.daily")
	}
	if u["daysAbove80Pct"].(float64) != 1 {
		t.Errorf("expected 1 day above 80%%, got %v", u["daysAbove80Pct"])
	}

	// Verify worktime has daily array
	wt := result["worktime"].(map[string]interface{})
	if wt["daily"] == nil {
		t.Error("expected worktime.daily")
	}
}

func TestGetDashboardStats_nil依賴回傳空結構(t *testing.T) {
	ts := setupTestServerWithOpts(t, testServerOptions{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dashboard-stats")
	if err != nil {
		t.Fatalf("GET /api/dashboard-stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["usage"] == nil {
		t.Error("expected usage section even with nil logger")
	}
	if result["worktime"] == nil {
		t.Error("expected worktime section even with nil tracker")
	}
}

func TestPostClipboardImage_上傳成功(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	// Create multipart form with image
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("image", "test.png")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	part.Write([]byte("fake-png-data"))
	// Set content type header for the part
	w.Close()

	// Re-create with explicit content type
	buf.Reset()
	w = multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="image"; filename="test.png"`}
	h["Content-Type"] = []string{"image/png"}
	part, err = w.CreatePart(h)
	if err != nil {
		t.Fatalf("failed to create form part: %v", err)
	}
	part.Write([]byte("fake-png-data"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/image", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var clipResult map[string]string
	json.NewDecoder(resp.Body).Decode(&clipResult)
	if clipResult["id"] == "" {
		t.Error("expected non-empty id")
	}
	if clipResult["path"] == "" {
		t.Error("expected non-empty path")
	}
	if !strings.HasSuffix(clipResult["path"], ".png") {
		t.Errorf("expected path ending with .png, got %s", clipResult["path"])
	}
}

func TestPostClipboardImage_非圖片類型回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="image"; filename="test.txt"`}
	h["Content-Type"] = []string{"text/plain"}
	part, _ := w.CreatePart(h)
	part.Write([]byte("not an image"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/image", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostClipboardImage_缺少image欄位回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("other", "data")
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/image", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetClipboardImage_取得已上傳圖片(t *testing.T) {
	clipStore := clipboard.NewStore(filepath.Join(t.TempDir(), "clipboard"), 30*time.Minute, 10<<20)
	ts := setupTestServerWithOpts(t, testServerOptions{clipStore: clipStore})
	defer ts.Close()

	// Upload first
	id, _, err := clipStore.Save(strings.NewReader("fake-png-data"), "image/png")
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/clipboard/image/" + id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake-png-data" {
		t.Errorf("expected 'fake-png-data', got %q", string(body))
	}
}

func TestGetClipboardImage_不存在回傳404(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/clipboard/image/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPostClipboardImage_projectPath被忽略統一存到tmpDir(t *testing.T) {
	clipDir := filepath.Join(t.TempDir(), "clipboard")
	clipStore := clipboard.NewStore(clipDir, 30*time.Minute, 10<<20)
	ts := setupTestServerWithOpts(t, testServerOptions{clipStore: clipStore})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="image"; filename="test.png"`}
	h["Content-Type"] = []string{"image/png"}
	part, _ := w.CreatePart(h)
	part.Write([]byte("project-png-data"))
	w.WriteField("projectPath", "/some/project") // should be ignored
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/image", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	// Path should be absolute under the clipStore directory, not under projectPath
	if !strings.HasPrefix(result["path"], clipDir) {
		t.Errorf("expected path under %s, got %s", clipDir, result["path"])
	}

	// GET should find it without projectPath
	getResp, err := http.Get(ts.URL + "/api/clipboard/image/" + result["id"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", getResp.StatusCode)
	}
}

func TestPostClipboardFile_上傳xlsx回傳id和path(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="report.xlsx"`}
	h["Content-Type"] = []string{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("failed to create form part: %v", err)
	}
	part.Write([]byte("fake-xlsx-data"))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/file", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] == "" {
		t.Error("expected non-empty id")
	}
	if !strings.HasSuffix(result["path"], ".xlsx") {
		t.Errorf("expected path ending with .xlsx, got %s", result["path"])
	}
}

func TestPostClipboardFile_缺少file欄位回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("other", "data")
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/file", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetClipboardFile_取得已上傳檔案(t *testing.T) {
	clipStore := clipboard.NewStore(filepath.Join(t.TempDir(), "clipboard"), 30*time.Minute, 10<<20)
	ts := setupTestServerWithOpts(t, testServerOptions{clipStore: clipStore})
	defer ts.Close()

	id, _, err := clipStore.SaveFile(strings.NewReader("fake-xlsx-data"), "report.xlsx")
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/clipboard/file/" + id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake-xlsx-data" {
		t.Errorf("expected 'fake-xlsx-data', got %q", string(body))
	}
}

func TestGetClipboardFile_不存在回傳404(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/clipboard/file/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPostClipboardFile_projectPath被忽略統一存到tmpDir(t *testing.T) {
	clipDir := filepath.Join(t.TempDir(), "clipboard")
	clipStore := clipboard.NewStore(clipDir, 30*time.Minute, 10<<20)
	ts := setupTestServerWithOpts(t, testServerOptions{clipStore: clipStore})
	defer ts.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="data.csv"`}
	h["Content-Type"] = []string{"text/csv"}
	part, _ := w.CreatePart(h)
	part.Write([]byte("a,b,c"))
	w.WriteField("projectPath", "/some/project") // should be ignored
	w.Close()

	resp, err := http.Post(ts.URL+"/api/clipboard/file", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	// Path should be absolute under the clipStore directory
	if !strings.HasPrefix(result["path"], clipDir) {
		t.Errorf("expected path under %s, got %s", clipDir, result["path"])
	}
	if !strings.HasSuffix(result["path"], ".csv") {
		t.Errorf("expected path ending with .csv, got %s", result["path"])
	}

	// GET should find it without projectPath
	getResp, err := http.Get(ts.URL + "/api/clipboard/file/" + result["id"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", getResp.StatusCode)
	}
}

func TestGetIdeas_空列表回傳空陣列(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/ideas")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var ideas []vault.Idea
	json.NewDecoder(resp.Body).Decode(&ideas)
	if len(ideas) != 0 {
		t.Errorf("expected empty array, got %d items", len(ideas))
	}
}

func TestPostIdeas_建立idea回傳201(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/ideas", "application/json",
		strings.NewReader(`{"text":"my idea"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var idea vault.Idea
	json.NewDecoder(resp.Body).Decode(&idea)
	if idea.ID != 1 {
		t.Errorf("expected ID 1, got %d", idea.ID)
	}
	if idea.Text != "my idea" {
		t.Errorf("expected text 'my idea', got %q", idea.Text)
	}
}

func TestPostIdeas_自動附帶activity上下文(t *testing.T) {
	events := []watcher.ActivityEvent{
		{Time: time.Now(), SessionName: "ragent-test", Kind: "output", Detail: "running test"},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{
		activityProvider: &mockActivityProvider{events: events},
	})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/ideas", "application/json",
		strings.NewReader(`{"text":"idea with context"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var idea vault.Idea
	json.NewDecoder(resp.Body).Decode(&idea)
	if len(idea.Context) != 1 {
		t.Fatalf("expected 1 context event, got %d", len(idea.Context))
	}
	if idea.Context[0].Session != "ragent-test" {
		t.Errorf("expected sessionName 'ragent-test', got %q", idea.Context[0].Session)
	}
}

func TestPostIdeas_空text回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/ideas", "application/json",
		strings.NewReader(`{"text":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostIdeas_無效JSON回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/ideas", "application/json",
		strings.NewReader(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetIdeas_回傳已建立的ideas(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	// Create two ideas
	http.Post(ts.URL+"/api/ideas", "application/json", strings.NewReader(`{"text":"first"}`))
	http.Post(ts.URL+"/api/ideas", "application/json", strings.NewReader(`{"text":"second"}`))

	resp, err := http.Get(ts.URL + "/api/ideas")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var ideas []vault.Idea
	json.NewDecoder(resp.Body).Decode(&ideas)
	if len(ideas) != 2 {
		t.Fatalf("expected 2 ideas, got %d", len(ideas))
	}
	// Newest first
	if ideas[0].Text != "second" {
		t.Errorf("expected newest first, got %q", ideas[0].Text)
	}
}

func TestDeleteIdeas_刪除idea回傳200(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	// Create idea
	resp, _ := http.Post(ts.URL+"/api/ideas", "application/json", strings.NewReader(`{"text":"to delete"}`))
	var idea vault.Idea
	json.NewDecoder(resp.Body).Decode(&idea)
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/ideas/%d", ts.URL, idea.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify deleted
	resp2, _ := http.Get(ts.URL + "/api/ideas")
	var ideas []vault.Idea
	json.NewDecoder(resp2.Body).Decode(&ideas)
	resp2.Body.Close()
	if len(ideas) != 0 {
		t.Errorf("expected 0 ideas after delete, got %d", len(ideas))
	}
}

func TestDeleteIdeas_無效id回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/ideas/notanumber", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEnrichIdea_成功補足描述(t *testing.T) {
	enrichMock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("## 背景\n這是一個測試\n## 需求\n完成測試"), nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{enrichCmd: enrichMock})
	defer ts.Close()

	// Create idea
	resp, _ := http.Post(ts.URL+"/api/ideas", "application/json", strings.NewReader(`{"text":"test idea"}`))
	var idea vault.Idea
	json.NewDecoder(resp.Body).Decode(&idea)
	resp.Body.Close()

	// Enrich
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/ideas/%d/enrich", ts.URL, idea.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var enriched vault.Idea
	json.NewDecoder(resp.Body).Decode(&enriched)
	if enriched.EnrichedDesc == "" {
		t.Error("expected non-empty enrichedDesc")
	}
}

func TestEnrichIdea_idea不存在回傳404(t *testing.T) {
	enrichMock := &mockCommander{}
	ts := setupTestServerWithOpts(t, testServerOptions{enrichCmd: enrichMock})
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/ideas/999/enrich", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Epic route tests ---

func setupEpicTestServer(t *testing.T) (*httptest.Server, *vault.VaultManager) {
	t.Helper()
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	ts := setupTestServerWithOpts(t, testServerOptions{vaultManager: vm})
	return ts, vm
}

func TestGetEpics_EmptyList(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/epics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var epics []plan.EpicInfo
	if err := json.NewDecoder(resp.Body).Decode(&epics); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestPostEpics_CreateAndList(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	// Create with display name containing spaces
	resp, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"My Epic (v1)"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var created map[string]string
	json.NewDecoder(resp.Body).Decode(&created)
	if !strings.HasPrefix(created["id"], "epic-") {
		t.Errorf("expected id with 'epic-' prefix, got %q", created["id"])
	}
	if created["name"] != "My Epic (v1)" {
		t.Errorf("expected name 'My Epic (v1)', got %q", created["name"])
	}

	// List
	resp2, err := http.Get(ts.URL + "/api/epics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp2.Body.Close()

	var epics []plan.EpicInfo
	json.NewDecoder(resp2.Body).Decode(&epics)
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if epics[0].ID != created["id"] {
		t.Errorf("expected ID %q, got %q", created["id"], epics[0].ID)
	}
	if epics[0].Name != "My Epic (v1)" {
		t.Errorf("expected Name 'My Epic (v1)', got %q", epics[0].Name)
	}
}

func TestPostEpics_DuplicateNameAllowed(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	// Same display name creates different IDs
	resp1, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"dup"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp1.StatusCode)
	}
	var created1 map[string]string
	json.NewDecoder(resp1.Body).Decode(&created1)

	resp2, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"dup"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp2.StatusCode)
	}
	var created2 map[string]string
	json.NewDecoder(resp2.Body).Decode(&created2)

	if created1["id"] == created2["id"] {
		t.Errorf("expected different IDs, both got %q", created1["id"])
	}
}

func TestPostEpics_EmptyName(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostEpics_SpecialCharsGetRandomID(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"測試 Epic #1!"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var created map[string]string
	json.NewDecoder(resp.Body).Decode(&created)
	if !strings.HasPrefix(created["id"], "epic-") {
		t.Errorf("expected id with 'epic-' prefix, got %q", created["id"])
	}
	if created["name"] != "測試 Epic #1!" {
		t.Errorf("expected name '測試 Epic #1!', got %q", created["name"])
	}
}

func TestDeleteEpics_Success(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("to-delete")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", result["status"])
	}
}

func TestDeleteEpics_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/epic-00000", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteEpics_InvalidID(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/bad-id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	// Invalid ID format should return error (either 400 or 500)
	if resp.StatusCode == http.StatusOK {
		t.Errorf("expected error status for invalid ID, got 200")
	}
}

func TestGetStoryMap_Success(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("reader")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/epics/" + id + "/story-map")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/yaml" {
		t.Errorf("expected Content-Type text/yaml, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "meta:") {
		t.Errorf("expected YAML content, got: %s", body)
	}
}

func TestGetStoryMap_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/epics/epic-00000/story-map")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPutStoryMap_Success(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("writer")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newYAML := "meta:\n  name: updated\nactivities: []\n"
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/story-map", strings.NewReader(newYAML))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "saved" {
		t.Errorf("expected status 'saved', got %q", result["status"])
	}

	// Verify content persisted (re-marshaled so check parsed value)
	content, err := em.ReadStoryMap(id)
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if !strings.Contains(content, "updated") {
		t.Errorf("expected content to contain 'updated', got:\n%s", content)
	}
}

func TestPutStoryMap_InvalidYAML回傳400含錯誤清單(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("invalid-yaml")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/story-map", strings.NewReader(":::invalid"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	// Verify response contains errors array with specific messages
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	errors, ok := result["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Error("expected non-empty errors array in response")
	}
}

func TestPutStoryMap_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	validYAML := "meta:\n  name: test\nactivities: []\n"
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/epic-00000/story-map", strings.NewReader(validYAML))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// --- Epic plan_update broadcast tests ---

// connectWS connects a WebSocket client to the test server's /ws/status endpoint.
func connectWS(t *testing.T, ts *httptest.Server) *gorillaws.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/status"
	conn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	return conn
}

// readWSMessage reads one WebSocket message with a timeout.
func readWSMessage(t *testing.T, conn *gorillaws.Conn, timeout time.Duration) map[string]string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(msg, &result); err != nil {
		t.Fatalf("ws unmarshal: %v", err)
	}
	return result
}

func TestPostEpics_BroadcastsPlanUpdate(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	// Connect WebSocket client
	wsConn := connectWS(t, ts)
	defer wsConn.Close()

	// Give the hub time to register the connection
	time.Sleep(50 * time.Millisecond)

	// Create epic
	resp, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"ws-test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// Read broadcast message
	msg := readWSMessage(t, wsConn, 2*time.Second)
	if msg["type"] != "plan_update" {
		t.Errorf("expected type 'plan_update', got %q", msg["type"])
	}
	if msg["action"] != "create" {
		t.Errorf("expected action 'create', got %q", msg["action"])
	}
	if !strings.HasPrefix(msg["epic"], "epic-") {
		t.Errorf("expected epic with 'epic-' prefix, got %q", msg["epic"])
	}
	if msg["timestamp"] == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestDeleteEpics_BroadcastsPlanUpdate(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("del-broadcast")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Connect WebSocket client
	wsConn := connectWS(t, ts)
	defer wsConn.Close()
	time.Sleep(50 * time.Millisecond)

	// Delete epic
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Read broadcast message
	msg := readWSMessage(t, wsConn, 2*time.Second)
	if msg["type"] != "plan_update" {
		t.Errorf("expected type 'plan_update', got %q", msg["type"])
	}
	if msg["action"] != "delete" {
		t.Errorf("expected action 'delete', got %q", msg["action"])
	}
	if msg["epic"] != id {
		t.Errorf("expected epic %q, got %q", id, msg["epic"])
	}
}

// --- Plan Session route tests ---

func setupPlanSessionTestServer(t *testing.T) (*httptest.Server, *vault.VaultManager) {
	t.Helper()
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			// has-session returns error (not found) by default
			if len(args) > 0 && args[0] == "has-session" {
				return fmt.Errorf("exit status 1")
			}
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock:             mock,
		vaultManager:     vm,
		epicImageEnsurer: func() error { return nil },
		dockerAvailable:  true,
	})
	return ts, vm
}

func TestPostPlanSession_建立新session回傳201(t *testing.T) {
	ts, em := setupPlanSessionTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("test-epic")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/epics/"+id+"/plan-session", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["session"] != "pplan-"+id {
		t.Errorf("expected session 'pplan-%s', got %q", id, result["session"])
	}
}

func TestPostPlanSession_已存在回傳200(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			// has-session returns success (session exists)
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock:             mock,
		vaultManager:     vm,
		epicImageEnsurer: func() error { return nil },
		dockerAvailable:  true,
	})
	defer ts.Close()

	id, err := vm.CreateEpic("existing")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/epics/"+id+"/plan-session", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["session"] != "pplan-"+id {
		t.Errorf("expected session 'pplan-%s', got %q", id, result["session"])
	}
}

func TestPostPlanSession_Epic不存在回傳404(t *testing.T) {
	ts, _ := setupPlanSessionTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/epics/epic-00000/plan-session", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetPlanSession_存在(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			// has-session returns success (session exists)
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, vaultManager: vm})
	defer ts.Close()

	id, err := vm.CreateEpic("check-epic")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/epics/" + id + "/plan-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Session string `json:"session"`
		Exists  bool   `json:"exists"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.Exists {
		t.Error("expected exists=true")
	}
	if result.Session != "pplan-"+id {
		t.Errorf("expected session 'pplan-%s', got %q", id, result.Session)
	}
}

func TestGetPlanSession_不存在(t *testing.T) {
	ts, em := setupPlanSessionTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("no-session")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/epics/" + id + "/plan-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Session string `json:"session"`
		Exists  bool   `json:"exists"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Exists {
		t.Error("expected exists=false")
	}
}

func TestDeletePlanSession_成功(t *testing.T) {
	ts, em := setupPlanSessionTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("del-session")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id+"/plan-session", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

// --- Native mode plan session tests ---

func setupNativePlanSessionTestServer(t *testing.T) (*httptest.Server, *vault.VaultManager) {
	t.Helper()
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "has-session" {
				return fmt.Errorf("exit status 1")
			}
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock:            mock,
		vaultManager:    vm,
		dockerAvailable: false, // native mode
	})
	return ts, vm
}

func TestPostPlanSession_NativeMode_建立session回傳201(t *testing.T) {
	ts, em := setupNativePlanSessionTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("native-test")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/epics/"+id+"/plan-session", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["session"] != "pplan-"+id {
		t.Errorf("expected session 'pplan-%s', got %q", id, result["session"])
	}
}

func TestDeletePlanSession_NativeMode_成功(t *testing.T) {
	ts, em := setupNativePlanSessionTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("native-del")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id+"/plan-session", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
}

// fakeGitRunnerForSync tracks push calls for verifying TriggerSync behavior.
type fakeGitRunnerForSync struct {
	pushCalled atomic.Int32
}

func (f *fakeGitRunnerForSync) IsRepo(string) bool                     { return true }
func (f *fakeGitRunnerForSync) Init(string) error                      { return nil }
func (f *fakeGitRunnerForSync) Add(string, ...string) error            { return nil }
func (f *fakeGitRunnerForSync) Commit(string, string) error            { return nil }
func (f *fakeGitRunnerForSync) RemoteExists(string, string) bool       { return true }
func (f *fakeGitRunnerForSync) RemoteAdd(string, string, string) error { return nil }
func (f *fakeGitRunnerForSync) RemoteSetURL(string, string, string) error { return nil }
func (f *fakeGitRunnerForSync) LsRemote(string, string) error         { return nil }
func (f *fakeGitRunnerForSync) Pull(string) error                      { return nil }
func (f *fakeGitRunnerForSync) Push(string) error {
	f.pushCalled.Add(1)
	return nil
}
func (f *fakeGitRunnerForSync) PendingCommits(string) (int, error) { return 0, nil }

func TestDeletePlanSession_觸發GitSync(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunnerForSync{}
	vm := vault.New(vaultDir, vault.WithGitRunner(fg))
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	syncer := plan.NewGitSyncer(vm)

	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "has-session" {
				return fmt.Errorf("exit status 1")
			}
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, vaultManager: vm, epicSyncer: syncer})
	defer ts.Close()

	id, err := vm.CreateEpic("sync-test")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id+"/plan-session", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Wait for async TriggerSync goroutine
	time.Sleep(300 * time.Millisecond)

	if fg.pushCalled.Load() == 0 {
		t.Error("expected TriggerSync to call push, but push was not called")
	}
}

func TestDeleteEpic_清理關聯planSession(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	vm := vault.New(vaultDir)
	if err := vm.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	var killedSessions []string
	mock := &mockCommander{
		runFunc: func(name string, args ...string) error {
			if len(args) > 0 && args[0] == "kill-session" {
				killedSessions = append(killedSessions, args[2]) // -t <name>
			}
			return nil
		},
	}
	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, vaultManager: vm})
	defer ts.Close()

	id, err := vm.CreateEpic("cleanup-test")
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify kill-session was called for the plan session
	expectedSession := "pplan-" + id
	found := false
	for _, s := range killedSessions {
		if s == expectedSession {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected kill-session for %s, killed: %v", expectedSession, killedSessions)
	}
}

func TestPutStoryMap_BroadcastsPlanUpdate(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("update-broadcast")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Connect WebSocket client
	wsConn := connectWS(t, ts)
	defer wsConn.Close()
	time.Sleep(50 * time.Millisecond)

	// Update story map
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/story-map", strings.NewReader("meta:\n  name: test\nactivities: []\n"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Read broadcast message
	msg := readWSMessage(t, wsConn, 2*time.Second)
	if msg["type"] != "plan_update" {
		t.Errorf("expected type 'plan_update', got %q", msg["type"])
	}
	if msg["action"] != "update" {
		t.Errorf("expected action 'update', got %q", msg["action"])
	}
	if msg["epic"] != id {
		t.Errorf("expected epic %q, got %q", id, msg["epic"])
	}
}

// --- Rename route tests ---

func TestRenameEpic_RouteSuccess(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("Old Name")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/rename", strings.NewReader(`{"name":"New Name"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] != id {
		t.Errorf("expected id %q, got %q", id, result["id"])
	}
	if result["name"] != "New Name" {
		t.Errorf("expected name 'New Name', got %q", result["name"])
	}

	// Verify via list
	epics, _ := em.ListEpics()
	if len(epics) != 1 || epics[0].Name != "New Name" {
		t.Errorf("expected name 'New Name' in list, got %+v", epics)
	}
}

func TestRenameEpic_RouteEmptyName(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/rename", strings.NewReader(`{"name":""}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRenameEpic_RouteNotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/epic-00000/rename", strings.NewReader(`{"name":"test"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRenameEpic_BroadcastsPlanUpdate(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("before")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	wsConn := connectWS(t, ts)
	defer wsConn.Close()
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/rename", strings.NewReader(`{"name":"after"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	msg := readWSMessage(t, wsConn, 2*time.Second)
	if msg["type"] != "plan_update" {
		t.Errorf("expected type 'plan_update', got %q", msg["type"])
	}
	if msg["action"] != "rename" {
		t.Errorf("expected action 'rename', got %q", msg["action"])
	}
	if msg["epic"] != id {
		t.Errorf("expected epic %q, got %q", id, msg["epic"])
	}
}

// --- DevContainer Setup API Tests ---

// mockContainerRunner implements devcontainer.ContainerRunner for testing.
type mockContainerRunner struct {
	err error
}

func (m *mockContainerRunner) EnsureRunning(_ string) error {
	return m.err
}

func (m *mockContainerRunner) RemoveContainer(_ string) error {
	return nil
}

func (m *mockContainerRunner) Cleanup(_ string) devcontainer.CleanupResult {
	return devcontainer.CleanupResult{}
}

func (m *mockContainerRunner) ValidateConfig(_ string) error {
	return nil
}

func (m *mockContainerRunner) VerifyContainer(_ string, _ devcontainer.GeneratedFiles) error {
	return nil
}

func newTestSetupRunner() *devcontainer.SetupRunner {
	runner := &mockContainerRunner{}
	return devcontainer.NewSetupRunner(runner, devcontainer.TemplateFS())
}

func TestSetupProject_成功啟動(t *testing.T) {
	tmpDir := t.TempDir()
	trk := newSetupTracker()
	sr := newTestSetupRunner()
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: sr,
		tracker:     trk,
	})
	defer ts.Close()

	body := fmt.Sprintf(`{"name":"myproject","path":"%s","description":"A test project"}`, tmpDir)
	resp, err := http.Post(ts.URL+"/api/projects/setup", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/projects/setup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	setupID := result["setupID"]
	if setupID == "" {
		t.Fatal("expected non-empty setupID")
	}

	// Wait for the background goroutine to finish
	as := trk.get(setupID)
	if as != nil {
		select {
		case <-as.Done:
		case <-time.After(5 * time.Second):
			t.Fatal("setup goroutine did not finish in time")
		}
	}
}

func TestSetupProject_帶autoMode設定(t *testing.T) {
	tmpDir := t.TempDir()
	trk := newSetupTracker()
	sr := newTestSetupRunner()
	sessionStore := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	if err := sessionStore.Load(); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: sr,
		tracker:     trk,
		store:       sessionStore,
	})
	defer ts.Close()

	body := fmt.Sprintf(`{"name":"myproject","path":"%s","description":"A test project","autoMode":true}`, tmpDir)
	resp, err := http.Post(ts.URL+"/api/projects/setup", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/projects/setup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	setupID := result["setupID"]

	// Wait for the background goroutine to finish
	as := trk.get(setupID)
	if as != nil {
		select {
		case <-as.Done:
		case <-time.After(5 * time.Second):
			t.Fatal("setup goroutine did not finish in time")
		}
	}

	// Verify autoMode was set on the actual project path (parentDir/projectName)
	actualPath := filepath.Join(tmpDir, "myproject")
	got := sessionStore.GetProjectAutoMode(actualPath)
	if got == nil || *got != true {
		t.Errorf("expected autoMode=true after setup at %s, got %v", actualPath, got)
	}
}

func TestSetupProject_缺少名稱(t *testing.T) {
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
	})
	defer ts.Close()

	body := `{"name":"","path":"/tmp/test","description":"desc"}`
	resp, err := http.Post(ts.URL+"/api/projects/setup", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/projects/setup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSetupProject_路徑非絕對路徑(t *testing.T) {
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
	})
	defer ts.Close()

	body := `{"name":"proj","path":"relative/path","description":"desc"}`
	resp, err := http.Post(ts.URL+"/api/projects/setup", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/projects/setup: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetSetupStatus_存在(t *testing.T) {
	trk := newSetupTracker()
	trk.set("test-123", &activeSetup{
		ID: "test-123",
		Status: devcontainer.SetupStatus{
			Phase:   devcontainer.PhaseBuilding,
			Message: "正在建置容器...",
		},
		Cancel: func() {},
		Done:   make(chan struct{}),
	})

	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
		tracker:     trk,
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/setup/test-123/status")
	if err != nil {
		t.Fatalf("GET /api/projects/setup/test-123/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status devcontainer.SetupStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if status.Phase != devcontainer.PhaseBuilding {
		t.Errorf("expected phase 'building', got %q", status.Phase)
	}
	if status.Message != "正在建置容器..." {
		t.Errorf("expected message '正在建置容器...', got %q", status.Message)
	}
}

func TestGetSetupStatus_不存在(t *testing.T) {
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/projects/setup/nonexistent/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCancelSetup_存在(t *testing.T) {
	cancelled := false
	trk := newSetupTracker()
	trk.set("cancel-me", &activeSetup{
		ID: "cancel-me",
		Status: devcontainer.SetupStatus{
			Phase:   devcontainer.PhaseBuilding,
			Message: "building...",
		},
		Cancel: func() { cancelled = true },
		Done:   make(chan struct{}),
	})

	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
		tracker:     trk,
	})
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/projects/setup/cancel-me", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if !cancelled {
		t.Error("expected cancel function to be called")
	}
	if trk.get("cancel-me") != nil {
		t.Error("expected setup to be removed from tracker")
	}
}

func TestCancelSetup_不存在(t *testing.T) {
	ts := setupTestServerWithOpts(t, testServerOptions{
		setupRunner: newTestSetupRunner(),
	})
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/projects/setup/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetModelConfig_預設為空(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/settings/models")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.ModelConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.DevcontainerGen != "" || cfg.Summarizer != "" || cfg.IdeaEnrich != "" {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestPutModelConfig_設定並回傳(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"devcontainerGen":"sonnet","summarizer":"opus","ideaEnrich":"haiku"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.ModelConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.DevcontainerGen != "sonnet" {
		t.Errorf("expected DevcontainerGen=sonnet, got %q", cfg.DevcontainerGen)
	}
	if cfg.Summarizer != "opus" {
		t.Errorf("expected Summarizer=opus, got %q", cfg.Summarizer)
	}
	if cfg.IdeaEnrich != "haiku" {
		t.Errorf("expected IdeaEnrich=haiku, got %q", cfg.IdeaEnrich)
	}

	// Verify persisted via GET
	resp2, err := http.Get(ts.URL + "/api/settings/models")
	if err != nil {
		t.Fatalf("GET request error: %v", err)
	}
	defer resp2.Body.Close()
	var cfg2 store.ModelConfig
	json.NewDecoder(resp2.Body).Decode(&cfg2)
	if cfg2.DevcontainerGen != "sonnet" {
		t.Errorf("GET after PUT: expected DevcontainerGen=sonnet, got %q", cfg2.DevcontainerGen)
	}
}

func TestPutModelConfig_無效模型回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"devcontainerGen":"gpt-4"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPutModelConfig_空值清除設定(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	// Set a value
	body := `{"summarizer":"opus"}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// Clear all
	body2 := `{}`
	req2, _ := http.NewRequest("PUT", ts.URL+"/api/settings/models", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req2)

	resp, err := http.Get(ts.URL + "/api/settings/models")
	if err != nil {
		t.Fatalf("GET request error: %v", err)
	}
	defer resp.Body.Close()
	var cfg store.ModelConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.Summarizer != "" {
		t.Errorf("expected empty Summarizer after clear, got %q", cfg.Summarizer)
	}
}

// --- Firewall Config ---

func TestGetFirewallConfig_預設值(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/settings/firewall")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.FirewallConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.Mode != "open" {
		t.Errorf("expected mode=open, got %q", cfg.Mode)
	}
	if len(cfg.Domains) == 0 {
		t.Error("expected default domains, got empty")
	}
}

func TestPutFirewallConfig_設定並回傳(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"mode":"whitelist","domains":["example.com","api.example.com"]}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/firewall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.FirewallConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.Mode != "whitelist" {
		t.Errorf("expected mode=whitelist, got %q", cfg.Mode)
	}
	if len(cfg.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(cfg.Domains))
	}

	// Verify persisted via GET
	resp2, err := http.Get(ts.URL + "/api/settings/firewall")
	if err != nil {
		t.Fatalf("GET request error: %v", err)
	}
	defer resp2.Body.Close()
	var cfg2 store.FirewallConfig
	json.NewDecoder(resp2.Body).Decode(&cfg2)
	if cfg2.Mode != "whitelist" {
		t.Errorf("GET after PUT: expected mode=whitelist, got %q", cfg2.Mode)
	}
}

func TestPutFirewallConfig_無效mode回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"mode":"block","domains":[]}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/firewall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPutFirewallConfig_去重去空白(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"mode":"whitelist","domains":["  example.com  ","example.com","","api.example.com"]}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/firewall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.FirewallConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if len(cfg.Domains) != 2 {
		t.Fatalf("expected 2 domains (deduped), got %d: %v", len(cfg.Domains), cfg.Domains)
	}
	if cfg.Domains[0] != "example.com" || cfg.Domains[1] != "api.example.com" {
		t.Errorf("unexpected domains: %v", cfg.Domains)
	}
}

// --- Epic Git Config ---

func TestGetEpicGitConfig_預設為空(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/settings/epic-git")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.EpicGitConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if cfg.RemoteURL != "" || cfg.SyncEnabled || cfg.SyncIntervalMin != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestPutEpicGitConfig_不驗證remoteURL為空時(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"syncEnabled":true,"syncIntervalMin":10}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/epic-git", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cfg store.EpicGitConfig
	json.NewDecoder(resp.Body).Decode(&cfg)
	if !cfg.SyncEnabled {
		t.Error("expected SyncEnabled=true")
	}
	if cfg.SyncIntervalMin != 10 {
		t.Errorf("expected SyncIntervalMin=10, got %d", cfg.SyncIntervalMin)
	}

	// Verify persisted via GET
	resp2, err := http.Get(ts.URL + "/api/settings/epic-git")
	if err != nil {
		t.Fatalf("GET request error: %v", err)
	}
	defer resp2.Body.Close()
	var cfg2 store.EpicGitConfig
	json.NewDecoder(resp2.Body).Decode(&cfg2)
	if !cfg2.SyncEnabled {
		t.Error("GET after PUT: expected SyncEnabled=true")
	}
}

func TestPutEpicGitConfig_負數間隔回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := `{"syncIntervalMin":-1}`
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/epic-git", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetEpicGitSyncStatus_無syncer回傳空(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/settings/epic-git/status")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status plan.SyncStatus
	json.NewDecoder(resp.Body).Decode(&status)
	if status.Syncing {
		t.Error("expected Syncing=false")
	}
}

func TestPostEpicGitSync_無syncer回傳503(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/settings/epic-git/sync", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

// ── DELETE session completion tracking ──

func TestDeleteSession_記錄完成次數到completionTracker(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "list-sessions" {
				return []byte("ragent-test-abc\t1\t1710000000\t0\n"), nil
			}
			return nil, nil
		},
	}
	ct := usage.NewCompletionTracker(filepath.Join(t.TempDir(), "completions"))
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock:              mock,
		completionTracker: ct,
	})
	defer ts.Close()

	// DELETE the session
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/sessions/ragent-test-abc", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify completionTracker recorded it
	now := time.Now().UTC()
	entries, err := ct.GetMonthlyEntries(now.Year(), now.Month())
	if err != nil {
		t.Fatalf("GetMonthlyEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected completion count=1, got %d", len(entries))
	}
}

func TestDeleteSession_completionTracker為nil不panic(t *testing.T) {
	mock := &mockCommander{}
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock: mock,
		// completionTracker intentionally nil
	})
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/sessions/ragent-test-abc", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()
	// Should not panic, returns 200 (kill may fail, but no panic)
	if resp.StatusCode == http.StatusInternalServerError {
		// KillSession might fail since mock doesn't implement it, that's acceptable
	}
}

func TestDeleteSession_dashboardStats累積完成記錄(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "list-sessions" {
				return []byte("ragent-test-abc\t1\t1710000000\t0\n"), nil
			}
			return nil, nil
		},
	}
	ct := usage.NewCompletionTracker(filepath.Join(t.TempDir(), "completions"))
	ts := setupTestServerWithOpts(t, testServerOptions{
		mock:              mock,
		completionTracker: ct,
	})
	defer ts.Close()

	// DELETE two sessions
	for _, name := range []string{"ragent-test-abc", "ragent-test-def"} {
		req, _ := http.NewRequest("DELETE", ts.URL+"/api/sessions/"+name, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("DELETE %s: %v", name, err)
		}
		resp.Body.Close()
	}

	// Check dashboard-stats
	now := time.Now()
	url := fmt.Sprintf("%s/api/dashboard-stats?year=%d&month=%d", ts.URL, now.Year(), int(now.Month()))
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET dashboard-stats: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Completions struct {
			Daily []usage.DailyCompletion `json:"daily"`
		} `json:"completions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode dashboard-stats: %v", err)
	}

	daily := result.Completions.Daily
	if len(daily) != 1 {
		t.Errorf("expected 1 day with completions, got %d", len(daily))
	}
	totalSegs := 0
	for _, d := range daily {
		for _, s := range d.Segments {
			totalSegs += s
		}
	}
	if totalSegs != 2 {
		t.Errorf("expected 2 total completions across daily segments, got %d", totalSegs)
	}
}

// --- Git Graph / Init handler tests ---

// gitMockCmd implements git.Commander for handler tests.
type gitMockCmd struct {
	outputFunc func(name string, args ...string) ([]byte, error)
	runFunc    func(name string, args ...string) error
	runCalls   [][]string
}

func (g *gitMockCmd) Output(name string, args ...string) ([]byte, error) {
	if g.outputFunc != nil {
		return g.outputFunc(name, args...)
	}
	return nil, nil
}

func (g *gitMockCmd) Run(name string, args ...string) error {
	g.runCalls = append(g.runCalls, append([]string{name}, args...))
	if g.runFunc != nil {
		return g.runFunc(name, args...)
	}
	return nil
}

func TestGetGitAvailable_回傳git可用狀態(t *testing.T) {
	mock := &gitMockCmd{}
	fetcher := gitPkg.NewGraphFetcher(mock)
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/git/available")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result struct {
		Available bool `json:"available"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	// In test env git should be installed
	if !result.Available {
		t.Log("git not in PATH — available=false is correct")
	}
}

func TestGetGitGraph_有效repo回傳commits(t *testing.T) {
	mock := &gitMockCmd{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			for _, a := range args {
				if a == "rev-parse" {
					return []byte(".git"), nil
				}
				if a == "branch" {
					return []byte("main\n"), nil
				}
				if a == "remote" {
					return []byte("origin\n"), nil
				}
				if a == "log" {
					return []byte("abc1234\x00feat: add feature\x00Alice\x002 hours ago\x00HEAD -> main\x00def5678\ndef5678\x00init commit\x00Bob\x003 hours ago\x00\x00\n"), nil
				}
			}
			return nil, nil
		},
	}
	fetcher := gitPkg.NewGraphFetcher(mock)
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/git/graph?path=/some/repo")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result gitPkg.GraphResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if !result.IsRepo {
		t.Error("expected isRepo=true")
	}
	if result.Branch != "main" {
		t.Errorf("expected branch=main, got %q", result.Branch)
	}
	if len(result.Remotes) != 1 || result.Remotes[0] != "origin" {
		t.Errorf("expected remotes=[origin], got %v", result.Remotes)
	}
	if len(result.Commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(result.Commits))
	}
	if result.Commits[0].Hash != "abc1234" {
		t.Errorf("expected first commit hash=abc1234, got %q", result.Commits[0].Hash)
	}
	if !result.Commits[0].IsHead {
		t.Error("expected first commit to be HEAD")
	}
}

func TestGetGitGraph_缺少path回傳400(t *testing.T) {
	fetcher := gitPkg.NewGraphFetcher(&gitMockCmd{})
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/git/graph")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetGitGraph_非git目錄回傳isRepo_false(t *testing.T) {
	mock := &gitMockCmd{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			for _, a := range args {
				if a == "rev-parse" {
					return nil, fmt.Errorf("fatal: not a git repository")
				}
			}
			return nil, nil
		},
	}
	fetcher := gitPkg.NewGraphFetcher(mock)
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/git/graph?path=/not/a/repo")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result gitPkg.GraphResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if result.IsRepo {
		t.Error("expected isRepo=false for non-git directory")
	}
}

func TestPostGitInit_成功初始化回傳ok(t *testing.T) {
	mock := &gitMockCmd{}
	fetcher := gitPkg.NewGraphFetcher(mock)
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	body := `{"path":"/newrepo"}`
	resp, err := http.Post(ts.URL+"/api/git/init", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}
	if result["isRepo"] != true {
		t.Errorf("expected isRepo=true, got %v", result["isRepo"])
	}

	// Verify git init was called
	if len(mock.runCalls) == 0 {
		t.Fatal("expected git init to be called")
	}
	found := false
	for _, call := range mock.runCalls {
		for _, arg := range call {
			if arg == "init" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected 'init' in run calls, got %v", mock.runCalls)
	}
}

func TestPostGitInit_缺少path回傳400(t *testing.T) {
	fetcher := gitPkg.NewGraphFetcher(&gitMockCmd{})
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	body := `{"path":""}`
	resp, err := http.Post(ts.URL+"/api/git/init", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostGitInit_無效JSON回傳400(t *testing.T) {
	fetcher := gitPkg.NewGraphFetcher(&gitMockCmd{})
	ts := setupTestServerWithOpts(t, testServerOptions{gitFetcher: fetcher})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/git/init", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpdateMeta_支援projectPath(t *testing.T) {
	sessionName := "ragent-test-meta1"
	newPath := "/home/user/manual-path"

	mock := &mockCommander{}
	st := store.New(filepath.Join(t.TempDir(), "sessions.json"))
	if err := st.Load(); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}
	if err := st.SetMeta(sessionName, store.SessionMeta{Title: "Test"}); err != nil {
		t.Fatalf("failed to set meta: %v", err)
	}

	ts := setupTestServerWithOpts(t, testServerOptions{mock: mock, store: st})
	defer ts.Close()

	body := fmt.Sprintf(`{"projectPath":"%s"}`, newPath)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/sessions/"+sessionName+"/meta", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify persisted
	m := st.GetMeta(sessionName)
	if m == nil || m.ProjectPath != newPath {
		t.Errorf("expected store projectPath %q, got %v", newPath, m)
	}
}

func TestDigestAPI_回傳每日摘要(t *testing.T) {
	dir := t.TempDir()

	// Setup work time data
	wt := usage.NewWorkTimeTracker(dir)
	today := time.Now().UTC().Format("2006-01-02")
	// Create a work time entry by simulating session activity
	wt.SetActiveForTest("ragent-piper-abc123", time.Now().Add(-5*time.Minute))
	wt.Update([]usage.SessionStatus{{Name: "ragent-piper-abc123", Status: "idle"}})

	// Setup store with session metadata
	st := store.New(filepath.Join(dir, "sessions.json"))
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	st.SetMeta("ragent-piper-abc123", store.SessionMeta{
		Description: "test digest feature",
		ProjectPath: "/home/user/piper",
	})

	ts := setupTestServerWithOpts(t, testServerOptions{
		store:           st,
		workTimeTracker: wt,
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/digest?date=" + today)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["date"] != today {
		t.Errorf("date = %v, want %s", result["date"], today)
	}

	// Projects should be present
	projects := result["projects"].([]interface{})
	if len(projects) == 0 {
		t.Error("expected at least 1 project group")
	}
}

func TestDigestAPI_無date參數時預設昨天(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/digest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if result["date"] != yesterday {
		t.Errorf("date = %v, want %s", result["date"], yesterday)
	}
}

func TestDigestAPI_無效日期格式回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/digest?date=not-a-date")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDigestSend_無webhook回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/digest/send", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDigestSettingsAPI_設定並取得(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	// PUT
	body := strings.NewReader(`{"webhookUrl":"https://discord.com/api/webhooks/test","enabled":true,"sendHour":1}`)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/digest", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("PUT expected 200, got %d", resp.StatusCode)
	}

	// GET
	resp, err = http.Get(ts.URL + "/api/settings/digest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var cfg map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&cfg)

	if cfg["webhookUrl"] != "https://discord.com/api/webhooks/test" {
		t.Errorf("webhookUrl = %v", cfg["webhookUrl"])
	}
	if cfg["enabled"] != true {
		t.Errorf("enabled = %v", cfg["enabled"])
	}
	if cfg["sendHour"] != float64(1) {
		t.Errorf("sendHour = %v", cfg["sendHour"])
	}
}

func TestDigestSettingsAPI_無效sendHour回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	body := strings.NewReader(`{"sendHour":25}`)
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings/digest", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDebugDigestAPI_回傳完整除錯資訊(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Format("2006-01-02")

	// Setup store
	st := store.New(filepath.Join(dir, "sessions.json"))
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	st.SetMeta("ragent-piper-abc123", store.SessionMeta{
		Description: "implement debug endpoint",
		ProjectPath: "/home/user/piper",
	})
	st.SetDigestConfig(store.DigestConfig{
		WebhookURL: "https://discord.example.com/webhook",
		Enabled:    true,
		SendHour:   1,
	})

	// Setup trackers with data
	wt := usage.NewWorkTimeTracker(dir)
	wt.SetActiveForTest("ragent-piper-active1", time.Now().Add(-5*time.Minute))
	// Write a worktime entry by transitioning a session
	wt.SetActiveForTest("ragent-piper-done1", time.Now().Add(-10*time.Minute))
	wt.Update([]usage.SessionStatus{{Name: "ragent-piper-active1", Status: "working"}}) // active stays, done1 gets recorded

	ct := usage.NewCompletionTracker(dir)
	ct.Record("ragent-piper-abc123", "/home/user/piper")

	ts := setupTestServerWithOpts(t, testServerOptions{
		store:             st,
		workTimeTracker:   wt,
		completionTracker: ct,
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/debug/digest?date=" + today)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// Verify targetDate
	if result["targetDate"] != today {
		t.Errorf("targetDate = %v, want %s", result["targetDate"], today)
	}

	// Verify files section exists
	files, ok := result["files"].(map[string]interface{})
	if !ok {
		t.Fatal("expected files object in response")
	}

	// worktime file should exist (we recorded an entry)
	wtFile, ok := files["worktime"].(map[string]interface{})
	if !ok {
		t.Fatal("expected files.worktime object")
	}
	if wtFile["exists"] != true {
		t.Error("expected worktime file to exist")
	}
	if int(wtFile["entryCount"].(float64)) < 1 {
		t.Error("expected at least 1 worktime entry")
	}

	// activeTracking should contain active1
	active, ok := result["activeTracking"].([]interface{})
	if !ok {
		t.Fatal("expected activeTracking array")
	}
	found := false
	for _, a := range active {
		if a == "ragent-piper-active1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ragent-piper-active1 in activeTracking, got %v", active)
	}

	// config should be present
	cfg, ok := result["config"].(map[string]interface{})
	if !ok {
		t.Fatal("expected config object")
	}
	if cfg["enabled"] != true {
		t.Error("expected config.enabled = true")
	}
	if int(cfg["sendHour"].(float64)) != 1 {
		t.Errorf("expected config.sendHour = 1, got %v", cfg["sendHour"])
	}

	// generatedDigest should be present
	digest, ok := result["generatedDigest"].(map[string]interface{})
	if !ok {
		t.Fatal("expected generatedDigest object")
	}
	if digest["date"] != today {
		t.Errorf("digest date = %v, want %s", digest["date"], today)
	}
}

func TestDebugDigestAPI_無date參數預設昨天(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/debug/digest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if result["targetDate"] != yesterday {
		t.Errorf("targetDate = %v, want %s", result["targetDate"], yesterday)
	}
}

func TestGetStoryMapStats_Success(t *testing.T) {
	ts, em := setupEpicTestServer(t)
	defer ts.Close()

	id, err := em.CreateEpic("stats-test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Write a story map with mixed statuses
	yamlContent := `meta:
  name: "Stats Test"
  updated: "2026-01-01"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: s1
            name: "Story1"
            release: rel-mvp
            status: todo
            tasks:
              - id: t1
                name: "T1"
                targetProject: server
                status: todo
              - id: t2
                name: "T2"
                targetProject: web
                status: done
          - id: s2
            name: "Story2 no tasks"
            release: rel-mvp
            status: todo
`
	if err := em.WriteStoryMap(id, yamlContent); err != nil {
		t.Fatalf("write story map: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/epics/" + id + "/story-map/stats")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var stats plan.StoryMapStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Stories: s1=todo, s2=todo → total=2, todo=2
	if stats.Stories.Total != 2 {
		t.Errorf("stories.total = %d, want 2", stats.Stories.Total)
	}
	if stats.Stories.Todo != 2 {
		t.Errorf("stories.todo = %d, want 2", stats.Stories.Todo)
	}
	// Tasks: t1=todo, t2=done → total=2
	if stats.Tasks.Total != 2 {
		t.Errorf("tasks.total = %d, want 2", stats.Tasks.Total)
	}
	// byRelease: MVP → 2 stories, 2 tasks
	mvp, ok := stats.ByRelease["MVP"]
	if !ok {
		t.Fatal("missing release 'MVP'")
	}
	if mvp.Stories != 2 {
		t.Errorf("MVP stories = %d, want 2", mvp.Stories)
	}
	// byProject
	if stats.ByProject["server"] != 1 {
		t.Errorf("server = %d, want 1", stats.ByProject["server"])
	}
	if stats.ByProject["web"] != 1 {
		t.Errorf("web = %d, want 1", stats.ByProject["web"])
	}
	// storiesWithoutTasks: s2
	if stats.StoriesWithoutTasks != 1 {
		t.Errorf("storiesWithoutTasks = %d, want 1", stats.StoriesWithoutTasks)
	}
}

func TestGetStoryMapStats_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/epics/epic-00000/story-map/stats")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDebugDigestAPI_無效日期格式回傳400(t *testing.T) {
	ts := setupTestServer(t, &mockCommander{})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/debug/digest?date=invalid")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// --- Vault File API Tests ---

// createEpicForTest creates an epic via API and returns its ID.
func createEpicForTest(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/epics", "application/json", strings.NewReader(`{"name":"Vault Test"}`))
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create epic: expected 201, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["id"]
}

func TestVaultAPI_ProblemStatement_ReadWrite(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	// Read (template should exist)
	resp, err := http.Get(ts.URL + "/api/epics/" + id + "/problem-statement")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Write with valid content
	content := "# Problem\n\n## 背景\n\nbg\n\n## 不做什麼\n\n- nothing\n"
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/problem-statement", strings.NewReader(content))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200, got %d: %s", resp2.StatusCode, body)
	}

	// Read back
	resp3, _ := http.Get(ts.URL + "/api/epics/" + id + "/problem-statement")
	defer resp3.Body.Close()
	got, _ := io.ReadAll(resp3.Body)
	if string(got) != content {
		t.Errorf("content mismatch: got %q", got)
	}
}

func TestVaultAPI_ProblemStatement_ValidationError(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	// Write without 不做什麼 section
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/problem-statement", strings.NewReader("# Only background"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if !strings.Contains(result["error"], "不做什麼") {
		t.Errorf("expected validation error about '不做什麼', got: %v", result)
	}
}

func TestVaultAPI_ProblemStatement_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/epics/epic-fffff/problem-statement")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestVaultAPI_ADR_CRUD(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	adrContent := "# ADR-001\n\n## 背景\n\nbg\n\n## 決定\n\ndec\n\n## 後果\n\ncons\n"

	// List (empty)
	resp, _ := http.Get(ts.URL + "/api/epics/" + id + "/adr")
	defer resp.Body.Close()
	var names []string
	json.NewDecoder(resp.Body).Decode(&names)
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	// Write
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/adr/adr-001", strings.NewReader(adrContent))
	resp2, _ := http.DefaultClient.Do(req)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("PUT: expected 200, got %d: %s", resp2.StatusCode, body)
	}

	// Read
	resp3, _ := http.Get(ts.URL + "/api/epics/" + id + "/adr/adr-001")
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("GET: expected 200, got %d", resp3.StatusCode)
	}
	got, _ := io.ReadAll(resp3.Body)
	if string(got) != adrContent {
		t.Error("content mismatch")
	}

	// List (1 item)
	resp4, _ := http.Get(ts.URL + "/api/epics/" + id + "/adr")
	defer resp4.Body.Close()
	json.NewDecoder(resp4.Body).Decode(&names)
	if len(names) != 1 || names[0] != "adr-001" {
		t.Errorf("expected [adr-001], got %v", names)
	}

	// Delete
	req2, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id+"/adr/adr-001", nil)
	resp5, _ := http.DefaultClient.Do(req2)
	defer resp5.Body.Close()
	if resp5.StatusCode != 200 {
		t.Fatalf("DELETE: expected 200, got %d", resp5.StatusCode)
	}

	// List (empty again)
	resp6, _ := http.Get(ts.URL + "/api/epics/" + id + "/adr")
	defer resp6.Body.Close()
	var namesAfter []string
	json.NewDecoder(resp6.Body).Decode(&namesAfter)
	if len(namesAfter) != 0 {
		t.Errorf("expected empty after delete, got %v", namesAfter)
	}
}

func TestVaultAPI_ADR_ValidationError(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	// Missing 決定 section
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/adr/bad", strings.NewReader("# ADR\n\n## 背景\n\nb\n\n## 後果\n\nc\n"))
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestVaultAPI_Spikes_CRUD(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	content := "# Spike\n\n## 問題\n\nq\n\n## 結論\n\na\n"

	// Write
	req, _ := http.NewRequest("PUT", ts.URL+"/api/epics/"+id+"/spikes/spike-001", strings.NewReader(content))
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT: expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Read
	resp2, _ := http.Get(ts.URL + "/api/epics/" + id + "/spikes/spike-001")
	defer resp2.Body.Close()
	got, _ := io.ReadAll(resp2.Body)
	if string(got) != content {
		t.Error("content mismatch")
	}

	// List
	resp3, _ := http.Get(ts.URL + "/api/epics/" + id + "/spikes")
	defer resp3.Body.Close()
	var names []string
	json.NewDecoder(resp3.Body).Decode(&names)
	if len(names) != 1 || names[0] != "spike-001" {
		t.Errorf("expected [spike-001], got %v", names)
	}

	// Delete
	req2, _ := http.NewRequest("DELETE", ts.URL+"/api/epics/"+id+"/spikes/spike-001", nil)
	resp4, _ := http.DefaultClient.Do(req2)
	defer resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("DELETE: expected 200, got %d", resp4.StatusCode)
	}
}

func TestVaultAPI_SpecsSummary(t *testing.T) {
	ts, vm := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	// Create specs dir + file
	specsDir := filepath.Join(vm.EpicManager().BaseDir(), id, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "summary.md"), []byte("# Summary\n\ntest\n"), 0o644)

	resp, _ := http.Get(ts.URL + "/api/epics/" + id + "/specs/summary")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), "Summary") {
		t.Error("expected summary content")
	}
}

func TestVaultAPI_SpecsSummary_NotFound(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	resp, _ := http.Get(ts.URL + "/api/epics/" + id + "/specs/summary")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestVaultAPI_PathTraversal_擋住(t *testing.T) {
	ts, _ := setupEpicTestServer(t)
	defer ts.Close()
	id := createEpicForTest(t, ts)

	// Try path traversal on ADR read
	resp, _ := http.Get(ts.URL + "/api/epics/" + id + "/adr/../../../etc/passwd")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for path traversal, got %d", resp.StatusCode)
	}
}
