package plan

import (
	"strings"
	"testing"
)

func TestGenerateBrief(t *testing.T) {
	data := BriefData{
		TaskID:        "task-a1",
		TaskName:      "實作登入 API",
		StoryID:       "story-aaa11",
		StoryName:     "使用者登入",
		EpicName:      "Test Epic",
		ActivityName:  "使用者管理",
		StepName:      "登入流程",
		ReleaseLabel:  "MVP",
		Tags:          "auth, core",
		TargetProject: "piper",
		Description:   "身為使用者，我想要登入系統",
		ADRContent:    "決定使用 JWT token",
		SpikeContent:  "確認 OAuth 可行",
		SiblingTasks: []SiblingTask{
			{ID: "task-a2", Name: "實作登入 UI", TargetProject: "frontend", Status: "todo"},
		},
		Acceptance: []string{"使用者能以帳密登入", "登入失敗顯示錯誤"},
	}

	result, err := GenerateBrief(data)
	if err != nil {
		t.Fatalf("GenerateBrief failed: %v", err)
	}

	checks := []string{
		"# Task Brief: task-a1 — 實作登入 API",
		"| Story | story-aaa11 — 使用者登入 |",
		"| Epic | Test Epic |",
		"| Activity | 使用者管理 |",
		"身為使用者，我想要登入系統",
		"決定使用 JWT token",
		"確認 OAuth 可行",
		"| task-a2 | 實作登入 UI | frontend | todo |",
		"使用者能以帳密登入",
		"登入失敗顯示錯誤",
		"- [ ] （由 /breakdown 補充）",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("brief missing %q", check)
		}
	}
}

func TestGenerateBrief_WithTaskAcceptance(t *testing.T) {
	data := BriefData{
		TaskID:        "task-a1",
		TaskName:      "API",
		StoryID:       "s1",
		StoryName:     "Story",
		EpicName:      "Epic",
		TargetProject: "backend",
		Description:   "做什麼",
		TaskAcceptance: []string{
			"GET /api/items 回傳 200",
			"body 含 items 陣列",
		},
	}
	result, err := GenerateBrief(data)
	if err != nil {
		t.Fatalf("GenerateBrief: %v", err)
	}
	if !strings.Contains(result, "- [ ] GET /api/items 回傳 200") {
		t.Errorf("missing first acceptance:\n%s", result)
	}
	if !strings.Contains(result, "- [ ] body 含 items 陣列") {
		t.Errorf("missing second acceptance")
	}
	if strings.Contains(result, "由 /breakdown 補充") {
		t.Errorf("placeholder should not appear when TaskAcceptance is present")
	}
}

func TestGenerateBrief_Defaults(t *testing.T) {
	data := BriefData{
		TaskID:   "task-xxx",
		TaskName: "Test",
	}

	result, err := GenerateBrief(data)
	if err != nil {
		t.Fatalf("GenerateBrief failed: %v", err)
	}

	if !strings.Contains(result, "無") {
		t.Error("expected default '無' for empty ADR/Spike")
	}
	if !strings.Contains(result, "（無描述）") {
		t.Error("expected default description placeholder")
	}
	if !strings.Contains(result, "（無）") {
		t.Error("expected empty siblings placeholder")
	}
}

func TestGenerateBrief_WithAAN(t *testing.T) {
	data := BriefData{
		TaskID:               "task-a1",
		TaskName:             "Test Task",
		StoryID:              "s1",
		StoryName:            "Test Story",
		EpicName:             "Test Epic",
		ActivityName:         "Act",
		StepName:             "Step",
		ReleaseLabel:         "MVP",
		Tags:                 "backend",
		TargetProject:        "piper",
		TargetProjectDisplay: "piper",
		Description:          "Do something",
		Always:               []string{"run tests", "lint code"},
		Ask:                  []string{"add dependency"},
		Never:                []string{"change API format"},
	}

	result, err := GenerateBrief(data)
	if err != nil {
		t.Fatalf("GenerateBrief: %v", err)
	}

	if !strings.Contains(result, "## 行動邊界") {
		t.Error("missing 行動邊界 section")
	}
	if !strings.Contains(result, "### Always") {
		t.Error("missing Always subsection")
	}
	if !strings.Contains(result, "run tests") {
		t.Error("missing always rule content")
	}
	if !strings.Contains(result, "### Ask") {
		t.Error("missing Ask subsection")
	}
	if !strings.Contains(result, "add dependency") {
		t.Error("missing ask rule content")
	}
	if !strings.Contains(result, "### Never") {
		t.Error("missing Never subsection")
	}
	if !strings.Contains(result, "change API format") {
		t.Error("missing never rule content")
	}
}

func TestGenerateBrief_WithoutAAN(t *testing.T) {
	data := BriefData{
		TaskID:               "task-a1",
		TaskName:             "Test Task",
		StoryID:              "s1",
		StoryName:            "Test Story",
		EpicName:             "Test Epic",
		ActivityName:         "Act",
		StepName:             "Step",
		ReleaseLabel:         "MVP",
		Tags:                 "backend",
		TargetProject:        "piper",
		TargetProjectDisplay: "piper",
		Description:          "Do something",
	}

	result, err := GenerateBrief(data)
	if err != nil {
		t.Fatalf("GenerateBrief: %v", err)
	}

	if !strings.Contains(result, "## 行動邊界") {
		t.Error("missing 行動邊界 section")
	}
	if !strings.Contains(result, "本 Story 未定義行動邊界") {
		t.Error("should show '未定義行動邊界' when no A/A/N")
	}
	if strings.Contains(result, "### Always") {
		t.Error("should not show Always subsection when empty")
	}
}

func TestReferencesEmbedded(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"TaskBriefRule", TaskBriefRule},
		{"WaveIsolationRule", WaveIsolationRule},
		{"SubmitSkill", SubmitSkill},
		{"BreakdownSkill", BreakdownSkill},
		{"VerifySkill", VerifySkill},
		{"CheckBriefsScript", CheckBriefsScript},
		{"VerifySetupWorktreeScript", VerifySetupWorktreeScript},
		{"VerifyDiffGateScript", VerifyDiffGateScript},
		{"VerifyCleanupWorktreeScript", VerifyCleanupWorktreeScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.content == "" {
				t.Errorf("%s is empty", tc.name)
			}
		})
	}
}
