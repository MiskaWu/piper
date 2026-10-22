package plan

import _ "embed"

// InfraVersion tracks the version of dispatched infrastructure files
// (rules, skills, scripts). Bump this when any embedded file changes.
const InfraVersion = 17

//go:embed references/submit-skill.md
var SubmitSkill string

//go:embed references/breakdown-skill.md
var BreakdownSkill string

//go:embed references/verify-skill.md
var VerifySkill string

//go:embed references/manifest-skill.md
var ManifestSkill string

//go:embed references/bind-skill.md
var BindSkill string

//go:embed references/task-brief-rule.md
var TaskBriefRule string

//go:embed references/wave-isolation-rule.md
var WaveIsolationRule string

//go:embed references/check-briefs.sh
var CheckBriefsScript string

//go:embed references/verify-setup-worktree.sh
var VerifySetupWorktreeScript string

//go:embed references/verify-diff-gate.sh
var VerifyDiffGateScript string

//go:embed references/verify-cleanup-worktree.sh
var VerifyCleanupWorktreeScript string
