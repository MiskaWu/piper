package devcontainer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// semverRe matches a semver-like version number (e.g. "2.1.100", "1.0.25-beta.1").
var semverRe = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?`)

// ParseClaudeVersion extracts the semver portion from a version string.
// Works with both "2.1.100 (Claude Code)" and plain "2.1.100".
// Returns empty string if no version pattern is found.
func ParseClaudeVersion(raw string) string {
	return semverRe.FindString(raw)
}

// Commander abstracts command execution for testability.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
	OutputWithStderr(name string, args ...string) (stdout, stderr []byte, err error)
	Run(name string, args ...string) error
}

// Project represents a project with optional devcontainer configuration.
type Project struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	ContainerState string `json:"containerState"` // "running" | "stopped" | "error" | "none"
	ContainerID    string `json:"containerId,omitempty"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
}

// Manager handles devcontainer lifecycle operations.
type Manager struct {
	cmd Commander
}

// NewManager creates a new Manager with the given Commander.
func NewManager(cmd Commander) *Manager {
	return &Manager{cmd: cmd}
}

// containerInfo holds parsed docker container metadata.
type containerInfo struct {
	localFolder string
	state       string
	name        string
	id          string
}

// GetProjectStates takes a list of known project paths and returns their
// container states by querying Docker.
func (m *Manager) GetProjectStates(paths []string) ([]Project, error) {
	if len(paths) == 0 {
		return []Project{}, nil
	}

	containers, err := m.listContainers()
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	containerMap := make(map[string]containerInfo)
	for _, c := range containers {
		key := normalizeWSLPath(c.localFolder)
		isNative := c.localFolder == key // not a UNC path
		existing, exists := containerMap[key]
		if !exists {
			containerMap[key] = c
		} else {
			existingNative := normalizeWSLPath(existing.localFolder) == existing.localFolder
			// Prefer: (1) running over non-running, (2) native Linux path over UNC
			if betterState(c.state, existing.state) || (c.state == existing.state && isNative && !existingNative) {
				containerMap[key] = c
			}
		}
	}

	var projects []Project
	for _, path := range paths {
		name := filepath.Base(path)
		p := Project{
			Name:           name,
			Path:           path,
			ContainerState: "none",
		}
		if c, ok := containerMap[path]; ok {
			p.ContainerID = c.id
			p.ContainerState = mapDockerState(c.state)
		}
		projects = append(projects, p)
	}

	return projects, nil
}

// listContainers queries docker for containers with devcontainer labels.
func (m *Manager) listContainers() ([]containerInfo, error) {
	out, err := m.cmd.Output("docker", "ps", "-a",
		"--filter", "label=devcontainer.local_folder",
		"--format", `{{.Label "devcontainer.local_folder"}}	{{.State}}	{{.Names}}	{{.ID}}`)
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	var containers []containerInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		containers = append(containers, containerInfo{
			localFolder: parts[0],
			state:       parts[1],
			name:        parts[2],
			id:          parts[3],
		})
	}

	return containers, nil
}

// betterState returns true if newState should replace oldState.
// "running" always wins over other states.
func betterState(newState, oldState string) bool {
	return newState == "running" && oldState != "running"
}

// normalizeWSLPath converts a WSL UNC path (e.g. \\wsl.localhost\Distro\home\user\proj)
// to a Linux path (/home/user/proj). Non-UNC paths are returned unchanged.
func normalizeWSLPath(path string) string {
	// Match \\wsl.localhost\Distro\... or \\wsl$\Distro\...
	if !strings.HasPrefix(path, `\\wsl`) {
		return path
	}
	// Split: ["", "", "wsl.localhost", "Distro", "home\user\proj"]
	parts := strings.SplitN(path, `\`, 5)
	if len(parts) < 5 {
		return path
	}
	return "/" + strings.ReplaceAll(parts[4], `\`, "/")
}

// mapDockerState maps docker container state strings to our state enum.
func mapDockerState(dockerState string) string {
	switch dockerState {
	case "running":
		return "running"
	case "exited", "created":
		return "stopped"
	case "dead", "removing":
		return "error"
	default:
		return "stopped"
	}
}

// DetectAutoMode reads the project's .devcontainer/devcontainer.json and checks
// if postAttachCommand contains --dangerously-skip-permissions.
func (m *Manager) DetectAutoMode(projectPath string) bool {
	data, err := os.ReadFile(filepath.Join(projectPath, ".devcontainer", "devcontainer.json"))
	if err != nil {
		return false
	}
	var config struct {
		PostAttachCommand json.RawMessage `json:"postAttachCommand"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}
	return strings.Contains(string(config.PostAttachCommand), "--dangerously-skip-permissions")
}

// CLIAvailable checks if the devcontainer CLI is installed and in PATH.
func (m *Manager) CLIAvailable() bool {
	_, err := m.cmd.Output("devcontainer", "--help")
	return err == nil
}

// maxStderrLines is the maximum number of stderr lines to include in error messages.
const maxStderrLines = 30

// EnsureRunning starts the devcontainer for the given project path.
// If the container is already running, devcontainer up is a no-op.
func (m *Manager) EnsureRunning(projectPath string) error {
	stdout, stderr, err := m.cmd.OutputWithStderr("devcontainer", "up", "--workspace-folder", projectPath)
	if err != nil {
		return fmt.Errorf("devcontainer up for %s: %s", projectPath, formatBuildError(stdout, stderr))
	}
	return nil
}

// devcontainerOutput represents the JSON response from devcontainer CLI.
type devcontainerOutput struct {
	Outcome     string `json:"outcome"`
	Message     string `json:"message"`
	Description string `json:"description"`
}

// formatBuildError builds a human-readable error from devcontainer CLI output.
// It parses the JSON stdout for the description and appends the tail of stderr
// (the Docker build log) so users can see which build step failed.
func formatBuildError(stdout, stderr []byte) string {
	var sb strings.Builder

	// Try to extract description from devcontainer JSON output
	var dcOut devcontainerOutput
	if json.Unmarshal(stdout, &dcOut) == nil && dcOut.Description != "" {
		sb.WriteString(dcOut.Description)
	} else if len(stdout) > 0 {
		sb.WriteString(strings.TrimSpace(string(stdout)))
	} else {
		sb.WriteString("unknown error")
	}

	// Append tail of stderr (Docker build log)
	if stderrTail := tailLines(string(stderr), maxStderrLines); stderrTail != "" {
		sb.WriteString("\n--- build log (last ")
		sb.WriteString(fmt.Sprintf("%d", min(countLines(string(stderr)), maxStderrLines)))
		sb.WriteString(" lines) ---\n")
		sb.WriteString(stderrTail)
	}

	return sb.String()
}

// tailLines returns the last n non-empty lines from s.
func tailLines(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// countLines returns the number of lines in s.
func countLines(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// Rebuild removes the existing container so it can be recreated.
// The actual devcontainer up will happen inside the tmux session when
// the user creates a new session for this project.
func (m *Manager) Rebuild(projectPath string) error {
	containers, err := m.listContainers()
	if err != nil {
		return fmt.Errorf("find container for rebuild: %w", err)
	}

	for _, c := range containers {
		if normalizeWSLPath(c.localFolder) == projectPath {
			if err := m.cmd.Run("docker", "rm", "-f", c.id); err != nil {
				return fmt.Errorf("remove container %s: %w", c.id, err)
			}
			return nil
		}
	}

	return nil
}

// Stop stops the devcontainer for the given project path.
// If no container is found, it returns nil (no-op).
func (m *Manager) Stop(projectPath string) error {
	containers, err := m.listContainers()
	if err != nil {
		return fmt.Errorf("find container for stop: %w", err)
	}

	for _, c := range containers {
		if normalizeWSLPath(c.localFolder) == projectPath {
			if err := m.cmd.Run("docker", "stop", c.id); err != nil {
				return fmt.Errorf("stop container %s: %w", c.id, err)
			}
			return nil
		}
	}

	return nil
}

// CleanupResult holds the result of a full cleanup operation.
type CleanupResult struct {
	ContainerErr error
	ImageErr     error
}

// Cleanup removes the container and its image for the given project path.
// Both operations are best-effort: container failure does not block image removal.
func (m *Manager) Cleanup(projectPath string) CleanupResult {
	var result CleanupResult

	containerID, err := m.findContainerID(projectPath)
	if err != nil {
		result.ContainerErr = err
		return result
	}
	if containerID == "" {
		return result
	}

	// Get image ID before removing container
	imageID := m.getContainerImage(containerID)

	// Remove container
	if err := m.cmd.Run("docker", "rm", "-f", containerID); err != nil {
		result.ContainerErr = fmt.Errorf("remove container %s: %w", containerID, err)
	}

	// Remove image (best-effort)
	if imageID != "" {
		if err := m.cmd.Run("docker", "rmi", imageID); err != nil {
			result.ImageErr = fmt.Errorf("remove image %s: %w", imageID, err)
		}
	}

	return result
}

// RemoveContainer removes only the container for the given project path.
func (m *Manager) RemoveContainer(projectPath string) error {
	containerID, err := m.findContainerID(projectPath)
	if err != nil {
		return err
	}
	if containerID == "" {
		return nil
	}
	if err := m.cmd.Run("docker", "rm", "-f", containerID); err != nil {
		return fmt.Errorf("remove container %s: %w", containerID, err)
	}
	return nil
}

// RemoveImage removes only the image for the container associated with the given project path.
func (m *Manager) RemoveImage(projectPath string) error {
	containerID, err := m.findContainerID(projectPath)
	if err != nil {
		return err
	}
	if containerID == "" {
		return nil
	}
	imageID := m.getContainerImage(containerID)
	if imageID == "" {
		return nil
	}
	if err := m.cmd.Run("docker", "rmi", imageID); err != nil {
		return fmt.Errorf("remove image %s: %w", imageID, err)
	}
	return nil
}

// findContainerID returns the container ID for the given project path, or "" if not found.
func (m *Manager) findContainerID(projectPath string) (string, error) {
	containers, err := m.listContainers()
	if err != nil {
		return "", fmt.Errorf("find container: %w", err)
	}
	for _, c := range containers {
		if normalizeWSLPath(c.localFolder) == projectPath {
			return c.id, nil
		}
	}
	return "", nil
}

// getContainerImage returns the image ID for a container, or "" on error.
func (m *Manager) getContainerImage(containerID string) string {
	out, err := m.cmd.Output("docker", "inspect", "--format", "{{.Image}}", containerID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ValidateConfig runs devcontainer read-configuration to pre-validate the config.
func (m *Manager) ValidateConfig(projectPath string) error {
	_, stderr, err := m.cmd.OutputWithStderr(
		"devcontainer", "read-configuration", "--workspace-folder", projectPath)
	if err != nil {
		errMsg := strings.TrimSpace(string(stderr))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Errorf("validate config: %s", errMsg)
	}
	return nil
}

// VerifyContainer runs post-build verification inside the container.
// It checks that essential directories and runtimes are available.
func (m *Manager) VerifyContainer(projectPath string, files GeneratedFiles) error {
	containerID, err := m.findContainerID(projectPath)
	if err != nil {
		return fmt.Errorf("verify container: %w", err)
	}
	if containerID == "" {
		return fmt.Errorf("verify container: no container found for %s", projectPath)
	}

	// Check /workspace exists
	if _, err := m.execInContainer(containerID, "test -d /workspace"); err != nil {
		return fmt.Errorf("verify container: /workspace directory not found")
	}

	// Check /home/node/.claude exists
	if _, err := m.execInContainer(containerID, "test -d /home/node/.claude"); err != nil {
		return fmt.Errorf("verify container: /home/node/.claude directory not found")
	}

	// Check init-firewall.sh is executable
	if _, hasFirewall := files["init-firewall.sh"]; hasFirewall {
		if _, err := m.execInContainer(containerID, "test -x /usr/local/bin/init-firewall.sh"); err != nil {
			return fmt.Errorf("verify container: init-firewall.sh not executable")
		}
	}

	// Language runtime detection from Dockerfile
	if df, ok := files["Dockerfile"]; ok {
		if err := m.verifyLanguageRuntimes(containerID, df); err != nil {
			return err
		}
	}

	return nil
}

// verifyLanguageRuntimes detects languages from the Dockerfile and verifies their runtimes.
func (m *Manager) verifyLanguageRuntimes(containerID, dockerfile string) error {
	lower := strings.ToLower(dockerfile)

	type langCheck struct {
		patterns []string
		cmd      string
		name     string
	}

	checks := []langCheck{
		{[]string{"go_version", "go.dev", "golang"}, "go version", "Go"},
		{[]string{"from node:", "node_version", "nvm"}, "node --version", "Node.js"},
		{[]string{"python", "python_version"}, "python3 --version", "Python"},
		{[]string{"rustup", "cargo", "rustc"}, "rustc --version", "Rust"},
	}

	for _, check := range checks {
		matched := false
		for _, p := range check.patterns {
			if strings.Contains(lower, p) {
				matched = true
				break
			}
		}
		if matched {
			if _, err := m.execInContainer(containerID, check.cmd); err != nil {
				return fmt.Errorf("verify container: %s runtime not available (%s failed)", check.name, check.cmd)
			}
		}
	}

	return nil
}

// execInContainer runs a command inside a container and returns its output.
func (m *Manager) execInContainer(containerID, cmd string) (string, error) {
	out, err := m.cmd.Output("docker", "exec", containerID, "sh", "-c", cmd)
	return string(out), err
}

// CleanupAll removes the container and its image, returning the first error encountered.
// This satisfies the ContainerRunner interface's Cleanup method return type requirement.
func (m *Manager) CleanupAll(projectPath string) error {
	result := m.Cleanup(projectPath)
	if result.ContainerErr != nil {
		return result.ContainerErr
	}
	return result.ImageErr
}

// GetContainerLogs returns the last N lines of logs for the given container.
func (m *Manager) GetContainerLogs(containerID string, lines int) (string, error) {
	out, err := m.cmd.Output("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerID)
	if err != nil {
		return "", fmt.Errorf("docker logs for %s: %w", containerID, err)
	}
	return string(out), nil
}

// GetClaudeVersion returns the Claude Code version installed inside the container.
// The output of "claude --version" varies (e.g. "2.1.100 (Claude Code)") —
// ParseClaudeVersion extracts the semver portion.
func (m *Manager) GetClaudeVersion(containerID string) (string, error) {
	out, err := m.execInContainer(containerID, "claude --version")
	if err != nil {
		return "", fmt.Errorf("get claude version in %s: %w", containerID, err)
	}
	version := ParseClaudeVersion(strings.TrimSpace(out))
	if version == "" {
		return "", fmt.Errorf("cannot parse version from %q", out)
	}
	return version, nil
}

// UpdateClaude updates Claude Code inside the container to the latest version.
// Returns the version string after update.
func (m *Manager) UpdateClaude(containerID string) (string, error) {
	if _, err := m.execInContainer(containerID, "sudo npm install -g @anthropic-ai/claude-code@latest"); err != nil {
		return "", fmt.Errorf("update claude in %s: %w", containerID, err)
	}
	return m.GetClaudeVersion(containerID)
}

var (
	claudeVersionCache   string
	claudeVersionCacheAt time.Time
	claudeVersionCacheMu sync.Mutex
)

const claudeVersionCacheTTL = 1 * time.Hour

// GetLatestClaudeVersionCached returns the latest Claude Code version, cached for 1 hour.
func GetLatestClaudeVersionCached(ctx context.Context) string {
	claudeVersionCacheMu.Lock()
	defer claudeVersionCacheMu.Unlock()
	if claudeVersionCache != "" && time.Since(claudeVersionCacheAt) < claudeVersionCacheTTL {
		return claudeVersionCache
	}
	v, err := FetchLatestClaudeVersion(ctx)
	if err != nil {
		return claudeVersionCache // return stale value on error
	}
	claudeVersionCache = v
	claudeVersionCacheAt = time.Now()
	return v
}

// FetchLatestClaudeVersion queries the npm registry for the latest Claude Code version.
func FetchLatestClaudeVersion(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://registry.npmjs.org/@anthropic-ai/claude-code/latest", nil)
	if err != nil {
		return "", fmt.Errorf("create npm request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch npm registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned %d", resp.StatusCode)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return "", fmt.Errorf("parse npm response: %w", err)
	}
	version := ParseClaudeVersion(pkg.Version)
	if version == "" {
		return "", fmt.Errorf("npm registry returned unparseable version: %q", pkg.Version)
	}
	return version, nil
}

var claudeVersionArgRe = regexp.MustCompile(`"CLAUDE_CODE_VERSION"\s*:\s*"[^"]*"`)

// PinClaudeVersion updates the CLAUDE_CODE_VERSION build arg in a project's
// devcontainer.json to the given version string (e.g. "2.3.5").
// This busts Docker build cache so that `devcontainer up` re-runs npm install.
func PinClaudeVersion(projectPath, version string) error {
	dcPath := filepath.Join(projectPath, ".devcontainer", "devcontainer.json")

	data, err := os.ReadFile(dcPath)
	if err != nil {
		return fmt.Errorf("read devcontainer.json: %w", err)
	}

	replacement := fmt.Sprintf(`"CLAUDE_CODE_VERSION": "%s"`, version)
	updated := claudeVersionArgRe.ReplaceAll(data, []byte(replacement))

	if bytes.Equal(data, updated) {
		return nil // already pinned to this version
	}

	if err := os.WriteFile(dcPath, updated, 0o644); err != nil {
		return fmt.Errorf("write devcontainer.json: %w", err)
	}

	return nil
}
