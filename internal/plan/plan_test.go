package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewEpicManager(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)
	if m.baseDir != dir {
		t.Errorf("expected baseDir %s, got %s", dir, m.baseDir)
	}
}

// --- generateEpicID ---

func TestGenerateEpicID_Format(t *testing.T) {
	id := generateEpicID()
	if !epicIDPattern.MatchString(id) {
		t.Errorf("generated ID %q does not match epic-XXXXX pattern", id)
	}
}

func TestGenerateEpicID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateEpicID()
		if seen[id] {
			t.Fatalf("duplicate ID %q after %d iterations", id, i)
		}
		seen[id] = true
	}
}

// --- validateEpicID ---

func TestValidateEpicID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "epic-a1b2c", false},
		{"valid all digits", "epic-12345", false},
		{"valid all letters", "epic-abcde", false},
		{"missing prefix", "abcde", true},
		{"too short", "epic-a1b2", true},
		{"too long", "epic-a1b2c3", true},
		{"uppercase hex", "epic-A1B2C", true},
		{"empty", "", true},
		{"old slug format", "my-epic", true},
		{"unicode slug", "測試-Epic-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEpicID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEpicID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

// --- ListEpics ---

func TestListEpics_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestListEpics_BaseDirNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	m := NewEpicManager(dir)

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestListEpics_ReturnsIDAndDisplayName(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("我的 Epic (v1)")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}

	e := epics[0]
	if e.ID != id {
		t.Errorf("expected ID %q, got %q", id, e.ID)
	}
	if e.Name != "我的 Epic (v1)" {
		t.Errorf("expected Name %q, got %q", "我的 Epic (v1)", e.Name)
	}
	if e.Updated == "" {
		t.Error("expected non-empty Updated")
	}
}

func TestListEpics_IgnoresNonEpicDirs(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	// Create a valid epic
	if _, err := m.CreateEpic("valid"); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Create a directory that doesn't match epic-XXXXX pattern (old slug format)
	oldDir := filepath.Join(dir, "my-old-epic")
	os.MkdirAll(oldDir, 0o755)
	os.WriteFile(filepath.Join(oldDir, "story-map.yaml"), []byte("meta:\n  name: \"old\"\n"), 0o644)

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(epics) != 1 {
		t.Errorf("expected 1 epic (ignoring old format), got %d", len(epics))
	}
}

func TestListEpics_FallbackToIDWhenNameNotInYAML(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Overwrite YAML without a name field
	if err := m.WriteStoryMap(id, "meta:\n  owner: someone\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	// Should fall back to directory name (epic ID)
	if epics[0].Name != id {
		t.Errorf("expected fallback Name %q, got %q", id, epics[0].Name)
	}
}

// --- CreateEpic ---

func TestCreateEpic_ReturnsRandomID(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("my-epic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !epicIDPattern.MatchString(id) {
		t.Errorf("expected epic-XXXXX format, got %q", id)
	}

	// Check directory structure
	epicDir := filepath.Join(dir, id)
	assertFileExists(t, filepath.Join(epicDir, "story-map.yaml"))
	assertFileExists(t, filepath.Join(epicDir, "problem-statement.md"))
	assertDirExists(t, filepath.Join(epicDir, "adr"))
	assertDirExists(t, filepath.Join(epicDir, "spikes"))
}

func TestCreateEpic_DisplayNameInYAML(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("測試 Epic #1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ID should be random, not derived from title
	if !epicIDPattern.MatchString(id) {
		t.Errorf("expected epic-XXXXX format, got %q", id)
	}

	// Verify YAML has the original display name
	content, err := os.ReadFile(filepath.Join(dir, id, "story-map.yaml"))
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if !strings.Contains(string(content), `name: "測試 Epic #1"`) {
		t.Errorf("YAML should contain display name, got:\n%s", content)
	}
}

func TestCreateEpic_MultipleEpicsGetUniqueIDs(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id, err := m.CreateEpic("same name")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if ids[id] {
			t.Fatalf("duplicate ID %q at iteration %d", id, i)
		}
		ids[id] = true
	}
}

func TestCreateEpic_EmptyNameReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	_, err := m.CreateEpic("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestCreateEpic_WhitespaceOnlyNameReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	_, err := m.CreateEpic("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only name, got nil")
	}
}

func TestCreateEpic_TemplateHasDisplayNameReplaced(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("My Feature (v2)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// story-map.yaml has the display name
	content, err := os.ReadFile(filepath.Join(dir, id, "story-map.yaml"))
	if err != nil {
		t.Fatalf("read story-map.yaml: %v", err)
	}
	if !strings.Contains(string(content), `name: "My Feature (v2)"`) {
		t.Errorf("story-map.yaml should contain display name, got:\n%s", content)
	}

	// problem-statement.md has the display name
	content, err = os.ReadFile(filepath.Join(dir, id, "problem-statement.md"))
	if err != nil {
		t.Fatalf("read problem-statement.md: %v", err)
	}
	if !strings.Contains(string(content), "# My Feature (v2)") {
		t.Errorf("problem-statement.md should contain display name, got:\n%s", content)
	}
}

// --- DeleteEpic ---

func TestDeleteEpic_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("to-delete")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = m.DeleteEpic(id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	epicDir := filepath.Join(dir, id)
	if _, err := os.Stat(epicDir); !os.IsNotExist(err) {
		t.Errorf("expected epic dir to be removed, but still exists")
	}
}

func TestDeleteEpic_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	err := m.DeleteEpic("epic-00000")
	if err == nil {
		t.Fatal("expected error for missing epic, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestDeleteEpic_InvalidID(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	err := m.DeleteEpic("bad-id")
	if err == nil {
		t.Fatal("expected error for invalid ID, got nil")
	}
	if !strings.Contains(err.Error(), "invalid epic id") {
		t.Errorf("expected 'invalid epic id' in error, got: %v", err)
	}
}

// --- ReadStoryMap / WriteStoryMap ---

func TestReadStoryMap_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("reader")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	content, err := m.ReadStoryMap(id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(content, "meta:") {
		t.Errorf("expected YAML content with 'meta:', got:\n%s", content)
	}
}

func TestReadStoryMap_EpicNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	_, err := m.ReadStoryMap("epic-00000")
	if err == nil {
		t.Fatal("expected error for missing epic")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestReadStoryMap_InvalidID(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	_, err := m.ReadStoryMap("nope")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
	if !strings.Contains(err.Error(), "invalid epic id") {
		t.Errorf("expected 'invalid epic id' in error, got: %v", err)
	}
}

func TestWriteStoryMap_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("writer")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newContent := "meta:\n  name: updated\n"
	err = m.WriteStoryMap(id, newContent)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := m.ReadStoryMap(id)
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if got != newContent {
		t.Errorf("expected:\n%s\ngot:\n%s", newContent, got)
	}
}

func TestWriteStoryMap_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("atomic")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := m.WriteStoryMap(id, "new content"); err != nil {
		t.Fatalf("write: %v", err)
	}

	tmpFile := filepath.Join(dir, id, "story-map.yaml.tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be cleaned up")
	}
}

func TestWriteStoryMap_EpicNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	err := m.WriteStoryMap("epic-00000", "content")
	if err == nil {
		t.Fatal("expected error for missing epic")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// --- RenameEpic ---

func TestRenameEpic_Success(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("Original Name")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = m.RenameEpic(id, "New Name")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}

	// Verify name changed in YAML
	content, err := m.ReadStoryMap(id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(content, `name: "New Name"`) {
		t.Errorf("expected name to be updated, got:\n%s", content)
	}
	// Verify old name is gone
	if strings.Contains(content, "Original Name") {
		t.Errorf("old name should be replaced, got:\n%s", content)
	}

	// Verify directory unchanged
	assertDirExists(t, filepath.Join(dir, id))
}

func TestRenameEpic_ReflectsInList(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("Before")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := m.RenameEpic(id, "After"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	epics, err := m.ListEpics()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if epics[0].Name != "After" {
		t.Errorf("expected Name 'After', got %q", epics[0].Name)
	}
	if epics[0].ID != id {
		t.Errorf("expected ID %q unchanged, got %q", id, epics[0].ID)
	}
}

func TestRenameEpic_EmptyNameReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	id, err := m.CreateEpic("test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = m.RenameEpic(id, "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestRenameEpic_EpicNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewEpicManager(dir)

	err := m.RenameEpic("epic-00000", "New Name")
	if err == nil {
		t.Fatal("expected error for missing epic")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// --- extractDisplayName ---

func TestExtractDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		fallback string
		expected string
	}{
		{"double quoted", "meta:\n  name: \"My Epic\"", "fb", "My Epic"},
		{"single quoted", "meta:\n  name: 'My Epic'", "fb", "My Epic"},
		{"unquoted", "meta:\n  name: Hello World", "fb", "Hello World"},
		{"chinese", "meta:\n  name: \"測試 Epic\"", "fb", "測試 Epic"},
		{"no name field", "meta:\n  owner: someone", "fallback-id", "fallback-id"},
		{"empty name", "meta:\n  name: \"\"", "fb", "fb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDisplayName(tt.yaml, tt.fallback)
			if got != tt.expected {
				t.Errorf("extractDisplayName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Helpers
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("expected file %s to exist: %v", path, err)
		return
	}
	if info.IsDir() {
		t.Errorf("expected %s to be a file, but it's a directory", path)
	}
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("expected directory %s to exist: %v", path, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory, but it's a file", path)
	}
}

