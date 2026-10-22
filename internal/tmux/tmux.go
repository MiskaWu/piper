package tmux

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Commander abstracts command execution for testability.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

// ExecCommander implements Commander using os/exec.
type ExecCommander struct{}

func (e *ExecCommander) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// OutputWithStderr captures stdout and stderr separately.
func (e *ExecCommander) OutputWithStderr(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	return stdout, stderr.Bytes(), err
}

func (e *ExecCommander) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// TmuxSession represents a tmux session.
type TmuxSession struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Created  string `json:"created"`
	Attached bool   `json:"attached"`
}

// Client wraps tmux CLI interactions.
// HookInjector is called when a local session is created to inject
// Claude Code hook configuration into the workspace directory.
type HookInjector func(workDir string) error

type Client struct {
	cmd          Commander
	piperHost    string       // non-loopback host IP for DevContainer → Piper API (e.g. "192.168.1.10:8622")
	localHost    string       // loopback host:port for local sessions (e.g. "127.0.0.1:8622")
	hookInjector HookInjector // optional: injects hooks into workspace settings for local sessions
}

// NewClient creates a new tmux Client with the given Commander.
func NewClient(cmd Commander) *Client {
	return &Client{cmd: cmd}
}

// SetPiperHost sets the Piper API address (host:port) for DevContainer injection.
// This is injected as PIPER_HOST env var so container hooks can POST directly to Piper.
func (c *Client) SetPiperHost(hostPort string) {
	c.piperHost = hostPort
}

// SetLocalHost sets the loopback address (host:port) used as PIPER_HOST
// for local (non-DevContainer) sessions.
func (c *Client) SetLocalHost(hostPort string) {
	c.localHost = hostPort
}

// SetHookInjector sets the function called to inject Claude Code hook
// configuration into the workspace directory when a local session is created.
func (c *Client) SetHookInjector(fn HookInjector) {
	c.hookInjector = fn
}

// Check verifies tmux is available by running `tmux -V`.
func (c *Client) Check() error {
	_, err := c.cmd.Output("tmux", "-V")
	if err != nil {
		return fmt.Errorf("tmux not available: %w", err)
	}
	return nil
}

// ListSessions lists tmux sessions filtered to those containing "claude" in the name.
// Returns an empty slice (not an error) if the tmux server is not running.
func (c *Client) ListSessions() ([]TmuxSession, error) {
	out, err := c.cmd.Output("tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_created}\t#{session_attached}")
	if err != nil {
		// tmux returns error when no server is running.
		// Check err message, stdout, and stderr (from exec.ExitError).
		combined := err.Error() + "\n" + string(out)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			combined += "\n" + string(exitErr.Stderr)
		}
		if strings.Contains(combined, "no server running") ||
			strings.Contains(combined, "no current client") ||
			strings.Contains(combined, "No such file or directory") {
			return []TmuxSession{}, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var sessions []TmuxSession
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		if !strings.Contains(name, "ragent") {
			continue
		}

		windows := 0
		fmt.Sscanf(parts[1], "%d", &windows)

		attached := parts[3] == "1"

		sessions = append(sessions, TmuxSession{
			Name:     name,
			Windows:  windows,
			Created:  parts[2],
			Attached: attached,
		})
	}
	if sessions == nil {
		sessions = []TmuxSession{}
	}
	return sessions, nil
}

// SanitizeProjectName converts a project directory name into a tmux-safe string.
// Lowercase, replace _ with -, strip non-[a-z0-9-], truncate to 20 chars.
func SanitizeProjectName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	result := b.String()
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// ExtractGroupFromName extracts the project group from a session name.
// Format: ragent-{project}-{shortID} → returns {project}.
// Old format ragent-{shortID} returns "".
// Also handles uniqueName suffix like ragent-{project}-{shortID}-2.
func ExtractGroupFromName(name string) string {
	if !strings.HasPrefix(name, "ragent-") {
		return ""
	}
	rest := name[len("ragent-"):]
	if rest == "" {
		return ""
	}

	// Split into parts
	parts := strings.Split(rest, "-")

	// Try to find the 10-char hex shortID segment.
	// It could be followed by a numeric uniqueName suffix (e.g. "-2").
	// Walk from the end to skip numeric suffixes, then check for hex.
	endIdx := len(parts) - 1

	// Skip trailing numeric suffix from uniqueName (e.g. "2", "10")
	if endIdx > 0 && isNumeric(parts[endIdx]) {
		endIdx--
	}

	// The segment at endIdx should be the 10-char hex shortID
	if endIdx < 0 || !isHex10(parts[endIdx]) {
		return ""
	}

	// Everything before the shortID is the project group
	if endIdx == 0 {
		// ragent-{shortID} — old format, no project segment
		return ""
	}
	return strings.Join(parts[:endIdx], "-")
}

func isHex10(s string) bool {
	if len(s) != 10 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GenerateShortID returns a random 5-byte hex string (10 chars, e.g. "a3f1b2c9d0").
// Used for generating unique, human-irrelevant session identifiers.
func GenerateShortID() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen in practice
		return "00000"
	}
	return hex.EncodeToString(b)
}

// uniqueName returns a session name that doesn't conflict with existing sessions.
// If the base name is already taken, it appends -2, -3, etc.
func (c *Client) uniqueName(base string) (string, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return "", fmt.Errorf("check existing sessions: %w", err)
	}
	existing := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		existing[s.Name] = true
	}
	if !existing[base] {
		return base, nil
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !existing[candidate] {
			return candidate, nil
		}
	}
}

// CreateSession creates a new detached tmux session running claude.
// If the name doesn't start with "ragent", it prepends "ragent-".
// If workDir is non-empty, the session starts in that directory.
// Returns the actual session name used (with prefix applied).
func (c *Client) CreateSession(name string, workDir string) (string, error) {
	if !strings.HasPrefix(name, "ragent") {
		name = "ragent-" + name
	}
	name, err := c.uniqueName(name)
	if err != nil {
		return "", err
	}

	// Inject hook settings before starting claude so it picks up the config.
	if workDir != "" && c.hookInjector != nil {
		if err := c.hookInjector(workDir); err != nil {
			// Non-fatal: session still works, just without hook-based status push.
			fmt.Fprintf(os.Stderr, "WARNING: failed to inject hooks for %s: %v\n", workDir, err)
		}
	}

	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	args = append(args, "claude")
	if err := c.cmd.Run("tmux", args...); err != nil {
		return "", fmt.Errorf("create session %s: %w", name, err)
	}
	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for session %s: %w", name, err)
	}
	if err := c.setSessionEnv(name); err != nil {
		return name, fmt.Errorf("set session env for %s: %w", name, err)
	}
	return name, nil
}

// CreateSessionWithCommand creates a new detached tmux session running a custom command.
// If the name doesn't start with "ragent", it prepends "ragent-".
// If workDir is non-empty, the session starts in that directory.
// Returns the actual session name used (with prefix applied).
func (c *Client) CreateSessionWithCommand(name, workDir, command string) (string, error) {
	if !strings.HasPrefix(name, "ragent") {
		name = "ragent-" + name
	}
	name, err := c.uniqueName(name)
	if err != nil {
		return "", err
	}
	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	args = append(args, command)
	if err := c.cmd.Run("tmux", args...); err != nil {
		return "", fmt.Errorf("create session %s: %w", name, err)
	}
	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for session %s: %w", name, err)
	}
	if err := c.setSessionEnv(name); err != nil {
		return name, fmt.Errorf("set session env for %s: %w", name, err)
	}
	return name, nil
}

// enableMouse enables mouse support for a tmux session so that
// scrolling works in the web terminal.
func (c *Client) enableMouse(name string) error {
	return c.cmd.Run("tmux", "set-option", "-t", name, "mouse", "on")
}

// setSessionEnv injects PIPER_SESSION and PIPER_HOST env vars into the tmux session
// so that Claude Code hooks can identify which piper session they belong to
// and where to POST status updates.
func (c *Client) setSessionEnv(name string) error {
	if err := c.cmd.Run("tmux", "set-environment", "-t", name, "PIPER_SESSION", name); err != nil {
		return err
	}
	// For local sessions, use loopback address; DevContainers get PIPER_HOST
	// via --remote-env in CreateDevcontainerSession.
	if c.localHost != "" {
		if err := c.cmd.Run("tmux", "set-environment", "-t", name, "PIPER_HOST", c.localHost); err != nil {
			return err
		}
	}
	return nil
}

// KillSession kills a tmux session by name.
func (c *Client) KillSession(name string) error {
	return c.cmd.Run("tmux", "kill-session", "-t", name)
}

// RenameSession renames a tmux session.
func (c *Client) RenameSession(old, new string) error {
	return c.cmd.Run("tmux", "rename-session", "-t", old, new)
}

// CapturePane captures the last 50 lines of a tmux pane.
func (c *Client) CapturePane(name string) (string, error) {
	out, err := c.cmd.Output("tmux", "capture-pane", "-t", name, "-p", "-S", "-50")
	if err != nil {
		return "", fmt.Errorf("capture pane %s: %w", name, err)
	}
	return string(out), nil
}

// GetSessionCreatedAt returns the creation time of a tmux session.
func (c *Client) GetSessionCreatedAt(name string) (time.Time, error) {
	out, err := c.cmd.Output("tmux", "display-message", "-t", name, "-p", "#{session_created}")
	if err != nil {
		return time.Time{}, fmt.Errorf("get session created time %s: %w", name, err)
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse session created time %s: %w", name, err)
	}
	return time.Unix(ts, 0), nil
}

// GetPanePID returns the PID of the process running in the tmux pane.
func (c *Client) GetPanePID(name string) (int, error) {
	out, err := c.cmd.Output("tmux", "display-message", "-t", name, "-p", "#{pane_pid}")
	if err != nil {
		return 0, fmt.Errorf("get pane PID %s: %w", name, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse pane PID %s: %w", name, err)
	}
	return pid, nil
}

// SendKeys sends keys to a tmux session followed by Enter.
func (c *Client) SendKeys(name, keys string) error {
	return c.cmd.Run("tmux", "send-keys", "-t", name, keys, "Enter")
}

// TypeKeys sends literal text to a tmux session without pressing Enter.
func (c *Client) TypeKeys(name, text string) error {
	return c.cmd.Run("tmux", "send-keys", "-t", name, "-l", text)
}

// HasSession checks if a tmux session with the given name exists.
func (c *Client) HasSession(name string) (bool, error) {
	err := c.cmd.Run("tmux", "has-session", "-t", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// ListPlanSessions lists tmux sessions with "pplan-" prefix.
// Returns an empty slice (not an error) if the tmux server is not running.
func (c *Client) ListPlanSessions() ([]TmuxSession, error) {
	out, err := c.cmd.Output("tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_created}\t#{session_attached}")
	if err != nil {
		combined := err.Error() + "\n" + string(out)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			combined += "\n" + string(exitErr.Stderr)
		}
		if strings.Contains(combined, "no server running") ||
			strings.Contains(combined, "no current client") ||
			strings.Contains(combined, "No such file or directory") {
			return []TmuxSession{}, nil
		}
		return nil, fmt.Errorf("list plan sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var sessions []TmuxSession
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, "pplan-") {
			continue
		}
		windows := 0
		fmt.Sscanf(parts[1], "%d", &windows)
		attached := parts[3] == "1"
		sessions = append(sessions, TmuxSession{
			Name:     name,
			Windows:  windows,
			Created:  parts[2],
			Attached: attached,
		})
	}
	if sessions == nil {
		sessions = []TmuxSession{}
	}
	return sessions, nil
}

// PlanSessionPrefixes are the tmux session prefixes for plan-type sessions.
var PlanSessionPrefixes = []string{"pplan-", "pdisp-", "pretro-"}

// ListAllPlanTypeSessions lists tmux sessions with pplan-/pdisp-/pretro- prefixes
// in a single tmux call. Returns an empty slice if the tmux server is not running.
func (c *Client) ListAllPlanTypeSessions() ([]TmuxSession, error) {
	out, err := c.cmd.Output("tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_created}\t#{session_attached}")
	if err != nil {
		combined := err.Error() + "\n" + string(out)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			combined += "\n" + string(exitErr.Stderr)
		}
		if strings.Contains(combined, "no server running") ||
			strings.Contains(combined, "no current client") ||
			strings.Contains(combined, "No such file or directory") {
			return []TmuxSession{}, nil
		}
		return nil, fmt.Errorf("list plan-type sessions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var sessions []TmuxSession
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		isPlanType := false
		for _, prefix := range PlanSessionPrefixes {
			if strings.HasPrefix(name, prefix) {
				isPlanType = true
				break
			}
		}
		if !isPlanType {
			continue
		}
		windows := 0
		fmt.Sscanf(parts[1], "%d", &windows)
		attached := parts[3] == "1"
		sessions = append(sessions, TmuxSession{
			Name:     name,
			Windows:  windows,
			Created:  parts[2],
			Attached: attached,
		})
	}
	if sessions == nil {
		sessions = []TmuxSession{}
	}
	return sessions, nil
}

// CleanOrphanPlanSessions kills plan sessions whose epic no longer exists.
// epicIDs is the set of valid epic slug IDs.
func (c *Client) CleanOrphanPlanSessions(epicIDs map[string]bool) error {
	sessions, err := c.ListPlanSessions()
	if err != nil {
		return fmt.Errorf("clean orphan plan sessions: %w", err)
	}
	for _, s := range sessions {
		slug := strings.TrimPrefix(s.Name, "pplan-")
		if !epicIDs[slug] {
			_ = c.KillSession(s.Name)
		}
	}
	return nil
}

// CreateEpicContainerSession creates a tmux session running a Docker container
// for plan/dispatch/retro epic sessions. The tmuxScript should be built via
// epiccontainer.BuildTmuxScript(). If the container fails to start, the session
// stays alive so the user can see the error.
func (c *Client) CreateEpicContainerSession(name, tmuxScript string) (string, error) {
	exists, _ := c.HasSession(name)
	if exists {
		return name, nil
	}

	if err := c.cmd.Run("tmux", "new-session", "-d", "-s", name, "bash", "-c", tmuxScript); err != nil {
		return "", fmt.Errorf("create epic container session %s: %w", name, err)
	}

	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for epic container session %s: %w", name, err)
	}

	return name, nil
}

// CreateNativeEpicSession creates a tmux session running claude directly
// (without Docker) for plan/dispatch/retro epic sessions. The workDir should
// be a temp directory containing CLAUDE.md prepared by epiccontainer.PrepareNativeWorkspace.
// Unlike the Docker variant, claude runs without --dangerously-skip-permissions
// so the user retains permission review control.
func (c *Client) CreateNativeEpicSession(name, workDir string) (string, error) {
	exists, _ := c.HasSession(name)
	if exists {
		return name, nil
	}

	if c.hookInjector != nil {
		if err := c.hookInjector(workDir); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: failed to inject hooks for epic session %s: %v\n", name, err)
		}
	}

	if err := c.cmd.Run("tmux", "new-session", "-d", "-s", name, "-c", workDir, "claude"); err != nil {
		return "", fmt.Errorf("create native epic session %s: %w", name, err)
	}

	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for native epic session %s: %w", name, err)
	}

	if err := c.setSessionEnv(name); err != nil {
		return name, fmt.Errorf("set session env for native epic session %s: %w", name, err)
	}

	return name, nil
}

// DCOption configures optional behavior for CreateDevcontainerSession.
type DCOption func(*dcOptions)

type dcOptions struct {
	resumeID string
}

// WithResumeID adds --resume <id> to the claude command inside the container.
func WithResumeID(id string) DCOption {
	return func(o *dcOptions) { o.resumeID = id }
}

// CreateDevcontainerSession creates a tmux session that runs devcontainer up
// followed by devcontainer exec -- claude. If devcontainer up fails, the session
// stays alive so the user can read the error output.
func (c *Client) CreateDevcontainerSession(name, projectPath string, autoMode bool, opts ...DCOption) (string, error) {
	var o dcOptions
	for _, fn := range opts {
		fn(&o)
	}
	if !strings.HasPrefix(name, "ragent") {
		name = "ragent-" + name
	}
	name, err := c.uniqueName(name)
	if err != nil {
		return "", err
	}
	claudeCmd := "claude"
	if autoMode {
		claudeCmd = "claude --dangerously-skip-permissions"
	}
	if o.resumeID != "" {
		claudeCmd += " --resume " + o.resumeID
	}
	clipMount := "type=bind,source=/tmp/piper-clipboard,target=/tmp/piper-clipboard"
	innerCmd := fmt.Sprintf(`bash -c 'exec %s'`, claudeCmd)
	remoteEnvs := fmt.Sprintf("--remote-env PIPER_SESSION=%s", name)
	if c.piperHost != "" {
		remoteEnvs += fmt.Sprintf(" --remote-env PIPER_HOST=%s", c.piperHost)
	}
	script := fmt.Sprintf(
		`devcontainer up --workspace-folder %s --mount %s && devcontainer exec %s --workspace-folder %s -- %s || { echo "=== 啟動失敗，按 Enter 關閉 ==="; read; }`,
		projectPath, clipMount, remoteEnvs, projectPath, innerCmd,
	)
	if err := c.cmd.Run("tmux", "new-session", "-d", "-s", name, "bash", "-c", script); err != nil {
		return "", fmt.Errorf("create devcontainer session %s: %w", name, err)
	}
	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for session %s: %w", name, err)
	}
	if err := c.setSessionEnv(name); err != nil {
		return name, fmt.Errorf("set session env for %s: %w", name, err)
	}
	return name, nil
}

// ListSetupSessions returns tmux sessions with "psetup-" prefix.
// Returns an empty slice (not an error) if the tmux server is not running.
func (c *Client) ListSetupSessions() ([]TmuxSession, error) {
	out, err := c.cmd.Output("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if strings.Contains(string(out), "no server running") || strings.Contains(err.Error(), "exit status") {
			return nil, nil
		}
		return nil, fmt.Errorf("list setup sessions: %w", err)
	}
	var sessions []TmuxSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "psetup-") {
			sessions = append(sessions, TmuxSession{Name: name})
		}
	}
	return sessions, nil
}

// CreateSetupSession creates a DevContainer setup session with "psetup-" prefix.
// If the session already exists (idempotent), it returns the existing name.
func (c *Client) CreateSetupSession(projectName, workDir string) (string, error) {
	name := "psetup-" + SanitizeProjectName(projectName)

	exists, _ := c.HasSession(name)
	if exists {
		return name, nil
	}

	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	if err := c.cmd.Run("tmux", args...); err != nil {
		return "", fmt.Errorf("create setup session %s: %w", name, err)
	}

	if err := c.enableMouse(name); err != nil {
		return name, fmt.Errorf("enable mouse for setup session %s: %w", name, err)
	}

	return name, nil
}

// CleanOrphanSetupSessions kills setup sessions whose project is no longer registered.
// validProjects maps sanitized project names to true.
func (c *Client) CleanOrphanSetupSessions(validProjects map[string]bool) error {
	sessions, err := c.ListSetupSessions()
	if err != nil {
		return fmt.Errorf("clean orphan setup sessions: %w", err)
	}
	for _, s := range sessions {
		slug := strings.TrimPrefix(s.Name, "psetup-")
		if !validProjects[slug] {
			_ = c.KillSession(s.Name)
		}
	}
	return nil
}
