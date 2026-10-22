package devcontainer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockCommander records calls and returns pre-configured responses.
type mockCommander struct {
	outputFunc           func(name string, args ...string) ([]byte, error)
	outputWithStderrFunc func(name string, args ...string) ([]byte, []byte, error)
	runFunc              func(name string, args ...string) error
	calls                []string
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, fmt.Sprintf("Output: %s %s", name, strings.Join(args, " ")))
	if m.outputFunc != nil {
		return m.outputFunc(name, args...)
	}
	return nil, nil
}

func (m *mockCommander) OutputWithStderr(name string, args ...string) ([]byte, []byte, error) {
	m.calls = append(m.calls, fmt.Sprintf("OutputWithStderr: %s %s", name, strings.Join(args, " ")))
	if m.outputWithStderrFunc != nil {
		return m.outputWithStderrFunc(name, args...)
	}
	return nil, nil, nil
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.calls = append(m.calls, fmt.Sprintf("Run: %s %s", name, strings.Join(args, " ")))
	if m.runFunc != nil {
		return m.runFunc(name, args...)
	}
	return nil
}

func TestGetProjectStates_正規化UNC路徑後匹配容器(t *testing.T) {
	// VS Code 建立的容器使用 WSL UNC 路徑，應正規化後匹配
	projectPath := "/home/user/my-project"
	wslPath := `\\wsl.localhost\Ubuntu-24.04\home\user\my-project`
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\tabc123\n", wslPath)), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{projectPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ContainerState != "running" {
		t.Errorf("expected state running (UNC normalized), got %s", projects[0].ContainerState)
	}
	if projects[0].ContainerID != "abc123" {
		t.Errorf("expected container ID abc123, got %s", projects[0].ContainerID)
	}
}

func TestGetProjectStates_兩容器時優先匹配Linux路徑容器(t *testing.T) {
	// 同一專案有 UNC 和 Linux 路徑兩個容器，優先匹配 Linux 路徑的
	projectPath := "/home/user/my-project"
	wslPath := `\\wsl.localhost\Ubuntu-24.04\home\user\my-project`
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				lines := fmt.Sprintf("%s\trunning\tvscode-container\taaa111\n", wslPath) +
					fmt.Sprintf("%s\trunning\tweb-container\tbbb222\n", projectPath)
				return []byte(lines), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{projectPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ContainerState != "running" {
		t.Errorf("expected state running, got %s", projects[0].ContainerState)
	}
	if projects[0].ContainerID != "bbb222" {
		t.Errorf("expected container ID bbb222, got %s", projects[0].ContainerID)
	}
}

func TestGetProjectStates_同路徑多容器時優先取running(t *testing.T) {
	// chartless 場景：同一路徑有 running + exited 兩個容器
	projectPath := "/home/user/chartless"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				lines := fmt.Sprintf("%s\trunning\tchartless_devcontainer-devcontainer-1\taaa111\n", projectPath) +
					fmt.Sprintf("%s\texited\tgracious_ardinghelli\tbbb222\n", projectPath)
				return []byte(lines), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{projectPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ContainerState != "running" {
		t.Errorf("expected state running, got %s", projects[0].ContainerState)
	}
	if projects[0].ContainerID != "aaa111" {
		t.Errorf("expected container ID aaa111, got %s", projects[0].ContainerID)
	}
}

func TestGetProjectStates_同路徑多容器exited在前running在後(t *testing.T) {
	// 順序相反：exited 先出現，running 後出現，仍應選 running
	projectPath := "/home/user/chartless"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				lines := fmt.Sprintf("%s\texited\told-container\tbbb222\n", projectPath) +
					fmt.Sprintf("%s\trunning\tnew-container\taaa111\n", projectPath)
				return []byte(lines), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{projectPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projects[0].ContainerState != "running" {
		t.Errorf("expected state running, got %s", projects[0].ContainerState)
	}
	if projects[0].ContainerID != "aaa111" {
		t.Errorf("expected container ID aaa111, got %s", projects[0].ContainerID)
	}
}

func TestNormalizeWSLPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"UNC wsl.localhost", `\\wsl.localhost\Ubuntu-24.04\home\user\proj`, "/home/user/proj"},
		{"UNC wsl$", `\\wsl$\Ubuntu\home\user\proj`, "/home/user/proj"},
		{"Linux path unchanged", "/home/user/proj", "/home/user/proj"},
		{"Empty string", "", ""},
		{"Short UNC path", `\\wsl.localhost\Distro`, `\\wsl.localhost\Distro`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWSLPath(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeWSLPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetProjectStates_空路徑清單回傳空列表(t *testing.T) {
	mock := &mockCommander{}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected no docker calls for empty paths, got %v", mock.calls)
	}
}

func TestGetProjectStates_匹配容器狀態(t *testing.T) {
	projectPath := "/home/user/my-project"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\tabc123\n", projectPath)), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{projectPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "my-project" {
		t.Errorf("expected name my-project, got %s", projects[0].Name)
	}
	if projects[0].ContainerState != "running" {
		t.Errorf("expected state running, got %s", projects[0].ContainerState)
	}
	if projects[0].ContainerID != "abc123" {
		t.Errorf("expected container ID abc123, got %s", projects[0].ContainerID)
	}
}

func TestGetProjectStates_無容器時狀態為none(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	projects, err := mgr.GetProjectStates([]string{"/home/user/orphan"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ContainerState != "none" {
		t.Errorf("expected state none, got %s", projects[0].ContainerState)
	}
}

func TestGetProjectStates_多種容器狀態(t *testing.T) {
	tests := []struct {
		dockerState   string
		expectedState string
	}{
		{"running", "running"},
		{"exited", "stopped"},
		{"created", "stopped"},
		{"dead", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.dockerState, func(t *testing.T) {
			path := "/home/user/proj"
			mock := &mockCommander{
				outputFunc: func(name string, args ...string) ([]byte, error) {
					if name == "docker" {
						return []byte(fmt.Sprintf("%s\t%s\tcontainer\tid123\n", path, tt.dockerState)), nil
					}
					return nil, nil
				},
			}
			mgr := NewManager(mock)

			projects, err := mgr.GetProjectStates([]string{path})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if projects[0].ContainerState != tt.expectedState {
				t.Errorf("expected state %s, got %s", tt.expectedState, projects[0].ContainerState)
			}
		})
	}
}

func TestGetProjectStates_docker錯誤(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("docker not found")
		},
	}
	mgr := NewManager(mock)

	_, err := mgr.GetProjectStates([]string{"/some/path"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureRunning_成功(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			return []byte(`{"outcome":"success"}`), nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.EnsureRunning("/tmp/projects/my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	expected := "OutputWithStderr: devcontainer up --workspace-folder /tmp/projects/my-app"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestEnsureRunning_失敗時包含devcontainer描述(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			stdout := []byte(`{"outcome":"error","message":"Command failed: docker buildx build ...","description":"An error occurred building the image."}`)
			stderr := []byte("Step 8/14 : RUN npm install -g @anthropic-ai/claude-code@latest\nnpm ERR! network timeout")
			return stdout, stderr, fmt.Errorf("exit status 1")
		},
	}
	mgr := NewManager(mock)

	err := mgr.EnsureRunning("/tmp/projects/my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	// 應包含 devcontainer CLI 的 description
	if !strings.Contains(errMsg, "An error occurred building the image") {
		t.Errorf("error should contain devcontainer description, got: %s", errMsg)
	}
	// 應包含 stderr 中的 Docker build log
	if !strings.Contains(errMsg, "npm ERR! network timeout") {
		t.Errorf("error should contain Docker build log from stderr, got: %s", errMsg)
	}
}

func TestEnsureRunning_失敗時stderr過長只取末尾(t *testing.T) {
	// 產生 50 行 stderr，應只保留最後 30 行
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	fullStderr := strings.Join(lines, "\n")

	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			stdout := []byte(`{"outcome":"error","description":"build failed"}`)
			return stdout, []byte(fullStderr), fmt.Errorf("exit status 1")
		},
	}
	mgr := NewManager(mock)

	err := mgr.EnsureRunning("/tmp/projects/my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	// 不應包含前面的行
	if strings.Contains(errMsg, "line 1\n") {
		t.Errorf("error should not contain early lines, got: %s", errMsg)
	}
	// 應包含最後幾行
	if !strings.Contains(errMsg, "line 50") {
		t.Errorf("error should contain last line, got: %s", errMsg)
	}
}

func TestEnsureRunning_失敗時stdout非JSON仍包含原始輸出(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			return []byte("unexpected output"), []byte("some error"), fmt.Errorf("exit status 1")
		},
	}
	mgr := NewManager(mock)

	err := mgr.EnsureRunning("/tmp/projects/my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "some error") {
		t.Errorf("error should contain stderr, got: %s", errMsg)
	}
}

func TestEnsureRunning_失敗時無stderr仍有基本錯誤訊息(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			stdout := []byte(`{"outcome":"error","description":"CLI not found"}`)
			return stdout, nil, fmt.Errorf("exit status 127")
		},
	}
	mgr := NewManager(mock)

	err := mgr.EnsureRunning("/tmp/projects/my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "CLI not found") {
		t.Errorf("error should contain devcontainer description, got: %s", errMsg)
	}
}

func TestRebuild(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			return []byte("{}"), nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.Rebuild(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	if !strings.HasPrefix(mock.calls[0], "Output: docker ps") {
		t.Errorf("expected docker ps call first, got %q", mock.calls[0])
	}
	expectedRm := "Run: docker rm -f xyz999"
	if mock.calls[1] != expectedRm {
		t.Errorf("expected %q, got %q", expectedRm, mock.calls[1])
	}
}

func TestCLIAvailable_有安裝時回傳true(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "devcontainer" && len(args) > 0 && args[0] == "--help" {
				return []byte("devcontainer CLI v0.1.0"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	if !mgr.CLIAvailable() {
		t.Error("expected CLIAvailable() = true when devcontainer --help succeeds")
	}
}

func TestCLIAvailable_未安裝時回傳false(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "devcontainer" {
				return nil, fmt.Errorf("exec: \"devcontainer\": executable file not found in $PATH")
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	if mgr.CLIAvailable() {
		t.Error("expected CLIAvailable() = false when devcontainer not found")
	}
}

func TestDetectAutoMode_有postAttachCommand含skipPermissions(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"name": "test",
		"postAttachCommand": "echo '\\nalias claude=\"claude --dangerously-skip-permissions\"' >> ~/.bashrc"
	}`), 0o644)

	mgr := NewManager(&mockCommander{})
	if !mgr.DetectAutoMode(dir) {
		t.Error("expected DetectAutoMode=true when postAttachCommand contains --dangerously-skip-permissions")
	}
}

func TestDetectAutoMode_無postAttachCommand(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"name": "test",
		"postStartCommand": "echo hello"
	}`), 0o644)

	mgr := NewManager(&mockCommander{})
	if mgr.DetectAutoMode(dir) {
		t.Error("expected DetectAutoMode=false when no postAttachCommand")
	}
}

func TestDetectAutoMode_postAttachCommand不含skipPermissions(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"name": "test",
		"postAttachCommand": "echo hello"
	}`), 0o644)

	mgr := NewManager(&mockCommander{})
	if mgr.DetectAutoMode(dir) {
		t.Error("expected DetectAutoMode=false when postAttachCommand doesn't contain skip-permissions")
	}
}

func TestDetectAutoMode_無devcontainer目錄(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(&mockCommander{})
	if mgr.DetectAutoMode(dir) {
		t.Error("expected DetectAutoMode=false when .devcontainer/ doesn't exist")
	}
}

func TestDetectAutoMode_JSON格式錯誤(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`not valid json`), 0o644)

	mgr := NewManager(&mockCommander{})
	if mgr.DetectAutoMode(dir) {
		t.Error("expected DetectAutoMode=false when JSON is invalid")
	}
}

func TestStop_找到容器並停止(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.Stop(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(mock.calls), mock.calls)
	}
	if !strings.HasPrefix(mock.calls[0], "Output: docker ps") {
		t.Errorf("expected docker ps call first, got %q", mock.calls[0])
	}
	expectedStop := "Run: docker stop xyz999"
	if mock.calls[1] != expectedStop {
		t.Errorf("expected %q, got %q", expectedStop, mock.calls[1])
	}
}

func TestStop_找不到容器不報錯(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.Stop("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected no error for missing container, got: %v", err)
	}
}

func TestStop_docker錯誤時回傳wrapped_error(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte("/some/path\trunning\tcontainer\tid123\n"), nil
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			return fmt.Errorf("permission denied")
		},
	}
	mgr := NewManager(mock)

	err := mgr.Stop("/some/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "stop container") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestCleanup_刪除容器和Image(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:abc123image"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	result := mgr.Cleanup(projectPath)
	if result.ContainerErr != nil {
		t.Fatalf("unexpected container error: %v", result.ContainerErr)
	}
	if result.ImageErr != nil {
		t.Fatalf("unexpected image error: %v", result.ImageErr)
	}

	// Verify: docker ps → docker inspect → docker rm -f → docker rmi
	expectedCalls := []string{
		"Output: docker ps -a --filter label=devcontainer.local_folder --format {{.Label \"devcontainer.local_folder\"}}\t{{.State}}\t{{.Names}}\t{{.ID}}",
		"Output: docker inspect --format {{.Image}} xyz999",
		"Run: docker rm -f xyz999",
		"Run: docker rmi sha256:abc123image",
	}
	if len(mock.calls) != len(expectedCalls) {
		t.Fatalf("expected %d calls, got %d: %v", len(expectedCalls), len(mock.calls), mock.calls)
	}
	for i, expected := range expectedCalls {
		if mock.calls[i] != expected {
			t.Errorf("call[%d]: expected %q, got %q", i, expected, mock.calls[i])
		}
	}
}

func TestCleanup_找不到容器不報錯(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	result := mgr.Cleanup("/nonexistent/path")
	if result.ContainerErr != nil {
		t.Errorf("expected no container error, got: %v", result.ContainerErr)
	}
	if result.ImageErr != nil {
		t.Errorf("expected no image error, got: %v", result.ImageErr)
	}
}

func TestCleanup_Image刪除失敗不影響整體(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:abc123image"), nil
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			if name == "docker" && len(args) > 0 && args[0] == "rmi" {
				return fmt.Errorf("image is referenced in multiple repositories")
			}
			return nil
		},
	}
	mgr := NewManager(mock)

	result := mgr.Cleanup(projectPath)
	if result.ContainerErr != nil {
		t.Fatalf("container removal should succeed, got: %v", result.ContainerErr)
	}
	if result.ImageErr == nil {
		t.Fatal("expected image error, got nil")
	}
	if !strings.Contains(result.ImageErr.Error(), "image is referenced") {
		t.Errorf("expected wrapped error, got: %v", result.ImageErr)
	}
}

func TestCleanup_容器刪除失敗時仍嘗試刪Image(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:abc123image"), nil
			}
			return nil, nil
		},
		runFunc: func(name string, args ...string) error {
			if name == "docker" && len(args) > 0 && args[0] == "rm" {
				return fmt.Errorf("permission denied")
			}
			return nil
		},
	}
	mgr := NewManager(mock)

	result := mgr.Cleanup(projectPath)
	if result.ContainerErr == nil {
		t.Fatal("expected container error, got nil")
	}
	// Should still attempt image removal
	if result.ImageErr != nil {
		t.Fatalf("image removal should succeed, got: %v", result.ImageErr)
	}
}

func TestRemoveContainer_刪除容器(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.RemoveContainer(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedRm := "Run: docker rm -f xyz999"
	if mock.calls[1] != expectedRm {
		t.Errorf("expected %q, got %q", expectedRm, mock.calls[1])
	}
}

func TestRemoveImage_刪除Image(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:abc123image"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.RemoveImage(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedRmi := "Run: docker rmi sha256:abc123image"
	found := false
	for _, call := range mock.calls {
		if call == expectedRmi {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in calls, got %v", expectedRmi, mock.calls)
	}
}

func TestValidateConfig_成功(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			return []byte(`{"configuration":{}}`), nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.ValidateConfig("/tmp/projects/my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	expected := "OutputWithStderr: devcontainer read-configuration --workspace-folder /tmp/projects/my-app"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestValidateConfig_失敗時包含stderr(t *testing.T) {
	mock := &mockCommander{
		outputWithStderrFunc: func(name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("invalid Feature: ghcr.io/devcontainers/features/foo"), fmt.Errorf("exit status 1")
		},
	}
	mgr := NewManager(mock)

	err := mgr.ValidateConfig("/tmp/projects/my-app")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid Feature") {
		t.Errorf("error should contain stderr, got: %v", err)
	}
}

func TestVerifyContainer_驗證成功(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			// docker exec — all checks pass
			if name == "docker" && len(args) > 0 && args[0] == "exec" {
				return []byte("ok"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	files := GeneratedFiles{
		"Dockerfile":      "FROM node:20",
		"init-firewall.sh": "#!/bin/bash",
	}
	err := mgr.VerifyContainer(projectPath, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyContainer_找不到容器(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(""), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.VerifyContainer("/nonexistent", GeneratedFiles{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no container found") {
		t.Errorf("expected 'no container found' error, got: %v", err)
	}
}

func TestVerifyContainer_workspace目錄不存在(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	execCalls := 0
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "exec" {
				execCalls++
				if execCalls == 1 {
					// First exec: test -d /workspace → fails
					return nil, fmt.Errorf("exit status 1")
				}
				return []byte("ok"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.VerifyContainer(projectPath, GeneratedFiles{"Dockerfile": "FROM node:20"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "/workspace directory not found") {
		t.Errorf("expected workspace error, got: %v", err)
	}
}

func TestVerifyContainer_語言Runtime偵測(t *testing.T) {
	projectPath := "/tmp/projects/go-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tgo-container\tabc123\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "exec" {
				cmd := args[len(args)-1]
				// Fail go version check
				if strings.Contains(cmd, "go version") {
					return nil, fmt.Errorf("exit status 127")
				}
				return []byte("ok"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	files := GeneratedFiles{
		"Dockerfile": "FROM node:20\nENV GO_VERSION=1.22\nRUN apt-get update",
	}
	err := mgr.VerifyContainer(projectPath, files)
	if err == nil {
		t.Fatal("expected error for missing Go runtime, got nil")
	}
	if !strings.Contains(err.Error(), "Go runtime not available") {
		t.Errorf("expected Go runtime error, got: %v", err)
	}
}

func TestExecInContainer(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "exec" {
				return []byte("command output"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	out, err := mgr.execInContainer("abc123", "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "command output" {
		t.Errorf("expected 'command output', got %q", out)
	}
	expected := "Output: docker exec abc123 sh -c echo hello"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestCleanupAll_成功(t *testing.T) {
	projectPath := "/tmp/projects/my-app"
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte(fmt.Sprintf("%s\trunning\tmy-container\txyz999\n", projectPath)), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "inspect" {
				return []byte("sha256:img123"), nil
			}
			return nil, nil
		},
	}
	mgr := NewManager(mock)

	err := mgr.CleanupAll(projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetContainerLogs(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("log line 1\nlog line 2\n"), nil
		},
	}
	mgr := NewManager(mock)

	logs, err := mgr.GetContainerLogs("abc123", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs != "log line 1\nlog line 2\n" {
		t.Errorf("unexpected logs: %q", logs)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	expected := "Output: docker logs --tail 50 abc123"
	if mock.calls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.calls[0])
	}
}

func TestParseClaudeVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"純版本號", "2.1.100", "2.1.100"},
		{"帶後綴", "2.1.100 (Claude Code)", "2.1.100"},
		{"帶前綴v", "v2.1.100", "2.1.100"},
		{"pre-release", "2.1.100-beta.1", "2.1.100-beta.1"},
		{"無版本號", "Claude Code", ""},
		{"空字串", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseClaudeVersion(tt.input)
			if got != tt.expect {
				t.Errorf("ParseClaudeVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestGetClaudeVersion_成功取得版本(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("2.1.100 (Claude Code)\n"), nil
		},
	}
	mgr := NewManager(mock)

	version, err := mgr.GetClaudeVersion("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "2.1.100" {
		t.Errorf("expected version 2.1.100, got %q", version)
	}
}

func TestGetClaudeVersion_指令失敗(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("command not found")
		},
	}
	mgr := NewManager(mock)

	_, err := mgr.GetClaudeVersion("abc123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "get claude version") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestUpdateClaude_成功更新(t *testing.T) {
	callCount := 0
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// npm install
				return []byte("added 1 package\n"), nil
			}
			// claude --version (called by GetClaudeVersion after update)
			return []byte("1.0.30\n"), nil
		},
	}
	mgr := NewManager(mock)

	version, err := mgr.UpdateClaude("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "1.0.30" {
		t.Errorf("expected version 1.0.30, got %q", version)
	}
	if callCount != 2 {
		t.Errorf("expected 2 docker exec calls, got %d", callCount)
	}
}

func TestUpdateClaude_npm安裝失敗(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("npm ERR!")
		},
	}
	mgr := NewManager(mock)

	_, err := mgr.UpdateClaude("abc123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "update claude") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestPinClaudeVersion_替換latest為具體版本(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
  "build": {
    "args": {
      "CLAUDE_CODE_VERSION": "latest"
    }
  }
}`), 0o644)

	err := PinClaudeVersion(dir, "2.3.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dcDir, "devcontainer.json"))
	if !strings.Contains(string(data), `"CLAUDE_CODE_VERSION": "2.3.5"`) {
		t.Errorf("expected version pinned to 2.3.5, got:\n%s", data)
	}
}

func TestPinClaudeVersion_替換舊版本為新版本(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
  "build": {
    "args": {
      "CLAUDE_CODE_VERSION": "2.2.0"
    }
  }
}`), 0o644)

	err := PinClaudeVersion(dir, "2.3.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dcDir, "devcontainer.json"))
	if !strings.Contains(string(data), `"CLAUDE_CODE_VERSION": "2.3.5"`) {
		t.Errorf("expected version updated to 2.3.5, got:\n%s", data)
	}
}

func TestPinClaudeVersion_版本相同時不寫入(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	content := []byte(`{
  "build": {
    "args": {
      "CLAUDE_CODE_VERSION": "2.3.5"
    }
  }
}`)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), content, 0o644)
	info, _ := os.Stat(filepath.Join(dcDir, "devcontainer.json"))
	origModTime := info.ModTime()

	err := PinClaudeVersion(dir, "2.3.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info2, _ := os.Stat(filepath.Join(dcDir, "devcontainer.json"))
	if !info2.ModTime().Equal(origModTime) {
		t.Error("expected file not to be rewritten when version unchanged")
	}
}

func TestPinClaudeVersion_檔案不存在時回傳錯誤(t *testing.T) {
	dir := t.TempDir()
	err := PinClaudeVersion(dir, "2.3.5")
	if err == nil {
		t.Fatal("expected error for missing devcontainer.json")
	}
}
