package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type storeData struct {
	Sessions             map[string]*SessionMeta `json:"sessions"`
	Projects             json.RawMessage         `json:"projects,omitempty"`
	Ideas                []*Idea                 `json:"ideas,omitempty"`
	NextIdeaID           int                     `json:"nextIdeaId,omitempty"`
	ModelConfig          *ModelConfig            `json:"modelConfig,omitempty"`
	EpicGitConfig        *EpicGitConfig          `json:"epicGitConfig,omitempty"`
	FirewallConfig       *FirewallConfig         `json:"firewallConfig,omitempty"`
	DigestConfig         *DigestConfig           `json:"digestConfig,omitempty"`
	RecoveryConfig       *RecoveryConfig         `json:"recoveryConfig,omitempty"`
}

// Store persists session metadata, project settings, and application
// configuration to a single JSON file with atomic writes.
type Store struct {
	path                 string
	sessions             map[string]*SessionMeta
	projects             []ProjectEntry
	ideas                []*Idea
	nextIdeaID           int
	modelConfig          ModelConfig
	epicGitConfig        EpicGitConfig
	firewallConfig       *FirewallConfig
	digestConfig         *DigestConfig
	recoveryConfig       *RecoveryConfig
	mu                   sync.RWMutex
}

// New creates a new Store with the given path. If path is empty, defaults to
// ~/.local/share/piper/sessions.json (XDG_DATA_HOME).
func New(path string) *Store {
	if path == "" {
		dataDir := os.Getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, _ := os.UserHomeDir()
			dataDir = filepath.Join(home, ".local", "share")
		}
		path = filepath.Join(dataDir, "piper", "sessions.json")
	}
	return &Store{
		path:     path,
		sessions: make(map[string]*SessionMeta),
	}
}

// Path returns the file path used by the store.
func (s *Store) Path() string {
	return s.path
}

// Load reads and parses the JSON file. If the file doesn't exist, initializes
// an empty sessions map. Creates the parent directory if needed.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create store directory %s: %w", dir, err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.sessions = make(map[string]*SessionMeta)
			return nil
		}
		return fmt.Errorf("read store file %s: %w", s.path, err)
	}

	var sd storeData
	if err := json.Unmarshal(data, &sd); err != nil {
		return fmt.Errorf("parse store file %s: %w", s.path, err)
	}

	if sd.Sessions == nil {
		sd.Sessions = make(map[string]*SessionMeta)
	}
	s.sessions = sd.Sessions
	s.projects = parseProjects(sd.Projects)
	s.ideas = sd.Ideas
	if s.ideas == nil {
		s.ideas = []*Idea{}
	}
	s.nextIdeaID = sd.NextIdeaID
	if sd.ModelConfig != nil {
		s.modelConfig = *sd.ModelConfig
	}
	if sd.EpicGitConfig != nil {
		s.epicGitConfig = *sd.EpicGitConfig
	}
	s.firewallConfig = sd.FirewallConfig
	s.digestConfig = sd.DigestConfig
	s.recoveryConfig = sd.RecoveryConfig
	return nil
}

// parseProjects handles backward-compatible deserialization of projects.
// Old format: ["path1", "path2"], new format: [{"path":"path1","autoMode":true}].
func parseProjects(raw json.RawMessage) []ProjectEntry {
	if len(raw) == 0 {
		return nil
	}

	// Try new format first
	var entries []ProjectEntry
	if err := json.Unmarshal(raw, &entries); err == nil && len(entries) > 0 {
		if entries[0].Path != "" {
			backfillSlugs(entries)
			return entries
		}
	}

	// Fall back to old format: []string
	var paths []string
	if err := json.Unmarshal(raw, &paths); err == nil {
		result := make([]ProjectEntry, len(paths))
		for i, p := range paths {
			result[i] = ProjectEntry{Path: p, Slug: PathToSlug(p)}
		}
		return result
	}

	return nil
}

// backfillSlugs generates slugs for entries that don't have one.
func backfillSlugs(entries []ProjectEntry) {
	for i := range entries {
		if entries[i].Slug == "" {
			entries[i].Slug = PathToSlug(entries[i].Path)
		}
	}
}

// Save performs an atomic write: marshal JSON (indented), write to a .tmp file,
// then os.Rename to the final path.
func (s *Store) Save() error {
	projectsJSON, err := json.Marshal(s.projects)
	if err != nil {
		return fmt.Errorf("marshal projects: %w", err)
	}
	var mc *ModelConfig
	if s.modelConfig != (ModelConfig{}) {
		mc = &s.modelConfig
	}
	var egc *EpicGitConfig
	if s.epicGitConfig != (EpicGitConfig{}) {
		egc = &s.epicGitConfig
	}
	sd := storeData{
		Sessions:             s.sessions,
		Projects:             projectsJSON,
		Ideas:                s.ideas,
		NextIdeaID:           s.nextIdeaID,
		ModelConfig:          mc,
		EpicGitConfig:        egc,
		FirewallConfig:       s.firewallConfig,
		DigestConfig:         s.digestConfig,
		RecoveryConfig:       s.recoveryConfig,
	}
	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store data: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file to %s: %w", s.path, err)
	}

	return nil
}
