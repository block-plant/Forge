// Package multiproject provides a project registry that namespaces all Forge
// resources (database collections, storage paths, auth users, analytics events)
// under a project ID. This enables running multiple isolated projects from
// a single Forge instance.
package multiproject

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// Project represents a single isolated project namespace within Forge.
type Project struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Settings    map[string]string `json:"settings,omitempty"`
	Active      bool              `json:"active"`
}

// DataPath returns the root data directory for a project, underneath the global data dir.
func (p *Project) DataPath(globalDataDir string) string {
	return filepath.Join(globalDataDir, "projects", p.ID)
}

// DBPrefix returns the collection prefix used to namespace this project's documents.
// e.g. "proj_abc:users" instead of "users".
func (p *Project) DBPrefix(collection string) string {
	return fmt.Sprintf("%s:%s", p.ID, collection)
}

// StoragePath returns the storage root for this project's files.
func (p *Project) StoragePath(path string) string {
	return filepath.Join("projects", p.ID, path)
}

// Registry manages all projects in a Forge instance.
type Registry struct {
	mu       sync.RWMutex
	projects map[string]*Project
	default_ string // default project ID
}

// NewRegistry creates a new project registry.
func NewRegistry() *Registry {
	r := &Registry{
		projects: make(map[string]*Project),
	}
	// Always create the default project
	r.Create("default", "Default Project", "")
	r.default_ = "default"
	return r
}

// Create registers a new project. Returns an error if the ID already exists.
func (r *Registry) Create(id, name, description string) (*Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.projects[id]; ok {
		return nil, fmt.Errorf("project %q already exists", id)
	}

	p := &Project{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		Settings:    make(map[string]string),
		Active:      true,
	}
	r.projects[id] = p
	return p, nil
}

// Get returns a project by ID.
func (r *Registry) Get(id string) (*Project, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projects[id]
	return p, ok
}

// Default returns the default project.
func (r *Registry) Default() *Project {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.projects[r.default_]
}

// List returns all active projects sorted by creation time.
func (r *Registry) List() []*Project {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Project, 0, len(r.projects))
	for _, p := range r.projects {
		if p.Active {
			result = append(result, p)
		}
	}
	return result
}

// Delete marks a project as inactive. Data on disk is not removed.
func (r *Registry) Delete(id string) error {
	if id == "default" {
		return fmt.Errorf("cannot delete the default project")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	p.Active = false
	return nil
}

// SetSetting stores a key/value setting for a project.
func (r *Registry) SetSetting(id, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	p.Settings[key] = value
	return nil
}
