package main

import (
	"fmt"
	"strings"
	"time"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/mailbox"
	"assembly/internal/task"

	"github.com/spf13/cobra"
)

// deps carries everything commands need; loaded once in main, passed down.
type deps struct {
	cfg *config.Config
}

func newRootCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "foreman",
		Short: "Central control for the assembly agent crew",
		Long: `foreman is the single point of control for a crew of pi agents
running in herdr: one project workspace, one worktree per issue,
one labeled pane per task.

Config: .assembly/config.json, overridden by FOREMAN_* env vars.`,
		SilenceUsage: true,
	}
}

// newTaskNewCmd builds `task new` or a type alias (plan/research/work/review).
func newTaskNewCmd(d deps, aliasType string) *cobra.Command {
	c := &cobra.Command{
		Use:   taskNewUse(aliasType),
		Short: taskNewShort(aliasType),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskType := aliasType
			if aliasType == "" {
				taskType = cmd.Flag("type").Value.String()
			}
			return runTaskNew(d, taskType, strings.Join(args, " "),
				cmd.Flag("issue").Value.String(),
				cmd.Flag("message").Value.String(),
				cmd.Flag("model").Value.String())
		},
	}
	c.Flags().String("type", "work", "task type: "+strings.Join(task.ValidTypes, "|"))
	c.Flags().String("issue", "", "issue id (Linear later); default: generated local id")
	c.Flags().String("message", "", "initial prompt to send to the agent")
	c.Flags().String("model", "", "pi model spec (overrides config)")
	if aliasType != "" {
		c.Flags().MarkHidden("type")
	}
	return c
}

func taskNewUse(aliasType string) string {
	if aliasType == "" {
		return "new TITLE..."
	}
	return aliasType + " TITLE..."
}

func taskNewShort(aliasType string) string {
	if aliasType == "" {
		return "Create a task: worktree per issue, pane per task"
	}
	return "Create a " + aliasType + " task"
}

func newTaskListCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tasks with live pane states",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskList(d)
		},
	}
}

func newTaskShowCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show REF",
		Short: "Show one task (id, id-type, or full name)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskShow(args[0])
		},
	}
}

func newTaskCloseCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "close REF",
		Short: "Close a task; removes the issue worktree when unused",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskClose(args[0])
		},
	}
}

var (
	promptWait    bool
	promptTimeout time.Duration
)

func newPromptCmd(d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "prompt REF TEXT",
		Short: "Send a prompt to a task's agent",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrompt(args[0], args[1], promptWait, promptTimeout)
		},
	}
	c.Flags().BoolVar(&promptWait, "wait", false, "wait until agent is done")
	c.Flags().DurationVar(&promptTimeout, "timeout", 30*time.Minute, "max wait when --wait")
	return c
}

var readLines int

func newReadCmd(d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "read REF",
		Short: "Read a task agent's terminal output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRead(args[0], readLines)
		},
	}
	c.Flags().IntVar(&readLines, "lines", 0, "recent lines to read (0 = full snapshot)")
	return c
}

var (
	waitUntil   string
	waitTimeout time.Duration
)

func newWaitCmd(d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "wait REF",
		Short: "Wait for a task agent to reach a state (idle|working|blocked|done)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return herdr.AgentWait(args[0], strings.Split(waitUntil, ","), waitTimeout)
		},
	}
	c.Flags().StringVar(&waitUntil, "until", "done,idle,blocked", "comma-separated states")
	c.Flags().DurationVar(&waitTimeout, "timeout", 0, "max wait (0 = forever)")
	return c
}

func newMailCmd(d deps) *cobra.Command {
	mail := &cobra.Command{Use: "mail", Short: "On-disk message bus between agents"}
	mail.AddCommand(newMailSendCmd(d), newMailListCmd(d))
	return mail
}

func newMailSendCmd(d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "send BODY",
		Short: "Write a message to a mailbox",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := mailbox.Send(
				cmd.Flag("from").Value.String(),
				cmd.Flag("to").Value.String(),
				cmd.Flag("type").Value.String(),
				strings.Join(args, " "))
			if err != nil {
				return err
			}
			fmt.Printf("sent %s -> %s (%s)\n", m.From, m.To, m.ID)
			return nil
		},
	}
	c.Flags().String("from", "user", "sender")
	c.Flags().String("to", "foreman", "recipient box")
	c.Flags().String("type", mailbox.TypeStatus, "question|result|handoff|status")
	return c
}

func newMailListCmd(d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "list [BOX]",
		Short: "List messages (newest first)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			box := ""
			if len(args) == 1 {
				box = args[0]
			}
			msgs, err := mailbox.List(box)
			if err != nil {
				return err
			}
			for _, m := range msgs {
				fmt.Printf("%-28s %-8s %-6s %s\n", m.ID, m.From, m.Type, firstLine(m.Body, 60))
			}
			if len(msgs) == 0 {
				fmt.Println("(mailbox empty)")
			}
			return nil
		},
	}
	return c
}

func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
