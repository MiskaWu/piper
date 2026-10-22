package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateIfNeeded_遷移epics和ideas(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(vaultDir, WithGitRunner(fg))
	if err := v.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Set up old epics dir
	oldEpicsDir := filepath.Join(t.TempDir(), "old-epics")
	epicDir := filepath.Join(oldEpicsDir, "epic-a1b2c")
	os.MkdirAll(epicDir, 0o755)
	os.WriteFile(filepath.Join(epicDir, "story-map.yaml"), []byte("meta:\n  name: Test\n"), 0o644)
	// Add a .git dir that should be skipped
	os.MkdirAll(filepath.Join(oldEpicsDir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(oldEpicsDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)

	// Set up old ideas
	ideas := []OldIdea{
		{ID: 1, Text: "first idea", CreatedAt: 1000000000},
		{ID: 2, Text: "second idea", EnrichedDesc: "enriched", CreatedAt: 1000000001},
	}

	if err := v.MigrateIfNeeded(oldEpicsDir, ideas, 2); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Verify epic was copied
	storyMapPath := filepath.Join(vaultDir, "epics", "epic-a1b2c", "story-map.yaml")
	if _, err := os.Stat(storyMapPath); err != nil {
		t.Errorf("expected epic story-map to exist: %v", err)
	}

	// Verify .git was NOT copied
	gitDir := filepath.Join(vaultDir, "epics", ".git")
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Error("expected .git to NOT be copied")
	}

	// Verify ideas were written
	idea1Path := filepath.Join(vaultDir, "inbox", "idea-00001.md")
	if _, err := os.Stat(idea1Path); err != nil {
		t.Errorf("expected idea-00001.md to exist: %v", err)
	}
	idea2Path := filepath.Join(vaultDir, "inbox", "idea-00002.md")
	if _, err := os.Stat(idea2Path); err != nil {
		t.Errorf("expected idea-00002.md to exist: %v", err)
	}

	// Verify enriched desc round-trips
	data, _ := os.ReadFile(idea2Path)
	parsed, err := parseIdeaFile(data)
	if err != nil {
		t.Fatalf("parse idea 2: %v", err)
	}
	if parsed.EnrichedDesc != "enriched" {
		t.Errorf("expected enriched desc 'enriched', got %q", parsed.EnrichedDesc)
	}

	// Verify marker exists
	markerPath := filepath.Join(vaultDir, ".migration-done")
	if _, err := os.Stat(markerPath); err != nil {
		t.Error("expected .migration-done marker to exist")
	}

	// Verify old epics dir was renamed
	if _, err := os.Stat(oldEpicsDir); !os.IsNotExist(err) {
		t.Error("expected old epics dir to be renamed")
	}
	bakDir := oldEpicsDir + ".bak"
	if _, err := os.Stat(bakDir); err != nil {
		t.Error("expected .bak dir to exist")
	}

	// Verify autoCommit was called
	found := false
	for _, msg := range fg.commits {
		if msg == "migrate: import epics and inbox" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected migrate commit, got %v", fg.commits)
	}
}

func TestMigrateIfNeeded_已遷移則跳過(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(vaultDir, WithGitRunner(fg))
	v.Init()

	// Write marker
	os.WriteFile(filepath.Join(vaultDir, ".migration-done"), []byte("done"), 0o644)

	fg.commits = nil
	if err := v.MigrateIfNeeded("", nil, 0); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(fg.commits) != 0 {
		t.Errorf("expected no commits when already migrated, got %v", fg.commits)
	}
}

func TestMigrateIfNeeded_無舊資料不報錯(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(vaultDir, WithGitRunner(fg))
	v.Init()

	if err := v.MigrateIfNeeded("/nonexistent", nil, 0); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Marker should still be written
	markerPath := filepath.Join(vaultDir, ".migration-done")
	if _, err := os.Stat(markerPath); err != nil {
		t.Error("expected .migration-done marker to exist even with no data")
	}
}

func TestMigrateIfNeeded_更新nextID(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(vaultDir, WithGitRunner(fg))
	v.Init()

	ideas := []OldIdea{
		{ID: 5, Text: "idea 5", CreatedAt: 1000000000},
	}

	if err := v.MigrateIfNeeded("", ideas, 10); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// New idea should get ID 11
	idea, err := v.AddIdea("new idea", nil)
	if err != nil {
		t.Fatalf("add idea: %v", err)
	}
	if idea.ID != 11 {
		t.Errorf("expected ID=11 after migration with nextID=10, got %d", idea.ID)
	}
}

func TestMigrateIfNeeded_含context的ideas(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(vaultDir, WithGitRunner(fg))
	v.Init()

	ideas := []OldIdea{
		{
			ID:        1,
			Text:      "idea with ctx",
			CreatedAt: 1000000000,
			Context: []OldIdeaContext{
				{SessionName: "ragent-test", Kind: "output", Detail: "running tests", Time: 1000},
			},
		},
	}

	if err := v.MigrateIfNeeded("", ideas, 1); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Read back and verify context
	idea, err := v.GetIdea(1)
	if err != nil {
		t.Fatalf("get idea: %v", err)
	}
	if idea == nil {
		t.Fatal("expected idea, got nil")
	}
	if len(idea.Context) != 1 {
		t.Fatalf("expected 1 context, got %d", len(idea.Context))
	}
	if idea.Context[0].Session != "ragent-test" {
		t.Errorf("expected session 'ragent-test', got %q", idea.Context[0].Session)
	}
}
