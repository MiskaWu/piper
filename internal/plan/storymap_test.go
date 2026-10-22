package plan

import (
	"strings"
	"testing"
)

const testStoryMapYAML = `meta:
  name: "Test Epic"
  updated: "2026-01-01T00:00:00Z"
  owner: "tester"
  releases:
    - id: rel-mvp01
      label: "MVP"
      color: blue
    - id: rel-v2
      label: "v2.0"

activities:
  - id: act-001
    name: "使用者管理"
    steps:
      - id: step-001
        name: "登入流程"
        stories:
          - id: story-aaa11
            name: "使用者登入"
            description: "身為使用者，我想要登入"
            tags: [auth, core]
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
        name: "權限管理"
        stories:
          - id: story-ccc33
            name: "角色設定"
            release: rel-v2
            status: todo
            tasks:
              - id: task-c1
                name: "角色設定 UI"
                targetProject: frontend
                status: todo
  - id: act-002
    name: "Dashboard"
    steps:
      - id: step-003
        name: "總覽頁"
        stories:
          - id: story-ddd44
            name: "統計面板"
            release: rel-mvp01
            status: todo
            tasks:
              - id: task-d1
                name: "統計面板 UI"
                targetProject: frontend
                status: todo
          - id: story-eee55
            name: "圖表元件"
            release: rel-mvp01
            status: todo
            tasks:
              - id: task-e1
                name: "圖表元件實作"
                targetProject: frontend
                status: todo
`

func TestValidateStoryMap_合法YAML通過驗證(t *testing.T) {
	sm, errs := ValidateStoryMap([]byte(testStoryMapYAML))
	if errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if sm.Meta.Name != "Test Epic" {
		t.Errorf("meta name = %q, want %q", sm.Meta.Name, "Test Epic")
	}
}

func TestValidateStoryMap_空內容回傳錯誤(t *testing.T) {
	_, errs := ValidateStoryMap([]byte(""))
	if errs == nil {
		t.Fatal("expected errors for empty content")
	}
}

func TestValidateStoryMap_非法YAML回傳錯誤(t *testing.T) {
	_, errs := ValidateStoryMap([]byte(":::invalid"))
	if errs == nil {
		t.Fatal("expected errors for invalid YAML")
	}
}

func TestValidateStoryMap_缺少meta名稱回傳錯誤(t *testing.T) {
	input := `meta:
  updated: "2026-01-01"
activities: []
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors for missing meta.name")
	}
	assertContainsError(t, errs, "meta.name")
}

func TestValidateStoryMap_Activity缺少ID回傳錯誤(t *testing.T) {
	input := `meta:
  name: "Test"
activities:
  - name: "Act1"
    steps: []
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors for activity missing id")
	}
	assertContainsError(t, errs, "missing id")
}

func TestValidateStoryMap_Story缺少tags時自動補空陣列(t *testing.T) {
	input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - id: task-001
                name: "Task1"
                targetProject: proj
                status: todo
`
	sm, errs := ValidateStoryMap([]byte(input))
	if errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	story := sm.Activities[0].Steps[0].Stories[0]
	if story.Tags == nil {
		t.Error("tags should be initialized to empty slice, not nil")
	}
}

func TestValidateStoryMap_重複StoryID回傳錯誤(t *testing.T) {
	input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-dup
            name: "Story1"
            status: todo
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: todo
          - id: story-dup
            name: "Story2"
            status: todo
            tasks:
              - id: task-002
                name: "T2"
                targetProject: proj
                status: todo
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors for duplicate story ID")
	}
	assertContainsError(t, errs, "story-dup")
	assertContainsError(t, errs, "2 times")
}

func TestValidateStoryMap_不合法Status回傳錯誤(t *testing.T) {
	input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: wip
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: todo
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors for invalid status")
	}
	assertContainsError(t, errs, "invalid status")
	assertContainsError(t, errs, "wip")
}

func TestValidateStoryMap_Release引用不存在回傳錯誤(t *testing.T) {
	input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            release: rel-nonexistent
            status: todo
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: todo
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors for release reference")
	}
	assertContainsError(t, errs, "rel-nonexistent")
	assertContainsError(t, errs, "not defined")
}

func TestValidateStoryMap_多個錯誤一次回傳(t *testing.T) {
	input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-dup
            name: "S1"
            status: invalid-status
            release: rel-missing
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: todo
          - id: story-dup
            name: "S2"
            status: todo
            tasks:
              - id: task-002
                name: "T2"
                targetProject: proj
                status: todo
`
	_, errs := ValidateStoryMap([]byte(input))
	if errs == nil {
		t.Fatal("expected errors")
	}
	// Should have at least 3 errors: bad status, bad release, duplicate ID
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateStoryMap_TaskValidation(t *testing.T) {
	t.Run("story無task允許存檔（向後相容）", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs != nil {
			t.Fatalf("expected no errors for story without tasks (backward compat), got: %v", errs)
		}
	})

	t.Run("task缺少ID回傳錯誤", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - name: "T1"
                targetProject: proj
                status: todo
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs == nil {
			t.Fatal("expected errors for task missing id")
		}
		assertContainsError(t, errs, "task[0] missing id")
	})

	t.Run("task缺少targetProject回傳錯誤", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - id: task-001
                name: "T1"
                status: todo
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs == nil {
			t.Fatal("expected errors for task missing targetProject")
		}
		assertContainsError(t, errs, "missing targetProject")
	})

	t.Run("重複taskID回傳錯誤", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - id: task-dup
                name: "T1"
                targetProject: proj
                status: todo
              - id: task-dup
                name: "T2"
                targetProject: proj
                status: todo
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs == nil {
			t.Fatal("expected errors for duplicate task ID")
		}
		assertContainsError(t, errs, "task-dup")
		assertContainsError(t, errs, "2 times")
	})

	t.Run("task缺少name回傳錯誤", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - id: task-001
                targetProject: proj
                status: todo
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs == nil {
			t.Fatal("expected errors for task missing name")
		}
		assertContainsError(t, errs, "task \"task-001\" missing name")
	})

	t.Run("task不合法status回傳錯誤", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: todo
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: invalid
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs == nil {
			t.Fatal("expected errors for task invalid status")
		}
		assertContainsError(t, errs, "task \"task-001\" has invalid status")
	})

	t.Run("task支援in-review狀態", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "Story1"
            status: in-review
            tasks:
              - id: task-001
                name: "T1"
                targetProject: proj
                status: in-review
`
		_, errs := ValidateStoryMap([]byte(input))
		if errs != nil {
			t.Fatalf("unexpected errors: %v", errs)
		}
	})
}

// assertContainsError checks that at least one error message contains the substring.
func assertContainsError(t *testing.T, errs []string, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Errorf("expected an error containing %q, got: %v", substr, errs)
}

func TestParseStoryMap(t *testing.T) {
	sm, err := ParseStoryMap([]byte(testStoryMapYAML))
	if err != nil {
		t.Fatalf("ParseStoryMap failed: %v", err)
	}

	if sm.Meta.Name != "Test Epic" {
		t.Errorf("meta name = %q, want %q", sm.Meta.Name, "Test Epic")
	}
	if len(sm.Meta.Releases) != 2 {
		t.Errorf("releases count = %d, want 2", len(sm.Meta.Releases))
	}
	if len(sm.Activities) != 2 {
		t.Errorf("activities count = %d, want 2", len(sm.Activities))
	}

	// Check nested structure
	stories := sm.Activities[0].Steps[0].Stories
	if len(stories) != 2 {
		t.Fatalf("step-001 stories count = %d, want 2", len(stories))
	}
	if stories[0].ID != "story-aaa11" {
		t.Errorf("first story ID = %q, want %q", stories[0].ID, "story-aaa11")
	}
	if len(stories[0].Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(stories[0].Tags))
	}
	// Check tasks
	if len(stories[0].Tasks) != 2 {
		t.Fatalf("story-aaa11 tasks count = %d, want 2", len(stories[0].Tasks))
	}
	if stories[0].Tasks[0].ID != "task-a1" {
		t.Errorf("first task ID = %q, want %q", stories[0].Tasks[0].ID, "task-a1")
	}
}

func TestParseStoryMap_InvalidYAML(t *testing.T) {
	_, err := ParseStoryMap([]byte(":::invalid"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestMarshalStoryMap_Roundtrip(t *testing.T) {
	sm, err := ParseStoryMap([]byte(testStoryMapYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	data, err := MarshalStoryMap(sm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sm2, err := ParseStoryMap(data)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if sm2.Meta.Name != sm.Meta.Name {
		t.Errorf("roundtrip name mismatch: %q vs %q", sm2.Meta.Name, sm.Meta.Name)
	}
	if len(sm2.Activities) != len(sm.Activities) {
		t.Errorf("roundtrip activities mismatch: %d vs %d", len(sm2.Activities), len(sm.Activities))
	}
}

func TestFindStory(t *testing.T) {
	sm, _ := ParseStoryMap([]byte(testStoryMapYAML))

	tests := []struct {
		id           string
		wantFound    bool
		wantActivity string
		wantStep     string
		wantRelease  string
		wantSiblings int
	}{
		{"story-aaa11", true, "使用者管理", "登入流程", "MVP", 1},
		{"story-ccc33", true, "使用者管理", "權限管理", "v2.0", 0},
		{"story-ddd44", true, "Dashboard", "總覽頁", "MVP", 1},
		{"story-zzz99", false, "", "", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			ctx, found := sm.FindStory(tt.id)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if !found {
				return
			}
			if ctx.ActivityName != tt.wantActivity {
				t.Errorf("activity = %q, want %q", ctx.ActivityName, tt.wantActivity)
			}
			if ctx.StepName != tt.wantStep {
				t.Errorf("step = %q, want %q", ctx.StepName, tt.wantStep)
			}
			if ctx.ReleaseLabel != tt.wantRelease {
				t.Errorf("release = %q, want %q", ctx.ReleaseLabel, tt.wantRelease)
			}
			if len(ctx.Siblings) != tt.wantSiblings {
				t.Errorf("siblings = %d, want %d", len(ctx.Siblings), tt.wantSiblings)
			}
		})
	}
}

func TestSetStoryStatus(t *testing.T) {
	sm, _ := ParseStoryMap([]byte(testStoryMapYAML))

	if !sm.SetStoryStatus("story-aaa11", "in-progress") {
		t.Fatal("SetStoryStatus returned false for existing story")
	}
	ctx, _ := sm.FindStory("story-aaa11")
	if ctx.Story.Status != "in-progress" {
		t.Errorf("status = %q, want %q", ctx.Story.Status, "in-progress")
	}

	if sm.SetStoryStatus("story-zzz99", "done") {
		t.Error("SetStoryStatus returned true for non-existing story")
	}
}

func TestFilterDispatchable(t *testing.T) {
	sm, _ := ParseStoryMap([]byte(testStoryMapYAML))

	t.Run("all releases", func(t *testing.T) {
		groups := sm.FilterDispatchable("")
		// Dispatchable tasks (task status=todo + has targetProject):
		//   task-a1→piper, task-a2→frontend (story-aaa11)
		//   task-c1→frontend (story-ccc33)
		//   task-d1→frontend (story-ddd44)
		//   task-e1→frontend (story-eee55)
		// task-b1 excluded: status=in-progress (story status is irrelevant)
		if len(groups) != 2 {
			t.Fatalf("groups count = %d, want 2 (piper + frontend)", len(groups))
		}
		if len(groups["piper"]) != 1 {
			t.Errorf("piper tasks = %d, want 1", len(groups["piper"]))
		}
		if len(groups["frontend"]) != 4 {
			t.Errorf("frontend tasks = %d, want 4", len(groups["frontend"]))
		}
		// Verify TaskContext fields
		piperCtx := groups["piper"][0]
		if piperCtx.Task.ID != "task-a1" {
			t.Errorf("piper task ID = %q, want %q", piperCtx.Task.ID, "task-a1")
		}
		if piperCtx.StoryID != "story-aaa11" {
			t.Errorf("story ID = %q, want %q", piperCtx.StoryID, "story-aaa11")
		}
		if piperCtx.ActivityName != "使用者管理" {
			t.Errorf("activity = %q, want %q", piperCtx.ActivityName, "使用者管理")
		}
		if len(piperCtx.SiblingTasks) != 1 {
			t.Errorf("sibling tasks = %d, want 1", len(piperCtx.SiblingTasks))
		}
		if len(piperCtx.Acceptance) != 2 {
			t.Errorf("acceptance = %d, want 2", len(piperCtx.Acceptance))
		}
	})

	t.Run("specific release", func(t *testing.T) {
		groups := sm.FilterDispatchable("rel-mvp01")
		// MVP stories: story-aaa11 (task-a1→piper, task-a2→frontend),
		//              story-ddd44 (task-d1→frontend),
		//              story-eee55 (task-e1→frontend)
		// story-ccc33 is rel-v2 → excluded
		if len(groups["piper"]) != 1 {
			t.Errorf("piper MVP tasks = %d, want 1", len(groups["piper"]))
		}
		if len(groups["frontend"]) != 3 {
			t.Errorf("frontend MVP tasks = %d, want 3", len(groups["frontend"]))
		}
		// task-c1 (story-ccc33, rel-v2) should be excluded
		for _, ctx := range groups["frontend"] {
			if ctx.Task.ID == "task-c1" {
				t.Error("task-c1 should be excluded from MVP filter")
			}
		}
	})
}

func TestFindTask(t *testing.T) {
	sm, _ := ParseStoryMap([]byte(testStoryMapYAML))

	tests := []struct {
		id              string
		wantFound       bool
		wantStoryID     string
		wantActivity    string
		wantStep        string
		wantRelease     string
		wantSiblings    int
		wantAcceptance  int
	}{
		{"task-a1", true, "story-aaa11", "使用者管理", "登入流程", "MVP", 1, 2},
		{"task-a2", true, "story-aaa11", "使用者管理", "登入流程", "MVP", 1, 2},
		{"task-b1", true, "story-bbb22", "使用者管理", "登入流程", "MVP", 0, 0},
		{"task-c1", true, "story-ccc33", "使用者管理", "權限管理", "v2.0", 0, 0},
		{"task-zzz", false, "", "", "", "", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			ctx, found := sm.FindTask(tt.id)
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if !found {
				return
			}
			if ctx.StoryID != tt.wantStoryID {
				t.Errorf("storyID = %q, want %q", ctx.StoryID, tt.wantStoryID)
			}
			if ctx.ActivityName != tt.wantActivity {
				t.Errorf("activity = %q, want %q", ctx.ActivityName, tt.wantActivity)
			}
			if ctx.StepName != tt.wantStep {
				t.Errorf("step = %q, want %q", ctx.StepName, tt.wantStep)
			}
			if ctx.ReleaseLabel != tt.wantRelease {
				t.Errorf("release = %q, want %q", ctx.ReleaseLabel, tt.wantRelease)
			}
			if len(ctx.SiblingTasks) != tt.wantSiblings {
				t.Errorf("siblings = %d, want %d", len(ctx.SiblingTasks), tt.wantSiblings)
			}
			if len(ctx.Acceptance) != tt.wantAcceptance {
				t.Errorf("acceptance = %d, want %d", len(ctx.Acceptance), tt.wantAcceptance)
			}
		})
	}
}

func TestStats(t *testing.T) {
	// Use a dedicated YAML with mixed statuses and stories without tasks
	const statsYAML = `meta:
  name: "Stats Test"
  updated: "2026-01-01"
  releases:
    - id: rel-mvp
      label: "核心探索循環"
    - id: rel-v2
      label: "機運與成長"

activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: s1
            name: "Story1"
            release: rel-mvp
            status: todo
            tasks:
              - id: t1
                name: "T1"
                targetProject: projects/acme/server
                status: todo
              - id: t2
                name: "T2"
                targetProject: projects/acme/client
                status: in-progress
          - id: s2
            name: "Story2"
            release: rel-mvp
            status: in-progress
            tasks:
              - id: t3
                name: "T3"
                targetProject: projects/acme/server
                status: done
          - id: s3
            name: "Story3 no tasks"
            release: rel-mvp
            status: todo
          - id: s4
            name: "Story4 in-review"
            release: rel-v2
            status: in-review
            tasks:
              - id: t4
                name: "T4"
                targetProject: projects/acme/client
                status: in-review
      - id: step-002
        name: "Step2"
        stories:
          - id: s5
            name: "Story5 done"
            release: rel-mvp
            status: done
            tasks:
              - id: t5
                name: "T5"
                targetProject: projects/acme/server
                status: done
              - id: t6
                name: "T6"
                targetProject: projects/acme/client
                status: done
          - id: s6
            name: "Story6 no release no tasks"
            status: todo
`
	sm, err := ParseStoryMap([]byte(statsYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := sm.Stats()

	// Stories: s1=todo, s2=in-progress, s3=todo, s4=in-review, s5=done, s6=todo → total=6
	t.Run("story counts", func(t *testing.T) {
		if stats.Stories.Total != 6 {
			t.Errorf("stories.total = %d, want 6", stats.Stories.Total)
		}
		if stats.Stories.Todo != 3 {
			t.Errorf("stories.todo = %d, want 3", stats.Stories.Todo)
		}
		if stats.Stories.InProgress != 1 {
			t.Errorf("stories.inProgress = %d, want 1", stats.Stories.InProgress)
		}
		if stats.Stories.InReview != 1 {
			t.Errorf("stories.inReview = %d, want 1", stats.Stories.InReview)
		}
		if stats.Stories.Done != 1 {
			t.Errorf("stories.done = %d, want 1", stats.Stories.Done)
		}
	})

	// Tasks: t1=todo, t2=in-progress, t3=done, t4=in-review, t5=done, t6=done → total=6
	t.Run("task counts", func(t *testing.T) {
		if stats.Tasks.Total != 6 {
			t.Errorf("tasks.total = %d, want 6", stats.Tasks.Total)
		}
		if stats.Tasks.Todo != 1 {
			t.Errorf("tasks.todo = %d, want 1", stats.Tasks.Todo)
		}
		if stats.Tasks.InProgress != 1 {
			t.Errorf("tasks.inProgress = %d, want 1", stats.Tasks.InProgress)
		}
		if stats.Tasks.InReview != 1 {
			t.Errorf("tasks.inReview = %d, want 1", stats.Tasks.InReview)
		}
		if stats.Tasks.Done != 3 {
			t.Errorf("tasks.done = %d, want 3", stats.Tasks.Done)
		}
	})

	t.Run("byRelease uses label not ID", func(t *testing.T) {
		mvp, ok := stats.ByRelease["核心探索循環"]
		if !ok {
			t.Fatal("missing release '核心探索循環'")
		}
		// rel-mvp stories: s1, s2, s3, s5 → 4
		if mvp.Stories != 4 {
			t.Errorf("核心探索循環 stories = %d, want 4", mvp.Stories)
		}
		// rel-mvp tasks: t1, t2, t3, t5, t6 → 5
		if mvp.Tasks != 5 {
			t.Errorf("核心探索循環 tasks = %d, want 5", mvp.Tasks)
		}

		v2, ok := stats.ByRelease["機運與成長"]
		if !ok {
			t.Fatal("missing release '機運與成長'")
		}
		if v2.Stories != 1 {
			t.Errorf("機運與成長 stories = %d, want 1", v2.Stories)
		}
		if v2.Tasks != 1 {
			t.Errorf("機運與成長 tasks = %d, want 1", v2.Tasks)
		}
	})

	t.Run("stories without release grouped under empty key", func(t *testing.T) {
		noRel, ok := stats.ByRelease[""]
		if !ok {
			t.Fatal("missing empty-release group")
		}
		// s6 has no release
		if noRel.Stories != 1 {
			t.Errorf("no-release stories = %d, want 1", noRel.Stories)
		}
	})

	t.Run("byProject counts tasks by targetProject", func(t *testing.T) {
		// server: t1, t3, t5 → 3
		if stats.ByProject["projects/acme/server"] != 3 {
			t.Errorf("server tasks = %d, want 3", stats.ByProject["projects/acme/server"])
		}
		// client: t2, t4, t6 → 3
		if stats.ByProject["projects/acme/client"] != 3 {
			t.Errorf("client tasks = %d, want 3", stats.ByProject["projects/acme/client"])
		}
	})

	t.Run("storiesWithoutTasks", func(t *testing.T) {
		// s3 and s6 have no tasks
		if stats.StoriesWithoutTasks != 2 {
			t.Errorf("storiesWithoutTasks = %d, want 2", stats.StoriesWithoutTasks)
		}
	})
}

func TestStats_EmptyStoryMap(t *testing.T) {
	sm := &StoryMap{
		Meta: StoryMapMeta{Name: "Empty"},
	}
	stats := sm.Stats()
	if stats.Stories.Total != 0 {
		t.Errorf("expected 0 stories, got %d", stats.Stories.Total)
	}
	if stats.Tasks.Total != 0 {
		t.Errorf("expected 0 tasks, got %d", stats.Tasks.Total)
	}
	if len(stats.ByRelease) != 0 {
		t.Errorf("expected empty byRelease, got %v", stats.ByRelease)
	}
	if len(stats.ByProject) != 0 {
		t.Errorf("expected empty byProject, got %v", stats.ByProject)
	}
	if stats.StoriesWithoutTasks != 0 {
		t.Errorf("expected 0 storiesWithoutTasks, got %d", stats.StoriesWithoutTasks)
	}
}

func TestSetTaskStatus(t *testing.T) {
	sm, _ := ParseStoryMap([]byte(testStoryMapYAML))

	t.Run("更新存在的task", func(t *testing.T) {
		if !sm.SetTaskStatus("task-a1", "in-progress") {
			t.Fatal("SetTaskStatus returned false for existing task")
		}
		ctx, found := sm.FindTask("task-a1")
		if !found {
			t.Fatal("FindTask returned false after SetTaskStatus")
		}
		if ctx.Task.Status != "in-progress" {
			t.Errorf("status = %q, want %q", ctx.Task.Status, "in-progress")
		}
	})

	t.Run("更新到in-review", func(t *testing.T) {
		if !sm.SetTaskStatus("task-a1", "in-review") {
			t.Fatal("SetTaskStatus returned false")
		}
		ctx, _ := sm.FindTask("task-a1")
		if ctx.Task.Status != "in-review" {
			t.Errorf("status = %q, want %q", ctx.Task.Status, "in-review")
		}
	})

	t.Run("不存在的task回傳false", func(t *testing.T) {
		if sm.SetTaskStatus("task-zzz", "done") {
			t.Error("SetTaskStatus returned true for non-existing task")
		}
	})
}

func TestWarnings(t *testing.T) {
	t.Run("有release但沒tasks的story產出warning", func(t *testing.T) {
		input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "待拆分的Story"
            release: rel-mvp
            status: todo
`
		sm, err := ParseStoryMap([]byte(input))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		if len(warnings) != 1 {
			t.Fatalf("warnings count = %d, want 1, got: %v", len(warnings), warnings)
		}
		if !strings.Contains(warnings[0], "story-001") {
			t.Errorf("warning should mention story ID, got: %s", warnings[0])
		}
		if !strings.Contains(warnings[0], "MVP") {
			t.Errorf("warning should mention release label, got: %s", warnings[0])
		}
	})

	t.Run("有tasks的story不產出尚未拆分tasks的warning", func(t *testing.T) {
		sm, err := ParseStoryMap([]byte(testStoryMapYAML))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		for _, w := range warnings {
			if strings.Contains(w, "尚未拆分 tasks") {
				t.Errorf("should not warn about missing tasks when tasks exist, got: %s", w)
			}
		}
	})

	t.Run("done和in-review的story不產出warning", func(t *testing.T) {
		input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-done
            name: "已完成"
            release: rel-mvp
            status: done
          - id: story-review
            name: "待驗收"
            release: rel-mvp
            status: in-review
`
		sm, err := ParseStoryMap([]byte(input))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		if len(warnings) != 0 {
			t.Errorf("expected no warnings for done/in-review stories, got: %v", warnings)
		}
	})

	t.Run("沒有release的story不產出warning", func(t *testing.T) {
		input := `meta:
  name: "Test"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "尚未安排release"
            status: todo
`
		sm, err := ParseStoryMap([]byte(input))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		if len(warnings) != 0 {
			t.Errorf("expected no warnings for story without release, got: %v", warnings)
		}
	})

	t.Run("有tasks但無acceptance的story產出warning", func(t *testing.T) {
		input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "缺驗收條件"
            release: rel-mvp
            status: todo
            tasks:
              - id: task-001
                name: "後端API"
                targetProject: piper
                status: todo
`
		sm, err := ParseStoryMap([]byte(input))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "story-001") && strings.Contains(w, "驗收條件") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about missing acceptance for story-001, got: %v", warnings)
		}
	})

	t.Run("有tasks且有acceptance的story不產出acceptance_warning", func(t *testing.T) {
		input := `meta:
  name: "Test"
  releases:
    - id: rel-mvp
      label: "MVP"
activities:
  - id: act-001
    name: "Act1"
    steps:
      - id: step-001
        name: "Step1"
        stories:
          - id: story-001
            name: "有驗收條件"
            release: rel-mvp
            status: todo
            acceptance: ["API 回傳 200"]
            tasks:
              - id: task-001
                name: "後端API"
                targetProject: piper
                status: todo
`
		sm, err := ParseStoryMap([]byte(input))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		warnings := sm.Warnings()
		for _, w := range warnings {
			if strings.Contains(w, "驗收條件") {
				t.Errorf("should not warn about acceptance when it exists, got: %s", w)
			}
		}
	})
}

func TestStoryMapAlwaysAskNever(t *testing.T) {
	yamlInput := `
meta:
  name: test
activities:
  - id: act-1
    name: Test
    steps:
      - id: step-1
        name: Step
        stories:
          - id: s1
            name: Story with AAN
            status: todo
            always: [run tests, lint code]
            ask: [add dependency]
            never: [change API format]
            tasks:
              - id: t1
                name: Task 1
                targetProject: proj
                status: todo
`
	sm, errs := ValidateStoryMap([]byte(yamlInput))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	story := sm.Activities[0].Steps[0].Stories[0]
	if len(story.Always) != 2 || story.Always[0] != "run tests" {
		t.Errorf("Always = %v, want [run tests, lint code]", story.Always)
	}
	if len(story.Ask) != 1 || story.Ask[0] != "add dependency" {
		t.Errorf("Ask = %v, want [add dependency]", story.Ask)
	}
	if len(story.Never) != 1 || story.Never[0] != "change API format" {
		t.Errorf("Never = %v, want [change API format]", story.Never)
	}

	// Verify roundtrip
	data, err := MarshalStoryMap(sm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sm2, err := ParseStoryMap(data)
	if err != nil {
		t.Fatalf("parse roundtrip: %v", err)
	}
	s2 := sm2.Activities[0].Steps[0].Stories[0]
	if len(s2.Always) != 2 {
		t.Errorf("roundtrip Always = %v", s2.Always)
	}
}

func TestStoryMapLinks(t *testing.T) {
	yamlInput := `
meta:
  name: test
activities:
  - id: act-1
    name: Test
    steps:
      - id: step-1
        name: Step
        stories:
          - id: s1
            name: Original
            status: done
            tasks:
              - id: t1
                name: Task
                targetProject: proj
                status: done
          - id: s2
            name: Follow-up
            status: todo
            links:
              - type: follows
                target: s1
            tasks:
              - id: t2
                name: Task
                targetProject: proj
                status: todo
`
	sm, errs := ValidateStoryMap([]byte(yamlInput))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	s2 := sm.Activities[0].Steps[0].Stories[1]
	if len(s2.Links) != 1 || s2.Links[0].Type != "follows" || s2.Links[0].Target != "s1" {
		t.Errorf("Links = %v", s2.Links)
	}
}

func TestStoryMapLinkWarning(t *testing.T) {
	yamlInput := `
meta:
  name: test
activities:
  - id: act-1
    name: Test
    steps:
      - id: step-1
        name: Step
        stories:
          - id: s1
            name: Story
            status: todo
            links:
              - type: follows
                target: nonexistent
            tasks:
              - id: t1
                name: Task
                targetProject: proj
                status: todo
`
	sm, errs := ValidateStoryMap([]byte(yamlInput))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	warnings := sm.Warnings()
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "nonexistent") && strings.Contains(w, "does not exist") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about nonexistent link target, got: %v", warnings)
	}
}

func TestFindTaskIncludesAAN(t *testing.T) {
	yamlInput := `
meta:
  name: test
activities:
  - id: act-1
    name: Test
    steps:
      - id: step-1
        name: Step
        stories:
          - id: s1
            name: Story
            status: todo
            always: [test everything]
            ask: [new deps]
            never: [break api]
            tasks:
              - id: t1
                name: Task
                targetProject: proj
                status: todo
`
	sm, _ := ValidateStoryMap([]byte(yamlInput))
	ctx, ok := sm.FindTask("t1")
	if !ok {
		t.Fatal("task not found")
	}
	if len(ctx.Always) != 1 || ctx.Always[0] != "test everything" {
		t.Errorf("Always = %v", ctx.Always)
	}
	if len(ctx.Ask) != 1 || ctx.Ask[0] != "new deps" {
		t.Errorf("Ask = %v", ctx.Ask)
	}
	if len(ctx.Never) != 1 || ctx.Never[0] != "break api" {
		t.Errorf("Never = %v", ctx.Never)
	}
}

func TestValidateStatusTransition(t *testing.T) {
	cases := []struct {
		name        string
		from        string
		to          string
		wantErr     bool
		errContains string
	}{
		{name: "相同狀態視為 no-op", from: "in-progress", to: "in-progress"},
		{name: "空 from 視為新 task，可進 todo", from: "", to: "todo"},
		{name: "空 from 可直接進 in-review", from: "", to: "in-review"},
		{name: "todo→in-progress", from: "todo", to: "in-progress"},
		{name: "in-progress→in-review（/submit 推進）", from: "in-progress", to: "in-review"},
		{name: "in-review→done（/verify 推進）", from: "in-review", to: "done"},
		{name: "in-review→in-progress 退回 fix loop", from: "in-review", to: "in-progress"},
		{name: "done→in-progress 重新開啟", from: "done", to: "in-progress"},

		{name: "todo→done 跳過 → 拒絕", from: "todo", to: "done", wantErr: true, errContains: "skip"},
		{name: "todo→in-review 跳過 → 拒絕", from: "todo", to: "in-review", wantErr: true, errContains: "skip"},
		{name: "in-progress→done 跳過 → 拒絕", from: "in-progress", to: "done", wantErr: true, errContains: "skip"},
		{name: "done→in-review 被禁止", from: "done", to: "in-review", wantErr: true, errContains: "done"},
		{name: "無效 target status", from: "todo", to: "bogus", wantErr: true, errContains: "invalid"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateStatusTransition(c.from, c.to)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if c.errContains != "" && !strings.Contains(err.Error(), c.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), c.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateStatusTransitions_StoryMap(t *testing.T) {
	oldYAML := []byte(`meta:
  name: Epic
activities:
  - id: act-1
    name: A
    steps:
      - id: step-1
        name: S
        stories:
          - id: story-1
            name: Story
            status: in-progress
            tasks:
              - id: task-1
                name: T1
                targetProject: proj
                status: in-progress
              - id: task-2
                name: T2
                targetProject: proj
                status: todo
`)
	newYAML := []byte(`meta:
  name: Epic
activities:
  - id: act-1
    name: A
    steps:
      - id: step-1
        name: S
        stories:
          - id: story-1
            name: Story
            status: in-review
            tasks:
              - id: task-1
                name: T1
                targetProject: proj
                status: done
              - id: task-2
                name: T2
                targetProject: proj
                status: in-progress
              - id: task-3
                name: T3 (new)
                targetProject: proj
                status: todo
`)

	oldSM, _ := ParseStoryMap(oldYAML)
	newSM, _ := ParseStoryMap(newYAML)

	errs := ValidateStatusTransitions(oldSM, newSM)
	if len(errs) != 1 {
		t.Fatalf("expected 1 transition error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "task-1") || !strings.Contains(errs[0], "skip") {
		t.Errorf("expected task-1 skip error, got: %v", errs[0])
	}
}

func TestTaskAcceptance_SerializationRoundtrip(t *testing.T) {
	yamlInput := []byte(`meta:
  name: Epic
activities:
  - id: act-1
    name: A
    steps:
      - id: step-1
        name: S
        stories:
          - id: story-1
            name: Story
            status: in-progress
            tasks:
              - id: task-1
                name: T
                targetProject: proj
                status: in-progress
                acceptance:
                  - "GET /api/items 回傳 200"
                  - "body 含 items 陣列"
`)
	sm, errs := ValidateStoryMap(yamlInput)
	if len(errs) != 0 {
		t.Fatalf("validation errors: %v", errs)
	}
	ctx, ok := sm.FindTask("task-1")
	if !ok {
		t.Fatal("task-1 not found")
	}
	if len(ctx.TaskAcceptance) != 2 {
		t.Fatalf("TaskAcceptance len = %d, want 2", len(ctx.TaskAcceptance))
	}
	if ctx.TaskAcceptance[0] != "GET /api/items 回傳 200" {
		t.Errorf("TaskAcceptance[0] = %q", ctx.TaskAcceptance[0])
	}

	out, err := MarshalStoryMap(sm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "acceptance:") {
		t.Errorf("marshaled output missing acceptance key:\n%s", string(out))
	}
}

func TestValidateStoryMap_EmptyTaskAcceptance(t *testing.T) {
	yamlInput := []byte(`meta:
  name: Epic
activities:
  - id: act-1
    name: A
    steps:
      - id: step-1
        name: S
        stories:
          - id: story-1
            name: Story
            status: in-progress
            tasks:
              - id: task-1
                name: T
                targetProject: proj
                status: in-progress
                acceptance:
                  - "valid line"
                  - ""
`)
	_, errs := ValidateStoryMap(yamlInput)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "task-1") && strings.Contains(e, "acceptance[1]") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected empty-acceptance error, got: %v", errs)
	}
}

