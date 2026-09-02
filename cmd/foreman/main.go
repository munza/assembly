package main

import (
	"fmt"
	"os"
	"time"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/orchestrator"
	"assembly/internal/state"
)

func main() {
	d := deps{cfg: config.Load(), store: state.Load()}
	root := newRootCmd(d)
	root.AddCommand(
		newAgentsCmd(d),
		newCloseCmd(d),
		newInitCmd(d),
		newPromptCmd(d),
		newReadCmd(d),
		newSpawnCmd(d),
		newWaitCmd(d),
	)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runInit writes a default config file for the given (or current) repo.
func runInit(repo string) error {
	if repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repo = cwd
	}
	if err := config.Default(repo).Save(); err != nil {
		return err
	}
	fmt.Println("wrote", config.ConfigPath())
	return nil
}

// runSpawn creates a pi agent workspace and registers it.
func runSpawn(d deps, name, task, model string) error {
	a, err := orchestrator.Spawn(d.cfg, d.store, name, task, model)
	if err != nil {
		return err
	}
	fmt.Printf("spawned %s (pane %s, workspace %s)\n", a.Name, a.PaneID, a.WorkspaceID)
	return nil
}

// runPrompt sends text to an agent, optionally blocking until done.
func runPrompt(name, text string, wait bool, timeout time.Duration) error {
	var until []string
	if wait {
		until = []string{"done", "idle", "blocked"}
	}
	return herdr.AgentPrompt(name, text, wait, until, timeout)
}

// runRead prints an agent's terminal output.
func runRead(name string, lines int) error {
	text, err := herdr.AgentRead(name, lines)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

// runAgents prints the agent table: registry joined with live herdr state.
func runAgents(d deps) error {
	live, err := herdr.ListAgents()
	if err != nil {
		return err
	}
	byPane := func(states []herdr.AgentState) map[string]herdr.AgentState {
		m := make(map[string]herdr.AgentState, len(states))
		for _, s := range states {
			m[s.PaneID] = s
		}
		return m
	}(live)

	fmt.Printf("%-16s %-10s %-8s %s\n", "NAME", "STATE", "PANE", "TASK")
	for _, a := range d.store.Agents {
		st := "gone"
		if l, ok := byPane[a.PaneID]; ok {
			st = l.Status
		}
		fmt.Printf("%-16s %-10s %-8s %s\n", a.Name, st, a.PaneID, a.Task)
	}
	if len(d.store.Agents) == 0 {
		fmt.Println("(no agents; spawn one: foreman spawn builder-1 --task fix-bug)")
	}
	return nil
}

// runClose stops an agent and frees its workspace.
func runClose(d deps, name string) error {
	if err := orchestrator.Close(d.store, name); err != nil {
		return err
	}
	fmt.Println("closed", name)
	return nil
}
