// Package orchestrator owns task lifecycle: create the issue worktree,
// open a labeled pane, start pi inside it, and track it in a task file.
package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"path/filepath"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/task"
)

// BinPath is where task new installs the foreman CLI for workers to call.
func BinPath(cfg *config.Config) string {
	return filepath.Join(config.StateDir(), "bin", "foreman")
}

// EnsureBin builds the foreman CLI into .assembly/bin/foreman (best
// effort: skips quietly if go is unavailable or the build fails).
func EnsureBin(cfg *config.Config) string {
	bin := BinPath(cfg)
	if _, err := exec.LookPath("go"); err == nil {
		_ = os.MkdirAll(filepath.Dir(bin), 0o755)
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/foreman")
		cmd.Dir = cfg.RepoDir
		if cmd.Run() == nil {
			abs, _ := filepath.Abs(bin)
			return abs
		}
	}
	if self, err := os.Executable(); err == nil {
		_ = os.MkdirAll(filepath.Dir(bin), 0o755)
		if copyFile(self, bin) == nil {
			abs, _ := filepath.Abs(bin)
			return abs
		}
	}
	return "foreman" // hope it's on PATH
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

// sanitizeAgentName makes a herdr-safe agent name: lowercase letters,
// digits, '-' and '_', max 32 chars.
func sanitizeAgentName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 32 {
		name = name[:32]
	}
	return name
}

// ensureWorktree returns the herdr workspace id for an issue, creating the
// git worktree if needed. One worktree per issue.
func ensureWorktree(cfg *config.Config, id, slug string) (string, error) {
	path := filepath.Join(cfg.WorktreeDir, id)
	if wt, err := herdr.FindWorktreeByPath(path); err != nil {
		return "", err
	} else if wt != nil && wt.WorkspaceID != "" {
		return wt.WorkspaceID, nil
	}
	branch := cfg.BranchPrefix + id + "-" + slug
	wsID, _, err := herdr.CreateWorktree(id, branch, path)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	return wsID, nil
}

// NewTask runs the full `task new` flow: issue worktree (shared per issue),
// one labeled pane, pi started inside, task file saved. One task = one pane.
func NewTask(cfg *config.Config, taskType, title, issue, message, model string) (*task.Task, error) {
	if !task.ValidType(taskType) {
		return nil, fmt.Errorf("invalid type %q (want one of %v)", taskType, task.ValidTypes)
	}
	tasks, err := task.Load()
	if err != nil {
		return nil, err
	}
	id := issue
	if id == "" {
		id = task.NextLocalID(tasks)
	}
	slug := task.Slug(title)
	if slug == "" {
		return nil, fmt.Errorf("title has no usable words for a slug")
	}

	wsID, err := ensureWorktree(cfg, id, slug)
	if err != nil {
		return nil, err
	}

	// One task is one tab in the issue worktree's workspace.
	tabLabel := taskType + "-" + slug
	tabID, paneID, err := herdr.CreateTab(wsID, tabLabel)
	if err != nil {
		return nil, fmt.Errorf("create tab: %w", err)
	}

	t := &task.Task{
		ID:          id,
		Type:        taskType,
		Slug:        slug,
		Title:       title,
		State:       task.StatePicked,
		Branch:      cfg.BranchPrefix + id + "-" + slug,
		WorkspaceID: wsID,
		TabID:       tabID,
		TabLabel:    tabLabel,
		PaneID:      paneID,
		Message:     message,
	}

	// Start pi in the tab. Agent name stays short (<type>-<id>); the tab
	// carries the full label. herdr requires lowercase/digits/-/_ (max 32).
	agentName := sanitizeAgentName(taskType + "-" + id)
	agentArgs := []string{}
	if model != "" {
		agentArgs = append(agentArgs, "--model", model)
	} else if cfg.Model != "" {
		agentArgs = append(agentArgs, "--model", cfg.Model)
	}
	if err := herdr.AgentStart(agentName, "pi", t.PaneID, agentArgs...); err != nil {
		_ = herdr.CloseTab(t.TabID) // roll back the empty tab
		return nil, fmt.Errorf("start pi: %w", err)
	}
	if err := t.Save(); err != nil {
		return nil, err
	}

	// Workers need the foreman CLI and their task context. A custom
	// --message rides on top of the standard prompt.
	prompt := t.Prompt(EnsureBin(cfg))
	if message != "" {
		prompt += "\n\nAdditional instructions:\n" + message
	}
	if err := herdr.AgentPrompt(t.PaneID, prompt, false, nil, 0); err != nil {
		return nil, fmt.Errorf("send initial prompt: %w", err)
	}
	t.State = task.StateWorking
	return t, t.Save()
}

// CloseTask marks a task done and frees its tab. The issue worktree is
// removed only when no sibling tasks still use it.
func CloseTask(t *task.Task) error {
	if err := herdr.CloseTab(t.TabID); err != nil {
		return err
	}
	t.State = task.StateDone
	if err := t.Save(); err != nil {
		return err
	}
	tasks, err := task.Load()
	if err != nil {
		return err
	}
	if len(task.OpenForWorkspace(tasks, t.WorkspaceID)) == 0 {
		return herdr.RemoveWorktree(t.WorkspaceID, false)
	}
	return nil
}
