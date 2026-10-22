package retro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockStoryMapIO implements StoryMapIO for testing.
type mockStoryMapIO struct {
	data map[string]string
}

func (m *mockStoryMapIO) ReadStoryMap(epicID string) (string, error) {
	return m.data[epicID], nil
}

// mockResolver implements ProjectResolver for testing.
type mockResolver struct {
	projects map[string]string
}

func (m *mockResolver) ResolveProject(name string) (string, bool) {
	p, ok := m.projects[name]
	return p, ok
}

const testStoryMapYAML = `meta:
  name: "Test Epic"
  updated: "2026-04-16T00:00:00Z"
  releases:
    - id: r1
      label: MVP
activities:
  - id: act-1
    name: Core
    steps:
      - id: step-1
        name: Step
        stories:
          - id: s1
            name: Story with AAN
            status: done
            release: r1
            always: [run tests]
            ask: [new deps]
            never: [break api]
            tasks:
              - id: task-a1
                name: Task A1
                targetProject: proj-a
                status: done
          - id: s2
            name: Story without AAN
            status: done
            release: r1
            tasks:
              - id: task-b1
                name: Task B1
                targetProject: proj-a
                status: done
`

func TestCollectMetrics(t *testing.T) {
	// Setup temp project dir with metrics.jsonl
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "proj-a")
	planDir := filepath.Join(projDir, ".plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	metrics := []SubTaskMetric{
		{Task: "task-a1", Timestamp: "2026-04-16T10:00:00Z", PassedFirstTry: true, GapCount: 0},
		{Task: "task-b1", Timestamp: "2026-04-16T11:00:00Z", PassedFirstTry: false, GapCount: 2, GapTypes: []string{"missing_test", "boundary_case"}, ReworkCount: 1},
	}
	var lines []string
	for _, m := range metrics {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	metricsContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(planDir, "metrics.jsonl"), []byte(metricsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": projDir}},
	)

	result, err := svc.CollectMetrics("epic-1")
	if err != nil {
		t.Fatalf("CollectMetrics: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d metrics, want 2", len(result))
	}
	if !result[0].PassedFirstTry {
		t.Error("first metric should have passed_first_try=true")
	}
	if result[1].GapCount != 2 {
		t.Errorf("second metric gap_count = %d, want 2", result[1].GapCount)
	}
}

func TestCollectMetrics_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": tmpDir}},
	)

	result, err := svc.CollectMetrics("epic-1")
	if err != nil {
		t.Fatalf("CollectMetrics: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d metrics, want 0", len(result))
	}
}

func TestGenerateReport(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "proj-a")
	planDir := filepath.Join(projDir, ".plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	metrics := []SubTaskMetric{
		{Task: "task-a1", PassedFirstTry: true, GapCount: 0},
		{Task: "task-b1", PassedFirstTry: false, GapCount: 3, GapTypes: []string{"missing_test", "missing_test", "acceptance_uncovered"}, GapDetails: []string{"handler 欄位命名不一致"}, ReworkCount: 1},
	}
	var lines []string
	for _, m := range metrics {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	os.WriteFile(filepath.Join(planDir, "metrics.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": projDir}},
	)

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	// Check report contains all sections
	sections := []string{
		"# Retro: MVP",
		"總覽",
		"執行品質",
		"規劃品質診斷",
		"指標盲區檢查",
		"建議行動",
		"一次通過率",
		"missing_test",
		"acceptance_uncovered",
	}
	for _, section := range sections {
		if !strings.Contains(report, section) {
			t.Errorf("report missing: %s", section)
		}
	}

	// Check uncategorized gaps are listed
	if !strings.Contains(report, "handler 欄位命名不一致") {
		t.Error("report should contain uncategorized gap detail")
	}
}

func TestGenerateReport_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": tmpDir}},
	)

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if !strings.Contains(report, "未分類 gap：0 筆") {
		t.Error("should show 0 uncategorized gaps")
	}
}

func TestReadWriteActions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "retro", "actions.yaml")

	af := &ActionsFile{
		Actions: []Action{
			{ID: "a1", FromRetro: "release-r1", Description: "加 missing_test 進 Always 規則", Status: "todo"},
			{ID: "a2", FromRetro: "release-r1", Description: "觀察 context 污染", Status: "done", AppliedAt: "2026-04-20"},
		},
	}

	if err := WriteActions(path, af); err != nil {
		t.Fatalf("WriteActions: %v", err)
	}

	result, err := ReadActions(path)
	if err != nil {
		t.Fatalf("ReadActions: %v", err)
	}

	if len(result.Actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(result.Actions))
	}
	if result.Actions[0].Status != "todo" {
		t.Errorf("action 0 status = %s, want todo", result.Actions[0].Status)
	}
	if result.Actions[1].AppliedAt != "2026-04-20" {
		t.Errorf("action 1 applied_at = %s", result.Actions[1].AppliedAt)
	}
}

func TestReadActions_NotExist(t *testing.T) {
	result, err := ReadActions("/nonexistent/actions.yaml")
	if err != nil {
		t.Fatalf("ReadActions: %v", err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("got %d actions, want 0", len(result.Actions))
	}
}

func TestGenerateReport_VerifyStats(t *testing.T) {
	tmpDir := t.TempDir()
	projDir := filepath.Join(tmpDir, "proj-a")
	planDir := filepath.Join(projDir, ".plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}

	metrics := []SubTaskMetric{
		{Task: "task-a1", PassedFirstTry: true, VerifyLoopCount: 0, VerifyPassedFirstTry: true},
		{Task: "task-b1", PassedFirstTry: true, VerifyLoopCount: 3, VerifyPassedFirstTry: false},
		{Task: "task-c1", PassedFirstTry: false, VerifyLoopCount: 10, VerifyExhausted: true, DiffGateBlocked: true, DoneSamplingFailed: true,
			GapCount: 1, GapTypes: []string{"sampling_failed"}},
	}
	var lines []string
	for _, m := range metrics {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	os.WriteFile(filepath.Join(planDir, "metrics.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": projDir}},
	)

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	for _, want := range []string{
		"## Verify 統計",
		"已跑過 /submit 的 task | 3",
		"/submit 耗盡重試上限 | 1",
		"Diff-gate 擋下",
		"/verify 抽驗失敗 | 1",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n---\n%s", want, report)
		}
	}
}

func TestGenerateReport_VerifyStats_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": tmpDir}},
	)
	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !strings.Contains(report, "本 release 無自測指標資料") {
		t.Errorf("should show empty-verify message:\n%s", report)
	}
	if !strings.Contains(report, "本 release 無契約 changelog 資料") {
		t.Errorf("should show empty-contract message:\n%s", report)
	}
}

// mockContractLoader implements ContractLoader for testing.
type mockContractLoader struct {
	list map[string][]string // epicID → ids
	read map[string]string   // epicID/id → YAML
}

func (m *mockContractLoader) ListContracts(epicID string) ([]string, error) {
	return m.list[epicID], nil
}
func (m *mockContractLoader) ReadContract(epicID, id string) (string, error) {
	return m.read[epicID+"/"+id], nil
}

func TestGenerateReport_ContractStats(t *testing.T) {
	loader := &mockContractLoader{
		list: map[string][]string{"epic-1": {"api-a", "api-b"}},
		read: map[string]string{
			"epic-1/api-a": `id: api-a
version: "1.1.0"
type: http-api
changelog:
  - version: "1.0.0"
    date: "2026-04-01"
    type: additive
    summary: initial
  - version: "1.1.0"
    date: "2026-04-10"
    type: breaking
    summary: rename field
    reason: legal
`,
			"epic-1/api-b": `id: api-b
version: "1.0.0"
type: http-api
changelog:
  - version: "1.0.0"
    date: "2026-04-02"
    type: additive
    summary: add endpoint
`,
		},
	}

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{}},
	).WithContractLoader(loader)

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	for _, want := range []string{
		"## 契約修改統計",
		"breaking | 1",
		"additive | 2",
		"| api-a | 1.0.0 | additive |",
		"| api-a | 1.1.0 | breaking | rename field | legal |",
		"| api-b | 1.0.0 | additive |",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n---\n%s", want, report)
		}
	}
}

// mockDecisionLoader implements DecisionLoader for testing.
type mockDecisionLoader struct {
	raw map[string]string
	err error
}

func (m *mockDecisionLoader) ReadRetroDecisions(epicID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.raw[epicID], nil
}

func writeMetrics(t *testing.T, dir string, metrics []SubTaskMetric) {
	t.Helper()
	planDir := filepath.Join(dir, ".plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, m := range metrics {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(planDir, "metrics.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateReport_DecisionReview_顯示與gap對應的決策(t *testing.T) {
	projDir := filepath.Join(t.TempDir(), "proj-a")
	writeMetrics(t, projDir, []SubTaskMetric{
		{Task: "task-a1", PassedFirstTry: true, GapCount: 0},
		{Task: "task-b1", PassedFirstTry: false, GapCount: 2, GapTypes: []string{"missing_test", "acceptance_uncovered"}},
	})

	// 一筆針對 task-a1（無 gap，應被忽略），一筆針對 story s2（task-b1 的 parent，應顯示），
	// 一筆針對無關 target（應忽略）
	decisions := strings.Join([]string{
		`{"ts":"2026-04-20T10:00:00Z","type":"tags","target":"task-a1","rationale":"推導 tags","output":"tags: [backend]"}`,
		`{"ts":"2026-04-20T10:05:00Z","type":"task_split","target":"s2","context":["story-map"],"rationale":"只切後端 task","alternatives":"考慮過拆前端","output":"1 task: task-b1"}`,
		`{"ts":"2026-04-20T10:10:00Z","type":"aan","target":"s999","rationale":"無關 target"}`,
	}, "\n") + "\n"

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": projDir}},
	).WithDecisionLoader(&mockDecisionLoader{raw: map[string]string{"epic-1": decisions}})

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	for _, want := range []string{
		"## 決策回顧",
		"### task-b1 (story s2) — 2 gap",
		"task_split",
		"rationale: 只切後端 task",
		"alternatives: 考慮過拆前端",
		"output: 1 task: task-b1",
		"context: story-map",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n---\n%s", want, report)
		}
	}

	for _, unwanted := range []string{
		"task-a1 (story s1)", // 無 gap，不應出現在決策回顧
		"s999",               // 無關 target
	} {
		if strings.Contains(report, unwanted) {
			t.Errorf("report should not contain %q", unwanted)
		}
	}
}

func TestGenerateReport_DecisionReview_無DecisionLoader時顯示未載入(t *testing.T) {
	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{}},
	)

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !strings.Contains(report, "本 release 未載入 decision log") {
		t.Errorf("report should note DecisionLoader not wired\n---\n%s", report)
	}
}

func TestGenerateReport_DecisionReview_有loader但無匹配時顯示空狀態(t *testing.T) {
	projDir := filepath.Join(t.TempDir(), "proj-a")
	writeMetrics(t, projDir, []SubTaskMetric{
		{Task: "task-a1", PassedFirstTry: true, GapCount: 0},
	})

	svc := New(
		&mockStoryMapIO{data: map[string]string{"epic-1": testStoryMapYAML}},
		&mockResolver{projects: map[string]string{"proj-a": projDir}},
	).WithDecisionLoader(&mockDecisionLoader{raw: map[string]string{"epic-1": ""}})

	report, err := svc.GenerateReport("epic-1", "MVP")
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !strings.Contains(report, "無對應的 decision log") {
		t.Errorf("report should show empty-state hint\n---\n%s", report)
	}
}

func TestParseJSONL_跳過格式錯誤行(t *testing.T) {
	raw := `{"ts":"2026-04-20T10:00:00Z","type":"tags","target":"s1","rationale":"ok"}
not valid json
{"ts":"2026-04-20T10:05:00Z","type":"aan","target":"s1","rationale":"ok2"}
`
	got := parseJSONL[DecisionEntry](strings.NewReader(raw), "decisions.jsonl")
	if len(got) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(got))
	}
	if got[0].Type != "tags" || got[1].Type != "aan" {
		t.Errorf("unexpected types: %v / %v", got[0].Type, got[1].Type)
	}
}
