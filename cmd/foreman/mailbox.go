package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"assembly/internal/mux"
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

	wait := &cobra.Command{
		Use:   "wait",
		Short: "Block until an unread message arrives, print it, and exit",
		Long: "Block until at least one unread message exists, print them, mark them\n" +
			"read, and exit. Unlike `inbox --follow`, this is one-shot: it exits on\n" +
			"the first delivery. That exit is the point — agent runtimes wake a\n" +
			"follow-up turn only when a background job *completes*, so a job that\n" +
			"never exits never wakes anything. Run this under the background-task\n" +
			"tool and re-arm it after every wake to receive each new message.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return waitMailbox()
		},
	}

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
			if workerSend && sendStatus == store.TaskDone && (t.Type == "plan" || t.Type == "research" || t.Type == "test") && !strings.Contains(args[1], "output/") {
				if wt, werr := store.ResolveWorktree(s, t.Worktree); werr == nil {
					expected := filepath.Join(store.Dir(), "output", reportPrefix(wt)+"-"+taskLabel(t)+".md")
					return fmt.Errorf("done message must mention the report file path (expected `%s`); write the report, then resend including the path", expected)
				}
				return fmt.Errorf("done message must mention the report file path under output/; write the report, then resend including the path")
			}
			if err := checkGateMarkers(t.Type, args[1]); err != nil {
				return err
			}
			m := &store.Message{TaskID: t.ID, From: from, Body: args[1], Status: sendStatus}
			if wt, werr := store.ResolveWorktree(s, t.Worktree); werr == nil {
				m.Project, m.Worktree, m.IssueID = wt.Project, wt.Slug, wt.IssueID
				m.Label = taskLabel(t)
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
			}
			if workerSend && sendStatus == store.TaskDone {
				recordDoneReport(s, t, args[1])
				relayResearchReports(s, t)
			}
			if err := store.Save(s); err != nil {
				return err
			}
			if workerSend && t.TabID != "" {
				// done always closes the tab: the worker is finished. failed closes
				// it only for the report-writing types; build/fix/review/respond
				// failures stay open so the foreman (or user) can inspect the tab.
				closesTab := sendStatus == store.TaskDone ||
					(sendStatus == store.TaskFailed && (t.Type == "research" || t.Type == "plan" || t.Type == "test"))
				if closesTab {
					closeTaskTab(s, t)
				}
			}
			if !workerSend && t.AgentName != "" && t.PaneID != "" {
				if err := mux.AgentPrompt(t.AgentName, args[1]); err != nil {
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
	cmd.AddCommand(inbox, send, wait)
	return cmd
}

// checkGateMarkers enforces the done-message contract the pipeline gates
// parse: a test worker's done must open with a VERDICT: line, a review
// worker's done must close with a FINDINGS: block. The prompt asks for
// these, but workers drift (observed: a review ending "Clean.") — the
// mailbox is where a malformed report bounces instead of silently
// breaking the gate that reads it.
func checkGateMarkers(typ, body string) error {
	if typ == "test" {
		first := body
		if i := strings.IndexAny(first, "\r\n"); i >= 0 {
				first = first[:i]
			}
		first = strings.TrimSpace(first)
		if strings.HasPrefix(first, "VERDICT:") {
				v := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(first, "VERDICT:")))
				if v == "pass" || v == "fail" {
					return nil
				}
			}
		return fmt.Errorf("test done message must start with `VERDICT: pass` or `VERDICT: fail`; resend with the verdict on the first line")
	}
	if typ == "review" {
		lines := strings.Split(body, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if !strings.HasPrefix(line, "FINDINGS:") {
				continue
				}
			rest := strings.TrimSpace(strings.TrimPrefix(line, "FINDINGS:"))
				if strings.EqualFold(rest, "none") {
					return nil
				}
				if rest == "" {
					for _, l := range lines[i+1:] {
						l = strings.TrimSpace(l)
						if len(l) >= 2 && l[0] >= '1' && l[0] <= '9' && (l[1] == '.' || l[1] == ')') {
							return nil
						}
					}
				}
				break
			}
		return fmt.Errorf("review done message must end with `FINDINGS: none` or `FINDINGS:` followed by numbered findings; resend in that shape")
	}
	return nil
}

// recordDoneReport appends the output/ path from a plan/research/test done
// message to the worktree's pipeline record, so report indexing survives
// without the foreman remembering a manual `pipeline report` step. No-op
// when no pipeline is registered (ad-hoc work).
func recordDoneReport(s *store.State, t *store.Task, body string) {
	if t.Type != "plan" && t.Type != "research" && t.Type != "test" {
		return
	}
	p, ok := s.Pipelines[t.Worktree]
	if !ok || p == nil {
		return
	}
	path := ""
	for _, f := range strings.Fields(body) {
		if strings.Contains(f, "output/") {
			path = f
			break
			}
	}
	if path == "" {
		return
	}
	for _, r := range p.Reports {
		if r == path {
				return
			}
	}
	p.Reports = append(p.Reports, path)
	p.Updated = time.Now()
}

// relayResearchReports delivers the collected research report paths to a
// running plan worker the moment the last research task reports done. The
// plan worker's prompt told it to end its turn and wait for exactly this
// message; before this hook, delivery depended on the foreman noticing,
// and the plan tab sat idle until it did.
func relayResearchReports(s *store.State, done *store.Task) {
	if done.Type != "research" {
		return
	}
	for _, t := range store.WorktreeTasks(s, done.Worktree) {
		if t.Type == "research" && t.ID != done.ID && t.Status != store.TaskDone && t.Status != store.TaskFailed {
			return
		}
	}
	p, ok := s.Pipelines[done.Worktree]
	if !ok || p == nil || len(p.Reports) == 0 {
		return
	}
	for _, t := range store.WorktreeTasks(s, done.Worktree) {
		if t.Type != "plan" || t.Status == store.TaskDone || t.Status == store.TaskFailed {
			continue
		}
		if t.AgentName == "" || t.PaneID == "" {
			continue
		}
		msg := "Research done, reports: " + strings.Join(p.Reports, ", ") + " — plan now."
		if err := mux.AgentPrompt(t.AgentName, msg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not relay research reports to plan task %s: %v\n", t.ID, err)
				return
		}
		fmt.Printf("relayed %d report path(s) to waiting plan task %s\n", len(p.Reports), t.ID)
		return
	}
}

func closeTaskTab(s *store.State, t *store.Task) {
	if wt, err := store.ResolveWorktree(s, t.Worktree); err == nil && wt.WorkspaceID != "" && mux.TabCount(wt.WorkspaceID) <= 1 {
		mux.WorkspaceCloseDetached(wt.WorkspaceID)
	} else {
		mux.TabCloseDetached(t.TabID)
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

// waitMailbox is the one-shot counterpart of followMailbox: it returns as
// soon as one unread message batch has been printed, instead of streaming
// forever.
func waitMailbox() error {
	if ms := drainUnread(); len(ms) > 0 {
		return nil
	}
	w, err := watchman.New()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.AddDir(store.MailboxDir()); err != nil {
		return err
	}
	// One more drain after arming the watcher closes the window between the
	// first drain and AddDir where a write could slip through unobserved.
	if ms := drainUnread(); len(ms) > 0 {
		return nil
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		return nil
	case <-w.Events:
		_ = drainUnread()
		return nil
	case err := <-w.Errors:
		return err
	}
}

func drainUnread() []*store.Message {
	ms, err := store.UnreadMessages()
	if err != nil {
		return nil
	}
	for _, m := range ms {
		printMessage(m)
	}
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	_ = store.MarkRead(ids...)
	return ms
}

// printMessage formats a mailbox message the same way watchman used to push
// it into the foreman pane, since that pushed text is now read directly (via
// `mailbox inbox --follow`, typically wrapped in a Monitor) instead.
func printMessage(m *store.Message) {
	head := "github event"
	if m.From != "watch" {
		head = m.From + " " + m.TaskID
		if m.Status != "" {
			head += " [" + m.Status + "]"
		}
	}
	if m.Worktree != "" {
		head += " " + m.Worktree
		var inner []string
		if m.Project != "" {
			inner = append(inner, m.Project)
		}
		if m.IssueID != "" {
			inner = append(inner, m.IssueID)
		}
		if len(inner) > 0 {
			head += " (" + strings.Join(inner, ", ") + ")"
		}
		if m.From != "watch" && m.Label != "" {
			head += " · " + m.Label
		}
	} else if m.From == "watch" && m.TaskID != "" && m.Worktree == "" {
		head += " " + m.TaskID
		if m.Project != "" {
			head += " (" + m.Project + ")"
		}
	}
	body := m.Body
	if len(body) > 4000 {
		body = body[:4000] + "\n...(truncated)"
	}
	fmt.Printf("[watchman] %s:\n%s\n", head, body)
}
