// Package hooksetup extracts Piper hook scripts to the local filesystem
// and injects Claude Code hook configuration into workspace settings.
//
// DevContainer sessions get hooks via postStartCommand, but local sessions
// (opened directly on a working directory) need the same setup. This package
// bridges that gap: Piper calls ExtractScripts on startup, then InjectSettings
// each time a local session is created.
package hooksetup

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// scriptsDir returns ~/.local/share/piper/ (respects XDG_DATA_HOME).
func scriptsDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "piper"), nil
}

// ExtractScripts copies piper-hook.mjs and piper-statusline.mjs from the
// embedded template FS to ~/.local/share/piper/. Existing files are overwritten
// so that upgrades always deploy the latest version.
func ExtractScripts(templateFS fs.FS) error {
	dir, err := scriptsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create scripts dir %s: %w", dir, err)
	}

	files := []string{"piper-hook.mjs", "piper-statusline.mjs"}
	for _, name := range files {
		data, err := fs.ReadFile(templateFS, "templates/"+name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		dst := filepath.Join(dir, name)
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}
	return nil
}

// ScriptPaths returns the absolute paths to the extracted hook scripts.
func ScriptPaths() (hookMjs, statuslineMjs string, err error) {
	dir, err := scriptsDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "piper-hook.mjs"),
		filepath.Join(dir, "piper-statusline.mjs"),
		nil
}

// hooksConfig builds the Claude Code settings.local.json content with
// absolute paths pointing to the extracted scripts.
func hooksConfig(hookPath, statuslinePath string) map[string]any {
	hookCmd := "node " + hookPath
	statuslineCmd := "node " + statuslinePath

	hookEntry := []any{
		map[string]any{
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": hookCmd,
				},
			},
		},
	}

	events := []string{
		"UserPromptSubmit", "Stop", "StopFailure",
		"PreToolUse", "Notification", "SubagentStart",
	}
	hooks := make(map[string]any, len(events))
	for _, ev := range events {
		hooks[ev] = hookEntry
	}

	return map[string]any{
		"hooks": hooks,
		"statusLine": map[string]any{
			"type":    "command",
			"command": statuslineCmd,
		},
	}
}

// InjectSettings writes Piper hook configuration into
// {workDir}/.claude/settings.local.json. The file is overwritten entirely.
func InjectSettings(workDir string) error {
	hookPath, statuslinePath, err := ScriptPaths()
	if err != nil {
		return err
	}

	dir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir in %s: %w", workDir, err)
	}

	config := hooksConfig(hookPath, statuslinePath)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hooks config: %w", err)
	}

	dst := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(dst, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
