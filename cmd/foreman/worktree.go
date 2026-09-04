package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/mux"
	"assembly/internal/linear"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var worktreeGetText = template.Must(template.New("worktree").Parse(`Slug:      {{.Worktree.Slug}}
Project:   {{.Worktree.Project}}
Branch:    {{.Worktree.Branch}}
Status:    {{.Worktree.Status}}
{{- if .Worktree.IssueID}}
Issue:     {{.Worktree.IssueID}}
{{- end}}
{{- if .Worktree.PR}}
PR:        #{{.Worktree.PR}}
{{- end}}
{{- if .Worktree.Path}}
Path:      {{.Worktree.Path}}
{{- end}}
{{- range .Tasks}}
Task:      {{.ID}} {{.Type}} {{.Status}} — {{.Note}}
{{- end}}
`))

var issueIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)
var prRefPattern = regexp.MustCompile(`^pr-[0-9]+$`)

type worktreeRow struct {
	Slug    string `json:"slug"`
	Project string `json:"project"`
	Branch  string `json:"branch"`
	Status  string `json:"status"`
	PR      string `json:"pr"`
}

func newWorktreeCmd() *cobra.Command {
	var addProject, addBase string
	var listProject, updateStatus string
	var removeForce bool

	list := &cobra.Command{
		Use:   "list",
		Short: "List worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wts := sortedWorktrees(s, listProject)
			if len(wts) == 0 {
				fmt.Println("no worktrees")
				return nil
			}
			rows := make([]worktreeRow, len(wts))
			for i, wt := range wts {
				pr := "-"
				if wt.PR > 0 {
					pr = fmt.Sprintf("#%d", wt.PR)
				}
				rows[i] = worktreeRow{Slug: wt.Slug, Project: wt.Project, Branch: wt.Branch, Status: wt.Status, PR: pr}
			}
			tableOutput(rows)
			return nil
		},
	}
	list.Flags().StringVar(&listProject, "project", "", "filter by project")

	add := &cobra.Command{
		Use:   "add <issue-id|slug> [words...]",
		Short: "Create a worktree for an issue or custom slug",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			var p *projView
			if addProject != "" {
				p, err = resolveProjectView(s, st, addProject)
				if err != nil {
					return err
				}
			} else {
				matched, err := projectsByIssuePrefix(st, args[0])
				if err != nil {
					return err
				}
				if len(matched) == 1 {
					p, err = resolveProjectView(s, st, matched[0])
					if err != nil {
						return err
					}
				} else if len(matched) > 1 {
					return fmt.Errorf("issue %s matches issue_prefix of multiple projects: %s; pass --project", args[0], strings.Join(matched, ", "))
				} else {
					p, err = resolveTargetProject(s, st, "")
					if err != nil {
						return err
					}
				}
			}
			ref := args[0]
			slug := strings.ToLower(strings.Join(args, "-"))
			issueID := ""
			if issueIDPattern.MatchString(ref) && !prRefPattern.MatchString(strings.ToLower(ref)) {
				issueID = strings.ToUpper(ref)
				if err := checkIssuePrefix(st, p, issueID); err != nil {
					return err
				}
			}
			if !isValidSlug(slug) {
				return fmt.Errorf("%q is not a valid worktree slug (lowercase letters, digits, hyphens)", slug)
			}
			if _, ok := s.Worktrees[slug]; ok {
				return fmt.Errorf("worktree %q already exists", slug)
			}
			title := ""
			if issueID != "" {
				if issue, err := linear.GetIssue(issueID, config.LinearAPIKey()); err == nil {
					title = issue.Title
				} else {
					fmt.Fprintf(os.Stderr, "warning: could not fetch issue %s: %v\n", issueID, err)
				}
			}
			if !mux.Available() {
				return fmt.Errorf("herdr not found in PATH")
			}
			if p.WorkspaceID == "" {
				if id := findWorkspaceByRoot(p.Path); id != "" {
					fmt.Printf("using existing workspace %s for project %s\n", id, p.Name)
					p.WorkspaceID = id
					if !flagDryRun {
						if err := setProjectWorkspace(s, p.Name, id); err != nil {
							return err
						}
					}
				}
			}
			if p.WorkspaceID == "" {
				if flagDryRun {
					fmt.Println("would run: " + planRun("herdr", "workspace", "create", "--cwd", p.Path, "--label", p.Name, "--no-focus"))
					return nil
				}
				id, err := mux.WorkspaceCreate(p.Path, p.Name)
				if err != nil {
					return err
				}
				p.WorkspaceID = id
				if err := setProjectWorkspace(s, p.Name, id); err != nil {
					return err
				}
			}
			if flagDryRun {
				fmt.Printf("would register worktree %s (project %s, branch %s, status %s)\n", slug, p.Name, slug, store.WtPlanning)
				fmt.Println("would run: " + planRun("herdr", "worktree", "create", "--workspace", p.WorkspaceID, "--branch", slug, "--label", slug))
				return nil
			}
			wsID, path, rootTabID, err := mux.WorktreeCreate(p.WorkspaceID, slug, addBase)
			if err != nil {
				return err
			}
			wt := &store.Worktree{
				Slug:        slug,
				Project:     p.Name,
				IssueID:     issueID,
				Branch:      slug,
				Path:        path,
				WorkspaceID: wsID,
				RootTabID:   rootTabID,
				Status:      store.WtPlanning,
			}
			s.Worktrees[slug] = wt
			if err := store.Save(s); err != nil {
				return err
			}
			output(wt, func() {
				fmt.Printf("created worktree %s (branch %s) in workspace %s\n", slug, slug, wsID)
				if title != "" {
					fmt.Printf("issue: %s %s\n", issueID, title)
				}
				if path != "" {
					fmt.Printf("path: %s\n", path)
				}
			})
			return nil
		},
	}
	add.Flags().StringVar(&addProject, "project", "", "project name (defaults to the project containing cwd)")
	add.Flags().StringVar(&addBase, "base", "", "base ref for the new branch (defaults to HEAD)")

	get := &cobra.Command{
		Use:   "get <worktree>",
		Short: "Show one worktree and its tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			tasks := store.WorktreeTasks(s, wt.Slug)
			type view struct {
				Worktree *store.Worktree `json:"worktree"`
				Tasks    []*store.Task   `json:"tasks"`
			}
			output(view{wt, tasks}, func() {
				_ = worktreeGetText.Execute(os.Stdout, view{wt, tasks})
			})
			return nil
		},
	}

	update := &cobra.Command{
		Use:   "update <worktree>",
		Short: "Update worktree status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if updateStatus == "" {
				return fmt.Errorf("--status is required")
			}
			if !store.ValidWorktreeStatus(updateStatus) {
				return fmt.Errorf("invalid status %q; valid: %s", updateStatus, strings.Join(store.WorktreeStatuses, "|"))
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			old := wt.Status
			if flagDryRun {
				fmt.Printf("would set worktree %s status %s -> %s\n", wt.Slug, old, updateStatus)
				return nil
			}
			wt.Status = updateStatus
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("worktree %s status %s -> %s\n", wt.Slug, old, updateStatus)
			return nil
		},
	}
	update.Flags().StringVar(&updateStatus, "status", "", "new status: "+strings.Join(store.WorktreeStatuses, "|"))

	var holdNote string
	hold := &cobra.Command{
		Use:   "hold <worktree> --note <text>",
		Short: "Record a paused pipeline decision or step on a worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(holdNote) == "" {
				return fmt.Errorf("--note is required: what is pending (question, options, context)")
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			if flagDryRun {
				fmt.Printf("would hold worktree %s: %s\n", wt.Slug, oneLine(holdNote))
				return nil
			}
			wt.Hold = holdNote
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("worktree %s on hold: %s\n", wt.Slug, oneLine(holdNote))
			return nil
		},
	}
	hold.Flags().StringVar(&holdNote, "note", "", "what is pending")

	resume := &cobra.Command{
		Use:   "resume <worktree>",
		Short: "Show and clear a worktree's hold (the paused step to redo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			if wt.Hold == "" {
				fmt.Printf("worktree %s has no hold\n", wt.Slug)
				return nil
			}
			if flagDryRun {
				fmt.Printf("would clear hold on worktree %s: %s\n", wt.Slug, oneLine(wt.Hold))
				return nil
			}
			note := wt.Hold
			wt.Hold = ""
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("resume worktree %s: %s\n", wt.Slug, note)
			return nil
		},
	}

	teardown := &cobra.Command{
		Use:   "teardown <worktree>",
		Short: "Stop task agents and close tabs; keep the worktree checkout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			tasks := store.WorktreeTasks(s, wt.Slug)
			if flagDryRun {
				for _, t := range tasks {
					if t.TabID != "" {
						fmt.Println("would run: " + planRun("herdr", "tab", "close", t.TabID))
					}
				}
				if wt.WorkspaceID != "" {
					fmt.Println("would run: " + planRun("herdr", "workspace", "close", wt.WorkspaceID))
				}
				return nil
			}
			for _, t := range tasks {
				if t.TabID != "" {
					if err := mux.TabClose(t.TabID); err != nil {
						fmt.Fprintf(os.Stderr, "warning: %v\n", err)
					}
					t.TabID, t.PaneID, t.AgentName = "", "", ""
				}
			}
			if wt.WorkspaceID != "" {
				if err := mux.WorkspaceClose(wt.WorkspaceID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				}
			}
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("torn down worktree %s (checkout kept)\n", wt.Slug)
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <worktree>",
		Short: "Delete the worktree, its checkout, and its tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			if flagDryRun {
				fmt.Printf("would remove worktree %s and %d tasks\n", wt.Slug, len(store.WorktreeTasks(s, wt.Slug)))
				if wt.WorkspaceID != "" {
					fmt.Println("would run: " + planRun("herdr", "worktree", "remove", "--workspace", wt.WorkspaceID, "--force"))
				}
				return nil
			}
			if err := removeWorktree(s, wt, removeForce); err != nil {
				return err
			}
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("removed worktree %s\n", wt.Slug)
			return nil
		},
	}
	remove.Flags().BoolVar(&removeForce, "force", false, "remove even if the checkout is dirty")

	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage per-issue git worktrees inside herdr",
	}
	cmd.AddCommand(list, add, get, update, hold, resume, teardown, remove)
	return cmd
}

func newResumeTopCmd() *cobra.Command {
	var refTask, refWorktree string
	c := &cobra.Command{
		Use:   "resume [--task <id> | --worktree <slug>]",
		Short: "Resume a held pipeline step: by task, worktree, or the only hold",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("pass --task <id> or --worktree <slug>, not a positional argument")
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			slug := ""
			if refTask != "" {
				t, terr := store.ResolveTask(s, refTask)
				if terr != nil {
				return terr
			}
			slug = t.Worktree
		} else if refWorktree != "" {
			if _, werr := store.ResolveWorktree(s, refWorktree); werr != nil {
				return werr
			}
			slug = refWorktree
		} else {
				var held []string
				for _, wt := range s.Worktrees {
					if wt.Hold != "" {
						held = append(held, wt.Slug)
					}
				}
				sort.Strings(held)
				if len(held) == 0 {
					fmt.Println("nothing on hold")
					return nil
				}
				if len(held) > 1 {
					fmt.Printf("multiple holds; pick one: %s\n", strings.Join(held, ", "))
					return nil
				}
				slug = held[0]
			}
			wt, err := store.ResolveWorktree(s, slug)
			if err != nil {
				return err
			}
			if wt.Hold == "" {
				fmt.Printf("worktree %s has no hold\n", wt.Slug)
				return nil
			}
			if flagDryRun {
				fmt.Printf("would clear hold on worktree %s: %s\n", wt.Slug, oneLine(wt.Hold))
				return nil
			}
			note := wt.Hold
			wt.Hold = ""
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("resume worktree %s: %s\n", wt.Slug, note)
			return nil
		},
	}
	c.Flags().StringVar(&refTask, "task", "", "resume via a task id (its worktree's hold)")
	c.Flags().StringVar(&refWorktree, "worktree", "", "resume a worktree's hold")
	return c
}

func findWorkspaceByRoot(path string) string {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		target = path
	}
	wss, err := mux.Workspaces()
	if err != nil {
		return ""
	}
	for _, ws := range wss {
		wt, _ := ws["worktree"].(map[string]any)
		if wt == nil {
			continue
		}
		if linked, _ := wt["is_linked_worktree"].(bool); linked {
			continue
		}
		root, _ := wt["repo_root"].(string)
		if root == "" {
			root, _ = wt["checkout_path"].(string)
		}
		if root == "" {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if root == target {
			if id, _ := ws["workspace_id"].(string); id != "" {
				return id
			}
		}
	}
	return ""
}

func removeWorktree(s *store.State, wt *store.Worktree, force bool) error {
	for _, t := range store.WorktreeTasks(s, wt.Slug) {
		if t.TabID != "" {
			if err := mux.TabClose(t.TabID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		delete(s.Tasks, t.ID)
	}
	if n, err := store.DeleteWorktreeMessages(wt.Slug); err == nil && n > 0 {
		fmt.Printf("purged %d mailbox message(s)\n", n)
	}
	prefixes := []string{wt.Slug}
	if wt.IssueID != "" {
		prefixes = append(prefixes, strings.ToLower(wt.IssueID))
	}
	for _, p := range prefixes {
		matches, _ := filepath.Glob(filepath.Join(store.Dir(), "output", p+"-*.md"))
		for _, f := range matches {
			if err := os.Remove(f); err == nil {
				fmt.Printf("deleted report %s\n", filepath.Base(f))
			}
		}
	}
	if wt.WorkspaceID != "" {
		if err := mux.WorktreeRemove(wt.WorkspaceID, true); err != nil {
			if !strings.Contains(err.Error(), "workspace_not_found") {
				if !force {
					return err
				}
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else if wt.Path != "" {
				if cerr := git.RemoveWorktreeCheckout(wt.Path); cerr != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", cerr)
				}
			}
		}
	}
	delete(s.Worktrees, wt.Slug)
	return nil
}

func isValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func sortedWorktrees(s *store.State, project string) []*store.Worktree {
	var wts []*store.Worktree
	for _, wt := range s.Worktrees {
		if project == "" || wt.Project == project {
			wts = append(wts, wt)
		}
	}
	sort.Slice(wts, func(i, j int) bool { return wts[i].Slug < wts[j].Slug })
	return wts
}
