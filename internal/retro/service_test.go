package retro

import (
	"os"
	"strings"
	"testing"
)

// fakeExecutorStore implements ExecutorStore with in-memory maps.
type fakeExecutorStore struct {
	storyMap  map[string]string            // epicID → YAML
	reports   map[string]map[string]string // epicID → {reportName → content}
	snapshots map[string]map[string][]byte // epicID → {reportName → json bytes}
	actions   map[string]string            // epicID → actions.yaml
	writes    int                          // how many times WriteStoryMap fired
}

func newFakeStore() *fakeExecutorStore {
	return &fakeExecutorStore{
		storyMap:  map[string]string{},
		reports:   map[string]map[string]string{},
		snapshots: map[string]map[string][]byte{},
		actions:   map[string]string{},
	}
}

func (f *fakeExecutorStore) ReadStoryMap(epicID string) (string, error) {
	return f.storyMap[epicID], nil
}

func (f *fakeExecutorStore) WriteStoryMap(epicID, content string) error {
	f.storyMap[epicID] = content
	f.writes++
	return nil
}

func (f *fakeExecutorStore) ListRetroReports(epicID string) ([]string, error) {
	m := f.reports[epicID]
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names, nil
}

func (f *fakeExecutorStore) ReadRetroReport(epicID, name string) (string, error) {
	m := f.reports[epicID]
	if c, ok := m[name]; ok {
		return c, nil
	}
	return "", nil
}

func (f *fakeExecutorStore) WriteRetroReport(epicID, name, content string) error {
	if f.reports[epicID] == nil {
		f.reports[epicID] = map[string]string{}
	}
	f.reports[epicID][name] = content
	return nil
}

func (f *fakeExecutorStore) WriteRetroReportJSON(epicID, name string, data []byte) error {
	if f.snapshots[epicID] == nil {
		f.snapshots[epicID] = map[string][]byte{}
	}
	f.snapshots[epicID][name] = data
	return nil
}

func (f *fakeExecutorStore) ReadRetroReportJSON(epicID, name string) ([]byte, error) {
	m := f.snapshots[epicID]
	if data, ok := m[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeExecutorStore) ReadRetroActions(epicID string) (string, error) {
	return f.actions[epicID], nil
}

func (f *fakeExecutorStore) WriteRetroActions(epicID, content string) error {
	if f.actions == nil {
		f.actions = map[string]string{}
	}
	f.actions[epicID] = content
	return nil
}

type fakeResolver struct{}

func (fakeResolver) ResolveProject(name string) (string, bool) { return "", false }

const testRetroYAML = `meta:
  name: "Retro Test Epic"
  updated: "2026-04-22"
  releases:
    - id: rel-mvp
      label: "MVP"
    - id: rel-v2
      label: "v2"

activities:
  - id: act-1
    name: "Act 1"
    steps:
      - id: step-1
        name: "Step 1"
        stories:
          - id: s-ready
            name: "Ready to close"
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
              - id: t2
                name: T2
                targetProject: proj
                status: done
          - id: s-partial
            name: "Partial"
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t3
                name: T3
                targetProject: proj
                status: done
              - id: t4
                name: T4
                targetProject: proj
                status: in-progress
          - id: s-already-done
            name: "Already done"
            release: rel-v2
            status: done
            tasks:
              - id: t5
                name: T5
                targetProject: proj
                status: done
`

func TestPreview_FindsClosableStoriesAndRetroActions(t *testing.T) {
	store := newFakeStore()
	store.storyMap["epic-1"] = testRetroYAML
	svc := New(store, fakeResolver{}).WithExecutor(store)

	preview, err := svc.Preview("epic-1")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if preview.EpicName != "Retro Test Epic" {
		t.Errorf("EpicName = %q", preview.EpicName)
	}
	if len(preview.ClosableStories) != 1 || preview.ClosableStories[0].StoryID != "s-ready" {
		t.Errorf("ClosableStories = %+v, want single entry for s-ready", preview.ClosableStories)
	}

	// rel-v2 already all-done → retro 報告沒有 → should generate
	// rel-mvp 的 s-ready 會被關 + s-partial 還未完成 → rel-mvp 不算 retro-ready
	if len(preview.RetroActions) != 1 {
		t.Fatalf("RetroActions len = %d, want 1", len(preview.RetroActions))
	}
	if preview.RetroActions[0].ReleaseID != "rel-v2" {
		t.Errorf("retro release = %q, want rel-v2", preview.RetroActions[0].ReleaseID)
	}
	if preview.RetroActions[0].Action != "generate" {
		t.Errorf("action = %q, want generate", preview.RetroActions[0].Action)
	}
}

func TestExecute_ClosesStoriesAndGeneratesRetro(t *testing.T) {
	store := newFakeStore()
	// Simpler YAML: one story ready to close AND its release becomes retro-ready after closure.
	store.storyMap["epic-1"] = `meta:
  name: "Simple"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: a1
    name: A
    steps:
      - id: s1
        name: S
        stories:
          - id: st-ready
            name: Ready
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
`
	svc := New(store, fakeResolver{}).WithExecutor(store)

	result, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.ClosedStories) != 1 || result.ClosedStories[0].StoryID != "st-ready" {
		t.Errorf("closed stories = %+v", result.ClosedStories)
	}
	if len(result.Retros) != 1 {
		t.Fatalf("retros len = %d, want 1", len(result.Retros))
	}
	if result.Retros[0].Status != "generated" {
		t.Errorf("retro status = %q, want generated", result.Retros[0].Status)
	}

	// Report should be stored and be longer than the threshold.
	report := store.reports["epic-1"][result.Retros[0].ReportName]
	if len(report) < MinRetroReportSize {
		t.Errorf("report size = %d, want >= %d", len(report), MinRetroReportSize)
	}

	if store.writes != 1 {
		t.Errorf("WriteStoryMap called %d times, want 1", store.writes)
	}
}

func TestExecute_IdempotentOnSecondRun(t *testing.T) {
	store := newFakeStore()
	store.storyMap["epic-1"] = `meta:
  name: "Simple"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: a1
    name: A
    steps:
      - id: s1
        name: S
        stories:
          - id: st-ready
            name: Ready
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
`
	svc := New(store, fakeResolver{}).WithExecutor(store)

	if _, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"}); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	result, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"})
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	// Second run: story already closed, no more closure; retro report exists, should skip.
	if len(result.ClosedStories) != 0 {
		t.Errorf("2nd run closed stories = %+v, want 0", result.ClosedStories)
	}
	if len(result.Retros) != 1 || result.Retros[0].Status != "skipped" {
		t.Errorf("2nd run retros = %+v, want single skipped", result.Retros)
	}
}

func TestExecute_ForceReleasesRegenerate(t *testing.T) {
	store := newFakeStore()
	store.storyMap["epic-1"] = `meta:
  name: "Simple"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: a1
    name: A
    steps:
      - id: s1
        name: S
        stories:
          - id: st-ready
            name: Ready
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
`
	svc := New(store, fakeResolver{}).WithExecutor(store)

	// First run generates the report.
	if _, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"}); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	// Second run with ForceReleases should regenerate.
	result, err := svc.Execute(ExecuteRequest{EpicID: "epic-1", ForceReleases: []string{"rel-mvp"}})
	if err != nil {
		t.Fatalf("forced Execute: %v", err)
	}
	if len(result.Retros) != 1 || result.Retros[0].Status != "regenerated" {
		t.Errorf("forced retros = %+v, want single regenerated", result.Retros)
	}
}

func TestExecute_InvalidReportTriggersRegeneration(t *testing.T) {
	store := newFakeStore()
	store.storyMap["epic-1"] = `meta:
  name: "Simple"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: a1
    name: A
    steps:
      - id: s1
        name: S
        stories:
          - id: st-ready
            name: Ready
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
`
	svc := New(store, fakeResolver{}).WithExecutor(store)

	// Seed a too-short report so hasValidRetroReport returns false.
	store.reports["epic-1"] = map[string]string{
		"release-mvp": strings.Repeat("x", MinRetroReportSize-10),
	}

	result, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Tiny stub is below threshold → treated as missing → action=generate.
	if len(result.Retros) != 1 || result.Retros[0].Status != "generated" {
		t.Errorf("retros = %+v, want single generated", result.Retros)
	}
	// After execution, the report should have been replaced with a real one above threshold.
	fresh := store.reports["epic-1"]["release-mvp"]
	if len(fresh) < MinRetroReportSize {
		t.Errorf("replacement report size = %d, below threshold", len(fresh))
	}
}

func TestPreview_NoExecutorReturnsError(t *testing.T) {
	svc := New(newFakeStore(), fakeResolver{}) // no WithExecutor
	if _, err := svc.Preview("epic-1"); err == nil {
		t.Fatal("expected error when executor store is not configured")
	}
}

func TestExecute_WritesJSONSnapshot(t *testing.T) {
	store := newFakeStore()
	store.storyMap["epic-1"] = `meta:
  name: "Simple"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: a1
    name: A
    steps:
      - id: s1
        name: S
        stories:
          - id: st-ready
            name: Ready
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t1
                name: T1
                targetProject: proj
                status: done
`
	svc := New(store, fakeResolver{}).WithExecutor(store)

	if _, err := svc.Execute(ExecuteRequest{EpicID: "epic-1"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	snapshot, ok := store.snapshots["epic-1"]["release-mvp"]
	if !ok || len(snapshot) == 0 {
		t.Fatal("expected JSON snapshot to be written alongside markdown")
	}
	if !strings.Contains(string(snapshot), "\"releaseLabel\"") {
		t.Errorf("snapshot does not look like ReportData JSON: %s", snapshot)
	}
}

func TestActions_CRUDFlow(t *testing.T) {
	store := newFakeStore()
	svc := New(store, fakeResolver{}).WithExecutor(store)

	// Empty state
	actions, err := svc.ListActions("epic-1")
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected empty actions, got %d", len(actions))
	}

	// Add
	a1, err := svc.AddAction("epic-1", "release-mvp", "加 evidence 欄位")
	if err != nil {
		t.Fatalf("AddAction: %v", err)
	}
	if a1.ID == "" || a1.Status != ActionStatusTodo {
		t.Errorf("new action malformed: %+v", a1)
	}
	a2, err := svc.AddAction("epic-1", "", "整理 story 命名")
	if err != nil {
		t.Fatalf("AddAction 2: %v", err)
	}

	actions, _ = svc.ListActions("epic-1")
	if len(actions) != 2 {
		t.Fatalf("after add, len = %d, want 2", len(actions))
	}

	// Toggle done
	if err := svc.UpdateActionStatus("epic-1", a1.ID, ActionStatusDone); err != nil {
		t.Fatalf("UpdateActionStatus done: %v", err)
	}
	actions, _ = svc.ListActions("epic-1")
	found := false
	for _, a := range actions {
		if a.ID == a1.ID {
			found = true
			if a.Status != ActionStatusDone {
				t.Errorf("status = %q, want done", a.Status)
			}
			if a.AppliedAt == "" {
				t.Error("AppliedAt should be set when done")
			}
		}
	}
	if !found {
		t.Fatal("updated action not found")
	}

	// Toggle back to todo clears AppliedAt
	if err := svc.UpdateActionStatus("epic-1", a1.ID, ActionStatusTodo); err != nil {
		t.Fatalf("UpdateActionStatus todo: %v", err)
	}
	actions, _ = svc.ListActions("epic-1")
	for _, a := range actions {
		if a.ID == a1.ID && a.AppliedAt != "" {
			t.Errorf("AppliedAt should be cleared on revert, got %q", a.AppliedAt)
		}
	}

	// Invalid status rejected
	if err := svc.UpdateActionStatus("epic-1", a1.ID, "skipped"); err == nil {
		t.Error("expected error for invalid status")
	}

	// Delete
	if err := svc.DeleteAction("epic-1", a2.ID); err != nil {
		t.Fatalf("DeleteAction: %v", err)
	}
	actions, _ = svc.ListActions("epic-1")
	if len(actions) != 1 {
		t.Errorf("after delete, len = %d, want 1", len(actions))
	}

	// Unknown ID rejected
	if err := svc.DeleteAction("epic-1", "action-nope"); err == nil {
		t.Error("expected error deleting unknown action")
	}
}

func TestActions_AddRequiresDescription(t *testing.T) {
	store := newFakeStore()
	svc := New(store, fakeResolver{}).WithExecutor(store)
	if _, err := svc.AddAction("epic-1", "", "   "); err == nil {
		t.Error("expected error for empty description")
	}
}

func TestGetTrends_MixSnapshotAndLegacy(t *testing.T) {
	store := newFakeStore()
	// Pretend there are three reports: two with JSON snapshots, one legacy.
	store.reports["epic-1"] = map[string]string{
		"release-v1":  "# Retro v1\n\ncontent",
		"release-v2":  "# Retro v2\n\ncontent",
		"release-mvp": "# Retro MVP\n\ncontent (legacy, no json)",
	}
	store.snapshots["epic-1"] = map[string][]byte{
		"release-v1": []byte(`{"EpicName":"E","ReleaseLabel":"v1","FirstPassRate":60,"TotalTasks":5,"GapTotal":3,"Verify":{"AvgLoopCount":1.5}}`),
		"release-v2": []byte(`{"EpicName":"E","ReleaseLabel":"v2","FirstPassRate":80,"TotalTasks":4,"GapTotal":1,"Verify":{"AvgLoopCount":0.8}}`),
	}
	svc := New(store, fakeResolver{}).WithExecutor(store)

	trends, err := svc.GetTrends("epic-1")
	if err != nil {
		t.Fatalf("GetTrends: %v", err)
	}
	if len(trends.Releases) != 3 {
		t.Fatalf("Releases len = %d, want 3", len(trends.Releases))
	}
	// Sorted by name → release-mvp, release-v1, release-v2
	wantOrder := []string{"release-mvp", "release-v1", "release-v2"}
	for i, e := range trends.Releases {
		if e.ReportName != wantOrder[i] {
			t.Errorf("Releases[%d].ReportName = %q, want %q", i, e.ReportName, wantOrder[i])
		}
	}
	if trends.Releases[0].HasSnapshot {
		t.Error("legacy release-mvp should have HasSnapshot=false")
	}
	if trends.Releases[0].ReleaseLabel != "mvp" {
		t.Errorf("legacy release-mvp label = %q, want %q (fallback from name)", trends.Releases[0].ReleaseLabel, "mvp")
	}
	if !trends.Releases[1].HasSnapshot || trends.Releases[1].FirstPassRate != 60 {
		t.Errorf("release-v1 snapshot not populated: %+v", trends.Releases[1])
	}
	if trends.Releases[2].AvgLoopCount != 0.8 {
		t.Errorf("release-v2 AvgLoopCount = %v, want 0.8", trends.Releases[2].AvgLoopCount)
	}
}
