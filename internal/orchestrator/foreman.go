package orchestrator

import (
	"fmt"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/task"
)

// StartForeman brings up the liaison agent: a "foreman" tab in the project
// workspace with pi inside, primed with the foreman skill. One per project.
func StartForeman(cfg *config.Config) (paneID string, err error) {
	ws, err := herdr.FindWorkspaceByLabel(cfg.ProjectName)
	if err != nil {
		return "", err
	}
	if ws == nil {
		created, root, err := herdr.CreateWorkspace(cfg.ProjectName, cfg.RepoDir)
		if err != nil {
			return "", fmt.Errorf("create project workspace: %w", err)
		}
		ws = &created
		_ = root
	}

	if a := herdr.FindAgentByName("foreman"); a != nil {
		return a.PaneID, nil // already running
	}

	tabID, paneID, err := herdr.CreateTab(ws.ID, "foreman")
	if err != nil {
		return "", fmt.Errorf("create foreman tab: %w", err)
	}
	if err := herdr.AgentStart("foreman", "pi", paneID); err != nil {
		_ = herdr.CloseTab(tabID)
		return "", fmt.Errorf("start pi: %w", err)
	}

	prompt := "You are the foreman for project " + cfg.ProjectName + ". Follow the foreman skill.\n" +
		"foreman CLI: " + EnsureBin(cfg) + "\n" +
		"Check mail with it, dispatch tasks, keep the user informed. " +
		"Reply with: FOREMAN_READY"
	if err := herdr.AgentPrompt(paneID, prompt, false, nil, 0); err != nil {
		return "", fmt.Errorf("prime foreman: %w", err)
	}
	return paneID, nil
}

// SetState updates a task's state if valid.
func SetState(t *task.Task, state string) error {
	if !task.ValidState(state) {
		return fmt.Errorf("invalid state %q (want one of %v)", state, task.ValidStates)
	}
	t.State = state
	return t.Save()
}
