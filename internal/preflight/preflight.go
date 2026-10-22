package preflight

import (
	"fmt"
	"runtime"
	"strings"
)

// Dependency names used in DefaultDeps and FindResult.
const (
	DepDocker = "docker"
)

// Commander abstracts command execution for testability.
type Commander interface {
	Output(name string, args ...string) ([]byte, error)
}

// Dependency describes a tool that piper may need.
type Dependency struct {
	Name        string   // display name
	Cmd         string   // executable to check
	Args        []string // args for version check (e.g. []string{"-V"})
	Required    bool     // true = exit on missing; false = warn only
	Feature     string   // affected feature when missing (optional, for warnings)
	InstallHint string   // multi-platform install instructions
	OSOnly      string   // "linux", "darwin", or "" for all platforms
}

// Result holds the check outcome for a single dependency.
type Result struct {
	Dep Dependency
	OK  bool
}

// RunAll checks all dependencies and returns results.
// It skips dependencies that don't match the current OS.
func RunAll(cmd Commander, deps []Dependency) []Result {
	results := make([]Result, 0, len(deps))
	for _, d := range deps {
		if d.OSOnly != "" && d.OSOnly != runtime.GOOS {
			continue
		}
		_, err := cmd.Output(d.Cmd, d.Args...)
		results = append(results, Result{Dep: d, OK: err == nil})
	}
	return results
}

// HasRequiredFailures returns true if any required dependency is missing.
func HasRequiredFailures(results []Result) bool {
	for _, r := range results {
		if r.Dep.Required && !r.OK {
			return true
		}
	}
	return false
}

// FormatResults returns a human-readable summary of all check results.
func FormatResults(results []Result) string {
	var b strings.Builder
	b.WriteString("Dependency check:\n")
	for _, r := range results {
		if r.OK {
			fmt.Fprintf(&b, "  ✓ %s\n", r.Dep.Name)
		} else {
			label := "optional"
			if r.Dep.Required {
				label = "required"
			}
			msg := fmt.Sprintf("  ✗ %s (%s)", r.Dep.Name, label)
			if r.Dep.Feature != "" {
				msg += fmt.Sprintf(" — %s 功能不可用", r.Dep.Feature)
			}
			b.WriteString(msg + "\n")
			if r.Dep.InstallHint != "" {
				fmt.Fprintf(&b, "    Install: %s\n", r.Dep.InstallHint)
			}
		}
	}
	return b.String()
}

// FindResult returns the Result for the named dependency, or (Result{}, false) if not found.
func FindResult(results []Result, name string) (Result, bool) {
	for _, r := range results {
		if r.Dep.Name == name {
			return r, true
		}
	}
	return Result{}, false
}

// DefaultDeps returns the standard set of dependencies for piper.
func DefaultDeps() []Dependency {
	return []Dependency{
		{
			Name:     "tmux",
			Cmd:      "tmux",
			Args:     []string{"-V"},
			Required: true,
			InstallHint: `macOS: brew install tmux | Ubuntu: sudo apt install tmux
    Fedora: sudo dnf install tmux | Arch: sudo pacman -S tmux`,
		},
		{
			Name:        "claude",
			Cmd:         "claude",
			Args:        []string{"--version"},
			Required:    true,
			InstallHint: "npm install -g @anthropic-ai/claude-code",
		},
		{
			Name:        "git",
			Cmd:         "git",
			Args:        []string{"--version"},
			Required:    false,
			Feature:     "Epic Git 同步 / Git 線圖",
			InstallHint: "macOS: brew install git | Ubuntu: sudo apt install git",
		},
		{
			Name:        DepDocker,
			Cmd:         "docker",
			Args:        []string{"--version"},
			Required:    false,
			Feature:     "DevContainer",
			InstallHint: "https://docs.docker.com/get-docker/",
		},
		{
			Name:        "bwrap",
			Cmd:         "bwrap",
			Args:        []string{"--version"},
			Required:    false,
			Feature:     "Sandbox",
			InstallHint: "sudo apt install bubblewrap",
			OSOnly:      "linux",
		},
		{
			Name:        "socat",
			Cmd:         "socat",
			Args:        []string{"-V"},
			Required:    false,
			Feature:     "Sandbox 網路隔離",
			InstallHint: "sudo apt install socat",
			OSOnly:      "linux",
		},
	}
}
