package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_預設路徑使用XDG_DATA_HOME(t *testing.T) {
	// With XDG_DATA_HOME set
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	s := New("")
	want := "/tmp/xdg-data/piper/sessions.json"
	if s.Path() != want {
		t.Errorf("expected %q, got %q", want, s.Path())
	}
}

func TestNew_預設路徑fallback到home(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	s := New("")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "piper", "sessions.json")
	if s.Path() != want {
		t.Errorf("expected %q, got %q", want, s.Path())
	}
}

func TestStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load() from non-existent file: %v", err)
	}

	// Should have empty sessions
	meta := s.GetMeta("anything")
	if meta != nil {
		t.Errorf("expected nil for unknown session, got %+v", meta)
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "sessions.json")

	s1 := New(path)
	if err := s1.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := SessionMeta{
		Title:       "my-session",
		Description: "desc",
		Order:       3,
		ProjectPath: "/home/user/project",
	}
	if err := s1.SetMeta("sess1", want); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	// Create a new store instance and load from the same path
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load s2: %v", err)
	}

	got := s2.GetMeta("sess1")
	if got == nil {
		t.Fatal("expected session meta, got nil")
	}
	if got.Title != want.Title || got.Description != want.Description || got.Order != want.Order || got.ProjectPath != want.ProjectPath {
		t.Errorf("meta mismatch: got %+v, want %+v", got, want)
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.SetMeta("x", SessionMeta{Title: "x"}); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to not persist after save, but it exists")
	}

	// Final file should exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected final file to exist: %v", err)
	}
}

func TestStore_GetMeta_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := s.GetMeta("nonexistent")
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestStore_SetMeta(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	meta := SessionMeta{Title: "test", Description: "desc", Order: 1}
	if err := s.SetMeta("s1", meta); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	got := s.GetMeta("s1")
	if got == nil {
		t.Fatal("expected meta, got nil")
	}
	if got.Title != "test" || got.Description != "desc" || got.Order != 1 {
		t.Errorf("unexpected meta: %+v", got)
	}

	// Mutating returned copy should not affect store
	got.Title = "mutated"
	got2 := s.GetMeta("s1")
	if got2.Title != "test" {
		t.Errorf("GetMeta returned reference instead of copy")
	}
}

func TestStore_SetOrder(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Create some sessions
	s.SetMeta("a", SessionMeta{Title: "A"})
	s.SetMeta("b", SessionMeta{Title: "B"})
	s.SetMeta("c", SessionMeta{Title: "C"})

	if err := s.SetOrder([]string{"c", "a", "b"}); err != nil {
		t.Fatalf("SetOrder: %v", err)
	}

	cases := []struct {
		name      string
		wantOrder int
	}{
		{"c", 0},
		{"a", 1},
		{"b", 2},
	}
	for _, tc := range cases {
		got := s.GetMeta(tc.name)
		if got == nil {
			t.Fatalf("GetMeta(%q) = nil", tc.name)
		}
		if got.Order != tc.wantOrder {
			t.Errorf("GetMeta(%q).Order = %d, want %d", tc.name, got.Order, tc.wantOrder)
		}
	}
}

func TestStore_DeleteMeta(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	s.SetMeta("del", SessionMeta{Title: "to-delete"})
	if got := s.GetMeta("del"); got == nil {
		t.Fatal("expected meta before delete")
	}

	if err := s.DeleteMeta("del"); err != nil {
		t.Fatalf("DeleteMeta: %v", err)
	}

	if got := s.GetMeta("del"); got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestStore_AddProject_新增路徑(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := s.AddProject("/home/user/proj-a"); err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if err := s.AddProject("/home/user/proj-b"); err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	got := s.GetProjects()
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	if got[0] != "/home/user/proj-a" || got[1] != "/home/user/proj-b" {
		t.Errorf("unexpected projects: %v", got)
	}
}

func TestStore_AddProject_去重(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	s.AddProject("/home/user/proj-a")
	s.AddProject("/home/user/proj-a")

	got := s.GetProjects()
	if len(got) != 1 {
		t.Fatalf("expected 1 project (dedup), got %d", len(got))
	}
}

func TestStore_RemoveProject(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	s.AddProject("/home/user/proj-a")
	s.AddProject("/home/user/proj-b")

	if err := s.RemoveProject("/home/user/proj-a"); err != nil {
		t.Fatalf("RemoveProject: %v", err)
	}

	got := s.GetProjects()
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got))
	}
	if got[0] != "/home/user/proj-b" {
		t.Errorf("expected proj-b, got %s", got[0])
	}
}

func TestStore_RemoveProject_不存在的路徑不報錯(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := s.RemoveProject("/nonexistent"); err != nil {
		t.Fatalf("RemoveProject should not error for missing path: %v", err)
	}
}

func TestStore_GetProjects_回傳副本(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	s.AddProject("/home/user/proj")
	got := s.GetProjects()
	got[0] = "mutated"

	got2 := s.GetProjects()
	if got2[0] != "/home/user/proj" {
		t.Errorf("GetProjects returned reference instead of copy")
	}
}

func TestStore_SetProjectAutoMode_設定並持久化(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := New(path)
	s1.Load()
	s1.AddProject("/home/user/proj-a")

	autoMode := true
	if err := s1.SetProjectAutoMode("/home/user/proj-a", &autoMode); err != nil {
		t.Fatalf("SetProjectAutoMode: %v", err)
	}

	// Reload from disk
	s2 := New(path)
	s2.Load()
	got := s2.GetProjectAutoMode("/home/user/proj-a")
	if got == nil || *got != true {
		t.Errorf("expected autoMode=true after reload, got %v", got)
	}
}

func TestStore_GetProjectAutoMode_未設定回傳nil(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()
	s.AddProject("/home/user/proj-a")

	got := s.GetProjectAutoMode("/home/user/proj-a")
	if got != nil {
		t.Errorf("expected nil for unset autoMode, got %v", *got)
	}
}

func TestStore_GetProjectAutoMode_不存在的專案回傳nil(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	got := s.GetProjectAutoMode("/nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent project, got %v", *got)
	}
}

func TestStore_SetProjectAutoMode_設為false(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()
	s.AddProject("/home/user/proj")

	autoMode := false
	s.SetProjectAutoMode("/home/user/proj", &autoMode)
	got := s.GetProjectAutoMode("/home/user/proj")
	if got == nil || *got != false {
		t.Errorf("expected autoMode=false, got %v", got)
	}
}

func TestStore_SetProjectAutoMode_清除設定(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()
	s.AddProject("/home/user/proj")

	autoMode := true
	s.SetProjectAutoMode("/home/user/proj", &autoMode)
	s.SetProjectAutoMode("/home/user/proj", nil)
	got := s.GetProjectAutoMode("/home/user/proj")
	if got != nil {
		t.Errorf("expected nil after clearing, got %v", *got)
	}
}

func TestStore_舊格式相容_純字串陣列(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	// Write old format: projects as string array
	oldData := `{"sessions":{},"projects":["/home/user/proj-a","/home/user/proj-b"]}`
	os.WriteFile(path, []byte(oldData), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load old format: %v", err)
	}

	projects := s.GetProjects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects from old format, got %d", len(projects))
	}
	if projects[0] != "/home/user/proj-a" {
		t.Errorf("expected proj-a, got %s", projects[0])
	}

	// AutoMode should be nil for old format
	got := s.GetProjectAutoMode("/home/user/proj-a")
	if got != nil {
		t.Errorf("expected nil autoMode for old format, got %v", *got)
	}
}

func TestStore_Projects_持久化(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := New(path)
	s1.Load()
	s1.AddProject("/home/user/proj-a")

	s2 := New(path)
	s2.Load()
	got := s2.GetProjects()
	if len(got) != 1 || got[0] != "/home/user/proj-a" {
		t.Errorf("expected persisted project, got %v", got)
	}
}

func TestStore_GetSessionsByProject_正確分組回傳(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.SetMeta("sess-a", SessionMeta{Title: "A", ProjectPath: "/proj/alpha"})
	s.SetMeta("sess-b", SessionMeta{Title: "B", ProjectPath: "/proj/alpha"})
	s.SetMeta("sess-c", SessionMeta{Title: "C", ProjectPath: "/proj/beta"})
	s.SetMeta("sess-d", SessionMeta{Title: "D"}) // no project

	got := s.GetSessionsByProject()

	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	if len(got["/proj/alpha"]) != 2 {
		t.Errorf("expected 2 sessions for alpha, got %d", len(got["/proj/alpha"]))
	}
	if len(got["/proj/beta"]) != 1 {
		t.Errorf("expected 1 session for beta, got %d", len(got["/proj/beta"]))
	}
}

func TestStore_GetSessionsByProject_無匹配回傳空map(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.SetMeta("sess-a", SessionMeta{Title: "A"}) // no project

	got := s.GetSessionsByProject()
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestStore_GetMeta_回傳nil當session不存在(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := s.GetMeta("nonexistent"); got != nil {
		t.Errorf("expected nil for unknown session, got %+v", got)
	}
}

func TestStore_GetIdeas_讀取舊格式ideas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	// Write old format with ideas (for migration read)
	oldData := `{"sessions":{},"ideas":[{"id":1,"text":"old idea","createdAt":1000}],"nextIdeaId":1}`
	os.WriteFile(path, []byte(oldData), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ideas := s.GetIdeas()
	if len(ideas) != 1 {
		t.Fatalf("expected 1 idea, got %d", len(ideas))
	}
	if ideas[0].Text != "old idea" {
		t.Errorf("expected 'old idea', got %q", ideas[0].Text)
	}
	if s.GetNextIdeaID() != 1 {
		t.Errorf("expected nextIdeaID=1, got %d", s.GetNextIdeaID())
	}
}

func TestStore_GetIdeas_無ideas欄位(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	oldData := `{"sessions":{}}`
	os.WriteFile(path, []byte(oldData), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load old format: %v", err)
	}

	ideas := s.GetIdeas()
	if len(ideas) != 0 {
		t.Errorf("expected 0 ideas for old format, got %d", len(ideas))
	}
}

// groupByName is a test helper that extracts group from session name.
// Simulates: "ragent-piper-abc123" → "piper", "ragent-abc123" → ""
func groupByName(name string) string {
	// Simple test helper: strip "ragent-" prefix, check if there's a project segment
	if !strings.HasPrefix(name, "ragent-") {
		return ""
	}
	rest := name[len("ragent-"):]
	parts := strings.Split(rest, "-")
	if len(parts) <= 1 {
		return "" // old format or no project
	}
	// Last part is shortID, everything before is project
	return strings.Join(parts[:len(parts)-1], "-")
}

func TestInsertAfterGroup_空store回傳0(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.json"))
	s.Load()
	order := s.InsertAfterGroup("piper", groupByName)
	if order != 0 {
		t.Errorf("expected 0, got %d", order)
	}
}

func TestInsertAfterGroup_無匹配時追加到末尾(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.json"))
	s.Load()
	s.SetMeta("ragent-alpha-aaa", SessionMeta{Order: 0})
	s.SetMeta("ragent-alpha-bbb", SessionMeta{Order: 1})

	order := s.InsertAfterGroup("beta", groupByName)
	if order != 2 {
		t.Errorf("expected 2 (append), got %d", order)
	}
}

func TestInsertAfterGroup_同group聚合(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.json"))
	s.Load()
	s.SetMeta("ragent-piper-aaa", SessionMeta{Order: 0})
	s.SetMeta("ragent-other-bbb", SessionMeta{Order: 1})
	s.SetMeta("ragent-other-ccc", SessionMeta{Order: 2})

	// Insert another "piper" — should go at order 1, pushing "other" sessions up
	order := s.InsertAfterGroup("piper", groupByName)
	if order != 1 {
		t.Errorf("expected 1 (after piper at 0), got %d", order)
	}

	// Verify other sessions were shifted
	m2 := s.GetMeta("ragent-other-bbb")
	if m2.Order != 2 {
		t.Errorf("expected order to be shifted to 2, got %d", m2.Order)
	}
	m3 := s.GetMeta("ragent-other-ccc")
	if m3.Order != 3 {
		t.Errorf("expected order to be shifted to 3, got %d", m3.Order)
	}
}

func TestInsertAfterGroup_空group追加到末尾(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.json"))
	s.Load()
	s.SetMeta("ragent-piper-aaa", SessionMeta{Order: 0})

	order := s.InsertAfterGroup("", groupByName)
	if order != 1 {
		t.Errorf("expected 1 (append), got %d", order)
	}
}

func TestStore_AddProjectWithDescription_新專案(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := New(path)
	s.Load()

	if err := s.AddProjectWithDescription("/home/user/proj", "my project description"); err != nil {
		t.Fatalf("AddProjectWithDescription: %v", err)
	}

	// Verify it's in GetProjects
	projects := s.GetProjects()
	if len(projects) != 1 || projects[0] != "/home/user/proj" {
		t.Fatalf("expected 1 project, got %v", projects)
	}

	// Reload from disk and verify description persisted
	s2 := New(path)
	s2.Load()

	s2.mu.RLock()
	defer s2.mu.RUnlock()
	if len(s2.projects) != 1 {
		t.Fatalf("expected 1 project after reload, got %d", len(s2.projects))
	}
	if s2.projects[0].Description != "my project description" {
		t.Errorf("expected description 'my project description', got %q", s2.projects[0].Description)
	}
	if s2.projects[0].Path != "/home/user/proj" {
		t.Errorf("expected path '/home/user/proj', got %q", s2.projects[0].Path)
	}
}

func TestStore_AddProjectWithDescription_已存在更新描述(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	// Add with initial description
	s.AddProjectWithDescription("/home/user/proj", "initial desc")

	// Update description for same path
	if err := s.AddProjectWithDescription("/home/user/proj", "updated desc"); err != nil {
		t.Fatalf("AddProjectWithDescription (update): %v", err)
	}

	// Should still be only 1 project (dedup)
	projects := s.GetProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project (dedup), got %d", len(projects))
	}

	// Verify description was updated
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.projects[0].Description != "updated desc" {
		t.Errorf("expected description 'updated desc', got %q", s.projects[0].Description)
	}
}

func TestStore_SetProjectOrder_重排順序(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.AddProject("/home/user/proj-a")
	s.AddProject("/home/user/proj-b")
	s.AddProject("/home/user/proj-c")

	if err := s.SetProjectOrder([]string{"/home/user/proj-c", "/home/user/proj-a", "/home/user/proj-b"}); err != nil {
		t.Fatalf("SetProjectOrder: %v", err)
	}

	got := s.GetProjects()
	if len(got) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(got))
	}
	want := []string{"/home/user/proj-c", "/home/user/proj-a", "/home/user/proj-b"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestStore_SetProjectOrder_持久化(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := New(path)
	s1.Load()
	s1.AddProject("/home/user/proj-a")
	s1.AddProject("/home/user/proj-b")

	s1.SetProjectOrder([]string{"/home/user/proj-b", "/home/user/proj-a"})

	// Reload from disk
	s2 := New(path)
	s2.Load()
	got := s2.GetProjects()
	if len(got) != 2 || got[0] != "/home/user/proj-b" || got[1] != "/home/user/proj-a" {
		t.Errorf("expected [proj-b, proj-a] after reload, got %v", got)
	}
}

func TestStore_SetProjectOrder_忽略不存在的路徑(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.AddProject("/home/user/proj-a")
	s.AddProject("/home/user/proj-b")

	// Include a non-existent path — should be ignored, existing ones reordered
	if err := s.SetProjectOrder([]string{"/home/user/proj-b", "/nonexistent", "/home/user/proj-a"}); err != nil {
		t.Fatalf("SetProjectOrder: %v", err)
	}

	got := s.GetProjects()
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	if got[0] != "/home/user/proj-b" || got[1] != "/home/user/proj-a" {
		t.Errorf("expected [proj-b, proj-a], got %v", got)
	}
}

func TestStore_SetProjectOrder_保留metadata(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.AddProjectWithDescription("/home/user/proj-a", "desc-a")
	s.AddProjectWithDescription("/home/user/proj-b", "desc-b")
	autoMode := true
	s.SetProjectAutoMode("/home/user/proj-a", &autoMode)

	s.SetProjectOrder([]string{"/home/user/proj-b", "/home/user/proj-a"})

	// Verify metadata preserved
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.projects[0].Path != "/home/user/proj-b" || s.projects[0].Description != "desc-b" {
		t.Errorf("proj-b metadata lost: %+v", s.projects[0])
	}
	if s.projects[1].Path != "/home/user/proj-a" || s.projects[1].Description != "desc-a" {
		t.Errorf("proj-a metadata lost: %+v", s.projects[1])
	}
	if s.projects[1].AutoMode == nil || *s.projects[1].AutoMode != true {
		t.Errorf("proj-a autoMode lost")
	}
}

func TestStore_GetModelConfig_預設為空(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	cfg := s.GetModelConfig()
	if cfg.DevcontainerGen != "" || cfg.Summarizer != "" || cfg.IdeaEnrich != "" {
		t.Errorf("expected empty model config, got %+v", cfg)
	}
}

func TestStore_SetModelConfig_設定並持久化(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := New(path)
	s1.Load()

	cfg := ModelConfig{
		DevcontainerGen: "sonnet",
		Summarizer:      "opus",
		IdeaEnrich:      "haiku",
	}
	if err := s1.SetModelConfig(cfg); err != nil {
		t.Fatalf("SetModelConfig: %v", err)
	}

	// Reload from disk
	s2 := New(path)
	s2.Load()
	got := s2.GetModelConfig()
	if got.DevcontainerGen != "sonnet" {
		t.Errorf("expected DevcontainerGen=sonnet, got %q", got.DevcontainerGen)
	}
	if got.Summarizer != "opus" {
		t.Errorf("expected Summarizer=opus, got %q", got.Summarizer)
	}
	if got.IdeaEnrich != "haiku" {
		t.Errorf("expected IdeaEnrich=haiku, got %q", got.IdeaEnrich)
	}
}

func TestStore_SetModelConfig_部分更新(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	// Set only one field
	cfg := ModelConfig{Summarizer: "sonnet"}
	if err := s.SetModelConfig(cfg); err != nil {
		t.Fatalf("SetModelConfig: %v", err)
	}

	got := s.GetModelConfig()
	if got.Summarizer != "sonnet" {
		t.Errorf("expected Summarizer=sonnet, got %q", got.Summarizer)
	}
	if got.DevcontainerGen != "" {
		t.Errorf("expected empty DevcontainerGen, got %q", got.DevcontainerGen)
	}
}

func TestStore_GetModelConfig_回傳副本(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.SetModelConfig(ModelConfig{Summarizer: "opus"})
	got := s.GetModelConfig()
	got.Summarizer = "mutated"

	got2 := s.GetModelConfig()
	if got2.Summarizer != "opus" {
		t.Errorf("GetModelConfig returned reference instead of copy")
	}
}

func TestStore_ModelConfig_舊格式無modelConfig欄位(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	// Write old format without modelConfig
	oldData := `{"sessions":{}}`
	os.WriteFile(path, []byte(oldData), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load old format: %v", err)
	}

	cfg := s.GetModelConfig()
	if cfg != (ModelConfig{}) {
		t.Errorf("expected zero ModelConfig for old format, got %+v", cfg)
	}
}

func TestStore_GetFirewallConfig_預設值(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	cfg := s.GetFirewallConfig()
	if cfg.Mode != "open" {
		t.Errorf("expected mode=open, got %q", cfg.Mode)
	}
	if len(cfg.Domains) == 0 {
		t.Error("expected default domains, got empty")
	}
	// Verify some known default domains
	found := false
	for _, d := range cfg.Domains {
		if d == "api.anthropic.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected api.anthropic.com in default domains")
	}
}

func TestStore_SetFirewallConfig_設定並持久化(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := New(path)
	s1.Load()

	cfg := FirewallConfig{
		Mode:    "whitelist",
		Domains: []string{"example.com", "api.example.com"},
	}
	if err := s1.SetFirewallConfig(cfg); err != nil {
		t.Fatalf("SetFirewallConfig: %v", err)
	}

	// Reload from disk
	s2 := New(path)
	s2.Load()
	got := s2.GetFirewallConfig()
	if got.Mode != "whitelist" {
		t.Errorf("expected mode=whitelist, got %q", got.Mode)
	}
	if len(got.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(got.Domains))
	}
	if got.Domains[0] != "example.com" || got.Domains[1] != "api.example.com" {
		t.Errorf("unexpected domains: %v", got.Domains)
	}
}

func TestStore_GetFirewallConfig_回傳副本(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "sessions.json"))
	s.Load()

	s.SetFirewallConfig(FirewallConfig{Mode: "whitelist", Domains: []string{"a.com"}})
	got := s.GetFirewallConfig()
	got.Domains[0] = "mutated.com"

	got2 := s.GetFirewallConfig()
	if got2.Domains[0] != "a.com" {
		t.Errorf("GetFirewallConfig returned reference instead of copy")
	}
}

func TestStore_FirewallConfig_舊格式無此欄位(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	oldData := `{"sessions":{}}`
	os.WriteFile(path, []byte(oldData), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg := s.GetFirewallConfig()
	if cfg.Mode != "open" {
		t.Errorf("expected default mode=open, got %q", cfg.Mode)
	}
	if len(cfg.Domains) == 0 {
		t.Error("expected default domains for old format")
	}
}

func TestStore_Description向後相容_無description欄位(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	// Write JSON with projects that have no description field
	data := `{"sessions":{},"projects":[{"path":"/home/user/proj-a","autoMode":true},{"path":"/home/user/proj-b"}]}`
	os.WriteFile(path, []byte(data), 0o644)

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	projects := s.GetProjects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Description should be empty string (zero value)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Description != "" {
			t.Errorf("expected empty description for %s, got %q", p.Path, p.Description)
		}
	}
}

func TestStore_SetClaudeSessionID_已存在session(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := New(path)
	s.Load()
	s.SetMeta("sess1", SessionMeta{Title: "test"})

	err := s.SetClaudeSessionID("sess1", "claude-abc-123")
	if err != nil {
		t.Fatalf("SetClaudeSessionID: %v", err)
	}

	got := s.GetMeta("sess1")
	if got.ClaudeSessionID != "claude-abc-123" {
		t.Errorf("expected ClaudeSessionID %q, got %q", "claude-abc-123", got.ClaudeSessionID)
	}

	// Verify persistence
	s2 := New(path)
	s2.Load()
	got2 := s2.GetMeta("sess1")
	if got2.ClaudeSessionID != "claude-abc-123" {
		t.Errorf("after reload: expected ClaudeSessionID %q, got %q", "claude-abc-123", got2.ClaudeSessionID)
	}
}

func TestStore_SetClaudeSessionID_不存在session(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s := New(path)
	s.Load()

	err := s.SetClaudeSessionID("nonexistent", "claude-abc-123")
	if err != nil {
		t.Fatalf("SetClaudeSessionID for nonexistent session should not error: %v", err)
	}

	got := s.GetMeta("nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent session, got %+v", got)
	}
}
