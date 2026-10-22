package epiccontainer

import _ "embed"

//go:embed references/plan-claude.md
var planClaudeMD string

//go:embed references/dispatch-claude.md
var dispatchClaudeMD string

//go:embed references/retro-claude.md
var retroClaudeMD string

//go:embed references/story-splitting-rule.md
var StorySplittingRule string

//go:embed references/design-skill.md
var designSkill string
