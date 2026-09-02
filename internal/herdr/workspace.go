package herdr

import (
	"fmt"
)

// Workspace is a herdr workspace (container of tabs and panes).
type Workspace struct {
	ID     string `json:"workspace_id"`
	Label  string `json:"label"`
	Status string `json:"agent_status"`
}

// Pane is a single terminal inside a workspace.
type Pane struct {
	ID          string `json:"pane_id"`
	WorkspaceID string `json:"workspace_id"`
	Status      string `json:"agent_status"`
	Cwd         string `json:"cwd"`
}

type workspaceListResult struct {
	Workspaces []Workspace `json:"workspaces"`
}

// ListWorkspaces returns all workspaces in the current session.
func ListWorkspaces() ([]Workspace, error) {
	var r workspaceListResult
	if err := run(&r, "workspace", "list"); err != nil {
		return nil, err
	}
	return r.Workspaces, nil
}

type workspaceCreateResult struct {
	Workspace Workspace `json:"workspace"`
	RootPane  Pane      `json:"root_pane"`
}

// CreateWorkspace makes a new workspace and returns it with its root pane.
func CreateWorkspace(label, cwd string) (Workspace, Pane, error) {
	args := []string{"workspace", "create", "--label", label}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	var r workspaceCreateResult
	if err := run(&r, args...); err != nil {
		return Workspace{}, Pane{}, err
	}
	return r.Workspace, r.RootPane, nil
}

// CloseWorkspace closes a workspace by id, killing its panes.
func CloseWorkspace(id string) error {
	return run(nil, "workspace", "close", id)
}

// SplitPane splits an existing pane and returns the new pane.
func SplitPane(paneID, direction, cwd string) (Pane, error) {
	args := []string{"pane", "split", paneID, "--direction", direction}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	var p Pane
	if err := run(&p, args...); err != nil {
		return Pane{}, err
	}
	return p, nil
}

// PaneRead returns recent terminal output of a pane (plain text).
func PaneRead(paneID string, lines int) (string, error) {
	args := []string{"pane", "read", paneID}
	if lines > 0 {
		args = append(args, "--lines", fmt.Sprintf("%d", lines))
	}
	return runRaw(args...)
}

// ClosePane closes a single pane.
func ClosePane(paneID string) error {
	return run(nil, "pane", "close", paneID)
}
