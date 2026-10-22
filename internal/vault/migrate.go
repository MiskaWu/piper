package vault

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OldIdea represents an idea in the old store format (sessions.json).
type OldIdea struct {
	ID           int
	Text         string
	Context      []OldIdeaContext
	EnrichedDesc string
	CreatedAt    int64
}

// OldIdeaContext is the old context format from store.IdeaContext.
type OldIdeaContext struct {
	SessionName string
	Kind        string
	Detail      string
	Time        int64
}

// MigrateIfNeeded migrates old epics and ideas into the vault.
// It checks for a .migration-done marker and skips if already done.
func (v *VaultManager) MigrateIfNeeded(oldEpicsDir string, ideas []OldIdea, nextID int) error {
	markerPath := filepath.Join(v.baseDir, ".migration-done")
	if _, err := os.Stat(markerPath); err == nil {
		return nil // already migrated
	}

	migrated := false

	// Migrate epics
	if oldEpicsDir != "" {
		if info, err := os.Stat(oldEpicsDir); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(oldEpicsDir)
			for _, entry := range entries {
				if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "epic-") {
					continue
				}
				srcDir := filepath.Join(oldEpicsDir, entry.Name())
				dstDir := filepath.Join(v.baseDir, "epics", entry.Name())
				if _, err := os.Stat(dstDir); err == nil {
					continue // already exists in vault
				}
				if err := copyDir(srcDir, dstDir); err != nil {
					log.Printf("WARNING: migrate epic %s: %v", entry.Name(), err)
					continue
				}
				migrated = true
			}
		}
	}

	// Migrate ideas
	if len(ideas) > 0 {
		for _, old := range ideas {
			var ctx []IdeaContext
			for _, c := range old.Context {
				ctx = append(ctx, IdeaContext{
					Session: c.SessionName,
					Kind:    c.Kind,
					Detail:  c.Detail,
					Time:    c.Time,
				})
			}

			created := time.Unix(old.CreatedAt, 0)
			if old.CreatedAt == 0 {
				created = time.Now()
			}

			idea := &Idea{
				ID:           old.ID,
				Text:         old.Text,
				Context:      ctx,
				EnrichedDesc: old.EnrichedDesc,
				Created:      created,
			}

			data, err := marshalIdea(idea)
			if err != nil {
				log.Printf("WARNING: migrate idea %d: %v", old.ID, err)
				continue
			}

			ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(old.ID))
			if err := os.WriteFile(ideaPath, data, 0o644); err != nil {
				log.Printf("WARNING: migrate idea %d: %v", old.ID, err)
				continue
			}
			migrated = true
		}

		// Update nextID if needed
		if nextID > v.nextID {
			v.nextID = nextID
			metaPath := filepath.Join(v.baseDir, "meta.yaml")
			if err := saveMeta(metaPath, v.nextID); err != nil {
				log.Printf("WARNING: migrate: save meta: %v", err)
			}
		}
	}

	if migrated {
		v.autoCommit("migrate: import epics and inbox", "-A")
	}

	// Write marker
	if err := os.WriteFile(markerPath, []byte(fmt.Sprintf("migrated at %s\n", time.Now().Format(time.RFC3339))), 0o644); err != nil {
		return fmt.Errorf("migrate: write marker: %w", err)
	}

	// Rename old epics dir to .bak
	if oldEpicsDir != "" {
		if _, err := os.Stat(oldEpicsDir); err == nil {
			bakDir := oldEpicsDir + ".bak"
			if err := os.Rename(oldEpicsDir, bakDir); err != nil {
				log.Printf("WARNING: migrate: rename old epics dir: %v", err)
			}
		}
	}

	return nil
}

// copyDir copies a directory recursively, skipping .git/ directories.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("copy %s: %w", rel, err)
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}
