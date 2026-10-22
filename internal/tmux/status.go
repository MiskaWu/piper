package tmux

import (
	"regexp"
	"strings"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// GetLastSignificantLine returns the last meaningful output line from captured pane content.
//
// Strategy: find the last border line (───) as anchor, then scan upward from it
// skipping timing lines (✻) and table/box-drawing lines (containing │).
// If no anchor is found, fall back to scanning from the bottom skipping known UI chrome.
func GetLastSignificantLine(content string) string {
	lines := strings.Split(content, "\n")

	// Find last border anchor (───)
	anchorIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.Count(trimmed, "─") >= 3 && isBoxDrawing(trimmed) {
			anchorIdx = i
			break
		}
	}

	if anchorIdx >= 0 {
		// Scan upward from anchor, skip non-content lines
		for i := anchorIdx - 1; i >= 0; i-- {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				continue
			}
			// Table/box-drawing: lines containing │
			if strings.ContainsRune(trimmed, '│') {
				continue
			}
			// Box-drawing frame lines (┌┐└┘ etc.)
			if isBoxDrawing(trimmed) {
				continue
			}
			// Hardcoded UI chrome patterns
			if isUIChrome(trimmed) {
				continue
			}
			return trimmed
		}
		return ""
	}

	// Fallback: no anchor found (e.g. working state without ───)
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if isUIChrome(trimmed) {
			continue
		}
		return trimmed
	}
	return ""
}

// isUIChrome returns true if the line is a Claude Code TUI decoration
// that should be skipped when extracting the last meaningful content.
func isUIChrome(line string) bool {
	// Always check border lines structurally (3+ ─ and only box-drawing chars)
	if strings.Count(line, "─") >= 3 && isBoxDrawing(line) {
		return true
	}

	return isUIChromeDefault(line)
}

// isUIChromeDefault is the legacy hardcoded UI chrome detection.
// Used as fallback when no dynamic UIChromePatterns are configured.
func isUIChromeDefault(line string) bool {
	// Status bar: ⏵⏵ bypass permissions on (shift+tab to cycle)
	if strings.Contains(line, "⏵") {
		return true
	}
	// Input prompt
	if line == "❯" {
		return true
	}
	// Timing lines: ✻ Brewed for / ✻ Took
	if strings.HasPrefix(line, "✻") {
		return true
	}
	// Tool output / Tip lines (⎿ prefix — always supplementary to the line above)
	if strings.HasPrefix(line, "⎿") {
		return true
	}
	// Thinking/working status: · Scampering… (5m 41s · ↓ 4.2k tokens)
	if strings.HasPrefix(line, "·") && strings.Contains(line, "…") {
		return true
	}
	// Thinking/working status (new format): ✢ Photosynthesizing… (57s · ↓ 795 tokens)
	if strings.HasPrefix(line, "✢") && strings.Contains(line, "…") {
		return true
	}
	// Interrupt hints
	lower := strings.ToLower(line)
	if strings.Contains(lower, "esc to interrupt") || strings.Contains(lower, "ctrl+c to interrupt") {
		return true
	}
	// Thinking verbs (standalone, three dots style)
	thinkingWords := []string{"Thinking", "Moseying", "Crystallizing", "Cooking"}
	for _, w := range thinkingWords {
		if strings.Contains(line, w) && strings.HasSuffix(line, "...") {
			return true
		}
	}
	// Shell prompt
	if line == "$" {
		return true
	}
	return false
}

// isBoxDrawing returns true if the line consists entirely of box-drawing characters.
func isBoxDrawing(line string) bool {
	for _, r := range line {
		switch r {
		case '─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼':
			// box-drawing, ok
		default:
			return false
		}
	}
	return true
}

func detectClaudePrompt(lines []string) bool {
	hasPrompt := false
	hasBorder := false
	for _, line := range lines {
		if strings.Contains(line, "❯") {
			hasPrompt = true
		}
		if strings.Count(line, "─") >= 3 {
			hasBorder = true
		}
	}
	return hasPrompt && hasBorder
}
