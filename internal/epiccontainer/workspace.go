package epiccontainer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SessionType identifies the kind of epic session.
type SessionType string

const (
	SessionPlan     SessionType = "plan"
	SessionDispatch SessionType = "dispatch"
	SessionRetro    SessionType = "retro"
)

// prepareWorkspaceBase creates the temp directory and writes CLAUDE.md.
// Shared by PrepareWorkspace (Docker) and PrepareNativeWorkspace (native).
func prepareWorkspaceBase(sessionName, epicSlug string, sessionType SessionType) (string, error) {
	dir := filepath.Join(os.TempDir(), "piper-epic-"+sessionName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("prepare workspace: %w", err)
	}

	claudeMD, err := buildClaudeMD(epicSlug, sessionType)
	if err != nil {
		return "", fmt.Errorf("prepare workspace: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeMD), 0o644); err != nil {
		return "", fmt.Errorf("prepare workspace: write CLAUDE.md: %w", err)
	}

	// design skill 只在 plan workspace 注入，不放 host ~/.claude 也不 dispatch 到 target，避免污染其他專案
	if sessionType == SessionPlan {
		skillDir := filepath.Join(dir, ".claude", "skills", "design")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return "", fmt.Errorf("prepare workspace: create design skill dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(designSkill), 0o644); err != nil {
			return "", fmt.Errorf("prepare workspace: write design skill: %w", err)
		}
	}

	return dir, nil
}

// PrepareWorkspace creates a temp directory containing CLAUDE.md for the container,
// plus isolation subdirs for Docker volume mounts.
func PrepareWorkspace(sessionName, epicSlug string, sessionType SessionType) (string, error) {
	dir, err := prepareWorkspaceBase(sessionName, epicSlug, sessionType)
	if err != nil {
		return "", err
	}

	// Create isolation dirs for sessions/projects (mounted over ~/.claude/sessions and ~/.claude/projects)
	for _, sub := range []string{"sessions", "projects"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return "", fmt.Errorf("prepare workspace: create %s dir: %w", sub, err)
		}
	}

	return dir, nil
}

// PrepareNativeWorkspace creates a temp directory containing CLAUDE.md for native
// (non-Docker) epic sessions. No isolation subdirs are created.
func PrepareNativeWorkspace(sessionName, epicSlug string, sessionType SessionType) (string, error) {
	return prepareWorkspaceBase(sessionName, epicSlug, sessionType)
}

// CleanWorkspace removes the temp workspace directory.
func CleanWorkspace(sessionName string) {
	dir := filepath.Join(os.TempDir(), "piper-epic-"+sessionName)
	os.RemoveAll(dir)
}

// buildClaudeMD generates CLAUDE.md content by combining the embedded reference
// template with the epic slug.
func buildClaudeMD(epicSlug string, sessionType SessionType) (string, error) {
	var tmpl string
	switch sessionType {
	case SessionPlan:
		tmpl = planClaudeMD
	case SessionDispatch:
		tmpl = dispatchClaudeMD
	case SessionRetro:
		tmpl = retroClaudeMD
	default:
		return "", fmt.Errorf("unknown session type: %s", sessionType)
	}

	result := strings.ReplaceAll(tmpl, "{{EPIC_SLUG}}", epicSlug)
	return result, nil
}
