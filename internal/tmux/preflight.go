package tmux

import "fmt"

// RequireTmux checks that tmux is installed and available in PATH.
// Returns a user-friendly error with install hints if not found.
func (c *Client) RequireTmux() error {
	if err := c.Check(); err != nil {
		return fmt.Errorf(`tmux is required but not found in PATH.

Install tmux:
  macOS:   brew install tmux
  Ubuntu:  sudo apt install tmux
  Fedora:  sudo dnf install tmux
  Arch:    sudo pacman -S tmux

以上為常見安裝方式，建議依照你的作業系統查詢最適合的安裝方式。`)
	}
	return nil
}
