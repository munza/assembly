package mux

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
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

func WorktreeCreate(workspaceID, branch, base string) (string, string, string, error) {
	args := []string{"worktree", "create", "--workspace", workspaceID, "--branch", branch}
	if base != "" {
		args = append(args, "--base", base)
	}
	res, err := RunJSON(args...)
	if err != nil {
		return "", "", "", err
	}
	id := str(res, "result", "workspace", "workspace_id")
	if id == "" {
		return "", "", "", fmt.Errorf("herdr worktree create: no workspace_id in response")
	}
	path := str(res, "result", "worktree", "path")
	if path == "" {
		path = str(res, "result", "root_pane", "cwd")
	}
	tabID := str(res, "result", "tab", "tab_id")
	return id, path, tabID, nil
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

func TabCreate(workspaceID, cwd, label string, env map[string]string) (string, string, error) {
	args := []string{"tab", "create", "--workspace", workspaceID, "--label", label, "--no-focus"}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	for k, v := range env {
		args = append(args, "--env", k+"="+v)
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

// TabCloseDetached closes a tab from a detached process. Used when a worker's
// own mailbox send closes its tab: the sender must survive long enough to
// finish writing the message.
func TabCloseDetached(tabID string) {
	cmd := exec.Command("sh", "-c", "sleep 1; exec herdr tab close "+tabID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	_ = cmd.Start()
}

func AgentStart(name, paneID, kind string, agentArgs ...string) error {
	args := []string{"agent", "start", name, "--kind", kind, "--pane", paneID}
	if len(agentArgs) > 0 {
		args = append(args, "--")
		args = append(args, agentArgs...)
	}
	_, err := RunJSON(args...)
	return err
}

func AgentPrompt(name, text string) error {
	_, err := RunJSON("agent", "prompt", name, text)
	return err
}

// CurrentAgentKind reports the herdr agent kind (e.g. "claude", "pi") running
// in this process's own pane, identified via HERDR_PANE_ID. Used to default
// the worker harness to whatever the central foreman instance is itself
// running, instead of a fixed setting.
func CurrentAgentKind() (string, error) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return "", fmt.Errorf("HERDR_PANE_ID not set")
	}
	res, err := RunJSON("agent", "get", pane)
	if err != nil {
		return "", err
	}
	kind := str(res, "result", "agent", "agent")
	if kind == "" {
		return "", fmt.Errorf("herdr agent get %s: no agent kind in response", pane)
	}
	return kind, nil
}

// PaneAgentAlive reports whether the pane exists and still runs an agent
// (e.g. pi). A pane whose agent exited — tab left open on a shell — counts
// as gone: there is nothing left to deliver prompts to.
func PaneAgentAlive(paneID string) (bool, error) {
	res, err := RunJSON("pane", "list")
	if err != nil {
		return false, err
	}
	panes, _ := dig(res, "result", "panes").([]any)
	for _, p := range panes {
		if m, ok := p.(map[string]any); ok && m["pane_id"] == paneID {
			agent, _ := m["agent"].(string)
			return agent != "", nil
		}
	}
	return false, nil
}

func Workspaces() ([]map[string]any, error) {
	res, err := RunJSON("workspace", "list")
	if err != nil {
		return nil, err
	}
	list, _ := dig(res, "result", "workspaces").([]any)
	out := make([]map[string]any, 0, len(list))
	for _, w := range list {
		if m, ok := w.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// WorkspaceCloseDetached closes a workspace from a detached process, for the
// same reason as TabCloseDetached.
func WorkspaceCloseDetached(workspaceID string) {
	cmd := exec.Command("sh", "-c", "sleep 1; exec herdr workspace close "+workspaceID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	_ = cmd.Start()
}

func TabCount(workspaceID string) int {
	res, err := RunJSON("tab", "list", "--workspace", workspaceID)
	if err != nil {
		return -1
	}
	list, _ := dig(res, "result", "tabs").([]any)
	return len(list)
}

func WorktreeOpen(workspaceID, path, label string) (string, string, error) {
	args := []string{"worktree", "open", "--path", path, "--label", label}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	res, err := RunJSON(args...)
	if err != nil {
		return "", "", err
	}
	id := str(res, "result", "workspace", "workspace_id")
	if id == "" {
		return "", "", fmt.Errorf("herdr worktree open: no workspace_id in response")
	}
	return id, str(res, "result", "tab", "tab_id"), nil
}
