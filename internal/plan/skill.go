package plan

import (
	"fmt"
	"os"
	"path/filepath"
)

// DeploySkill writes a SKILL.md to {baseDir}/.claude/skills/{name}/SKILL.md.
// Used by dispatch (target projects) and epiccontainer (plan workspace) — both
// follow the same Claude Code project-level skill convention.
func DeploySkill(baseDir, name, content string) error {
	dir := filepath.Join(baseDir, ".claude", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s skill dir: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s skill: %w", name, err)
	}
	return nil
}
