package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/orchestrator"
	"assembly/internal/task"
	"assembly/internal/watcher"

	"github.com/spf13/cobra"
)

func main() {
	d := deps{cfg: config.Load()}

	taskCmd := &cobra.Command{Use: "task", Short: "Manage tasks"}
	taskCmd.AddCommand(
		newTaskNewCmd(d, ""),
		newTaskListCmd(d),
		newTaskShowCmd(d),
		newTaskCloseCmd(d),
		newTaskSetCmd(d),
	)

	root := newRootCmd(d)
	root.AddCommand(
		taskCmd,
		newStartCmd(d),
		newTaskNewCmd(d, task.TypePlan),
		newTaskNewCmd(d, task.TypeResearch),
		newTaskNewCmd(d, task.TypeWork),
		newTaskNewCmd(d, task.TypeReview),
		newPromptCmd(d),
		newReadCmd(d),
		newWaitCmd(d),
		newMailCmd(d),
		newWatchCmd(d),
	)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runTaskNew creates the worktree (per issue), pane (per task), and pi agent.
func runTaskNew(d deps, taskType, title, issue, message, model string) error {
	t, err := orchestrator.NewTask(d.cfg, taskType, title, issue, message, model)
	if err != nil {
		return err
	}
	fmt.Printf("task %s-%s-%s\n  tab    %s (%s)\n  pane   %s\n  branch %s\n",
		t.ID, t.Type, t.Slug, t.TabID, t.TabLabel, t.PaneID, t.Branch)
	return nil
}

// runTaskList prints tasks joined with live herdr pane states.
func runTaskList(d deps) error {
	tasks, err := task.Load()
	if err != nil {
		return err
	}
	live := livePaneStates()
	fmt.Printf("%-14s %-9s %-13s %-12s %s\n", "TASK", "TYPE", "STATE", "TAB", "TITLE")
	for _, t := range tasks {
		st := t.State
		if l, ok := live[t.PaneID]; ok {
			st = st + "/" + l
		}
		fmt.Printf("%-14s %-9s %-13s %-12s %s\n",
			t.ID+"-"+t.Type, t.Type, st, t.TabID, t.Title)
	}
	if len(tasks) == 0 {
		fmt.Println("(no tasks; create one: foreman task new \"fix login timeout\")")
	}
	return nil
}

// runTaskShow prints one task as JSON.
func runTaskShow(ref string) error {
	t, err := task.Find(ref)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// runTaskClose closes the pane, marks done, removes the worktree if unused.
func runTaskClose(ref string) error {
	t, err := task.Find(ref)
	if err != nil {
		return err
	}
	if err := orchestrator.CloseTask(t); err != nil {
		return err
	}
	fmt.Printf("closed %s-%s-%s\n", t.ID, t.Type, t.Slug)
	return nil
}

// paneFor resolves a task ref, agent name (e.g. "foreman"), or raw pane id
// to a herdr pane target.
func paneFor(ref string) (string, error) {
	if strings.Contains(ref, ":") { // already a pane id like wE:p2
		return ref, nil
	}
	if t, err := task.Find(ref); err == nil {
		return t.PaneID, nil
	}
	if a := herdr.FindAgentByName(ref); a != nil {
		return a.PaneID, nil
	}
	return "", fmt.Errorf("neither task nor agent: %s", ref)
}

// runPrompt sends text to a task's agent.
func runPrompt(ref, text string, wait bool, timeout time.Duration) error {
	pane, err := paneFor(ref)
	if err != nil {
		return err
	}
	var until []string
	if wait {
		until = []string{"done", "idle", "blocked"}
	}
	return herdr.AgentPrompt(pane, text, wait, until, timeout)
}

// runRead prints a task agent's terminal output.
func runRead(ref string, lines int) error {
	pane, err := paneFor(ref)
	if err != nil {
		return err
	}
	text, err := herdr.AgentRead(pane, lines)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

// runWatch runs the watcher loop (or a single pass with --once).
func runWatch(d deps, once bool) error {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if once {
		return watcher.Once(d.cfg, log)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return watcher.Run(ctx, d.cfg, log)
}

// runTaskSet updates a task's state (used by workers via the CLI).
func runTaskSet(ref, state string) error {
	t, err := task.Find(ref)
	if err != nil {
		return err
	}
	if err := orchestrator.SetState(t, state); err != nil {
		return err
	}
	fmt.Printf("%s -> %s\n", t.Ref(), state)
	return nil
}

// runStart brings up the foreman liaison agent.
func runStart(d deps) error {
	paneID, err := orchestrator.StartForeman(d.cfg)
	if err != nil {
		return err
	}
	fmt.Println("foreman agent running in pane", paneID)
	fmt.Println("(run 'foreman watch' in another terminal to feed it events)")
	return nil
}

// livePaneStates returns herdr agent states keyed by pane id.
func livePaneStates() map[string]string {
	agents, err := herdr.ListAgents()
	if err != nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(agents))
	for _, a := range agents {
		m[a.PaneID] = a.Status
	}
	return m
}
