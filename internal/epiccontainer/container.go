package epiccontainer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImageName is the Docker image used for epic sessions.
const ImageName = "piper-epic:latest"

// RunConfig holds parameters for starting a containerized epic session.
type RunConfig struct {
	SessionName string // e.g. "pplan-epic-a1b2c"
	ClaudeHome  string // ~/.claude path on host
	WorkDir     string // temp dir with CLAUDE.md
}

// BuildDockerArgs constructs the full `docker run` argument list.
func BuildDockerArgs(cfg RunConfig) []string {
	homeDir := filepath.Dir(cfg.ClaudeHome)
	clipDir := filepath.Join(os.TempDir(), "piper-clipboard")
	innerCmd := "exec claude --dangerously-skip-permissions"
	return []string{
		"run", "--rm", "-it",
		"--name", cfg.SessionName,
		"--user", "node",
		"--network", "host",
		"-v", cfg.ClaudeHome + ":/home/node/.claude",
		"-v", homeDir + "/.claude.json:/home/node/.claude.json",
		"-v", cfg.WorkDir + "/sessions:/home/node/.claude/sessions",
		"-v", cfg.WorkDir + "/projects:/home/node/.claude/projects",
		"-v", cfg.WorkDir + ":/workspace",
		"-v", clipDir + ":" + clipDir,
		"-w", "/workspace",
		"-e", "PIPER_SESSION=" + cfg.SessionName,
		"-e", "CLAUDE_CONFIG_DIR=/home/node/.claude",
		"-e", "NODE_OPTIONS=--max-old-space-size=4096",
		ImageName,
		"bash", "-c", innerCmd,
	}
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildTmuxScript wraps docker run args into a bash script suitable for tmux.
// Each argument is shell-quoted to prevent metacharacters (>, &&, etc.) in
// inner commands from being interpreted by the outer tmux bash.
func BuildTmuxScript(dockerArgs []string) string {
	quoted := make([]string, len(dockerArgs))
	for i, arg := range dockerArgs {
		quoted[i] = shellQuote(arg)
	}
	return fmt.Sprintf(
		`docker %s || { echo "=== 容器啟動失敗，按 Enter 關閉 ==="; read; }`,
		strings.Join(quoted, " "),
	)
}
