// Package herdr wraps the herdr CLI: running pi agents in panes,
// prompting them, waiting on their state, and reading their output.
package herdr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// envelope is the common JSON shape herdr CLI emits on stdout.
type envelope struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
}

// run executes `herdr <args...>`, decodes the JSON envelope, and unmarshals
// result into out (when out is non-nil).
func run(out any, args ...string) error {
	cmd := exec.Command("herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		return fmt.Errorf("herdr %v: %w: %s", args, err, truncate(msg, 500))
	}
	if out == nil {
		return nil
	}
	var env envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return fmt.Errorf("herdr %v: bad json: %w", args, err)
	}
	if len(env.Result) == 0 {
		return fmt.Errorf("herdr %v: no result", args)
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("herdr %v: decode result: %w", args, err)
	}
	return nil
}

// runRaw executes `herdr <args...>` and returns raw stdout as text.
// Used for commands that print terminal snapshots instead of JSON.
func runRaw(args ...string) (string, error) {
	cmd := exec.Command("herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		return "", fmt.Errorf("herdr %v: %w: %s", args, err, truncate(msg, 500))
	}
	return stdout.String(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// AgentState describes one detected agent from `herdr agent list`.
type AgentState struct {
	Agent       string `json:"agent"`
	Status      string `json:"agent_status"` // working | blocked | done | idle | unknown
	PaneID      string `json:"pane_id"`
	WorkspaceID string `json:"workspace_id"`
	Cwd         string `json:"cwd"`
	Title       string `json:"terminal_title_stripped"`
}

type agentListResult struct {
	Agents []AgentState `json:"agents"`
}

// ListAgents returns every agent herdr currently sees.
func ListAgents() ([]AgentState, error) {
	var r agentListResult
	if err := run(&r, "agent", "list"); err != nil {
		return nil, err
	}
	return r.Agents, nil
}

// AgentStart launches a supported agent (kind, e.g. "pi") inside a pane.
// Extra args after "--" are passed to the agent binary.
func AgentStart(name, kind, paneID string, agentArgs ...string) error {
	args := append([]string{"agent", "start", name, "--kind", kind, "--pane", paneID}, agentArgs...)
	return run(nil, args...)
}

// AgentPrompt submits a prompt. With wait, it blocks until the agent reaches
// one of the until states (default: idle, done, blocked).
func AgentPrompt(target, text string, wait bool, until []string, timeout time.Duration) error {
	args := []string{"agent", "prompt", target, text}
	if wait {
		args = append(args, "--wait")
		for _, u := range until {
			args = append(args, "--until", u)
		}
		if timeout > 0 {
			args = append(args, "--timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
		}
	}
	return run(nil, args...)
}

// AgentWait blocks until target reaches one of the until states.
func AgentWait(target string, until []string, timeout time.Duration) error {
	args := []string{"agent", "wait", target}
	for _, u := range until {
		args = append(args, "--until", u)
	}
	if timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	}
	return run(nil, args...)
}

// AgentRead returns recent terminal output of an agent (plain text).
func AgentRead(target string, lines int) (string, error) {
	args := []string{"agent", "read", target}
	if lines > 0 {
		args = append(args, "--lines", fmt.Sprintf("%d", lines))
	}
	return runRaw(args...)
}
