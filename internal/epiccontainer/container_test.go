package epiccontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDockerArgs_包含必要參數(t *testing.T) {
	cfg := RunConfig{
		SessionName: "pplan-epic-a1b2c",
		ClaudeHome:  "/home/user/.claude",
		WorkDir:     "/tmp/piper-epic-pplan-epic-a1b2c",
	}
	args := BuildDockerArgs(cfg)

	joined := strings.Join(args, " ")

	clipDir := filepath.Join(os.TempDir(), "piper-clipboard")
	checks := []string{
		"run", "--rm", "-it",
		"--name pplan-epic-a1b2c",
		"--user node",
		"--network host",
		"-v /home/user/.claude:/home/node/.claude",
		"-v /home/user/.claude.json:/home/node/.claude.json",
		"-v /tmp/piper-epic-pplan-epic-a1b2c/sessions:/home/node/.claude/sessions",
		"-v /tmp/piper-epic-pplan-epic-a1b2c/projects:/home/node/.claude/projects",
		"-v /tmp/piper-epic-pplan-epic-a1b2c:/workspace",
		"-v " + clipDir + ":" + clipDir,
		"-w /workspace",
		"-e PIPER_SESSION=pplan-epic-a1b2c",
		"-e CLAUDE_CONFIG_DIR=/home/node/.claude",
		"-e NODE_OPTIONS=--max-old-space-size=4096",
		ImageName,
		"--dangerously-skip-permissions",
	}

	for _, check := range checks {
		if !strings.Contains(joined, check) {
			t.Errorf("expected args to contain %q, got: %s", check, joined)
		}
	}
}

func TestBuildTmuxScript_包含fallback(t *testing.T) {
	args := []string{"run", "--rm", "-it", "--name", "test", ImageName}
	script := BuildTmuxScript(args)

	if !strings.Contains(script, "docker") {
		t.Error("expected docker command")
	}
	if !strings.Contains(script, "容器啟動失敗") {
		t.Error("expected failure fallback message")
	}
}

func TestBuildTmuxScript_innerCmd含元字元時正確引號保護(t *testing.T) {
	cfg := RunConfig{
		SessionName: "pdisp-epic-427de",
		ClaudeHome:  "/home/node/.claude",
		WorkDir:     "/tmp/piper-epic-pdisp-epic-427de",
	}
	args := BuildDockerArgs(cfg)
	script := BuildTmuxScript(args)

	// innerCmd 應被 single quote 包裹，使 shell 元字元不被 outer bash 解析
	if !strings.Contains(script, "'exec claude --dangerously-skip-permissions'") {
		t.Errorf("innerCmd 未被正確引號包裹\nscript: %s", script)
	}
	// bash 和 -c 也應各自被引號包裹
	if !strings.Contains(script, "'bash' '-c'") {
		t.Errorf("bash -c 參數未被引號包裹\nscript: %s", script)
	}
}
