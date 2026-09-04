package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"assembly/internal/config"
	"assembly/internal/mux"
	"assembly/internal/harness"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var taskTypes = []string{"plan", "research", "build", "test", "fix", "review", "respond"}

type taskRow struct {
	ID       string `json:"id"`
	Worktree string `json:"worktree"`
	Held     bool   `json:"held,omitempty"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Note     string `json:"note"`
}

type statusFilter struct {
	status   string
	typ      string
	worktree string
}

func newTaskCmd() *cobra.Command {
	var (
		addSlug, addType, addNote, addWorktree string
		addGeneral, addThread                  bool
		listFilter                             statusFilter
		updateStatus, updateNote               string
	)

	list := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			tasks := filteredTasks(s, listFilter)
			if len(tasks) == 0 {
				fmt.Println("no tasks")
				return nil
			}
			rows := make([]taskRow, len(tasks))
			for i, t := range tasks {
			status := t.Status
			held := false
			if wt, ok := s.Worktrees[t.Worktree]; ok && wt.Hold != "" {
				held = true
				status = t.Status + " (held)"
			}
				rows[i] = taskRow{ID: t.ID, Worktree: t.Worktree, Held: held, Type: t.Type, Status: status, Note: oneLine(t.Note)}
			}
			tableOutput(rows)
			return nil
		},
	}
	list.Flags().StringVar(&listFilter.status, "status", "", "filter by status")
	list.Flags().StringVar(&listFilter.typ, "type", "", "filter by type")
	list.Flags().StringVar(&listFilter.worktree, "worktree", "", "filter by worktree")

	add := &cobra.Command{
		Use:   "add",
		Short: "Create a task",
		RunE: func(cmd *cobra.Command, args []string) error {
			noteKind := ""
			if addThread {
				noteKind = "thread"
			} else if addGeneral {
				noteKind = "general"
			}
			t, err := addTask(addType, addNote, addSlug, addWorktree, noteKind)
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
	add.Flags().StringVar(&addSlug, "slug", "", "short unique slug for the task")
	add.Flags().StringVar(&addType, "type", "", "task type: "+strings.Join(taskTypes, "|"))
	add.Flags().StringVar(&addNote, "note", "", "what the task should do")
	add.Flags().StringVar(&addWorktree, "worktree", "", "target worktree (defaults to the only worktree)")
	add.Flags().BoolVar(&addGeneral, "general", false, "note is a general note")
	add.Flags().BoolVar(&addThread, "thread", false, "note is tied to a review thread")

	get := &cobra.Command{
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

	execute := &cobra.Command{
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
				return fmt.Errorf("task %s already has an agent (tab %s); teardown first", t.ID, tabLabel(t))
			}
			if t.Type == "build" || t.Type == "test" || t.Type == "fix" {
				for _, pt := range store.WorktreeTasks(s, wt.Slug) {
					if pt.Type == "plan" && pt.ID != t.ID && pt.Status != store.TaskDone && pt.Status != store.TaskFailed {
						return fmt.Errorf("plan task %s is %s; %s starts only after plan is done or failed", pt.ID, pt.Status, t.Type)
					}
				}
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
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			harnessName := cfg.Harness
			if harnessName == "" {
				if detected, derr := mux.CurrentAgentKind(); derr == nil {
					harnessName = detected
				}
			}
			h, err := harness.For(harnessName)
			if err != nil {
				return err
			}
			stateDir, err := filepath.Abs(store.Dir())
			if err != nil {
				return err
			}
			prompt := buildPrompt(t, wt, bin, stateDir, pendingResearch(s, wt.Slug))
			env := map[string]string{"FOREMAN_STATE_DIR": stateDir, "FOREMAN_BIN": bin}
			if flagDryRun {
				fmt.Println("would run: " + planRun("herdr", "tab", "create", "--workspace", wt.WorkspaceID, "--label", label, "--no-focus", "--env", "FOREMAN_STATE_DIR="+stateDir, "--env", "FOREMAN_BIN="+bin))
				startArgs := []string{"agent", "start", name, "--kind", h.Kind, "--pane", "<new-pane>"}
				if len(h.Args) > 0 {
					startArgs = append(startArgs, "--")
					startArgs = append(startArgs, h.Args...)
				}
				fmt.Println("would run: " + planRun("herdr", startArgs...))
				fmt.Println("would run: " + planRun("herdr", "agent", "prompt", name, prompt))
				fmt.Printf("would set task %s status %s -> %s\n", t.ID, t.Status, store.TaskInProgress)
				if wt.RootTabID != "" {
					fmt.Println("would run: " + planRun("herdr", "tab", "close", wt.RootTabID))
				}
				return nil
			}
			tabID, paneID, err := mux.TabCreate(wt.WorkspaceID, wt.Path, label, env)
			if err != nil && wt.Path != "" {
				if newID, newRootTab, openErr := mux.WorktreeOpen(wt.Path, wt.Slug); openErr == nil && newID != "" {
					wt.WorkspaceID = newID
					wt.RootTabID = newRootTab
					_ = store.Save(s)
					tabID, paneID, err = mux.TabCreate(wt.WorkspaceID, wt.Path, label, env)
				}
			}
			if err != nil {
				return err
			}
			if err := mux.AgentStart(name, paneID, h.Kind, h.Args...); err != nil {
				_ = mux.TabClose(tabID)
				return err
			}
			if err := mux.AgentPrompt(name, prompt); err != nil {
				return err
			}
			if wt.RootTabID != "" {
				if err := mux.TabClose(wt.RootTabID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not close root tab %s: %v\n", wt.RootTabID, err)
				} else {
					wt.RootTabID = ""
				}
			}
			t.TabID, t.PaneID, t.AgentName = tabID, paneID, name
			t.Status = store.TaskInProgress
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("task %s running as agent %s in tab %s\n", t.ID, name, label)
			return nil
		},
	}

	update := &cobra.Command{
		Use:   "update <task-id>",
		Short: "Update task status or note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if updateStatus == "" && updateNote == "" {
				return fmt.Errorf("nothing to update; pass --status and/or --note")
			}
			if updateStatus != "" && !store.ValidTaskStatus(updateStatus) {
				return fmt.Errorf("invalid status %q; valid: %s", updateStatus, strings.Join(store.TaskStatuses, "|"))
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
				if updateStatus != "" {
					fmt.Printf("would set task %s status %s -> %s\n", t.ID, t.Status, updateStatus)
				}
				if updateNote != "" {
					fmt.Printf("would set task %s note -> %s\n", t.ID, oneLine(updateNote))
				}
				return nil
			}
			if updateStatus != "" {
				t.Status = updateStatus
			}
			if updateNote != "" {
				t.Note = updateNote
			}
			if err := store.Save(s); err != nil {
				return err
			}
			printTask(t)
			return nil
		},
	}
	update.Flags().StringVar(&updateStatus, "status", "", "new status: "+strings.Join(store.TaskStatuses, "|"))
	update.Flags().StringVar(&updateNote, "note", "", "replace the task note")

	teardown := &cobra.Command{
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
			if err := mux.TabClose(t.TabID); err != nil {
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

	remove := &cobra.Command{
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
				if err := mux.TabClose(t.TabID); err != nil {
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

	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks executed by pi agents in worktree tabs",
	}
	cmd.AddCommand(list, add, get, execute, update, teardown, remove)
	return cmd
}

func addTask(typ, note, slug, worktreeRef, noteKind string) (*store.Task, error) {
	if typ == "" {
		return nil, fmt.Errorf("--type is required: %s", strings.Join(taskTypes, "|"))
	}
	if !slices.Contains(taskTypes, typ) {
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

func buildPrompt(t *store.Task, wt *store.Worktree, bin, stateDir string, researchPending []string) string {
	label := taskLabel(t)
	var b strings.Builder
	fmt.Fprintf(&b, "You are worker task %s (%s) in git worktree %q (branch %s).", t.ID, t.Type, wt.Slug, wt.Branch)
	if t.NoteKind != "" {
		fmt.Fprintf(&b, " Note kind: %s.", t.NoteKind)
	}
	fmt.Fprintf(&b, "\nTask: %s\n", t.Note)
	if wt.IssueID != "" {
		fmt.Fprintf(&b, "Issue: %s (run `%s issue get %s` for details).\n", wt.IssueID, bin, wt.IssueID)
	}
	if t.Type == "plan" && len(researchPending) > 0 {
		fmt.Fprintf(&b, "Research tasks %s are still running. Do NOT plan yet and do NOT poll the mailbox. End your turn and wait: their report paths will be delivered into this tab as a new message; plan using them when it arrives.\n", strings.Join(researchPending, ", "))
	}
	if t.Type == "plan" || t.Type == "research" || t.Type == "test" {
		report := filepath.Join(stateDir, "output", reportPrefix(wt)+"-"+label+".md")
		fmt.Fprintf(&b, "When finished, write your full report to `%s` (create the dir), then send ONE final mailbox message that mentions that exact file path with --status done. Your tab closes automatically.\n", report)
	}
	if t.Type == "test" {
		fmt.Fprintf(&b, "You are the test gate. Run the project's test suite as-is; do NOT modify code to make tests pass. Start your done message with the line `VERDICT: pass` or `VERDICT: fail` and put failing output in the report.\n")
	}
	if t.Type == "fix" {
		fmt.Fprintf(&b, "You are the fix stage. Implement exactly the findings in the task note; re-run the tests locally before reporting done.\n")
	}
	if t.Type == "build" || t.Type == "fix" || t.Type == "respond" {
		fmt.Fprintf(&b, "Commit your changes on the worktree branch with a brief plain message before reporting done; never report done with uncommitted changes.\n")
	}
	if t.Type == "review" {
		fmt.Fprintf(&b, "End your done message with `FINDINGS: none` if the work is clean, or `FINDINGS:` followed by one numbered finding per line.\n")
	}
	if t.Type == "plan" || t.Type == "build" || t.Type == "fix" {
		fmt.Fprintf(&b, "You may spawn parallel research when you need answers: `%s research \"<question>\" --worktree %s` then `%s task execute <new-task-id>`. Research reports to the central agent independently.\n", bin, wt.Slug, bin)
	}
	fmt.Fprintf(&b, "Work in the current directory only. Report progress with: %s mailbox send %s \"<summary>\" --status in-progress|self-review|done|blocked|failed\n", bin, t.ID)
	fmt.Fprintf(&b, "If you need user input, send --status blocked with your question in the message body (first line: QUESTION: <text>; then one line per OPTION: <text>). The central agent offers the question to the user and sends the answer back to you.\n")
	return b.String()
}

func taskLabel(t *store.Task) string {
	if t.Slug != "" {
		return t.Slug
	}
	return t.Type + "-" + t.ID
}

func pendingResearch(s *store.State, slug string) []string {
	var ids []string
	for _, t := range store.WorktreeTasks(s, slug) {
		if t.Type == "research" && t.Status != store.TaskDone && t.Status != store.TaskFailed {
			ids = append(ids, t.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func tabLabel(t *store.Task) string {
	if t.Slug != "" {
		return t.Slug
	}
	return t.Type + "-" + t.ID
}

func reportPrefix(wt *store.Worktree) string {
	if wt.IssueID != "" {
		return strings.ToLower(wt.IssueID)
	}
	return wt.Slug
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

var taskGetText = template.Must(template.New("task").Parse(`ID:        {{.ID}}
{{- if .Slug}}
Slug:      {{.Slug}}
{{- end}}
Worktree:  {{.Worktree}}
Type:      {{.Type}}
Status:    {{.Status}}
{{- if .NoteKind}}
NoteKind:  {{.NoteKind}}
{{- end}}
{{- if .TabID}}
Tab:       {{if .Slug}}{{.Slug}}{{else}}{{.Type}}-{{.ID}}{{end}} (agent {{.AgentName}})
{{- end}}
Note:      {{.Note}}
`))

func printTask(t *store.Task) {
	_ = taskGetText.Execute(os.Stdout, t)
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
