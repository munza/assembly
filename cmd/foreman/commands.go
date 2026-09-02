package main

import (
	"strings"
	"time"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/state"

	"github.com/spf13/cobra"
)

// deps carries everything commands need; loaded once in main, passed down.
type deps struct {
	cfg   *config.Config
	store *state.Store
}

func newRootCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "foreman",
		Short: "Central control for the assembly agent crew",
		Long: `foreman is the single point of control for a crew of pi agents
running in herdr: spawn workers, dispatch tasks, supervise, and report.

Config: .assembly/config.json, overridden by FOREMAN_* env vars.`,
		SilenceUsage: true,
	}
}

func newAgentsCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List tracked agents with live state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgents(d)
		},
	}
}

func newInitCmd(d deps) *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:   "init",
		Short: "Write .assembly/config.json with defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(repo)
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "path to the project repo agents work on (default: cwd)")
	return c
}

func newSpawnCmd(d deps) *cobra.Command {
	var task, model string
	c := &cobra.Command{
		Use:   "spawn NAME",
		Short: "Spawn a pi agent in a new herdr workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpawn(d, args[0], task, model)
		},
	}
	c.Flags().StringVar(&task, "task", "", "task label this agent works on")
	c.Flags().StringVar(&model, "model", "", "pi model spec, e.g. anthropic/claude-sonnet-4-5:high")
	return c
}

func newPromptCmd(d deps) *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	c := &cobra.Command{
		Use:   "prompt NAME TEXT",
		Short: "Send a prompt to an agent",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrompt(args[0], args[1], wait, timeout)
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "wait until agent is done")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "max wait when --wait")
	return c
}

func newReadCmd(d deps) *cobra.Command {
	var lines int
	c := &cobra.Command{
		Use:   "read NAME",
		Short: "Read an agent's terminal output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRead(args[0], lines)
		},
	}
	c.Flags().IntVar(&lines, "lines", 0, "recent lines to read (0 = full snapshot)")
	return c
}

func newWaitCmd(d deps) *cobra.Command {
	var (
		until   string
		timeout time.Duration
	)
	c := &cobra.Command{
		Use:   "wait NAME",
		Short: "Wait for an agent to reach a state (idle|working|blocked|done)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return herdr.AgentWait(args[0], strings.Split(until, ","), timeout)
		},
	}
	c.Flags().StringVar(&until, "until", "done,idle,blocked", "comma-separated states")
	c.Flags().DurationVar(&timeout, "timeout", 0, "max wait (0 = forever)")
	return c
}

func newCloseCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "close NAME",
		Short: "Stop an agent and free its workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClose(d, args[0])
		},
	}
}
