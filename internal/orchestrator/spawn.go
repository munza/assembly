// Package orchestrator owns agent lifecycle: spawn pi workers in herdr
// workspaces, track them on disk, and clean them up.
package orchestrator

import (
	"fmt"
	"time"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/state"
)

// Spawn creates a herdr workspace, launches a pi agent named name inside it,
// and registers it in the on-disk store.
func Spawn(cfg *config.Config, store *state.Store, name, task, model string) (*state.Agent, error) {
	if _, err := store.Get(name); err == nil {
		return nil, fmt.Errorf("agent %q already exists", name)
	}

	cwd := cfg.RepoDir
	ws, pane, err := herdr.CreateWorkspace(name, cwd)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	agentArgs := []string{}
	if model != "" {
		agentArgs = append(agentArgs, "--model", model)
	}
	if err := herdr.AgentStart(name, "pi", pane.ID, agentArgs...); err != nil {
		_ = herdr.CloseWorkspace(ws.ID)
		return nil, fmt.Errorf("start pi: %w", err)
	}

	a := &state.Agent{
		Name:        name,
		Kind:        "pi",
		PaneID:      pane.ID,
		WorkspaceID: ws.ID,
		Cwd:         cwd,
		Task:        task,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Put(a); err != nil {
		return nil, err
	}
	return a, nil
}

// Close shuts down an agent: closes its workspace and drops it from the store.
func Close(store *state.Store, name string) error {
	a, err := store.Get(name)
	if err != nil {
		return err
	}
	if err := herdr.CloseWorkspace(a.WorkspaceID); err != nil {
		return err
	}
	return store.Delete(name)
}
