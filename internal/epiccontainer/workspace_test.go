package epiccontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareWorkspace_建立CLAUDE_MD(t *testing.T) {
	sessionName := "pplan-epic-test1"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareWorkspace(sessionName, "epic-test1", SessionPlan)
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace dir not found: %v", err)
	}

	// Verify CLAUDE.md exists and contains epic slug
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "epic-test1") {
		t.Error("expected CLAUDE.md to contain epic slug")
	}
	if !strings.Contains(content, "規劃") || !strings.Contains(content, "Story Map") {
		t.Error("expected plan session content in CLAUDE.md")
	}
}

func TestPrepareWorkspace_Dispatch(t *testing.T) {
	sessionName := "pdisp-epic-test2"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareWorkspace(sessionName, "epic-test2", SessionDispatch)
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "派送") {
		t.Error("expected dispatch content")
	}
	if !strings.Contains(content, "dispatch/preview") {
		t.Error("expected dispatch API reference")
	}
}

func TestPrepareWorkspace_Retro(t *testing.T) {
	sessionName := "pretro-epic-test3"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareWorkspace(sessionName, "epic-test3", SessionRetro)
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Retro") {
		t.Error("expected retro content")
	}
	if !strings.Contains(content, "retro/preview") {
		t.Error("expected retro API reference")
	}
}

func TestCleanWorkspace_清理目錄(t *testing.T) {
	sessionName := "pplan-epic-cleanup"
	dir, err := PrepareWorkspace(sessionName, "epic-cleanup", SessionPlan)
	if err != nil {
		t.Fatalf("PrepareWorkspace: %v", err)
	}

	// Verify exists
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace should exist: %v", err)
	}

	CleanWorkspace(sessionName)

	// Verify removed
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected workspace to be removed")
	}
}

func TestPrepareNativeWorkspace_建立CLAUDE_MD但不建立子目錄(t *testing.T) {
	sessionName := "pplan-native-test"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareNativeWorkspace(sessionName, "epic-native", SessionPlan)
	if err != nil {
		t.Fatalf("PrepareNativeWorkspace: %v", err)
	}

	// CLAUDE.md should exist and contain epic slug
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(data), "epic-native") {
		t.Error("expected CLAUDE.md to contain epic slug")
	}

	// sessions/ and projects/ subdirs should NOT exist
	for _, sub := range []string{"sessions", "projects"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); !os.IsNotExist(err) {
			t.Errorf("expected %s dir to not exist in native workspace", sub)
		}
	}
}

func TestPrepareNativeWorkspace_Dispatch(t *testing.T) {
	sessionName := "pdisp-native-test"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareNativeWorkspace(sessionName, "epic-native-d", SessionDispatch)
	if err != nil {
		t.Fatalf("PrepareNativeWorkspace: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(data), "派送") {
		t.Error("expected dispatch content")
	}
}

func TestPrepareNativeWorkspace_清理正常(t *testing.T) {
	sessionName := "pplan-native-cleanup"
	dir, err := PrepareNativeWorkspace(sessionName, "epic-cleanup-n", SessionPlan)
	if err != nil {
		t.Fatalf("PrepareNativeWorkspace: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace should exist: %v", err)
	}

	CleanWorkspace(sessionName)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected workspace to be removed")
	}
}

func TestPrepareWorkspace_未知type回錯誤(t *testing.T) {
	_, err := PrepareWorkspace("test", "epic-x", SessionType("invalid"))
	if err == nil {
		t.Fatal("expected error for unknown session type")
	}
}

func TestPrepareWorkspace_DesignSkill依session類型注入(t *testing.T) {
	tests := []struct {
		name        string
		sessionType SessionType
		shouldExist bool
	}{
		{"plan session 注入 design skill", SessionPlan, true},
		{"dispatch session 不注入", SessionDispatch, false},
		{"retro session 不注入", SessionRetro, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionName := "pepic-design-" + string(tt.sessionType)
			defer CleanWorkspace(sessionName)

			dir, err := PrepareWorkspace(sessionName, "epic-design", tt.sessionType)
			if err != nil {
				t.Fatalf("PrepareWorkspace: %v", err)
			}

			skillPath := filepath.Join(dir, ".claude", "skills", "design", "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if tt.shouldExist {
				if err != nil {
					t.Fatalf("expected design SKILL.md, got: %v", err)
				}
				if !strings.Contains(string(data), "契約設計") {
					t.Error("design skill content missing expected marker")
				}
			} else {
				if err == nil {
					t.Errorf("expected no design skill for %s, but file exists", tt.sessionType)
				}
			}
		})
	}
}

func TestPrepareNativeWorkspace_Plan也注入DesignSkill(t *testing.T) {
	sessionName := "pplan-native-design"
	defer CleanWorkspace(sessionName)

	dir, err := PrepareNativeWorkspace(sessionName, "epic-native-design", SessionPlan)
	if err != nil {
		t.Fatalf("PrepareNativeWorkspace: %v", err)
	}

	skillPath := filepath.Join(dir, ".claude", "skills", "design", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected design skill in native plan workspace: %v", err)
	}
}
