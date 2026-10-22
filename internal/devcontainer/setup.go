package devcontainer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SetupPhase represents the current phase of the setup process.
type SetupPhase string

const (
	PhaseWriting    SetupPhase = "writing"
	PhaseValidating SetupPhase = "validating"
	PhaseBuilding   SetupPhase = "building"
	PhaseVerifying  SetupPhase = "verifying"
	PhaseDone       SetupPhase = "done"
	PhaseFailed     SetupPhase = "failed"
)

// SetupStatus holds the current state of a setup process.
type SetupStatus struct {
	Phase   SetupPhase `json:"phase"`
	Message string     `json:"message"`
	Error   string     `json:"error,omitempty"`
}

// ProgressCallback is called whenever the setup phase changes.
type ProgressCallback func(SetupStatus)

// SetupConfig holds parameters for the setup process.
type SetupConfig struct {
	ProjectName    string
	ProjectPath    string
	FirewallMode   string // "open" | "whitelist"
	AllowedDomains string // comma-separated domain list
}

// ContainerRunner manages container lifecycle.
type ContainerRunner interface {
	EnsureRunning(projectPath string) error
	RemoveContainer(projectPath string) error
	Cleanup(projectPath string) CleanupResult
	ValidateConfig(projectPath string) error
	VerifyContainer(projectPath string, files GeneratedFiles) error
}

// SetupRunner orchestrates the DevContainer setup process: assemble base files
// (no language layer; user adds packages live in the running container later),
// then validate, build, and verify the container in a single linear pass.
type SetupRunner struct {
	runner     ContainerRunner
	templateFS fs.FS
}

// NewSetupRunner creates a new SetupRunner with the given dependencies.
func NewSetupRunner(runner ContainerRunner, templateFS fs.FS) *SetupRunner {
	return &SetupRunner{
		runner:     runner,
		templateFS: templateFS,
	}
}

// Run creates a project subdirectory under cfg.ProjectPath, writes a base
// .devcontainer/, then validates, builds, and verifies the container.
// Returns the created project path. Progress is reported via the callback.
func (s *SetupRunner) Run(ctx context.Context, cfg SetupConfig, progress ProgressCallback) (string, error) {
	actualPath := filepath.Join(cfg.ProjectPath, cfg.ProjectName)

	if err := ctx.Err(); err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "已取消", Error: err.Error()})
		return "", fmt.Errorf("setup cancelled: %w", err)
	}

	genCfg := GenerateConfig{
		ProjectName:    cfg.ProjectName,
		ProjectPath:    actualPath,
		FirewallMode:   cfg.FirewallMode,
		AllowedDomains: cfg.AllowedDomains,
	}
	files, err := assembleDevcontainerFiles(s.templateFS, genCfg, "", nil)
	if err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "組裝配置失敗", Error: err.Error()})
		return "", fmt.Errorf("assemble base files: %w", err)
	}

	files, err = Sanitize(files, "")
	if err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "驗證配置失敗", Error: err.Error()})
		return "", fmt.Errorf("sanitize config: %w", err)
	}

	progress(SetupStatus{Phase: PhaseWriting, Message: "正在寫入配置檔案..."})
	if err := os.MkdirAll(actualPath, 0o755); err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "建立專案目錄失敗", Error: err.Error()})
		return "", fmt.Errorf("create project directory %s: %w", actualPath, err)
	}
	if err := writeFiles(actualPath, files); err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "寫入檔案失敗", Error: err.Error()})
		return "", fmt.Errorf("write files: %w", err)
	}

	if err := ctx.Err(); err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "已取消", Error: err.Error()})
		return "", fmt.Errorf("setup cancelled: %w", err)
	}

	progress(SetupStatus{Phase: PhaseValidating, Message: "正在驗證配置..."})
	if err := s.runner.ValidateConfig(actualPath); err != nil {
		progress(SetupStatus{Phase: PhaseFailed, Message: "驗證配置失敗", Error: err.Error()})
		return "", fmt.Errorf("validate config: %w", err)
	}

	progress(SetupStatus{Phase: PhaseBuilding, Message: "正在建置容器..."})
	if err := s.runner.EnsureRunning(actualPath); err != nil {
		_ = s.runner.Cleanup(actualPath)
		progress(SetupStatus{Phase: PhaseFailed, Message: "建置容器失敗", Error: err.Error()})
		return "", fmt.Errorf("build container: %w", err)
	}

	progress(SetupStatus{Phase: PhaseVerifying, Message: "正在驗證容器環境..."})
	if err := s.runner.VerifyContainer(actualPath, files); err != nil {
		_ = s.runner.Cleanup(actualPath)
		progress(SetupStatus{Phase: PhaseFailed, Message: "容器驗證失敗", Error: err.Error()})
		return "", fmt.Errorf("verify container: %w", err)
	}

	progress(SetupStatus{Phase: PhaseDone, Message: "設定完成"})
	return actualPath, nil
}

// writeFiles writes GeneratedFiles to the .devcontainer directory.
func writeFiles(projectPath string, files GeneratedFiles) error {
	dcDir := filepath.Join(projectPath, ".devcontainer")
	if err := os.MkdirAll(dcDir, 0o755); err != nil {
		return fmt.Errorf("create .devcontainer directory: %w", err)
	}

	for name, content := range files {
		filePath := filepath.Join(dcDir, name)
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", name, err)
		}
	}

	return nil
}
