package store

// IdeaContext captures activity context at the time an idea was recorded.
type IdeaContext struct {
	SessionName string `json:"sessionName"`
	Kind        string `json:"kind"`
	Detail      string `json:"detail"`
	Time        int64  `json:"time"`
}

// Idea represents a quick-capture inbox item.
type Idea struct {
	ID           int           `json:"id"`
	Text         string        `json:"text"`
	Context      []IdeaContext `json:"context,omitempty"`
	EnrichedDesc string        `json:"enrichedDesc,omitempty"`
	CreatedAt    int64         `json:"createdAt"`
}

// GetIdeas returns a copy of all ideas (newest first).
// Kept for migration — new ideas go through VaultManager.
func (s *Store) GetIdeas() []*Idea {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Idea, len(s.ideas))
	for i, idea := range s.ideas {
		cp := *idea
		result[i] = &cp
	}
	return result
}

// GetNextIdeaID returns the current nextIdeaID counter (used for migration).
func (s *Store) GetNextIdeaID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextIdeaID
}
