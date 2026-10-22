package devcontainer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mockRunner implements ContainerRunner for testing.
type mockRunner struct {
	ensureErr      error
	validateErr    error
	verifyErr      error
	calledFor      string
	cleanedPaths   []string
	validateCalls  int
	verifyCalls    int
	ensureCalls    int
}

func (m *mockRunner) EnsureRunning(projectPath string) error {
	m.calledFor = projectPath
	m.ensureCalls++
	return m.ensureErr
}

func (m *mockRunner) RemoveContainer(projectPath string) error {
	return nil
}

func (m *mockRunner) Cleanup(projectPath string) CleanupResult {
	m.cleanedPaths = append(m.cleanedPaths, projectPath)
	return CleanupResult{}
}

func (m *mockRunner) ValidateConfig(projectPath string) error {
	m.validateCalls++
	return m.validateErr
}

func (m *mockRunner) VerifyContainer(projectPath string, files GeneratedFiles) error {
	m.verifyCalls++
	return m.verifyErr
}

func TestSetupRunner_Run_完整流程成功(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &mockRunner{}
	sr := NewSetupRunner(runner, testTemplateFS())

	var phases []SetupPhase
	progress := func(s SetupStatus) {
		phases = append(phases, s.Phase)
	}

	actualPath, err := sr.Run(context.Background(), SetupConfig{
		ProjectName: "test-project",
		ProjectPath: tmpDir,
	}, progress)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "test-project")
	if actualPath != expectedPath {
		t.Errorf("expected actualPath %s, got %s", expectedPath, actualPath)
	}

	expectedPhases := []SetupPhase{PhaseWriting, PhaseValidating, PhaseBuilding, PhaseVerifying, PhaseDone}
	if len(phases) != len(expectedPhases) {
		t.Fatalf("expected %d phases, got %d: %v", len(expectedPhases), len(phases), phases)
	}
	for i, p := range expectedPhases {
		if phases[i] != p {
			t.Errorf("phase[%d]: expected %s, got %s", i, p, phases[i])
		}
	}

	dockerfile, err := os.ReadFile(filepath.Join(actualPath, ".devcontainer", "Dockerfile"))
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}
	if len(dockerfile) == 0 {
		t.Error("Dockerfile should not be empty")
	}

	dcJSON, err := os.ReadFile(filepath.Join(actualPath, ".devcontainer", "devcontainer.json"))
	if err != nil {
		t.Fatalf("failed to read devcontainer.json: %v", err)
	}
	if len(dcJSON) == 0 {
		t.Error("devcontainer.json should not be empty")
	}

	if runner.calledFor != expectedPath {
		t.Errorf("expected runner called for %s, got %s", expectedPath, runner.calledFor)
	}
}

func TestSetupRunner_Run_驗證階段失敗(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &mockRunner{validateErr: errors.New("invalid config")}
	sr := NewSetupRunner(runner, testTemplateFS())

	var lastStatus SetupStatus
	progress := func(s SetupStatus) {
		lastStatus = s
	}

	actualPath, err := sr.Run(context.Background(), SetupConfig{
		ProjectName: "test",
		ProjectPath: tmpDir,
	}, progress)
	if err == nil {
		t.Fatal("expected error from validate failure, got nil")
	}
	if actualPath != "" {
		t.Errorf("expected empty actualPath on error, got %s", actualPath)
	}
	if lastStatus.Phase != PhaseFailed {
		t.Errorf("expected last phase failed, got %s", lastStatus.Phase)
	}
	if runner.ensureCalls != 0 {
		t.Errorf("EnsureRunning should not be called when validate fails")
	}
}

func TestSetupRunner_Run_建置階段失敗(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &mockRunner{ensureErr: errors.New("docker not found")}
	sr := NewSetupRunner(runner, testTemplateFS())

	var lastStatus SetupStatus
	progress := func(s SetupStatus) {
		lastStatus = s
	}

	actualPath, err := sr.Run(context.Background(), SetupConfig{
		ProjectName: "test",
		ProjectPath: tmpDir,
	}, progress)
	if err == nil {
		t.Fatal("expected error from build failure, got nil")
	}
	if actualPath != "" {
		t.Errorf("expected empty actualPath on error, got %s", actualPath)
	}
	if lastStatus.Phase != PhaseFailed {
		t.Errorf("expected last phase failed, got %s", lastStatus.Phase)
	}
	if len(runner.cleanedPaths) != 1 {
		t.Errorf("expected cleanup once after build failure, got %d", len(runner.cleanedPaths))
	}
	if runner.verifyCalls != 0 {
		t.Errorf("VerifyContainer should not be called when build fails")
	}

	// Files should still exist (written before build attempt)
	if _, statErr := os.Stat(filepath.Join(tmpDir, "test", ".devcontainer", "Dockerfile")); statErr != nil {
		t.Error("expected Dockerfile to exist after build failure")
	}
}

func TestSetupRunner_Run_容器驗證失敗(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &mockRunner{verifyErr: errors.New("Go runtime not available")}
	sr := NewSetupRunner(runner, testTemplateFS())

	var lastStatus SetupStatus
	progress := func(s SetupStatus) {
		lastStatus = s
	}

	_, err := sr.Run(context.Background(), SetupConfig{
		ProjectName: "test",
		ProjectPath: tmpDir,
	}, progress)
	if err == nil {
		t.Fatal("expected error from verify failure, got nil")
	}
	if lastStatus.Phase != PhaseFailed {
		t.Errorf("expected last phase failed, got %s", lastStatus.Phase)
	}
	if len(runner.cleanedPaths) != 1 {
		t.Errorf("expected cleanup once after verify failure, got %d", len(runner.cleanedPaths))
	}
}

func TestSetupRunner_Run_Context已取消時提早結束(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &mockRunner{}
	sr := NewSetupRunner(runner, testTemplateFS())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var lastStatus SetupStatus
	progress := func(s SetupStatus) {
		lastStatus = s
	}

	_, err := sr.Run(ctx, SetupConfig{
		ProjectName: "test",
		ProjectPath: tmpDir,
	}, progress)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if lastStatus.Phase != PhaseFailed {
		t.Errorf("expected last phase failed, got %s", lastStatus.Phase)
	}
	if runner.validateCalls != 0 {
		t.Errorf("ValidateConfig should not be called when context already cancelled")
	}
}

func TestWriteFiles_寫入_devcontainer目錄(t *testing.T) {
	tmpDir := t.TempDir()

	files := GeneratedFiles{
		"devcontainer.json":  `{"name":"myproject","image":"mcr.microsoft.com/devcontainers/base:ubuntu"}`,
		"Dockerfile":         "FROM ubuntu:22.04\nRUN apt-get update",
		"docker-compose.yml": "version: '3'\nservices:\n  app:\n    build: .",
	}

	if err := writeFiles(tmpDir, files); err != nil {
		t.Fatalf("writeFiles failed: %v", err)
	}

	dcDir := filepath.Join(tmpDir, ".devcontainer")
	for name, expectedContent := range files {
		data, err := os.ReadFile(filepath.Join(dcDir, name))
		if err != nil {
			t.Errorf("failed to read %s: %v", name, err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("file %s: expected %q, got %q", name, expectedContent, string(data))
		}
	}
}
