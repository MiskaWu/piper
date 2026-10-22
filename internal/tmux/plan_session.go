package tmux

import "strings"

// PlanSessionInfo represents a parsed plan-type session.
type PlanSessionInfo struct {
	Epic string `json:"epic"` // epic slug (trimmed prefix)
	Type string `json:"type"` // "plan" | "dispatch" | "retro"
	Name string `json:"name"` // original tmux session name
}

// ParsePlanSessionName parses a tmux session name with pplan-/pdisp-/pretro- prefix
// into a PlanSessionInfo. Returns ok=false if the name does not match any prefix.
func ParsePlanSessionName(name string) (info PlanSessionInfo, ok bool) {
	for _, entry := range []struct {
		prefix string
		typ    string
	}{
		{"pplan-", "plan"},
		{"pdisp-", "dispatch"},
		{"pretro-", "retro"},
	} {
		if strings.HasPrefix(name, entry.prefix) {
			epic := strings.TrimPrefix(name, entry.prefix)
			if epic == "" {
				return PlanSessionInfo{}, false
			}
			return PlanSessionInfo{Epic: epic, Type: entry.typ, Name: name}, true
		}
	}
	return PlanSessionInfo{}, false
}
