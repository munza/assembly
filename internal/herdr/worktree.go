package herdr

// Worktree is a herdr git-worktree-backed workspace.
type Worktree struct {
	Branch      string `json:"branch"`
	Label       string `json:"label"`
	Path        string `json:"path"`
	WorkspaceID string `json:"open_workspace_id"`
	IsLinked    bool   `json:"is_linked_worktree"`
}

type worktreeListResult struct {
	Worktrees []Worktree `json:"worktrees"`
}

// ListWorktrees returns all worktree workspaces.
func ListWorktrees() ([]Worktree, error) {
	var r worktreeListResult
	if err := run(&r, "worktree", "list"); err != nil {
		return nil, err
	}
	return r.Worktrees, nil
}

// FindWorktreeByPath returns the worktree checked out at path, or nil.
func FindWorktreeByPath(path string) (*Worktree, error) {
	wts, err := ListWorktrees()
	if err != nil {
		return nil, err
	}
	for i := range wts {
		if wts[i].Path == path {
			return &wts[i], nil
		}
	}
	return nil, nil
}

type worktreeCreateResult struct {
	Workspace Workspace `json:"workspace"`
	RootPane  Pane      `json:"root_pane"`
}

// CreateWorktree makes a git worktree and its herdr workspace, returning
// the workspace id and root pane.
func CreateWorktree(label, branch, path string) (workspaceID, rootPaneID string, err error) {
	var r worktreeCreateResult
	args := []string{
		"worktree", "create",
		"--label", label,
		"--branch", branch,
		"--path", path,
		"--no-focus",
	}
	if err := run(&r, args...); err != nil {
		return "", "", err
	}
	return r.Workspace.ID, r.RootPane.ID, nil
}

// RemoveWorktree removes a worktree checkout (and its workspace).
func RemoveWorktree(workspaceID string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", workspaceID}
	if force {
		args = append(args, "--force")
	}
	return run(nil, args...)
}

// RenamePane sets a pane's label.
func RenamePane(paneID, label string) error {
	return run(nil, "pane", "rename", paneID, label)
}

// Tab is a labeled layout container inside a workspace.
type Tab struct {
	ID          string `json:"tab_id"`
	Label       string `json:"label"`
	WorkspaceID string `json:"workspace_id"`
}

type tabCreateResult struct {
	Tab  Tab  `json:"tab"`
	Root Pane `json:"root_pane"`
}

// CreateTab makes a labeled tab in a workspace and returns the tab id
// plus its root pane (where an agent can be started).
func CreateTab(workspaceID, label string) (tabID, paneID string, err error) {
	var r tabCreateResult
	if err := run(&r, "tab", "create", "--workspace", workspaceID, "--label", label, "--no-focus"); err != nil {
		return "", "", err
	}
	return r.Tab.ID, r.Root.ID, nil
}

// CloseTab closes a tab and its panes.
func CloseTab(tabID string) error {
	return run(nil, "tab", "close", tabID)
}
