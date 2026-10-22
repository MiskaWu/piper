package plan

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed templates/story-map.yaml
var storyMapTemplate string

//go:embed templates/problem-statement.md
var problemStatementTemplate string

// EpicInfo represents a summary of an epic for listing.
type EpicInfo struct {
	ID      string `json:"id"`      // stable random ID (epic-XXXXX)
	Name    string `json:"name"`    // display name from YAML meta
	Updated string `json:"updated"` // ISO timestamp from story-map.yaml mtime
}

// SyncableRepo provides git access for the sync layer.
type SyncableRepo interface {
	BaseDir() string
	Git() GitRunner
}

// EpicManager handles Epic CRUD operations (pure file I/O, no git).
// Git operations are handled by VaultManager.
type EpicManager struct {
	baseDir string
}

// NewEpicManager creates a new EpicManager with the given base directory.
func NewEpicManager(baseDir string) *EpicManager {
	return &EpicManager{baseDir: baseDir}
}

// generateEpicID generates a random epic ID in the format "epic-XXXXX" (5 hex chars).
func generateEpicID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random bytes: " + err.Error())
	}
	return "epic-" + hex.EncodeToString(b)[:5]
}

// epicIDPattern validates the epic-XXXXX format (5 hex chars).
var epicIDPattern = regexp.MustCompile(`^epic-[0-9a-f]{5}$`)

// validateEpicID checks that an ID matches the epic-XXXXX format.
func validateEpicID(id string) error {
	if !epicIDPattern.MatchString(id) {
		return fmt.Errorf("invalid epic id %q: must match epic-XXXXX format", id)
	}
	return nil
}

// namePattern extracts the YAML meta name value (quoted or unquoted).
var namePattern = regexp.MustCompile(`(?m)^\s*name:\s*(.+?)\s*$`)

// extractDisplayName extracts the display name from YAML content.
// Falls back to the provided fallback if not found or empty.
func extractDisplayName(yamlContent string, fallback string) string {
	m := namePattern.FindStringSubmatch(yamlContent)
	if m == nil {
		return fallback
	}
	raw := m[1]
	// Strip surrounding quotes
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			raw = raw[1 : len(raw)-1]
		}
	}
	if raw == "" {
		return fallback
	}
	return raw
}

// BaseDir returns the base directory for epic storage.
func (m *EpicManager) BaseDir() string { return m.baseDir }

// ListEpics scans baseDir and returns a list of epics with id, display name, and last modified time.
func (m *EpicManager) ListEpics() ([]EpicInfo, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []EpicInfo{}, nil
		}
		return nil, fmt.Errorf("list epics: %w", err)
	}

	var epics []EpicInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		// Only list directories matching the epic-XXXXX ID format
		if !epicIDPattern.MatchString(dirName) {
			continue
		}
		storyMapPath := filepath.Join(m.baseDir, dirName, "story-map.yaml")
		info, err := os.Stat(storyMapPath)
		if err != nil {
			// Skip directories without story-map.yaml
			continue
		}

		// Read display name from YAML
		displayName := dirName
		content, readErr := os.ReadFile(storyMapPath)
		if readErr == nil {
			displayName = extractDisplayName(string(content), dirName)
		}

		epics = append(epics, EpicInfo{
			ID:      dirName,
			Name:    displayName,
			Updated: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	if epics == nil {
		epics = []EpicInfo{}
	}
	return epics, nil
}

// CreateEpic creates the directory structure for a new epic.
// Generates a stable random ID (epic-XXXXX) as the directory name.
// The display name is stored only inside story-map.yaml and can be renamed freely.
func (m *EpicManager) CreateEpic(displayName string) (string, error) {
	trimmed := strings.TrimSpace(displayName)
	if trimmed == "" {
		return "", fmt.Errorf("create epic: display name must not be empty")
	}

	// Generate unique random ID, retry on collision
	var id string
	for i := 0; i < 10; i++ {
		candidate := generateEpicID()
		epicDir := filepath.Join(m.baseDir, candidate)
		if _, err := os.Stat(epicDir); os.IsNotExist(err) {
			id = candidate
			break
		}
	}
	if id == "" {
		return "", fmt.Errorf("create epic: failed to generate unique id after retries")
	}

	epicDir := filepath.Join(m.baseDir, id)

	// Create directory structure
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		return "", fmt.Errorf("create epic %s: %w", id, err)
	}
	if err := os.MkdirAll(filepath.Join(epicDir, "adr"), 0o755); err != nil {
		return "", fmt.Errorf("create epic %s adr dir: %w", id, err)
	}
	if err := os.MkdirAll(filepath.Join(epicDir, "spikes"), 0o755); err != nil {
		return "", fmt.Errorf("create epic %s spikes dir: %w", id, err)
	}

	// Write templates — use display name in content, random ID for directory
	now := time.Now().UTC().Format(time.RFC3339)
	storyMap := strings.ReplaceAll(storyMapTemplate, "{name}", trimmed)
	storyMap = strings.ReplaceAll(storyMap, "{date}", now)

	problemStatement := strings.ReplaceAll(problemStatementTemplate, "{name}", trimmed)

	if err := os.WriteFile(filepath.Join(epicDir, "story-map.yaml"), []byte(storyMap), 0o644); err != nil {
		return "", fmt.Errorf("create epic %s story-map: %w", id, err)
	}
	if err := os.WriteFile(filepath.Join(epicDir, "problem-statement.md"), []byte(problemStatement), 0o644); err != nil {
		return "", fmt.Errorf("create epic %s problem-statement: %w", id, err)
	}

	return id, nil
}

// DeleteEpic removes the entire epic directory. Takes epic ID.
func (m *EpicManager) DeleteEpic(id string) error {
	if err := validateEpicID(id); err != nil {
		return err
	}

	epicDir := filepath.Join(m.baseDir, id)
	if _, err := os.Stat(epicDir); os.IsNotExist(err) {
		return fmt.Errorf("delete epic %s: not found", id)
	}

	if err := os.RemoveAll(epicDir); err != nil {
		return fmt.Errorf("delete epic %s: %w", id, err)
	}

	return nil
}

// ReadStoryMap reads the YAML file as raw text. Takes epic ID.
func (m *EpicManager) ReadStoryMap(id string) (string, error) {
	if err := validateEpicID(id); err != nil {
		return "", err
	}

	epicDir := filepath.Join(m.baseDir, id)
	if _, err := os.Stat(epicDir); os.IsNotExist(err) {
		return "", fmt.Errorf("read story map %s: epic not found", id)
	}

	data, err := os.ReadFile(filepath.Join(epicDir, "story-map.yaml"))
	if err != nil {
		return "", fmt.Errorf("read story map %s: %w", id, err)
	}
	return string(data), nil
}

// WriteStoryMap does an atomic write (write to .tmp, then rename). Takes epic ID.
func (m *EpicManager) WriteStoryMap(id string, content string) error {
	if err := validateEpicID(id); err != nil {
		return err
	}

	epicDir := filepath.Join(m.baseDir, id)
	if _, err := os.Stat(epicDir); os.IsNotExist(err) {
		return fmt.Errorf("write story map %s: epic not found", id)
	}

	target := filepath.Join(epicDir, "story-map.yaml")
	tmpPath := target + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write story map %s: %w", id, err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write story map %s: %w", id, err)
	}

	return nil
}

// RenameEpic updates the display name inside story-map.yaml.
// The directory (epic ID) remains unchanged.
func (m *EpicManager) RenameEpic(id string, newName string) error {
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return fmt.Errorf("rename epic %s: new name must not be empty", id)
	}

	content, err := m.ReadStoryMap(id)
	if err != nil {
		return fmt.Errorf("rename epic %s: %w", id, err)
	}

	updated := namePattern.ReplaceAllString(content, `  name: "`+trimmed+`"`)
	if updated == content && !strings.Contains(content, trimmed) {
		// name field not found — shouldn't happen with our templates, but handle it
		return fmt.Errorf("rename epic %s: name field not found in story-map.yaml", id)
	}

	if err := m.WriteStoryMap(id, updated); err != nil {
		return fmt.Errorf("rename epic %s: %w", id, err)
	}
	return nil
}
