package dispatch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/miskawu/piper/internal/plan"
)

// mockStoryMapIO implements StoryMapIO for testing.
type mockStoryMapIO struct {
	content  string
	written  string
	writeErr error
}

func (m *mockStoryMapIO) ReadStoryMap(epicID string) (string, error) {
	return m.content, nil
}

func (m *mockStoryMapIO) WriteStoryMap(epicID string, content string) error {
	m.written = content
	return m.writeErr
}

// mockProjectResolver implements ProjectResolver for testing.
type mockProjectResolver struct {
	projects map[string]string
}

func (m *mockProjectResolver) ResolveProject(name string) (string, bool) {
	path, ok := m.projects[name]
	return path, ok
}

const testYAML = `meta:
  name: "Test Epic"
  updated: "2026-01-01T00:00:00Z"
  releases:
    - id: rel-mvp01
      label: "MVP"
activities:
  - id: act-001
    name: "核心功能"
    steps:
      - id: step-001
        name: "登入流程"
        stories:
          - id: story-aaa11
            name: "使用者登入"
            description: "身為使用者，我想要登入系統"
            tags: [auth]
            release: rel-mvp01
            status: todo
            acceptance: [使用者能以帳密登入, 登入失敗顯示錯誤]
            tasks:
              - id: task-a1
                name: "實作登入 API"
                targetProject: piper
                status: todo
              - id: task-a2
                name: "實作登入 UI"
                targetProject: frontend
                status: todo
          - id: story-bbb22
            name: "忘記密碼"
            release: rel-mvp01
            status: in-progress
            tasks:
              - id: task-b1
                name: "密碼重設 API"
                targetProject: piper
                status: in-progress
      - id: step-002
        name: "Dashboard"
        stories:
          - id: story-ccc33
            name: "統計面板"
            release: rel-mvp01
            status: todo
            tasks:
              - id: task-c1
                name: "統計面板 UI"
                targetProject: frontend
                status: todo
`

func TestPreview(t *testing.T) {
	tmpDir := t.TempDir()
	io := &mockStoryMapIO{content: testYAML}
	resolver := &mockProjectResolver{projects: map[string]string{
		"piper": filepath.Join(tmpDir, "piper"),
	}}

	svc := New(io, resolver, tmpDir)
	result, err := svc.Preview("epic-12345", "")
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if result.EpicName != "Test Epic" {
		t.Errorf("epicName = %q, want %q", result.EpicName, "Test Epic")
	}

	// piper: task-a1 (dispatchable), frontend: task-a2 + task-c1 (unresolved)
	piperTasks := result.Groups["piper"]
	if len(piperTasks) != 1 {
		t.Fatalf("piper tasks = %d, want 1", len(piperTasks))
	}
	if !piperTasks[0].Dispatchable {
		t.Error("task-a1 should be dispatchable")
	}
	if piperTasks[0].StoryID != "story-aaa11" {
		t.Errorf("storyID = %q, want %q", piperTasks[0].StoryID, "story-aaa11")
	}

	frontendTasks := result.Groups["frontend"]
	if len(frontendTasks) != 2 {
		t.Fatalf("frontend tasks = %d, want 2", len(frontendTasks))
	}
	if frontendTasks[0].Dispatchable {
		t.Error("frontend tasks should NOT be dispatchable (project not registered)")
	}

	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
}

func TestExecute(t *testing.T) {
	tmpDir := t.TempDir()
	piperDir := filepath.Join(tmpDir, "piper")
	os.MkdirAll(piperDir, 0o755)

	epicBase := filepath.Join(tmpDir, "epic")
	os.MkdirAll(epicBase, 0o755)

	io := &mockStoryMapIO{content: testYAML}
	resolver := &mockProjectResolver{projects: map[string]string{
		"piper": piperDir,
	}}

	svc := New(io, resolver, epicBase)
	result, err := svc.Execute(DispatchRequest{EpicID: "epic-12345"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should dispatch task-a1 (piper), skip task-a2 + task-c1 (frontend not registered)
	if len(result.Dispatched) != 1 {
		t.Fatalf("dispatched = %d, want 1", len(result.Dispatched))
	}
	if result.Dispatched[0].ID != "task-a1" {
		t.Errorf("dispatched ID = %q, want %q", result.Dispatched[0].ID, "task-a1")
	}
	if result.Dispatched[0].StoryID != "story-aaa11" {
		t.Errorf("dispatched storyID = %q, want %q", result.Dispatched[0].StoryID, "story-aaa11")
	}

	if len(result.Skipped) != 2 {
		t.Fatalf("skipped = %d, want 2", len(result.Skipped))
	}

	// Verify brief file created with task ID
	briefPath := filepath.Join(piperDir, ".plan", "briefs", "task-a1.md")
	data, err := os.ReadFile(briefPath)
	if err != nil {
		t.Fatalf("brief file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Task Brief: task-a1") {
		t.Error("brief missing task brief title")
	}
	if !strings.Contains(content, "story-aaa11") {
		t.Error("brief missing parent story reference")
	}
	if !strings.Contains(content, "Test Epic") {
		t.Error("brief missing epic name")
	}
	if !strings.Contains(content, "使用者能以帳密登入") {
		t.Error("brief missing story acceptance criteria")
	}

	// Verify skills deployed
	submitSkill := filepath.Join(piperDir, ".claude", "skills", "submit", "SKILL.md")
	if _, err := os.Stat(submitSkill); err != nil {
		t.Error("submit skill not deployed")
	}
	verifySkill := filepath.Join(piperDir, ".claude", "skills", "verify", "SKILL.md")
	if _, err := os.Stat(verifySkill); err != nil {
		t.Error("verify skill not deployed")
	}
	breakdownSkill := filepath.Join(piperDir, ".claude", "skills", "breakdown", "SKILL.md")
	if _, err := os.Stat(breakdownSkill); err != nil {
		t.Error("breakdown skill not deployed")
	}

	// Verify task-brief rule deployed
	taskBriefRule := filepath.Join(piperDir, ".claude", "rules", "task-brief.md")
	ruleData, err := os.ReadFile(taskBriefRule)
	if err != nil {
		t.Fatal("task-brief rule not deployed")
	}
	if !strings.Contains(string(ruleData), "Task Brief") {
		t.Error("task-brief rule missing expected content")
	}

	// Verify check-briefs script deployed
	scriptPath := filepath.Join(piperDir, ".claude", "scripts", "check-briefs.sh")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal("check-briefs script not deployed")
	}
	if !strings.Contains(string(scriptData), "驗收條件") {
		t.Error("check-briefs script missing expected content")
	}

	// Verify story map was written with status updates
	if io.written == "" {
		t.Fatal("story map was not written")
	}
	// Both task and story should be in-progress
	if !strings.Contains(io.written, "in-progress") {
		t.Error("story map should contain in-progress status")
	}

	// Verify done dir created
	doneDir := filepath.Join(piperDir, ".plan", "briefs", "done")
	if _, err := os.Stat(doneDir); err != nil {
		t.Error("done directory not created")
	}
}

func TestExecute_WithSpecificTaskIDs(t *testing.T) {
	tmpDir := t.TempDir()
	piperDir := filepath.Join(tmpDir, "piper")
	frontendDir := filepath.Join(tmpDir, "frontend")
	os.MkdirAll(piperDir, 0o755)
	os.MkdirAll(frontendDir, 0o755)

	epicBase := filepath.Join(tmpDir, "epic")
	os.MkdirAll(epicBase, 0o755)

	io := &mockStoryMapIO{content: testYAML}
	resolver := &mockProjectResolver{projects: map[string]string{
		"piper":    piperDir,
		"frontend": frontendDir,
	}}

	svc := New(io, resolver, epicBase)
	result, err := svc.Execute(DispatchRequest{
		EpicID:  "epic-12345",
		TaskIDs: []string{"task-c1"}, // only dispatch this one
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(result.Dispatched) != 1 {
		t.Fatalf("dispatched = %d, want 1", len(result.Dispatched))
	}
	if result.Dispatched[0].ID != "task-c1" {
		t.Errorf("dispatched ID = %q, want %q", result.Dispatched[0].ID, "task-c1")
	}
}

func TestExecute_WithADRAndSpike(t *testing.T) {
	tmpDir := t.TempDir()
	piperDir := filepath.Join(tmpDir, "piper")
	os.MkdirAll(piperDir, 0o755)

	epicBase := filepath.Join(tmpDir, "epic")
	os.MkdirAll(filepath.Join(epicBase, "adr"), 0o755)
	os.MkdirAll(filepath.Join(epicBase, "spikes"), 0o755)

	// Create ADR
	adrContent := "# ADR-001\n\n## 決定\n\n用 JWT。\n\n## 後果\n\n需要加 middleware。\n"
	os.WriteFile(filepath.Join(epicBase, "adr", "adr-001.md"), []byte(adrContent), 0o644)

	// Create Spike
	spikeContent := "# Spike-001\n\n## 結論\n\nOAuth 可行。\n"
	os.WriteFile(filepath.Join(epicBase, "spikes", "spike-001.md"), []byte(spikeContent), 0o644)

	yamlWithRefs := `meta:
  name: "Test"
  updated: "2026-01-01T00:00:00Z"
  releases:
    - id: rel-mvp01
      label: "MVP"
activities:
  - id: act-001
    name: "Auth"
    steps:
      - id: step-001
        name: "Login"
        stories:
          - id: story-ref01
            name: "有參考的 Story"
            release: rel-mvp01
            adr: adr-001
            spike: spike-001
            status: todo
            tasks:
              - id: task-ref01
                name: "實作有參考的功能"
                targetProject: piper
                status: todo
`

	io := &mockStoryMapIO{content: yamlWithRefs}
	resolver := &mockProjectResolver{projects: map[string]string{"piper": piperDir}}

	svc := New(io, resolver, epicBase)
	_, err := svc.Execute(DispatchRequest{EpicID: "epic-12345"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	briefPath := filepath.Join(piperDir, ".plan", "briefs", "task-ref01.md")
	data, err := os.ReadFile(briefPath)
	if err != nil {
		t.Fatalf("brief not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "用 JWT") {
		t.Error("brief missing ADR decision content")
	}
	if !strings.Contains(content, "需要加 middleware") {
		t.Error("brief missing ADR consequence content")
	}
	if !strings.Contains(content, "OAuth 可行") {
		t.Error("brief missing Spike conclusion content")
	}
}

func TestReadInfraVersion_不存在時回傳0(t *testing.T) {
	tmpDir := t.TempDir()
	v := ReadInfraVersion(tmpDir)
	if v != 0 {
		t.Errorf("ReadInfraVersion = %d, want 0", v)
	}
}

func TestReadInfraVersion_讀取新位置(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	os.WriteFile(filepath.Join(claudeDir, ".infra-version"), []byte("3"), 0o644)

	v := ReadInfraVersion(tmpDir)
	if v != 3 {
		t.Errorf("ReadInfraVersion = %d, want 3", v)
	}
}

func TestInfraUpdateAvailable(t *testing.T) {
	tmpDir := t.TempDir()

	// No version file → needs update
	if !InfraUpdateAvailable(tmpDir) {
		t.Error("should need update when no version file exists")
	}

	// Write current version → no update needed
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	os.WriteFile(filepath.Join(claudeDir, ".infra-version"), []byte(strconv.Itoa(plan.InfraVersion)), 0o644)

	if InfraUpdateAvailable(tmpDir) {
		t.Error("should NOT need update when version matches")
	}
}

func TestExecute_寫入版本檔(t *testing.T) {
	tmpDir := t.TempDir()
	piperDir := filepath.Join(tmpDir, "piper")
	os.MkdirAll(piperDir, 0o755)

	epicBase := filepath.Join(tmpDir, "epic")
	os.MkdirAll(epicBase, 0o755)

	io := &mockStoryMapIO{content: testYAML}
	resolver := &mockProjectResolver{projects: map[string]string{"piper": piperDir}}

	svc := New(io, resolver, epicBase)
	_, err := svc.Execute(DispatchRequest{EpicID: "epic-12345"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify infra version file was written
	v := ReadInfraVersion(piperDir)
	if v != plan.InfraVersion {
		t.Errorf("infra version = %d, want %d", v, plan.InfraVersion)
	}
}

func TestSyncInfra_更新基礎設施與版本(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "myproj")
	os.MkdirAll(projDir, 0o755)

	svc := New(&mockStoryMapIO{}, &mockProjectResolver{}, tmpDir)
	if err := svc.SyncInfra(projDir); err != nil {
		t.Fatalf("SyncInfra failed: %v", err)
	}

	// Skills/rules deployed
	if _, err := os.Stat(filepath.Join(projDir, ".claude", "skills", "submit", "SKILL.md")); err != nil {
		t.Error("submit skill not deployed")
	}
	if _, err := os.Stat(filepath.Join(projDir, ".claude", "skills", "verify", "SKILL.md")); err != nil {
		t.Error("verify skill not deployed")
	}
	if _, err := os.Stat(filepath.Join(projDir, ".claude", "rules", "task-brief.md")); err != nil {
		t.Error("task-brief rule not deployed")
	}
	if _, err := os.Stat(filepath.Join(projDir, ".claude", "rules", "wave-isolation.md")); err != nil {
		t.Error("wave-isolation rule not deployed")
	}

	// Version file written
	v := ReadInfraVersion(projDir)
	if v != plan.InfraVersion {
		t.Errorf("infra version = %d, want %d", v, plan.InfraVersion)
	}
}

func TestGetInfraStatus_回傳各專案版本狀態(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup: piper has been dispatched (has .plan/), frontend has not
	piperDir := filepath.Join(tmpDir, "piper")
	frontendDir := filepath.Join(tmpDir, "frontend")
	os.MkdirAll(filepath.Join(piperDir, ".plan"), 0o755)
	os.MkdirAll(frontendDir, 0o755)

	io := &mockStoryMapIO{content: testYAML}
	resolver := &mockProjectResolver{projects: map[string]string{
		"piper":    piperDir,
		"frontend": frontendDir,
	}}

	svc := New(io, resolver, tmpDir)
	status, err := svc.GetInfraStatus("epic-12345")
	if err != nil {
		t.Fatalf("GetInfraStatus failed: %v", err)
	}

	if status.CurrentVersion != plan.InfraVersion {
		t.Errorf("currentVersion = %d, want %d", status.CurrentVersion, plan.InfraVersion)
	}

	// Both projects should be listed (no longer filtered by .plan/ existence)
	if len(status.Projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(status.Projects))
	}
	for _, p := range status.Projects {
		if !p.UpdateAvailable {
			t.Errorf("project %q should need update (version 0)", p.Name)
		}
	}
}
