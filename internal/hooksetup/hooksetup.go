// Package hooksetup extracts Piper hook scripts to the local filesystem
// and merges Claude Code hook configuration into a project's settings.
//
// Piper calls ExtractScripts on startup. Each new session then goes through the
// same merge: InjectSettings points at the extracted scripts for a local
// session, and InjectContainerSettings at the scripts baked into the image for
// a DevContainer session, whose /workspace is the same project directory.
package hooksetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The extracted script names also identify Piper's own entries when merging
// into a settings file that holds the user's configuration too.
const (
	hookScript       = "piper-hook.mjs"
	statuslineScript = "piper-statusline.mjs"

	// containerScriptDir is where base.Dockerfile installs the scripts.
	containerScriptDir = "/usr/local/bin"
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

	files := []string{hookScript, statuslineScript}
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
	return filepath.Join(dir, hookScript),
		filepath.Join(dir, statuslineScript),
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

// InjectSettings adds Piper's hooks and status line to
// {workDir}/.claude/settings.local.json and leaves the rest of the file alone:
// the user's permissions, other keys and own hooks survive, and a status line
// the user set stays in place. Running it again changes nothing. A file that
// cannot be merged safely is left untouched and reported as an error.
func InjectSettings(workDir string) error {
	hookPath, statuslinePath, err := ScriptPaths()
	if err != nil {
		return err
	}
	return mergeIntoProject(workDir, hooksConfig(hookPath, statuslinePath))
}

// InjectContainerSettings does the same for a DevContainer session, with the
// script paths inside the container. A project whose .devcontainer has no Piper
// scripts built its image some other way, so hooks added there would only fail
// on every event; it is left alone.
func InjectContainerSettings(projectPath string) error {
	_, err := os.Stat(filepath.Join(projectPath, ".devcontainer", hookScript))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check DevContainer scripts in %s: %w", projectPath, err)
	}
	return mergeIntoProject(projectPath, hooksConfig(
		containerScriptDir+"/"+hookScript,
		containerScriptDir+"/"+statuslineScript,
	))
}

func mergeIntoProject(workDir string, config map[string]any) error {
	dir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir in %s: %w", workDir, err)
	}

	dst := filepath.Join(dir, "settings.local.json")
	// Write through a symlink rather than replacing it.
	if real, err := filepath.EvalSymlinks(dst); err == nil {
		dst = real
	}

	original, err := os.ReadFile(dst)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", dst, err)
	}

	settings, err := parseSettings(original)
	if err != nil {
		return fmt.Errorf("%s left unchanged: %w", dst, err)
	}
	if err := mergeSettings(settings, config); err != nil {
		return fmt.Errorf("%s left unchanged: %w", dst, err)
	}

	data, err := encodeSettings(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if bytes.Equal(data, original) {
		return nil
	}
	return writeFileAtomic(dst, data)
}

// parseSettings decodes an existing settings file. A missing, blank or null
// file counts as empty; anything else that is not a JSON object is an error.
func parseSettings(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	if !json.Valid(data) {
		return nil, errors.New("not valid JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep numbers exactly as the user wrote them
	var settings map[string]any
	if err := dec.Decode(&settings); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if settings == nil {
		return map[string]any{}, nil
	}
	return settings, nil
}

// mergeSettings adds Piper's configuration to the user's settings in place.
// For each event Piper listens to, stale Piper hooks are dropped and the
// current one goes last, so the user's own hooks keep their order.
func mergeSettings(user, piper map[string]any) error {
	if user["hooks"] == nil {
		user["hooks"] = map[string]any{}
	}
	hooks, ok := user["hooks"].(map[string]any)
	if !ok {
		return errors.New(`"hooks" is not an object`)
	}

	for event, entry := range piper["hooks"].(map[string]any) {
		var groups []any
		if hooks[event] != nil {
			if groups, ok = hooks[event].([]any); !ok {
				return fmt.Errorf(`"hooks.%s" is not an array`, event)
			}
		}
		hooks[event] = append(withoutPiperHooks(groups), entry.([]any)...)
	}

	if current := user["statusLine"]; current == nil || isPiperCommand(current, statuslineScript) {
		user["statusLine"] = piper["statusLine"]
	}
	return nil
}

// withoutPiperHooks removes Piper's hooks from a list of matcher groups. A
// group left empty by the removal is dropped; entries of an unknown shape are
// the user's and are kept as they are.
func withoutPiperHooks(groups []any) []any {
	kept := make([]any, 0, len(groups)+1)
	for _, g := range groups {
		group, _ := g.(map[string]any)
		inner, isList := group["hooks"].([]any)
		if !isList {
			kept = append(kept, g)
			continue
		}
		remaining := make([]any, 0, len(inner))
		for _, hook := range inner {
			if !isPiperCommand(hook, hookScript) {
				remaining = append(remaining, hook)
			}
		}
		switch {
		case len(remaining) == len(inner):
			kept = append(kept, g)
		case len(remaining) > 0:
			group["hooks"] = remaining
			kept = append(kept, group)
		}
	}
	return kept
}

// isPiperCommand reports whether a hook or status line entry runs the script.
func isPiperCommand(entry any, script string) bool {
	m, _ := entry.(map[string]any)
	cmd, _ := m["command"].(string)
	return strings.Contains(cmd, script)
}

func encodeSettings(settings map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep && and > in the user's commands readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFileAtomic replaces dst in one step, so a crash mid-write cannot lose
// the user's settings. An existing file keeps its permissions.
func writeFileAtomic(dst string, data []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(dst); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".settings.local.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", dst, err)
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return fmt.Errorf("replace %s: %w", dst, err)
	}
	return nil
}
