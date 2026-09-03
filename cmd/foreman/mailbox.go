package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"assembly/internal/herdr"
	"assembly/internal/store"
	"assembly/internal/watchman"

	"github.com/spf13/cobra"
)

func newMailboxCmd() *cobra.Command {
	var (
		inboxUnread bool
		inboxFollow bool
		sendStatus  string
	)

	inbox := &cobra.Command{
		Use:   "inbox",
		Short: "Read messages; shown messages are marked read",
		RunE: func(cmd *cobra.Command, args []string) error {
			ms, err := store.LoadMessages()
			if err != nil {
				return err
			}
			shown := ms
			if inboxUnread {
				shown, err = store.UnreadMessages()
				if err != nil {
					return err
				}
			}
			output(shown, func() {
				if len(shown) == 0 {
					fmt.Println("mailbox empty")
					return
				}
				for _, m := range shown {
					printMessage(m)
				}
			})
			if flagDryRun {
				return nil
			}
			ids := make([]string, len(shown))
			for i, m := range shown {
				ids[i] = m.ID
			}
			if err := store.MarkRead(ids...); err != nil {
				return err
			}
			if inboxFollow {
				return followMailbox()
			}
			return nil
		},
	}
	inbox.Flags().BoolVar(&inboxUnread, "unread", false, "show only unread messages")
	inbox.Flags().BoolVar(&inboxFollow, "follow", false, "keep watching for new messages")

	send := &cobra.Command{
		Use:   "send <task-id> <message>",
		Short: "Send a message for a task (workers report to foreman; foreman prompts workers)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			t, err := store.ResolveTask(s, args[0])
			if err != nil {
				return err
			}
			if sendStatus != "" && !store.ValidTaskStatus(sendStatus) {
				return fmt.Errorf("invalid status %q; valid: %s", sendStatus, joinTaskStatuses())
			}
			from := store.SenderLabel(t.PaneID)
			m := &store.Message{TaskID: t.ID, From: from, Body: args[1], Status: sendStatus}
			if flagDryRun {
				fmt.Printf("would record message from %s for task %s: %s\n", from, t.ID, oneLine(args[1]))
				if from == "foreman" && t.AgentName != "" {
					fmt.Println("would run: " + planRun("herdr", "agent", "prompt", t.AgentName, args[1]))
				}
				if sendStatus != "" {
					fmt.Printf("would set task %s status %s -> %s\n", t.ID, t.Status, sendStatus)
				}
				return nil
			}
			if err := store.AppendMessage(m); err != nil {
				return err
			}
			if sendStatus != "" {
				t.Status = sendStatus
				if err := store.Save(s); err != nil {
					return err
				}
			}
			if from == "foreman" && t.AgentName != "" && t.PaneID != "" {
				if err := herdr.AgentPrompt(t.AgentName, args[1]); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not prompt agent: %v\n", err)
				}
			}
			output(m, func() { fmt.Printf("sent (from %s): %s\n", from, oneLine(args[1])) })
			return nil
		},
	}
	send.Flags().StringVar(&sendStatus, "status", "", "report task status: "+joinTaskStatuses())

	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Message bus between the foreman and worker agents",
	}
	cmd.AddCommand(inbox, send)
	return cmd
}

func followMailbox() error {
	fmt.Println("following mailbox for new messages (ctrl+c to stop)...")
	w, err := watchman.New()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.AddDir(store.MailboxDir()); err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-sig:
			return nil
		case <-w.Events:
			ms, err := store.UnreadMessages()
			if err != nil {
				continue
			}
			for _, m := range ms {
				printMessage(m)
			}
			ids := make([]string, len(ms))
			for i, m := range ms {
				ids[i] = m.ID
			}
			_ = store.MarkRead(ids...)
		case err := <-w.Errors:
			return err
		}
	}
}

func printMessage(m *store.Message) {
	status := ""
	if m.Status != "" {
		status = " [" + m.Status + "]"
	}
	fmt.Printf("%s  %s  task %s%s\n  %s\n", m.Time.Local().Format(time.RFC3339), m.From, m.TaskID, status, m.Body)
}

func joinTaskStatuses() string {
	out := ""
	for i, s := range store.TaskStatuses {
		if i > 0 {
			out += "|"
		}
		out += s
	}
	return out
}
