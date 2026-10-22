package hooksetup

import (
	"encoding/json"
	"os"
	"path/filepath"
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
