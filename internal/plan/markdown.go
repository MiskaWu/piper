package plan

import (
	"strings"
)

// ExtractSection extracts the content under a given heading (up to the next same-level heading).
// The heading parameter should be the text without the '#' prefix (e.g., "決定" not "## 決定").
func ExtractSection(content string, heading string) string {
	lines := strings.Split(content, "\n")

	targetLevel := 0
	startIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}

		level := 0
		for _, ch := range trimmed {
			if ch == '#' {
				level++
			} else {
				break
			}
		}
		title := strings.TrimSpace(trimmed[level:])

		if startIdx == -1 {
			if title == heading {
				targetLevel = level
				startIdx = i + 1
			}
		} else {
			// Found next heading at same or higher level — stop
			if level <= targetLevel {
				return trimContent(lines[startIdx:i])
			}
		}
	}

	if startIdx == -1 {
		return ""
	}
	return trimContent(lines[startIdx:])
}

// trimContent joins lines and trims surrounding whitespace.
func trimContent(lines []string) string {
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
