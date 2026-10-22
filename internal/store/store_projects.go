package store

import "path/filepath"

// ProjectEntry holds per-project settings.
type ProjectEntry struct {
	Path        string `json:"path"`
	Slug        string `json:"slug,omitempty"`
	AutoMode    *bool  `json:"autoMode,omitempty"`
	Description string `json:"description,omitempty"`
}

// AddProject adds a project path to the store (deduplicated) and saves.
func (s *Store) AddProject(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.projects {
		if p.Path == path {
			return nil
		}
	}
	s.projects = append(s.projects, ProjectEntry{Path: path, Slug: PathToSlug(path)})
	return s.Save()
}

// AddProjectWithDescription adds a project with a description (deduplicated) and saves.
// If the project already exists, its description is updated.
func (s *Store) AddProjectWithDescription(path, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.projects {
		if p.Path == path {
			s.projects[i].Description = description
			return s.Save()
		}
	}
	s.projects = append(s.projects, ProjectEntry{Path: path, Slug: PathToSlug(path), Description: description})
	return s.Save()
}

// RemoveProject removes a project path from the store and saves.
func (s *Store) RemoveProject(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.projects {
		if p.Path == path {
			s.projects = append(s.projects[:i], s.projects[i+1:]...)
			return s.Save()
		}
	}
	return nil
}

// GetProjects returns a copy of the registered project paths.
func (s *Store) GetProjects() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, len(s.projects))
	for i, p := range s.projects {
		result[i] = p.Path
	}
	return result
}

// GetProjectEntries returns a copy of the registered project entries.
func (s *Store) GetProjectEntries() []ProjectEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ProjectEntry, len(s.projects))
	copy(result, s.projects)
	return result
}

// ResolveBySlug finds a project by its slug.
func (s *Store) ResolveBySlug(slug string) (ProjectEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.projects {
		if p.Slug == slug {
			return p, true
		}
	}
	return ProjectEntry{}, false
}

// ResolveByBasename finds a project by its directory basename (backward compatible).
func (s *Store) ResolveByBasename(name string) (ProjectEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.projects {
		if filepath.Base(p.Path) == name {
			return p, true
		}
	}
	return ProjectEntry{}, false
}

// GetProjectAutoMode returns the autoMode setting for a project, or nil if not set.
func (s *Store) GetProjectAutoMode(path string) *bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.projects {
		if p.Path == path {
			return p.AutoMode
		}
	}
	return nil
}

// SetProjectOrder reorders the projects slice to match the given path order.
// Paths not found in the store are ignored. Projects not in the order list
// are appended at the end in their original relative order.
func (s *Store) SetProjectOrder(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lookup := make(map[string]ProjectEntry, len(s.projects))
	for _, p := range s.projects {
		lookup[p.Path] = p
	}

	result := make([]ProjectEntry, 0, len(s.projects))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if entry, ok := lookup[path]; ok && !seen[path] {
			result = append(result, entry)
			seen[path] = true
		}
	}
	for _, p := range s.projects {
		if !seen[p.Path] {
			result = append(result, p)
		}
	}

	s.projects = result
	return s.Save()
}

// SetProjectAutoMode sets the autoMode for a project and saves.
func (s *Store) SetProjectAutoMode(path string, autoMode *bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.projects {
		if p.Path == path {
			s.projects[i].AutoMode = autoMode
			return s.Save()
		}
	}
	return nil
}
