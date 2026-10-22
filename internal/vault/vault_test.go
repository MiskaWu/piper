package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultInit_建立目錄結構(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{}
	v := New(dir, WithGitRunner(fg))

	if err := v.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify directories
	for _, sub := range []string{"", "epics", "inbox"} {
		path := filepath.Join(dir, sub)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", path)
		}
	}

	// Verify git init was called
	if !fg.inited {
		t.Error("expected git init to be called")
	}
}

func TestVaultInit_已有repo不重複init(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	fg := &fakeGitRunner{inited: true}
	v := New(dir, WithGitRunner(fg))

	if err := v.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// No error means success
}

func TestVault_BaseDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	v := New(dir, WithGitRunner(&fakeGitRunner{inited: true}))
	v.Init()

	if v.BaseDir() != dir {
		t.Errorf("expected %s, got %s", dir, v.BaseDir())
	}
}

func TestVault_Git(t *testing.T) {
	fg := &fakeGitRunner{inited: true}
	v := New(t.TempDir(), WithGitRunner(fg))

	if v.Git() != fg {
		t.Error("expected Git() to return injected runner")
	}
}

func TestVault_EpicManager(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	v := New(dir, WithGitRunner(&fakeGitRunner{inited: true}))
	v.Init()

	em := v.EpicManager()
	if em == nil {
		t.Fatal("expected non-nil EpicManager")
	}
	expectedBase := filepath.Join(dir, "epics")
	if em.BaseDir() != expectedBase {
		t.Errorf("expected EpicManager baseDir %s, got %s", expectedBase, em.BaseDir())
	}
}

func TestVault_CreateEpic(t *testing.T) {
	v, fg := newTestVault(t)

	id, err := v.CreateEpic("Test Epic")
	if err != nil {
		t.Fatalf("CreateEpic: %v", err)
	}
	if !strings.HasPrefix(id, "epic-") {
		t.Errorf("expected epic- prefix, got %q", id)
	}

	// Verify autoCommit was called
	found := false
	for _, msg := range fg.commits {
		if strings.Contains(msg, "create epic") && strings.Contains(msg, id) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected commit message for create epic, got %v", fg.commits)
	}

	// Verify file exists
	storyMapPath := filepath.Join(v.baseDir, "epics", id, "story-map.yaml")
	if _, err := os.Stat(storyMapPath); err != nil {
		t.Errorf("expected story-map.yaml to exist: %v", err)
	}
}

func TestVault_DeleteEpic(t *testing.T) {
	v, fg := newTestVault(t)

	id, _ := v.CreateEpic("To Delete")
	fg.commits = nil

	if err := v.DeleteEpic(id); err != nil {
		t.Fatalf("DeleteEpic: %v", err)
	}

	if len(fg.commits) != 1 || !strings.Contains(fg.commits[0], "delete epic") {
		t.Errorf("expected delete epic commit, got %v", fg.commits)
	}
}

func TestVault_WriteStoryMap(t *testing.T) {
	v, fg := newTestVault(t)

	id, _ := v.CreateEpic("Writer")
	fg.commits = nil

	if err := v.WriteStoryMap(id, "meta:\n  name: updated\n"); err != nil {
		t.Fatalf("WriteStoryMap: %v", err)
	}

	if len(fg.commits) != 1 || !strings.Contains(fg.commits[0], "update story-map") {
		t.Errorf("expected update story-map commit, got %v", fg.commits)
	}
}

func TestVault_RenameEpic(t *testing.T) {
	v, fg := newTestVault(t)

	id, _ := v.CreateEpic("Original")
	fg.commits = nil

	if err := v.RenameEpic(id, "Renamed"); err != nil {
		t.Fatalf("RenameEpic: %v", err)
	}

	if len(fg.commits) != 1 || !strings.Contains(fg.commits[0], "rename epic") {
		t.Errorf("expected rename epic commit, got %v", fg.commits)
	}

	// Verify name changed
	content, _ := v.ReadStoryMap(id)
	if !strings.Contains(content, "Renamed") {
		t.Errorf("expected renamed content, got %s", content)
	}
}

func TestVault_ListEpics(t *testing.T) {
	v, _ := newTestVault(t)

	v.CreateEpic("Epic A")
	v.CreateEpic("Epic B")

	epics, err := v.ListEpics()
	if err != nil {
		t.Fatalf("ListEpics: %v", err)
	}
	if len(epics) != 2 {
		t.Errorf("expected 2 epics, got %d", len(epics))
	}
}

func TestVault_ReadStoryMap(t *testing.T) {
	v, _ := newTestVault(t)

	id, _ := v.CreateEpic("Reader")
	content, err := v.ReadStoryMap(id)
	if err != nil {
		t.Fatalf("ReadStoryMap: %v", err)
	}
	if !strings.Contains(content, "Reader") {
		t.Error("expected content to contain epic name")
	}
}

// --- Problem Statement ---

func TestVault_ReadProblemStatement(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("PS Test")

	content, err := v.ReadProblemStatement(id)
	if err != nil {
		t.Fatalf("ReadProblemStatement: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty problem statement from template")
	}
}

func TestVault_ReadProblemStatement_不存在的epic(t *testing.T) {
	v, _ := newTestVault(t)

	_, err := v.ReadProblemStatement("epic-fffff")
	if err == nil {
		t.Fatal("expected error for non-existent epic")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestVault_WriteProblemStatement_驗證通過(t *testing.T) {
	v, fg := newTestVault(t)
	id, _ := v.CreateEpic("PS Write")
	fg.commits = nil

	content := "# Problem Statement\n\n## 背景\n\nsome background\n\n## 不做什麼\n\n- nothing\n"
	if err := v.WriteProblemStatement(id, content); err != nil {
		t.Fatalf("WriteProblemStatement: %v", err)
	}

	// Verify commit
	if len(fg.commits) != 1 || !strings.Contains(fg.commits[0], "update problem-statement") {
		t.Errorf("expected commit, got %v", fg.commits)
	}

	// Verify content
	got, _ := v.ReadProblemStatement(id)
	if got != content {
		t.Errorf("content mismatch")
	}
}

func TestVault_WriteProblemStatement_缺少不做什麼section(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("PS Validate")

	err := v.WriteProblemStatement(id, "# Problem Statement\n\n## 背景\n\nonly background\n")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "不做什麼") {
		t.Errorf("expected '不做什麼' in error, got: %v", err)
	}
}

// --- ADR ---

func TestVault_ADR_CRUD(t *testing.T) {
	v, fg := newTestVault(t)
	id, _ := v.CreateEpic("ADR Test")
	fg.commits = nil

	adrContent := "# ADR-001\n\n## 背景\n\nbg\n\n## 決定\n\ndec\n\n## 後果\n\ncons\n"

	// Write
	if err := v.WriteADR(id, "adr-001", adrContent); err != nil {
		t.Fatalf("WriteADR: %v", err)
	}

	// Read
	got, err := v.ReadADR(id, "adr-001")
	if err != nil {
		t.Fatalf("ReadADR: %v", err)
	}
	if got != adrContent {
		t.Error("content mismatch")
	}

	// List
	names, err := v.ListADRs(id)
	if err != nil {
		t.Fatalf("ListADRs: %v", err)
	}
	if len(names) != 1 || names[0] != "adr-001" {
		t.Errorf("expected [adr-001], got %v", names)
	}

	// Delete
	fg.commits = nil
	if err := v.DeleteADR(id, "adr-001"); err != nil {
		t.Fatalf("DeleteADR: %v", err)
	}

	names, _ = v.ListADRs(id)
	if len(names) != 0 {
		t.Errorf("expected empty list after delete, got %v", names)
	}
}

func TestVault_WriteADR_缺少必要section(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("ADR Validate")

	tests := []struct {
		name    string
		content string
		missing string
	}{
		{"缺背景", "# ADR\n\n## 決定\n\nd\n\n## 後果\n\nc\n", "背景"},
		{"缺決定", "# ADR\n\n## 背景\n\nb\n\n## 後果\n\nc\n", "決定"},
		{"缺後果", "# ADR\n\n## 背景\n\nb\n\n## 決定\n\nd\n", "後果"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.WriteADR(id, "bad-adr", tt.content)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.missing) {
				t.Errorf("expected '%s' in error, got: %v", tt.missing, err)
			}
		})
	}
}

// --- Spikes ---

func TestVault_Spike_CRUD(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Spike Test")

	content := "# Spike\n\n## 問題\n\nq\n\n## 結論\n\na\n"

	// Write (no validation)
	if err := v.WriteSpike(id, "spike-001", content); err != nil {
		t.Fatalf("WriteSpike: %v", err)
	}

	// Read
	got, err := v.ReadSpike(id, "spike-001")
	if err != nil {
		t.Fatalf("ReadSpike: %v", err)
	}
	if got != content {
		t.Error("content mismatch")
	}

	// List
	names, err := v.ListSpikes(id)
	if err != nil {
		t.Fatalf("ListSpikes: %v", err)
	}
	if len(names) != 1 || names[0] != "spike-001" {
		t.Errorf("expected [spike-001], got %v", names)
	}

	// Delete
	if err := v.DeleteSpike(id, "spike-001"); err != nil {
		t.Fatalf("DeleteSpike: %v", err)
	}

	names, _ = v.ListSpikes(id)
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

// --- Specs ---

func TestVault_ReadSpecsSummary(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Specs Test")

	// Create specs dir and file
	dir := filepath.Join(v.epic.BaseDir(), id, "specs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "summary.md"), []byte("# Summary\n\ncontent\n"), 0o644)

	got, err := v.ReadSpecsSummary(id)
	if err != nil {
		t.Fatalf("ReadSpecsSummary: %v", err)
	}
	if !strings.Contains(got, "Summary") {
		t.Error("expected summary content")
	}
}

func TestVault_ReadSpecsSummary_不存在(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("No Specs")

	_, err := v.ReadSpecsSummary(id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestVault_WriteSpecsSummary(t *testing.T) {
	v, fg := newTestVault(t)
	id, _ := v.CreateEpic("Write Summary")

	content := "# Updated Summary\n\nnew content\n"
	if err := v.WriteSpecsSummary(id, content); err != nil {
		t.Fatalf("WriteSpecsSummary: %v", err)
	}

	// Verify file written
	got, err := v.ReadSpecsSummary(id)
	if err != nil {
		t.Fatalf("ReadSpecsSummary after write: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}

	// Verify autoCommit
	found := false
	for _, msg := range fg.commits {
		if strings.Contains(msg, "update specs summary") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected autoCommit for specs summary")
	}
}

func TestVault_WriteSpecsSummary_建立specs目錄(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Auto Dir")

	// specs/ dir doesn't exist yet — WriteSpecsSummary should create it
	if err := v.WriteSpecsSummary(id, "# Test\n"); err != nil {
		t.Fatalf("WriteSpecsSummary: %v", err)
	}

	got, err := v.ReadSpecsSummary(id)
	if err != nil {
		t.Fatalf("ReadSpecsSummary: %v", err)
	}
	if !strings.Contains(got, "Test") {
		t.Error("expected content")
	}
}

func TestVault_ReadSpecsChangelog(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Changelog Test")

	dir := filepath.Join(v.epic.BaseDir(), id, "specs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "changelog.md"), []byte("## 2026-04-15\n\nchanges\n"), 0o644)

	got, err := v.ReadSpecsChangelog(id)
	if err != nil {
		t.Fatalf("ReadSpecsChangelog: %v", err)
	}
	if !strings.Contains(got, "2026-04-15") {
		t.Error("expected changelog content")
	}
}

func TestVault_ReadSpecsChangelog_不存在(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("No Changelog")

	_, err := v.ReadSpecsChangelog(id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestVault_WriteSpecsChangelog(t *testing.T) {
	v, fg := newTestVault(t)
	id, _ := v.CreateEpic("Write Changelog")

	content := "## 2026-04-15\n\n### 新增\n- feature X\n"
	if err := v.WriteSpecsChangelog(id, content); err != nil {
		t.Fatalf("WriteSpecsChangelog: %v", err)
	}

	got, err := v.ReadSpecsChangelog(id)
	if err != nil {
		t.Fatalf("ReadSpecsChangelog after write: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}

	found := false
	for _, msg := range fg.commits {
		if strings.Contains(msg, "update specs changelog") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected autoCommit for specs changelog")
	}
}

func TestVault_ListExtracts(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Extracts Test")

	// Empty — should return empty slice
	names, err := v.ListExtracts(id)
	if err != nil {
		t.Fatalf("ListExtracts empty: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	// Create extract files
	dir := filepath.Join(v.epic.BaseDir(), id, "specs", "extracts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "spec-v1.extract.md"), []byte("extract 1"), 0o644)
	os.WriteFile(filepath.Join(dir, "spec-v2.extract.md"), []byte("extract 2"), 0o644)
	os.WriteFile(filepath.Join(dir, "not-an-extract.md"), []byte("ignored"), 0o644) // wrong suffix

	names, err = v.ListExtracts(id)
	if err != nil {
		t.Fatalf("ListExtracts: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 extracts, got %d: %v", len(names), names)
	}
}

func TestVault_ReadExtract(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Read Extract")

	dir := filepath.Join(v.epic.BaseDir(), id, "specs", "extracts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "myspec.extract.md"), []byte("# Extract\n\ncontent\n"), 0o644)

	got, err := v.ReadExtract(id, "myspec")
	if err != nil {
		t.Fatalf("ReadExtract: %v", err)
	}
	if !strings.Contains(got, "Extract") {
		t.Error("expected extract content")
	}
}

func TestVault_ReadExtract_不存在(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("No Extract")

	_, err := v.ReadExtract(id, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestVault_ReadExtract_PathTraversal(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Extract Traversal")

	badNames := []string{"../etc/passwd", "../../secret", "foo/bar", "/absolute"}
	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if _, err := v.ReadExtract(id, name); err == nil {
				t.Error("expected error for path traversal")
			}
		})
	}
}

// --- Path traversal ---

func TestVault_PathTraversal_被擋(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Traversal")

	badNames := []string{"../etc/passwd", "../../secret", "foo/bar", "/absolute"}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			if _, err := v.ReadADR(id, name); err == nil {
				t.Error("expected error for path traversal")
			}
			if err := v.WriteADR(id, name, "x"); err == nil {
				t.Error("expected error for path traversal")
			}
			if _, err := v.ReadSpike(id, name); err == nil {
				t.Error("expected error for path traversal")
			}
			if err := v.WriteSpike(id, name, "x"); err == nil {
				t.Error("expected error for path traversal")
			}
		})
	}
}

func TestVault_AppendRetroDecision_成功累積並自動commit(t *testing.T) {
	v, fg := newTestVault(t)
	id, _ := v.CreateEpic("Decisions")
	fg.commits = nil

	entry1 := `{"ts":"2026-04-20T10:00:00Z","type":"tags","target":"story-s1","rationale":"推導 backend"}`
	entry2 := `{"ts":"2026-04-20T10:05:00Z","type":"aan","target":"story-s1","rationale":"測試底線"}`

	if err := v.AppendRetroDecision(id, []byte(entry1)); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := v.AppendRetroDecision(id, []byte(entry2)); err != nil {
		t.Fatalf("second append: %v", err)
	}

	got, err := v.ReadRetroDecisions(id)
	if err != nil {
		t.Fatalf("ReadRetroDecisions: %v", err)
	}
	want := entry1 + "\n" + entry2 + "\n"
	if got != want {
		t.Errorf("expected\n%q\ngot\n%q", want, got)
	}
	if len(fg.commits) != 2 {
		t.Errorf("expected 2 commits, got %d: %v", len(fg.commits), fg.commits)
	}
	for _, msg := range fg.commits {
		if !strings.Contains(msg, "retro decision") {
			t.Errorf("expected retro decision commit, got %q", msg)
		}
	}
}

func TestVault_AppendRetroDecision_拒絕無效輸入(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Validate")

	cases := []struct {
		name    string
		entry   string
		wantErr string
	}{
		{"空字串時回錯", "", "empty"},
		{"含換行時回錯", `{"a":1}` + "\n" + `{"b":2}`, "single line"},
		{"非合法 json 時回錯", `not json`, "invalid json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.AppendRetroDecision(id, []byte(tc.entry))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error to contain %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestVault_ReadRetroDecisions_不存在回空字串(t *testing.T) {
	v, _ := newTestVault(t)
	id, _ := v.CreateEpic("Empty")

	got, err := v.ReadRetroDecisions(id)
	if err != nil {
		t.Fatalf("ReadRetroDecisions: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
