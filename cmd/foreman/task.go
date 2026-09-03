package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"assembly/internal/herdr"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks executed by pi agents in worktree tabs",
}

var (
	taskSlug     string
	taskType     string
	taskNote     string
	taskWorktree string
	taskGeneral  bool
	taskThread   bool
	taskStatus   string
	taskListFilt statusFilter
)

type statusFilter struct {
	status   string
	typ      string
	worktree string
}

var taskTypes = []string{"plan", "research", "build", "review", "respond"}

func init() {
	taskAddCmd.Flags().StringVar(&taskSlug, "slug", "", "short unique slug for the task")
	taskAddCmd.Flags().StringVar(&taskType, "type", "", "task type: "+strings.Join(taskTypes, "|"))
	taskAddCmd.Flags().StringVar(&taskNote, "note", "", "what the task should do")
	taskAddCmd.Flags().StringVar(&taskWorktree, "worktree", "", "target worktree (defaults to the only worktree)")
	taskAddCmd.Flags().BoolVar(&taskGeneral, "general", false, "note is a general note")
	taskAddCmd.Flags().BoolVar(&taskThread, "thread", false, "note is tied to a review thread")
	taskListCmd.Flags().StringVar(&taskListFilt.status, "status", "", "filter by status")
	taskListCmd.Flags().StringVar(&taskListFilt.typ, "type", "", "filter by type")
	taskListCmd.Flags().StringVar(&taskListFilt.worktree, "worktree", "", "filter by worktree")
	taskUpdateCmd.Flags().StringVar(&taskStatus, "status", "", "new status: "+strings.Join(store.TaskStatuses, "|"))
	taskUpdateCmd.Flags().StringVar(&taskNote, "note", "", "replace the task note")
	taskCmd.AddCommand(taskListCmd, taskAddCmd, taskGetCmd, taskExecuteCmd, taskUpdateCmd, taskTeardownCmd, taskRemoveCmd)
	rootCmd.AddCommand(taskCmd)
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		tasks := filteredTasks(s, taskListFilt)
		output(tasks, func() {
			if len(tasks) == 0 {
				fmt.Println("no tasks")
				return
			}
			for _, t := range tasks {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\n", t.ID, t.Worktree, t.Type, t.Status, oneLine(t.Note))
			}
		})
		return nil
	},
}

var taskAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Create a task",
	RunE: func(cmd *cobra.Command, args []string) error {
		noteKind := ""
		if taskThread {
			noteKind = "thread"
		} else if taskGeneral {
			noteKind = "general"
		}
		t, err := addTask(taskType, taskNote, taskSlug, taskWorktree, noteKind)
		if err != nil {
			return err
		}
		if flagDryRun {
			return nil
		}
		output(t, func() {
			fmt.Printf("created task %s (%s) in worktree %s — %s\n", t.ID, t.Type, t.Worktree, oneLine(t.Note))
		})
		return nil
	},
}

var taskGetCmd = &cobra.Command{
	Use:   "get <task-id>",
	Short: "Show one task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		t, err := store.ResolveTask(s, args[0])
		if err != nil {
			return err
		}
		output(t, func() { printTask(t) })
		return nil
	},
}

var taskExecuteCmd = &cobra.Command{
	Use:   "execute <task-id>",
	Short: "Spawn a pi agent in a herdr tab and run the task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		t, err := store.ResolveTask(s, args[0])
		if err != nil {
			return err
		}
		wt, err := store.ResolveWorktree(s, t.Worktree)
		if err != nil {
			return err
		}
		if wt.WorkspaceID == "" {
			return fmt.Errorf("worktree %s has no herdr workspace; recreate it", wt.Slug)
		}
		if t.TabID != "" {
			return fmt.Errorf("task %s already has an agent (tab %s); teardown first", t.ID, t.TabID)
		}
		label := t.Slug
		if label == "" {
			label = t.Type + "-" + t.ID
		}
		name := agentName(label)
		bin := foremanBin()
		if bin == "foreman" {
			fmt.Fprintf(os.Stderr, "note: workers need the foreman binary; build it with: go build -o %s ./cmd/foreman\n", filepath.Join(store.Dir(), "bin", "foreman"))
		}
		prompt := buildPrompt(t, wt, bin)
		stateDir, err := filepath.Abs(store.Dir())
		if err != nil {
			return err
		}
		env := map[string]string{"FOREMAN_STATE_DIR": stateDir, "FOREMAN_BIN": bin}
		if flagDryRun {
			fmt.Println("would run: " + planRun("herdr", "tab", "create", "--workspace", wt.WorkspaceID, "--label", label, "--no-focus", "--env", "FOREMAN_STATE_DIR="+stateDir, "--env", "FOREMAN_BIN="+bin))
			fmt.Println("would run: " + planRun("herdr", "agent", "start", name, "--kind", "pi", "--pane", "<new-pane>"))
			fmt.Println("would run: " + planRun("herdr", "agent", "prompt", name, prompt))
			fmt.Printf("would set task %s status %s -> %s\n", t.ID, t.Status, store.TaskInProgress)
			return nil
		}
		tabID, paneID, err := herdr.TabCreate(wt.WorkspaceID, wt.Path, label, env)
		if err != nil {
			return err
		}
		if err := herdr.AgentStart(name, paneID); err != nil {
			_ = herdr.TabClose(tabID)
			return err
		}
		if err := herdr.AgentPrompt(name, prompt); err != nil {
			return err
		}
		t.TabID, t.PaneID, t.AgentName = tabID, paneID, name
		t.Status = store.TaskInProgress
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("task %s running as agent %s in tab %s\n", t.ID, name, tabID)
		return nil
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <task-id>",
	Short: "Update task status or note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskStatus == "" && taskNote == "" {
			return fmt.Errorf("nothing to update; pass --status and/or --note")
		}
		if taskStatus != "" && !store.ValidTaskStatus(taskStatus) {
			return fmt.Errorf("invalid status %q; valid: %s", taskStatus, strings.Join(store.TaskStatuses, "|"))
		}
		s, err := store.Load()
		if err != nil {
			return err
		}
		t, err := store.ResolveTask(s, args[0])
		if err != nil {
			return err
		}
		if flagDryRun {
			if taskStatus != "" {
				fmt.Printf("would set task %s status %s -> %s\n", t.ID, t.Status, taskStatus)
			}
			if taskNote != "" {
				fmt.Printf("would set task %s note -> %s\n", t.ID, oneLine(taskNote))
			}
			return nil
		}
		if taskStatus != "" {
			t.Status = taskStatus
		}
		if taskNote != "" {
			t.Note = taskNote
		}
		if err := store.Save(s); err != nil {
			return err
		}
		printTask(t)
		return nil
	},
}

var taskTeardownCmd = &cobra.Command{
	Use:   "teardown <task-id>",
	Short: "Stop the task agent and close its tab; keep the task record",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		t, err := store.ResolveTask(s, args[0])
		if err != nil {
			return err
		}
		if t.TabID == "" {
			fmt.Printf("task %s has no running agent\n", t.ID)
			return nil
		}
		if flagDryRun {
			fmt.Println("would run: " + planRun("herdr", "tab", "close", t.TabID))
			return nil
		}
		if err := herdr.TabClose(t.TabID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		t.TabID, t.PaneID, t.AgentName = "", "", ""
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("task %s agent stopped\n", t.ID)
		return nil
	},
}

var taskRemoveCmd = &cobra.Command{
	Use:   "remove <task-id>",
	Short: "Delete a task and close its tab if running",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		t, err := store.ResolveTask(s, args[0])
		if err != nil {
			return err
		}
		if flagDryRun {
			fmt.Printf("would remove task %s\n", t.ID)
			if t.TabID != "" {
				fmt.Println("would run: " + planRun("herdr", "tab", "close", t.TabID))
			}
			return nil
		}
		if t.TabID != "" {
			if err := herdr.TabClose(t.TabID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		delete(s.Tasks, t.ID)
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("removed task %s\n", t.ID)
		return nil
	},
}

func addTask(typ, note, slug, worktreeRef, noteKind string) (*store.Task, error) {
	if typ == "" {
		return nil, fmt.Errorf("--type is required: %s", strings.Join(taskTypes, "|"))
	}
	if !isValidTaskType(typ) {
		return nil, fmt.Errorf("invalid type %q; valid: %s", typ, strings.Join(taskTypes, "|"))
	}
	if note == "" {
		return nil, fmt.Errorf("--note is required")
	}
	s, err := store.Load()
	if err != nil {
		return nil, err
	}
	wt, err := resolveWorktreeForTask(s, worktreeRef)
	if err != nil {
		return nil, err
	}
	if slug != "" {
		for _, t := range s.Tasks {
			if t.Slug == slug {
				return nil, fmt.Errorf("task slug %q already used by %s", slug, t.ID)
			}
		}
	}
	t := &store.Task{
		ID:       store.NewTaskID(s),
		Slug:     slug,
		Worktree: wt.Slug,
		Type:     typ,
		Status:   store.TaskPending,
		Note:     note,
		NoteKind: noteKind,
	}
	if flagDryRun {
		fmt.Printf("would create task %s (%s) in worktree %s — %s\n", t.ID, t.Type, t.Worktree, oneLine(t.Note))
		return t, nil
	}
	s.Tasks[t.ID] = t
	if err := store.Save(s); err != nil {
		return nil, err
	}
	return t, nil
}

func foremanBin() string {
	if b := os.Getenv("FOREMAN_BIN"); b != "" {
		return b
	}
	candidate := filepath.Join(store.Dir(), "bin", "foreman")
	if _, err := os.Stat(candidate); err == nil {
		if abs, err := filepath.Abs(candidate); err == nil {
			return abs
		}
	}
	return "foreman"
}

func resolveWorktreeForTask(s *store.State, ref string) (*store.Worktree, error) {
	if ref != "" {
		return store.ResolveWorktree(s, ref)
	}
	if len(s.Worktrees) == 1 {
		for _, wt := range s.Worktrees {
			return wt, nil
		}
	}
	return nil, fmt.Errorf("pass --worktree (or have exactly one worktree)")
}

func buildPrompt(t *store.Task, wt *store.Worktree, bin string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are worker task %s (%s) in git worktree %q (branch %s).", t.ID, t.Type, wt.Slug, wt.Branch)
	if t.NoteKind != "" {
		fmt.Fprintf(&b, " Note kind: %s.", t.NoteKind)
	}
	fmt.Fprintf(&b, "\nTask: %s\n", t.Note)
	if wt.IssueID != "" {
		fmt.Fprintf(&b, "Issue: %s (run `%s issue get %s` for details).\n", wt.IssueID, bin, wt.IssueID)
	}
	fmt.Fprintf(&b, "Work in the current directory only. Report progress with: %s mailbox send %s \"<summary>\" --status in-progress|self-review|done|blocked|failed\n", bin, t.ID)
	return b.String()
}

func agentName(label string) string {
	name := strings.ToLower(label)
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('-')
		}
	}
	n := b.String()
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "w-" + n
	}
	if len(n) > 32 {
		n = n[:32]
	}
	return n
}

func isValidTaskType(t string) bool {
	for _, v := range taskTypes {
		if v == t {
			return true
		}
	}
	return false
}

func filteredTasks(s *store.State, f statusFilter) []*store.Task {
	var ts []*store.Task
	for _, t := range s.Tasks {
		if f.status != "" && t.Status != f.status {
			continue
		}
		if f.typ != "" && t.Type != f.typ {
			continue
		}
		if f.worktree != "" && t.Worktree != f.worktree {
			continue
		}
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].ID < ts[j].ID })
	return ts
}

func printTask(t *store.Task) {
	fmt.Printf("id       %s\n", t.ID)
	if t.Slug != "" {
		fmt.Printf("slug     %s\n", t.Slug)
	}
	fmt.Printf("worktree %s\n", t.Worktree)
	fmt.Printf("type     %s\n", t.Type)
	fmt.Printf("status   %s\n", t.Status)
	if t.NoteKind != "" {
		fmt.Printf("note-kind %s\n", t.NoteKind)
	}
	if t.TabID != "" {
		fmt.Printf("tab      %s (agent %s)\n", t.TabID, t.AgentName)
	}
	fmt.Printf("note     %s\n", t.Note)
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i] + "…"
	}
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}
