package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TrackedFiles represents the files modified by a Claude Code session.
type TrackedFiles struct {
	Files []string // relative file paths
	Count int
}

// snapshotLine is used for initial type check before full parsing.
type snapshotLine struct {
	Type string `json:"type"`
}

// fileHistorySnapshot represents the full structure of a file-history-snapshot entry.
type fileHistorySnapshot struct {
	Type     string `json:"type"`
	Snapshot struct {
		TrackedFileBackups map[string]json.RawMessage `json:"trackedFileBackups"`
	} `json:"snapshot"`
}

// claudeSessionEntry represents a JSON file in ~/.claude/sessions/.
type claudeSessionEntry struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"` // Unix milliseconds
}

// findSessionIDByPID reads sessions/{pid}.json and returns the sessionId.
// Returns empty string if the file does not exist or cannot be parsed.
func findSessionIDByPID(sessionsDir string, pid int) string {
	data, err := os.ReadFile(filepath.Join(sessionsDir, fmt.Sprintf("%d.json", pid)))
	if err != nil {
		return ""
	}
	var entry claudeSessionEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return ""
	}
	return entry.SessionID
}

// FindSessionIDByPanePID looks up ~/.claude/sessions/{pid}.json for host sessions.
func FindSessionIDByPanePID(pid int) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return findSessionIDByPID(filepath.Join(home, ".claude", "sessions"), pid)
}

// SessionJSONLPath returns the path to a Claude Code session JSONL file.
// The path format is ~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl
// where encoded-cwd replaces "/" with "-".
// Returns empty string if the file does not exist.
func SessionJSONLPath(sessionID, workDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	encodedCwd := strings.ReplaceAll(workDir, "/", "-")
	p := filepath.Join(home, ".claude", "projects", encodedCwd, sessionID+".jsonl")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// devcontainerDataDir returns the per-project data directory for a DevContainer.
// Format: ~/.claude-devcontainers/<encoded-projectPath>/
func devcontainerDataDir(projectPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	encoded := strings.ReplaceAll(projectPath, "/", "-")
	return filepath.Join(home, ".claude-devcontainers", encoded)
}

// SessionJSONLPathForDevcontainer returns the path to a DevContainer session JSONL file.
// DevContainers use per-project isolated directories under ~/.claude-devcontainers/.
// The container cwd is always "/workspace", so the encoded subdir is "-workspace".
func SessionJSONLPathForDevcontainer(sessionID, projectPath string) string {
	dataDir := devcontainerDataDir(projectPath)
	if dataDir == "" {
		return ""
	}
	p := filepath.Join(dataDir, "projects", "-workspace", sessionID+".jsonl")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// FindLatestSessionJSONL finds the most recently modified .jsonl file
// in the Claude projects directory for the given workDirs.
// Searches all provided directories and returns the overall newest match.
// This is useful when ClaudeSessionID is not known.
func FindLatestSessionJSONL(workDirs ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	var bestPath string
	var bestTime int64
	for _, wd := range workDirs {
		if wd == "" {
			continue
		}
		encodedCwd := strings.ReplaceAll(wd, "/", "-")
		dir := filepath.Join(home, ".claude", "projects", encodedCwd)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if t := info.ModTime().UnixNano(); t > bestTime {
				bestTime = t
				bestPath = filepath.Join(dir, e.Name())
			}
		}
	}
	return bestPath
}

// CountTrackedFiles reads a Claude Code session JSONL file and returns
// the files tracked in the last file-history-snapshot entry.
// Returns zero TrackedFiles if no snapshot found or on error.
func CountTrackedFiles(sessionJSONLPath string) TrackedFiles {
	f, err := os.Open(sessionJSONLPath)
	if err != nil {
		return TrackedFiles{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Session files can have very long lines; set buffer to 1MB.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var lastSnapshotLine []byte

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Quick check: only parse type field first for efficiency.
		var sl snapshotLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue
		}
		if sl.Type == "file-history-snapshot" {
			// Keep a copy since scanner reuses the buffer.
			lastSnapshotLine = make([]byte, len(line))
			copy(lastSnapshotLine, line)
		}
	}

	if lastSnapshotLine == nil {
		return TrackedFiles{}
	}

	var snapshot fileHistorySnapshot
	if err := json.Unmarshal(lastSnapshotLine, &snapshot); err != nil {
		return TrackedFiles{}
	}

	backups := snapshot.Snapshot.TrackedFileBackups
	if len(backups) == 0 {
		return TrackedFiles{}
	}

	files := make([]string, 0, len(backups))
	for path := range backups {
		files = append(files, path)
	}
	sort.Strings(files)

	return TrackedFiles{
		Files: files,
		Count: len(files),
	}
}
