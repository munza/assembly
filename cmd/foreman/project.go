package main

import (
	"fmt"
	"os"
	"path/filepath"

	"assembly/internal/git"
	"assembly/internal/herdr"
	"assembly/internal/settings"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage registered projects (stored in .assembly/settings.json)",
}

var projectName string

func init() {
	projectCmd.AddCommand(projectListCmd, projectAddCmd, projectGetCmd, projectRemoveCmd)
	projectAddCmd.Flags().StringVar(&projectName, "name", "", "project name (defaults to directory name)")
	rootCmd.AddCommand(projectCmd)
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := settings.Load()
		if err != nil {
			return err
		}
		s, err := store.Load()
		if err != nil {
			return err
		}
		ps := sortedProjectViews(s, st)
		output(ps, func() {
			if len(ps) == 0 {
				fmt.Println("no projects")
				return
			}
			for _, p := range ps {
				fmt.Printf("%s\t%s\t%s\n", p.Name, p.Repo, p.Path)
			}
		})
		return nil
	},
}

var projectAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Register a local repo as a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if !git.IsRepo(path) {
			return fmt.Errorf("%s is not a git repository", path)
		}
		name := projectName
		if name == "" {
			name = filepath.Base(path)
		}
		repo, err := git.Origin(path)
		if err != nil {
			return err
		}
		st, err := settings.Load()
		if err != nil {
			return err
		}
		if _, ok := st.Projects[name]; ok {
			return fmt.Errorf("project %q already exists", name)
		}
		if flagDryRun {
			fmt.Printf("would register project %s (%s) at %s\n", name, repo, path)
			return nil
		}
		st.Projects[name] = &settings.Project{Path: path, Repo: repo}
		if err := settings.Save(st); err != nil {
			return err
		}
		fmt.Printf("registered project %s (%s) at %s\n", name, repo, path)
		return nil
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <project>",
	Short: "Show one project with its worktrees",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := settings.Load()
		if err != nil {
			return err
		}
		s, err := store.Load()
		if err != nil {
			return err
		}
		p, err := resolveProjectView(s, st, args[0])
		if err != nil {
			return err
		}
		wts := store.ProjectWorktrees(s, p.Name)
		type view struct {
			Project   *projView         `json:"project"`
			Worktrees []*store.Worktree `json:"worktrees"`
		}
		output(view{p, wts}, func() {
			fmt.Printf("name       %s\nrepo       %s\npath       %s\nworkspace  %s\n", p.Name, p.Repo, p.Path, p.WorkspaceID)
			for _, wt := range wts {
				fmt.Printf("worktree   %s (%s, %s)\n", wt.Slug, wt.Branch, wt.Status)
			}
		})
		return nil
	},
}

var projectRemoveCmd = &cobra.Command{
	Use:   "remove <project>",
	Short: "Unregister a project (worktrees stay; use --purge to delete them too)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := settings.Load()
		if err != nil {
			return err
		}
		s, err := store.Load()
		if err != nil {
			return err
		}
		p, err := resolveProjectView(s, st, args[0])
		if err != nil {
			return err
		}
		wts := store.ProjectWorktrees(s, p.Name)
		if len(wts) > 0 && !projectPurge {
			return fmt.Errorf("project %s still has %d worktrees; remove them first or use --purge", p.Name, len(wts))
		}
		if flagDryRun {
			fmt.Printf("would unregister project %s\n", p.Name)
			for _, wt := range wts {
				fmt.Printf("would delete worktree %s and %d tasks\n", wt.Slug, len(store.WorktreeTasks(s, wt.Slug)))
			}
			return nil
		}
		for _, wt := range wts {
			if err := removeWorktree(s, wt, true); err != nil {
				return err
			}
		}
		if p.WorkspaceID != "" {
			if err := herdr.WorkspaceClose(p.WorkspaceID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		}
		delete(st.Projects, p.Name)
		delete(s.Projects, p.Name)
		if err := settings.Save(st); err != nil {
			return err
		}
		if err := store.Save(s); err != nil {
			return err
		}
		fmt.Printf("unregistered project %s\n", p.Name)
		return nil
	},
}
