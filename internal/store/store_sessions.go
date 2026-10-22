package store

// SessionMeta holds metadata for a single tmux session.
type SessionMeta struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DescriptionAuto bool   `json:"descriptionAuto,omitempty"`
	Order           int    `json:"order"`
	ProjectPath     string `json:"projectPath,omitempty"`
	ClaudeSessionID string `json:"claudeSessionId,omitempty"`
}

// AllSessions returns a copy of all session metadata.
func (s *Store) AllSessions() map[string]SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]SessionMeta, len(s.sessions))
	for name, meta := range s.sessions {
		result[name] = *meta
	}
	return result
}

// GetMeta returns a copy of the session metadata, or nil if not found.
func (s *Store) GetMeta(name string) *SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.sessions[name]
	if !ok {
		return nil
	}
	cp := *m
	return &cp
}

// SetMeta sets metadata for a session and saves to disk.
func (s *Store) SetMeta(name string, meta SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[name] = &meta
	return s.Save()
}

// SetAutoDescription updates only the Description and DescriptionAuto fields
// for a session, preserving all other metadata (e.g. ProjectPath).
// If the session does not exist in the store, this is a no-op.
func (s *Store) SetAutoDescription(name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.sessions[name]
	if !ok {
		return nil
	}
	m.Description = description
	m.DescriptionAuto = true
	return s.Save()
}

// SetOrder takes an ordered slice of session names and sets Order field
// (0, 1, 2...) accordingly, then saves.
func (s *Store) SetOrder(names []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, name := range names {
		if m, ok := s.sessions[name]; ok {
			m.Order = i
		}
	}
	return s.Save()
}

// DeleteMeta removes a session entry and saves.
func (s *Store) DeleteMeta(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, name)
	return s.Save()
}

// SetClaudeSessionID sets the ClaudeSessionID for a session and saves to disk.
// If the session does not exist in the store, this is a no-op.
func (s *Store) SetClaudeSessionID(name, claudeSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.sessions[name]
	if !ok {
		return nil
	}
	m.ClaudeSessionID = claudeSessionID
	return s.Save()
}

// InsertAfterGroup returns the order value for a new session that should be
// grouped with existing sessions sharing the same group. The extractGroup
// function maps a session name to its group key. It shifts existing sessions'
// order to make room. If no matching group is found, the new session is
// appended at the end. Caller must hold no lock; this method acquires mu.
func (s *Store) InsertAfterGroup(group string, extractGroup func(string) string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxOrder := -1
	globalMax := -1
	for name, m := range s.sessions {
		if m.Order > globalMax {
			globalMax = m.Order
		}
		if group != "" && extractGroup(name) == group && m.Order > maxOrder {
			maxOrder = m.Order
		}
	}

	if maxOrder < 0 {
		return globalMax + 1
	}

	insertAt := maxOrder + 1
	for _, m := range s.sessions {
		if m.Order >= insertAt {
			m.Order++
		}
	}
	_ = s.Save()

	return insertAt
}

// GetSessionsByProject returns a map of projectPath -> []sessionName for all
// sessions that have a non-empty ProjectPath.
func (s *Store) GetSessionsByProject() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string)
	for name, meta := range s.sessions {
		if meta.ProjectPath != "" {
			result[meta.ProjectPath] = append(result[meta.ProjectPath], name)
		}
	}
	return result
}
