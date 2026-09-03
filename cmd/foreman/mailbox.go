package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
				return fmt.Errorf("invalid status %q; valid: %s", sendStatus, strings.Join(store.TaskStatuses, "|"))
			}
			from := store.SenderLabel(t.PaneID)
			workerSend := from == "worker"
			if workerSend {
				from = t.Type
			}
			parent := ""
			if workerSend {
				parent = t.TabID
			} else if p := os.Getenv("HERDR_PANE_ID"); p != "" {
				parent = p
			}
			if workerSend && sendStatus == store.TaskDone && (t.Type == "plan" || t.Type == "research" || t.Type == "test") && !strings.Contains(args[1], "output/") {
				if wt, werr := store.ResolveWorktree(s, t.Worktree); werr == nil {
					expected := filepath.Join(store.Dir(), "output", reportPrefix(wt)+"-"+taskLabel(t)+".md")
					return fmt.Errorf("done message must mention the report file path (expected `%s`); write the report, then resend including the path", expected)
				}
				return fmt.Errorf("done message must mention the report file path under output/; write the report, then resend including the path")
			}
			m := &store.Message{TaskID: t.ID, From: from, Body: args[1], Status: sendStatus, ParentID: parent}
			if wt, werr := store.ResolveWorktree(s, t.Worktree); werr == nil {
				m.Project, m.Worktree, m.IssueID = wt.Project, wt.Slug, wt.IssueID
				m.TabLabel = tabLabel(t)
			}
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
			if workerSend && t.TabID != "" && (sendStatus == store.TaskDone || sendStatus == store.TaskFailed) && (t.Type == "research" || t.Type == "plan" || t.Type == "test") {
				closeTaskTab(s, t)
			}
			if !workerSend && t.AgentName != "" && t.PaneID != "" {
				if err := herdr.AgentPrompt(t.AgentName, args[1]); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not prompt agent: %v\n", err)
				}
			}
			output(m, func() { fmt.Printf("sent (from %s): %s\n", from, oneLine(args[1])) })
			return nil
		},
	}
	send.Flags().StringVar(&sendStatus, "status", "", "report task status: "+strings.Join(store.TaskStatuses, "|"))

	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Message bus between the foreman and worker agents",
	}
	cmd.AddCommand(inbox, send)
	return cmd
}

func closeTaskTab(s *store.State, t *store.Task) {
	if wt, err := store.ResolveWorktree(s, t.Worktree); err == nil && wt.WorkspaceID != "" && herdr.TabCount(wt.WorkspaceID) <= 1 {
		herdr.WorkspaceCloseDetached(wt.WorkspaceID)
	} else {
		herdr.TabCloseDetached(t.TabID)
	}
	t.TabID, t.PaneID, t.AgentName = "", "", ""
	_ = store.Save(s)
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
	fmt.Printf("%s  %s  task %s%s%s\n  %s\n", m.Time.Local().Format(time.RFC3339), m.From, m.TaskID, status, messageContext(m), m.Body)
}

func messageContext(m *store.Message) string {
	if m.Worktree == "" && m.Project == "" && m.IssueID == "" && m.TabLabel == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("  " + m.Worktree)
	var inner []string
	if m.Project != "" {
		inner = append(inner, m.Project)
	}
	if m.IssueID != "" {
		inner = append(inner, m.IssueID)
	}
	if len(inner) > 0 {
		b.WriteString(" (" + strings.Join(inner, ", ") + ")")
	}
	if m.TabLabel != "" {
		b.WriteString(" tab " + m.TabLabel)
	}
	return b.String()
}
