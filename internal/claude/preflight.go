package claude

import "fmt"

// Commander abstracts command execution for testability.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
}

// RequireClaude checks that the Claude CLI is installed and available in PATH.
// Returns a user-friendly error with install hints if not found.
func RequireClaude(cmd Commander) error {
	_, err := cmd.Output("claude", "--version")
	if err != nil {
		return fmt.Errorf(`claude CLI is required but not found in PATH.

Install Claude Code:
  npm:    npm install -g @anthropic-ai/claude-code

以上為常見安裝方式，建議依照你的作業系統查詢最適合的安裝方式。`)
	}
	return nil
}
