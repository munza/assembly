package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"assembly/internal/herdr"
	"assembly/internal/linear"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage per-issue git worktrees inside herdr",
}

var (
	wtProject    string
	wtBase       string
	wtStatus     string
	wtForce      bool
	projectPurge bool
)

func init() {
	worktreeAddCmd.Flags().StringVar(&wtProject, "project", "", "project name (defaults to the project containing cwd)")
	worktreeAddCmd.Flags().StringVar(&wtBase, "base", "", "base ref for the new branch (defaults to HEAD)")
	worktreeListCmd.Flags().StringVar(&wtProject, "project", "", "filter by project")
	worktreeUpdateCmd.Flags().StringVar(&wtStatus, "status", "", "new status: "+strings.Join(store.WorktreeStatuses, "|"))
	worktreeRemoveCmd.Flags().BoolVar(&wtForce, "force", false, "remove even if the checkout is dirty")
	projectRemoveCmd.Flags().BoolVar(&projectPurge, "purge", false, "also tear down and delete all its worktrees")
	worktreeCmd.AddCommand(worktreeListCmd, worktreeAddCmd, worktreeGetCmd, worktreeUpdateCmd, worktreeTeardownCmd, worktreeRemoveCmd)
	rootCmd.AddCommand(worktreeCmd)
}

var issueIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		wts := sortedWorktrees(s, wtProject)
		output(wts, func() {
			if len(wts) == 0 {
				fmt.Println("no worktrees")
				return
			}
			for _, wt := range wts {
				pr := "-"
				if wt.PR > 0 {
					pr = fmt.Sprintf("#%d", wt.PR)
				}
				fmt.Printf("%s\t%s\t%s\t%s\t%s\n", wt.Slug, wt.Project, wt.Branch, wt.Status, pr)
			}
		})
		return nil
	},
}

var worktreeAddCmd = &cobra.Command{
	Use:   "add <issue-id|slug>",
	Short: "Create a worktree for an issue or custom slug",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		p, err := resolveProject(s, wtProject)
		if err != nil {
			return err
		}
		ref := args[0]
		slug := strings.ToLower(ref)
		issueID := ""
		if issueIDPattern.MatchString(ref) {
			issueID = strings.ToUpper(ref)
			slug = strings.ToLower(issueID)
		}
		if !isValidSlug(slug) {
			return fmt.Errorf("%q is not a valid worktree slug (lowercase letters, digits, hyphens)", slug)
		}
		if _, ok := s.Worktrees[slug]; ok {
			return fmt.Errorf("worktree %q already exists", slug)
		}
		title := ""
		if issueID != "" {
			if issue, err := linear.GetIssue(issueID); err == nil {
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
			if err := store.Save(s); err != nil {
				return err
			}
		}
		wsID, path, err := herdr.WorktreeCreate(p.WorkspaceID, slug, wtBase)
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

var worktreeGetCmd = &cobra.Command{
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
			fmt.Printf("slug      %s\nproject   %s\nbranch    %s\nstatus    %s\n", wt.Slug, wt.Project, wt.Branch, wt.Status)
			if wt.IssueID != "" {
				fmt.Printf("issue     %s\n", wt.IssueID)
			}
			if wt.PR > 0 {
				fmt.Printf("pr        #%d\n", wt.PR)
			}
			if wt.Path != "" {
				fmt.Printf("path      %s\n", wt.Path)
			}
			for _, t := range tasks {
				fmt.Printf("task      %s %s %s — %s\n", t.ID, t.Type, t.Status, t.Note)
			}
		})
		return nil
	},
}

var worktreeUpdateCmd = &cobra.Command{
	Use:   "update <worktree>",
	Short: "Update worktree status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if wtStatus == "" {
			return fmt.Errorf("--status is required")
		}
		if !store.ValidWorktreeStatus(wtStatus) {
			return fmt.Errorf("invalid status %q; valid: %s", wtStatus, strings.Join(store.WorktreeStatuses, "|"))
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
			fmt.Printf("would set worktree %s status %s -> %s\n", wt.Slug, old, wtStatus)
			return nil
		}
		wt.Status = wtStatus
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("worktree %s status %s -> %s\n", wt.Slug, old, wtStatus)
		return nil
	},
}

var worktreeTeardownCmd = &cobra.Command{
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

var worktreeRemoveCmd = &cobra.Command{
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
		if err := removeWorktree(s, wt, wtForce); err != nil {
			return err
		}
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("removed worktree %s\n", wt.Slug)
		return nil
	},
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

func resolveProject(s *store.State, name string) (*store.Project, error) {
	if name != "" {
		return store.ResolveProject(s, name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, p := range s.Projects {
		if cwd == p.Path || strings.HasPrefix(cwd, p.Path+string(filepath.Separator)) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("cwd is not inside a registered project; pass --project")
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
