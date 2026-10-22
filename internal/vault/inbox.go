package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func ideaFilename(id int) string {
	return fmt.Sprintf("idea-%05d.md", id)
}

func parseIdeaID(filename string) (int, bool) {
	// idea-00042.md → 42
	name := strings.TrimSuffix(filename, ".md")
	if !strings.HasPrefix(name, "idea-") {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(name, "idea-"))
	if err != nil {
		return 0, false
	}
	return id, true
}

// AddIdea creates a new idea with the given text and context.
func (v *VaultManager) AddIdea(text string, ctx []IdeaContext) (*Idea, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.nextID++
	idea := &Idea{
		ID:      v.nextID,
		Text:    text,
		Context: ctx,
		Created: time.Now(),
	}

	// Save meta
	metaPath := filepath.Join(v.baseDir, "meta.yaml")
	if err := saveMeta(metaPath, v.nextID); err != nil {
		v.nextID--
		return nil, fmt.Errorf("add idea: %w", err)
	}

	// Write idea file
	data, err := marshalIdea(idea)
	if err != nil {
		return nil, fmt.Errorf("add idea: %w", err)
	}
	ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(idea.ID))
	if err := os.WriteFile(ideaPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("add idea: write file: %w", err)
	}

	v.autoCommit(fmt.Sprintf("add idea #%d", idea.ID), "inbox/"+ideaFilename(idea.ID), "meta.yaml")

	copy := *idea
	return &copy, nil
}

// GetIdeas returns all ideas sorted by ID descending (newest first).
func (v *VaultManager) GetIdeas() ([]*Idea, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	inboxDir := filepath.Join(v.baseDir, "inbox")
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Idea{}, nil
		}
		return nil, fmt.Errorf("get ideas: %w", err)
	}

	var ideas []*Idea
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := parseIdeaID(entry.Name()); !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(inboxDir, entry.Name()))
		if err != nil {
			continue
		}
		idea, err := parseIdeaFile(data)
		if err != nil {
			continue
		}
		ideas = append(ideas, idea)
	}

	// Sort by ID descending
	sort.Slice(ideas, func(i, j int) bool {
		return ideas[i].ID > ideas[j].ID
	})

	if ideas == nil {
		ideas = []*Idea{}
	}
	return ideas, nil
}

// GetIdea returns a single idea by ID.
func (v *VaultManager) GetIdea(id int) (*Idea, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(id))
	data, err := os.ReadFile(ideaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get idea %d: %w", id, err)
	}
	idea, err := parseIdeaFile(data)
	if err != nil {
		return nil, fmt.Errorf("get idea %d: %w", id, err)
	}
	copy := *idea
	return &copy, nil
}

// DeleteIdea removes an idea by ID.
func (v *VaultManager) DeleteIdea(id int) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(id))
	if err := os.Remove(ideaPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete idea %d: %w", id, err)
	}

	v.autoCommit(fmt.Sprintf("delete idea #%d", id), "-A")
	return nil
}

// UpdateIdeaText updates the text of an existing idea.
func (v *VaultManager) UpdateIdeaText(id int, text string) (*Idea, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(id))
	data, err := os.ReadFile(ideaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("idea %d not found", id)
		}
		return nil, fmt.Errorf("update idea %d: %w", id, err)
	}

	idea, err := parseIdeaFile(data)
	if err != nil {
		return nil, fmt.Errorf("update idea %d: %w", id, err)
	}

	idea.Text = text
	newData, err := marshalIdea(idea)
	if err != nil {
		return nil, fmt.Errorf("update idea %d: %w", id, err)
	}

	if err := os.WriteFile(ideaPath, newData, 0o644); err != nil {
		return nil, fmt.Errorf("update idea %d: %w", id, err)
	}

	v.autoCommit(fmt.Sprintf("update idea #%d", id), "inbox/"+ideaFilename(id))

	copy := *idea
	return &copy, nil
}

// UpdateIdeaEnrichedDesc updates the AI-enriched description for an idea.
func (v *VaultManager) UpdateIdeaEnrichedDesc(id int, desc string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	ideaPath := filepath.Join(v.baseDir, "inbox", ideaFilename(id))
	data, err := os.ReadFile(ideaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("idea %d not found", id)
		}
		return fmt.Errorf("update idea %d: %w", id, err)
	}

	idea, err := parseIdeaFile(data)
	if err != nil {
		return fmt.Errorf("update idea %d: %w", id, err)
	}

	idea.EnrichedDesc = desc
	newData, err := marshalIdea(idea)
	if err != nil {
		return fmt.Errorf("update idea %d: %w", id, err)
	}

	if err := os.WriteFile(ideaPath, newData, 0o644); err != nil {
		return fmt.Errorf("update idea %d: %w", id, err)
	}

	v.autoCommit(fmt.Sprintf("enrich idea #%d", id), "inbox/"+ideaFilename(id))
	return nil
}
