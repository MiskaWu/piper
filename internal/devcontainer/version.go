package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigTemplateVersion tracks the devcontainer template version.
// Increment when base.Dockerfile, devcontainerJSONTemplate, or shell scripts change.
const ConfigTemplateVersion = 16

// ProjectVersions holds the version metadata read from a project's devcontainer.json.
type ProjectVersions struct {
	TemplateVersion int `json:"templateVersion"`
}

// UpdateAvailable returns true if the project's template version is behind the current constant.
func (v ProjectVersions) UpdateAvailable() bool {
	return v.TemplateVersion < ConfigTemplateVersion
}

// GetProjectVersions reads the piper version metadata from a project's devcontainer.json.
// Returns zero values if the file doesn't exist or has no version metadata (pre-upgrade projects).
func GetProjectVersions(projectPath string) ProjectVersions {
	data, err := os.ReadFile(filepath.Join(projectPath, ".devcontainer", "devcontainer.json"))
	if err != nil {
		return ProjectVersions{}
	}

	var config struct {
		Piper *ProjectVersions `json:"piper"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ProjectVersions{}
	}
	if config.Piper == nil {
		return ProjectVersions{}
	}
	return *config.Piper
}

// ExtractLanguageFragment extracts the AI-generated language layer from an assembled Dockerfile.
// It returns everything after the language layer marker, or falls back to removing the FROM line.
func ExtractLanguageFragment(projectPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectPath, ".devcontainer", "Dockerfile"))
	if err != nil {
		return "", fmt.Errorf("read Dockerfile: %w", err)
	}
	return extractFragment(string(data)), nil
}

// ExtractVSCodeExtensions reads vscodeExtensions from a project's devcontainer.json.
// It returns the extensions list excluding the always-present "anthropic.claude-code".
func ExtractVSCodeExtensions(projectPath string) []string {
	data, err := os.ReadFile(filepath.Join(projectPath, ".devcontainer", "devcontainer.json"))
	if err != nil {
		return nil
	}

	var config struct {
		Customizations struct {
			VSCode struct {
				Extensions []string `json:"extensions"`
			} `json:"vscode"`
		} `json:"customizations"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}

	var exts []string
	for _, ext := range config.Customizations.VSCode.Extensions {
		if !strings.EqualFold(ext, "anthropic.claude-code") {
			exts = append(exts, ext)
		}
	}
	return exts
}

// UpgradeConfig holds the parameters needed to upgrade a project's DevContainer config.
type UpgradeConfig struct {
	ProjectPath    string
	ProjectName    string
	FirewallMode   string
	AllowedDomains string
}

// UpgradeResult reports the outcome of an upgrade operation.
type UpgradeResult struct {
	Upgraded          bool   `json:"upgraded"`
	LanguageExtracted bool   `json:"languageExtracted"`
	ExtractionSuccess bool   `json:"extractionSuccess,omitempty"`
	ExtractionError   string `json:"extractionError,omitempty"`
}

// Upgrader handles DevContainer config upgrades, including AI-powered language layer extraction.
type Upgrader struct {
	generator  *Generator
	templateFS fs.FS
}

// NewUpgrader creates a new Upgrader with the given Generator and template FS.
func NewUpgrader(gen *Generator, templateFS fs.FS) *Upgrader {
	return &Upgrader{generator: gen, templateFS: templateFS}
}

// UpgradeFiles regenerates the Dockerfile, devcontainer.json, and shell scripts using the
// current templates. If the existing language fragment is contaminated with base-image content,
// it calls claude -p to extract the real language layer.
func (u *Upgrader) UpgradeFiles(ctx context.Context, cfg UpgradeConfig) (*UpgradeResult, error) {
	// Read existing Dockerfile to get raw fragment
	dockerfileData, err := os.ReadFile(filepath.Join(cfg.ProjectPath, ".devcontainer", "Dockerfile"))
	if err != nil {
		return nil, fmt.Errorf("read Dockerfile: %w", err)
	}
	rawFragment := extractFragmentRaw(string(dockerfileData))

	// Extract existing VS Code extensions
	extensions := ExtractVSCodeExtensions(cfg.ProjectPath)

	// Determine the clean fragment and whether AI extraction is needed
	var fragment string
	contaminated := IsContaminatedFragment(rawFragment)
	if !contaminated {
		fragment = rawFragment
	}
	// contaminated → fragment stays "", will be updated after AI extraction

	// Build and write the initial clean files
	if err := u.writeUpgradeFiles(cfg, fragment, extensions); err != nil {
		return nil, err
	}

	result := &UpgradeResult{Upgraded: true}

	if contaminated && rawFragment != "" {
		result.LanguageExtracted = true

		overlay, err := u.generator.ExtractLanguageLayer(ctx, rawFragment)
		if err != nil {
			result.ExtractionError = err.Error()
			return result, nil // upgrade succeeded, extraction failed — non-fatal
		}

		result.ExtractionSuccess = true

		// Merge extracted extensions with existing ones
		allExtensions := mergeExtensions(extensions, overlay.VSCodeExtensions)

		// Re-write files with extracted language layer
		if err := u.writeUpgradeFiles(cfg, overlay.DockerfileFragment, allExtensions); err != nil {
			result.ExtractionError = fmt.Sprintf("write extracted files: %v", err)
			result.ExtractionSuccess = false
			return result, nil
		}
	}

	return result, nil
}

// writeUpgradeFiles assembles and writes the DevContainer files.
func (u *Upgrader) writeUpgradeFiles(cfg UpgradeConfig, fragment string, extensions []string) error {
	genCfg := GenerateConfig{
		ProjectName:    cfg.ProjectName,
		ProjectPath:    cfg.ProjectPath,
		FirewallMode:   cfg.FirewallMode,
		AllowedDomains: cfg.AllowedDomains,
	}
	files, err := assembleDevcontainerFiles(u.templateFS, genCfg, fragment, extensions)
	if err != nil {
		return err
	}
	return writeFiles(cfg.ProjectPath, files)
}

// mergeExtensions combines two extension lists, deduplicating case-insensitively.
func mergeExtensions(existing, extracted []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, ext := range existing {
		lower := strings.ToLower(ext)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, ext)
		}
	}
	for _, ext := range extracted {
		lower := strings.ToLower(ext)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, ext)
		}
	}
	return result
}

