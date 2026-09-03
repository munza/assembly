package herdr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func Available() bool {
	_, err := exec.LookPath("herdr")
	return err == nil
}

func RunJSON(args ...string) (map[string]any, error) {
	cmd := exec.Command("herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("herdr %s: %s", strings.Join(args, " "), msg)
	}
	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("herdr %s: unexpected output: %.200s", strings.Join(args, " "), stdout.String())
	}
	return res, nil
}

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func str(m map[string]any, path ...string) string {
	s, _ := dig(m, path...).(string)
	return s
}

func WorkspaceCreate(cwd, label string) (string, error) {
	res, err := RunJSON("workspace", "create", "--cwd", cwd, "--label", label, "--no-focus")
	if err != nil {
		return "", err
	}
	id := str(res, "result", "workspace", "workspace_id")
	if id == "" {
		return "", fmt.Errorf("herdr workspace create: no workspace_id in response")
	}
	return id, nil
}

func WorktreeCreate(workspaceID, branch, base string) (string, string, error) {
	args := []string{"worktree", "create", "--workspace", workspaceID, "--branch", branch}
	if base != "" {
		args = append(args, "--base", base)
	}
	res, err := RunJSON(args...)
	if err != nil {
		return "", "", err
	}
	id := str(res, "result", "workspace", "workspace_id")
	if id == "" {
		return "", "", fmt.Errorf("herdr worktree create: no workspace_id in response")
	}
	path := str(res, "result", "workspace", "cwd")
	if path == "" {
		path = str(res, "result", "cwd")
	}
	return id, path, nil
}

func WorkspaceClose(id string) error {
	_, err := RunJSON("workspace", "close", id)
	return err
}

func WorktreeRemove(id string, force bool) error {
	args := []string{"worktree", "remove", "--workspace", id}
	if force {
		args = append(args, "--force")
	}
	_, err := RunJSON(args...)
	return err
}

func TabCreate(workspaceID, cwd, label string) (string, string, error) {
	args := []string{"tab", "create", "--workspace", workspaceID, "--label", label, "--no-focus"}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	res, err := RunJSON(args...)
	if err != nil {
		return "", "", err
	}
	tabID := str(res, "result", "tab", "tab_id")
	paneID := str(res, "result", "root_pane", "pane_id")
	if tabID == "" || paneID == "" {
		return "", "", fmt.Errorf("herdr tab create: no tab_id or pane_id in response")
	}
	return tabID, paneID, nil
}

func TabClose(tabID string) error {
	_, err := RunJSON("tab", "close", tabID)
	return err
}

func AgentStart(name, paneID string) error {
	_, err := RunJSON("agent", "start", name, "--kind", "pi", "--pane", paneID)
	return err
}

func AgentPrompt(name, text string) error {
	_, err := RunJSON("agent", "prompt", name, text)
	return err
}
