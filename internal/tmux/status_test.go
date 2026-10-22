package tmux

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(testdataPath(name))
	if err != nil {
		t.Fatalf("failed to read testdata %s: %v", name, err)
	}
	return string(data)
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "color codes",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "multiple codes",
			input:    "\x1b[1m\x1b[32mbold green\x1b[0m normal",
			expected: "bold green normal",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2Ahello\x1b[K",
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := StripANSI(tc.input)
			if result != tc.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestGetLastSignificantLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 基本 fallback（無錨點時從底部往上）
		{
			name:     "simple lines without anchor",
			input:    "line1\nline2\nline3",
			expected: "line3",
		},
		{
			name:     "trailing empty lines",
			input:    "line1\nline2\n\n\n",
			expected: "line2",
		},
		{
			name:     "all empty",
			input:    "\n\n\n",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		// 錨點策略：從 ─── 上方取
		{
			name:     "錨點：─── 上方第一行有意義內容",
			input:    "The changes are ready.\n────────────────────\n❯\n",
			expected: "The changes are ready.",
		},
		{
			name:     "錨點：跳過 ✻ 計時行",
			input:    "All good.\n✻ Brewed for 37s\n────────────────────\n❯\n",
			expected: "All good.",
		},
		{
			name:     "錨點：跳過 ✻ Took",
			input:    "All good.\n✻ Took 5s\n────────────────────\n❯\n",
			expected: "All good.",
		},
		{
			name:     "錨點：─── 下方有 status bar 不影響",
			input:    "The changes are ready.\n✻ Brewed for 37s\n────────────────────\n❯\n⏵⏵ bypass permissions on (shift+tab to cycle)\n",
			expected: "The changes are ready.",
		},
		{
			name:     "錨點：─── 下方有新版狀態列不影響",
			input:    "The changes are ready.\n✻ Took 12s\n────────────────────\n❯\n[Opus 4.6]  ▓░░░░░░░░░ 19%  │  $0.29  │  ⎇ main\n",
			expected: "The changes are ready.",
		},
		{
			name:     "錨點：working 狀態也有 ───",
			input:    "Editing src/main.go...\n                        esc to interrupt\n────────────────────\n❯\n[Opus 4.6]  ▓░░░░░░░░░ 19%  │  $0.29  │  ⎇ main\n",
			expected: "Editing src/main.go...",
		},
		{
			name:     "錨點：跳過空行",
			input:    "Done with review.\n\n\n✻ Took 5s\n────────────────────\n❯\n",
			expected: "Done with review.",
		},
		// 表格跳過（含 │ 的行）
		{
			name:     "錨點：跳過表格行（含 │）",
			input:    "Here are the results:\n│ Name │ Status │\n│ foo  │ pass   │\n────────────────────\n❯\n",
			expected: "Here are the results:",
		},
		{
			name:     "錨點：跳過表格 + box-drawing 框線",
			input:    "Summary:\n┌──────┬────────┐\n│ Name │ Status │\n└──────┴────────┘\n────────────────────\n❯\n",
			expected: "Summary:",
		},
		// fallback：無 ─── 錨點時用舊邏輯
		{
			name:     "fallback：跳過 esc to interrupt",
			input:    "Editing src/main.go...\n                        esc to interrupt\n",
			expected: "Editing src/main.go...",
		},
		{
			name:     "fallback：跳過 Thinking...",
			input:    "Let me analyze.\n  Thinking...\n",
			expected: "Let me analyze.",
		},
		{
			name:     "fallback：跳過 · Scampering…",
			input:    "Let me check the code.\n· Scampering… (5m 41s · ↓ 4.2k tokens)\n",
			expected: "Let me check the code.",
		},
		{
			name:     "fallback：跳過 ⎿ 工具輸出行",
			input:    "Editing file.\n⎿  Updated src/main.go (+5 -3)\n",
			expected: "Editing file.",
		},
		{
			name:     "fallback：跳過 ✢ thinking indicator",
			input:    "Let me explore the code.\n✢ Photosynthesizing… (57s · ↓ 795 tokens)\n",
			expected: "Let me explore the code.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetLastSignificantLine(tc.input)
			if result != tc.expected {
				t.Errorf("GetLastSignificantLine(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestGetLastSignificantLine_FromTestdata(t *testing.T) {
	tests := []struct {
		file     string
		expected string
	}{
		{"claude_with_statusbar.txt", "All tests pass. The changes are ready for review."},
		{"claude_working_with_statusbar.txt", "Editing src/main.go..."},
		{"claude_needs_input_with_statusbar.txt", "Allow Read access to src/config.go?"},
		{"needs_input_claude_idle.txt", "所以你看到的行為是正常且符合設計的。"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			content := readTestdata(t, tc.file)
			result := GetLastSignificantLine(content)
			if result != tc.expected {
				t.Errorf("GetLastSignificantLine(%s) = %q, want %q", tc.file, result, tc.expected)
			}
		})
	}
}

