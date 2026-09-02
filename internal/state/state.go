// Package state tracks spawned agents and tasks on disk so foreman is
// restart-proof: after a crash or reboot it reconciles .assembly/agents.json
// against the live herdr session.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"assembly/internal/config"
)

// Agent is one spawned worker entry.
type Agent struct {
	Name        string    `json:"name"`
	Kind        string    `json:"kind"` // pi
	PaneID      string    `json:"pane_id"`
	WorkspaceID string    `json:"workspace_id"`
	Cwd         string    `json:"cwd"`
	Task        string    `json:"task,omitempty"` // task/label it is working on
	CreatedAt   time.Time `json:"created_at"`
}

// Store is the on-disk agent registry.
type Store struct {
	Agents map[string]*Agent `json:"agents"`
}

func path() string {
	return filepath.Join(config.StateDir(), "agents.json")
}

// Load reads the registry; empty store if missing.
func Load() *Store {
	s := &Store{Agents: map[string]*Agent{}}
	b, err := os.ReadFile(path())
	if err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

// Save writes the registry to disk.
func (s *Store) Save() error {
	if err := os.MkdirAll(config.StateDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), b, 0o644)
}

// Put adds or updates an agent entry.
func (s *Store) Put(a *Agent) error {
	s.Agents[a.Name] = a
	return s.Save()
}

// Get returns an agent by name.
func (s *Store) Get(name string) (*Agent, error) {
	a, ok := s.Agents[name]
	if !ok {
		return nil, errors.New("unknown agent: " + name)
	}
	return a, nil
}

// Delete removes an agent entry.
func (s *Store) Delete(name string) error {
	delete(s.Agents, name)
	return s.Save()
}
