package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"assembly/internal/config"
	"assembly/internal/herdr"
	"assembly/internal/linear"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var issueIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

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
		Use:   "add <issue-id|slug>",
		Short: "Create a worktree for an issue or custom slug",
		Args:  cobra.ExactArgs(1),
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
			slug := strings.ToLower(ref)
			issueID := ""
			if issueIDPattern.MatchString(ref) {
				issueID = strings.ToUpper(ref)
				slug = strings.ToLower(issueID)
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
			if !herdr.Available() {
				return fmt.Errorf("herdr not found in PATH")
			}
			if flagDryRun {
				fmt.Printf("would register worktree %s (project %s, branch %s, status %s)\n", slug, p.Name, slug, store.WtPlanning)
				if p.WorkspaceID == "" {
					fmt.Println("would run: " + planRun("herdr", "workspace", "create", "--cwd", p.Path, "--label", p.Name, "--no-focus"))
				}
				fmt.Println("would run: " + planRun("herdr", "worktree", "create", "--workspace", p.WorkspaceID, "--branch", slug, "--label", slug))
				return nil
			}
			if p.WorkspaceID == "" {
				id, err := herdr.WorkspaceCreate(p.Path, p.Name)
				if err != nil {
					return err
				}
				p.WorkspaceID = id
				if err := setProjectWorkspace(s, p.Name, id); err != nil {
					return err
				}
			}
			wsID, path, err := herdr.WorktreeCreate(p.WorkspaceID, slug, addBase)
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
				kv("Slug", "%s", wt.Slug)
				kv("Project", "%s", wt.Project)
				kv("Branch", "%s", wt.Branch)
				kv("Status", "%s", wt.Status)
				if wt.IssueID != "" {
					kv("Issue", "%s", wt.IssueID)
				}
				if wt.PR > 0 {
					kv("PR", "#%d", wt.PR)
				}
				if wt.Path != "" {
					kv("Path", "%s", wt.Path)
				}
				for _, t := range tasks {
					kv("Task", "%s %s %s — %s", t.ID, t.Type, t.Status, t.Note)
				}
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
					if err := herdr.TabClose(t.TabID); err != nil {
						fmt.Fprintf(os.Stderr, "warning: %v\n", err)
					}
					t.TabID, t.PaneID, t.AgentName = "", "", ""
				}
			}
			if wt.WorkspaceID != "" {
				if err := herdr.WorkspaceClose(wt.WorkspaceID); err != nil {
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
	cmd.AddCommand(list, add, get, update, teardown, remove)
	return cmd
}

func removeWorktree(s *store.State, wt *store.Worktree, force bool) error {
	for _, t := range store.WorktreeTasks(s, wt.Slug) {
		if t.TabID != "" {
			if err := herdr.TabClose(t.TabID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		delete(s.Tasks, t.ID)
	}
	if wt.WorkspaceID != "" {
		if err := herdr.WorktreeRemove(wt.WorkspaceID, true); err != nil {
			if !force {
				return err
			}
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
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
