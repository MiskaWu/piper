package hooksetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestExtractScripts_WritesFiles(t *testing.T) {
	// Use a temp dir as XDG_DATA_HOME so we don't touch real filesystem.
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	fakeFS := fstest.MapFS{
		"templates/piper-hook.mjs":       {Data: []byte("// hook script")},
		"templates/piper-statusline.mjs": {Data: []byte("// statusline script")},
	}

	if err := ExtractScripts(fakeFS); err != nil {
		t.Fatalf("ExtractScripts: %v", err)
	}

	piperDir := filepath.Join(tmpDir, "piper")
	for _, name := range []string{"piper-hook.mjs", "piper-statusline.mjs"} {
		path := filepath.Join(piperDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("expected %s to be executable, got %o", name, info.Mode().Perm())
		}
	}

	// Verify content
	data, _ := os.ReadFile(filepath.Join(piperDir, "piper-hook.mjs"))
	if string(data) != "// hook script" {
		t.Errorf("unexpected hook content: %q", data)
	}
}

func TestExtractScripts_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	piperDir := filepath.Join(tmpDir, "piper")
	os.MkdirAll(piperDir, 0o755)
	os.WriteFile(filepath.Join(piperDir, "piper-hook.mjs"), []byte("old"), 0o644)

	fakeFS := fstest.MapFS{
		"templates/piper-hook.mjs":       {Data: []byte("new")},
		"templates/piper-statusline.mjs": {Data: []byte("new")},
	}

	if err := ExtractScripts(fakeFS); err != nil {
		t.Fatalf("ExtractScripts: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(piperDir, "piper-hook.mjs"))
	if string(data) != "new" {
		t.Errorf("expected overwrite, got %q", data)
	}
}

func TestInjectSettings_CreatesSettingsFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	workDir := filepath.Join(tmpDir, "myproject")
	os.MkdirAll(workDir, 0o755)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	settingsPath := filepath.Join(workDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected settings file: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify hooks key exists with all events
	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		t.Fatal("expected hooks object")
	}
	expectedEvents := []string{
		"UserPromptSubmit", "Stop", "StopFailure",
		"PreToolUse", "Notification", "SubagentStart",
	}
	for _, ev := range expectedEvents {
		if _, ok := hooks[ev]; !ok {
			t.Errorf("missing hook event: %s", ev)
		}
	}

	// Verify statusLine exists
	sl, ok := config["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("expected statusLine object")
	}
	if sl["type"] != "command" {
		t.Errorf("expected statusLine type=command, got %v", sl["type"])
	}

	// Verify paths are absolute
	cmd := sl["command"].(string)
	piperDir := filepath.Join(tmpDir, "piper")
	expectedCmd := "node " + filepath.Join(piperDir, "piper-statusline.mjs")
	if cmd != expectedCmd {
		t.Errorf("statusLine command = %q, want %q", cmd, expectedCmd)
	}
}

func TestInjectSettings_CreatesClaudeDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	workDir := filepath.Join(tmpDir, "newproject")
	// Don't create .claude dir — InjectSettings should create it.
	os.MkdirAll(workDir, 0o755)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	info, err := os.Stat(filepath.Join(workDir, ".claude"))
	if err != nil {
		t.Fatalf("expected .claude dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected .claude to be a directory")
	}
}

func TestHooksConfig_AbsolutePaths(t *testing.T) {
	config := hooksConfig("/opt/piper/hook.mjs", "/opt/piper/statusline.mjs")

	hooks := config["hooks"].(map[string]any)
	// Check one event's command
	entries := hooks["Stop"].([]any)
	inner := entries[0].(map[string]any)
	hookList := inner["hooks"].([]any)
	entry := hookList[0].(map[string]any)
	if entry["command"] != "node /opt/piper/hook.mjs" {
		t.Errorf("unexpected command: %v", entry["command"])
	}

	sl := config["statusLine"].(map[string]any)
	if sl["command"] != "node /opt/piper/statusline.mjs" {
		t.Errorf("unexpected statusLine command: %v", sl["command"])
	}
}

// seedSettings creates a work dir whose settings.local.json holds content.
func seedSettings(t *testing.T, content string) (workDir, path string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)
	workDir = filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(filepath.Join(workDir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(workDir, ".claude", "settings.local.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return workDir, path
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	return config
}

// commandsFor flattens the hook commands of one event, in order.
func commandsFor(t *testing.T, config map[string]any, event string) []string {
	t.Helper()
	var commands []string
	groups, _ := config["hooks"].(map[string]any)[event].([]any)
	for _, g := range groups {
		for _, h := range g.(map[string]any)["hooks"].([]any) {
			commands = append(commands, h.(map[string]any)["command"].(string))
		}
	}
	return commands
}

func TestInjectSettings_KeepsUserSettings(t *testing.T) {
	workDir, path := seedSettings(t, `{
  "permissions": {"allow": ["Bash(make test && make lint)"], "deny": ["Read(.env)"]},
  "model": "opus",
  "cleanupPeriodDays": 1.50
}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	config := readJSON(t, path)
	wantPermissions := map[string]any{
		"allow": []any{"Bash(make test && make lint)"},
		"deny":  []any{"Read(.env)"},
	}
	if !reflect.DeepEqual(config["permissions"], wantPermissions) {
		t.Errorf("permissions = %v, want %v", config["permissions"], wantPermissions)
	}
	if config["model"] != "opus" {
		t.Errorf("model = %v, want opus", config["model"])
	}
	if len(commandsFor(t, config, "Stop")) != 1 {
		t.Errorf("Stop hooks = %v, want Piper's only", commandsFor(t, config, "Stop"))
	}

	raw, _ := os.ReadFile(path)
	for _, literal := range []string{"make test && make lint", "1.50"} {
		if !strings.Contains(string(raw), literal) {
			t.Errorf("expected %q written literally in:\n%s", literal, raw)
		}
	}
}

func TestInjectSettings_KeepsUserHooksBeforePipers(t *testing.T) {
	workDir, path := seedSettings(t, `{"hooks": {
  "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./guard.sh"}]}],
  "SessionStart": [{"hooks": [{"type": "command", "command": "./hello.sh"}]}]
}}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	config := readJSON(t, path)
	commands := commandsFor(t, config, "PreToolUse")
	if len(commands) != 2 || commands[0] != "./guard.sh" || !strings.Contains(commands[1], hookScript) {
		t.Errorf("PreToolUse commands = %v, want the user's then Piper's", commands)
	}
	first := config["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if first["matcher"] != "Bash" {
		t.Errorf("user's matcher lost: %v", first)
	}
	if got := commandsFor(t, config, "SessionStart"); !reflect.DeepEqual(got, []string{"./hello.sh"}) {
		t.Errorf("SessionStart commands = %v, want the user's only", got)
	}
}

func TestInjectSettings_RepeatRunChangesNothing(t *testing.T) {
	workDir, path := seedSettings(t, `{"model": "opus", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./done.sh"}]}]}}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("first InjectSettings: %v", err)
	}
	first, _ := os.ReadFile(path)
	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("second InjectSettings: %v", err)
	}
	second, _ := os.ReadFile(path)

	if string(first) != string(second) {
		t.Errorf("second run changed the file:\n%s\n---\n%s", first, second)
	}
	if got := commandsFor(t, readJSON(t, path), "Stop"); len(got) != 2 {
		t.Errorf("Stop commands = %v, want one user hook and one Piper hook", got)
	}
}

func TestInjectSettings_ReplacesStalePiperEntries(t *testing.T) {
	workDir, path := seedSettings(t, `{
  "statusLine": {"type": "command", "command": "node /old/piper/piper-statusline.mjs"},
  "hooks": {"Stop": [
    {"hooks": [{"type": "command", "command": "./done.sh"}, {"type": "command", "command": "node /old/piper/piper-hook.mjs"}]},
    {"hooks": [{"type": "command", "command": "node /old/piper/piper-hook.mjs"}]}
  ]}
}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "/old/piper") {
		t.Errorf("stale Piper path survived:\n%s", raw)
	}
	config := readJSON(t, path)
	commands := commandsFor(t, config, "Stop")
	if len(commands) != 2 || commands[0] != "./done.sh" {
		t.Errorf("Stop commands = %v, want the user's then the current Piper hook", commands)
	}
	if groups := config["hooks"].(map[string]any)["Stop"].([]any); len(groups) != 2 {
		t.Errorf("Stop groups = %d, want the emptied group dropped", len(groups))
	}
}

func TestInjectSettings_KeepsUserStatusLine(t *testing.T) {
	workDir, path := seedSettings(t, `{"statusLine": {"type": "command", "command": "./my-status.sh"}}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	got := readJSON(t, path)["statusLine"].(map[string]any)["command"]
	if got != "./my-status.sh" {
		t.Errorf("statusLine command = %v, want the user's", got)
	}
}

func TestInjectSettings_LeavesUnmergeableFileUntouched(t *testing.T) {
	cases := map[string]string{
		"invalid JSON":     `{"permissions": {"allow": [}`,
		"trailing garbage": `{"model": "opus"} oops`,
		"top-level array":  `["not", "settings"]`,
		"hooks not object": `{"hooks": "nope"}`,
		"event not a list": `{"hooks": {"Stop": {"command": "./x.sh"}}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			workDir, path := seedSettings(t, content)

			if err := InjectSettings(workDir); err == nil {
				t.Error("expected an error")
			}

			got, _ := os.ReadFile(path)
			if string(got) != content {
				t.Errorf("file changed to:\n%s", got)
			}
		})
	}
}

func TestInjectSettings_BlankFileCountsAsEmpty(t *testing.T) {
	for _, content := range []string{"", " \n", "null"} {
		workDir, path := seedSettings(t, content)

		if err := InjectSettings(workDir); err != nil {
			t.Fatalf("InjectSettings on %q: %v", content, err)
		}
		if got := commandsFor(t, readJSON(t, path), "Stop"); len(got) != 1 {
			t.Errorf("content %q: Stop commands = %v, want Piper's only", content, got)
		}
	}
}

func TestInjectSettings_KeepsModeAndLeavesNoTempFile(t *testing.T) {
	workDir, path := seedSettings(t, `{"model": "opus"}`)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("expected only the settings file, got %v", entries)
	}
}

func TestInjectSettings_WritesThroughSymlink(t *testing.T) {
	workDir, path := seedSettings(t, `{"model": "opus"}`)
	target := filepath.Join(t.TempDir(), "shared-settings.json")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a regular file")
	}
	config := readJSON(t, target)
	if config["model"] != "opus" || len(commandsFor(t, config, "Stop")) != 1 {
		t.Errorf("target not merged: %v", config)
	}
}

// seedDevcontainerProject is seedSettings for a project Piper generated a
// DevContainer config for.
func seedDevcontainerProject(t *testing.T, content string) (workDir, path string) {
	t.Helper()
	workDir, path = seedSettings(t, content)
	dir := filepath.Join(workDir, ".devcontainer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hookScript), []byte("// hook"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workDir, path
}

func TestInjectContainerSettings_MergesWithContainerPaths(t *testing.T) {
	workDir, path := seedDevcontainerProject(t, `{
  "permissions": {"deny": ["Read(.env)"]},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./done.sh"}]}]}
}`)

	if err := InjectContainerSettings(workDir); err != nil {
		t.Fatalf("InjectContainerSettings: %v", err)
	}

	config := readJSON(t, path)
	wantPermissions := map[string]any{"deny": []any{"Read(.env)"}}
	if !reflect.DeepEqual(config["permissions"], wantPermissions) {
		t.Errorf("permissions = %v, want %v", config["permissions"], wantPermissions)
	}
	wantStop := []string{"./done.sh", "node /usr/local/bin/piper-hook.mjs"}
	if got := commandsFor(t, config, "Stop"); !reflect.DeepEqual(got, wantStop) {
		t.Errorf("Stop commands = %v, want %v", got, wantStop)
	}
	got := config["statusLine"].(map[string]any)["command"]
	if got != "node /usr/local/bin/piper-statusline.mjs" {
		t.Errorf("statusLine command = %v, want the path inside the container", got)
	}
}

func TestInjectContainerSettings_SkipsProjectsWithoutPiperScripts(t *testing.T) {
	const content = `{"model": "opus"}`
	workDir, path := seedSettings(t, content)

	if err := InjectContainerSettings(workDir); err != nil {
		t.Fatalf("InjectContainerSettings: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != content {
		t.Errorf("file changed to:\n%s", got)
	}

	bare := t.TempDir()
	if err := InjectContainerSettings(bare); err != nil {
		t.Fatalf("InjectContainerSettings on bare project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bare, ".claude")); !os.IsNotExist(err) {
		t.Errorf("expected no .claude dir to be created, stat err = %v", err)
	}
}

func TestInject_SwitchingBetweenHostAndContainerKeepsOneEntry(t *testing.T) {
	workDir, path := seedDevcontainerProject(t, `{}`)

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("InjectSettings: %v", err)
	}
	if err := InjectContainerSettings(workDir); err != nil {
		t.Fatalf("InjectContainerSettings: %v", err)
	}
	config := readJSON(t, path)
	if got := commandsFor(t, config, "Stop"); !reflect.DeepEqual(got, []string{"node /usr/local/bin/piper-hook.mjs"}) {
		t.Errorf("after container injection, Stop commands = %v", got)
	}

	if err := InjectSettings(workDir); err != nil {
		t.Fatalf("second InjectSettings: %v", err)
	}
	got := commandsFor(t, readJSON(t, path), "Stop")
	if len(got) != 1 || strings.Contains(got[0], containerScriptDir) {
		t.Errorf("after local injection, Stop commands = %v", got)
	}
}
