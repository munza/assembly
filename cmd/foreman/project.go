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
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var projectGetText = template.Must(template.New("project").Parse(`Name:        {{.Project.Name}}
Repo:        {{.Project.Repo}}
Path:        {{.Project.Path}}
{{- if .Project.IssuePrefix}}
IssuePrefix: {{.Project.IssuePrefix}}
{{- end}}
Workspace:   {{.Project.WorkspaceID}}
{{- range .Worktrees}}
Worktree:    {{.Slug}} ({{.Branch}}, {{.Status}})
{{- end}}
`))

func newProjectCmd() *cobra.Command {
	var addName string
	var addIssuePrefix string
	var removePurge bool

	list := &cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := config.Load()
			if err != nil {
				return err
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			ps := sortedProjectViews(s, st)
			if len(ps) == 0 {
				fmt.Println("no projects")
				return nil
			}
			rows := make([]projectRow, len(ps))
			for i, p := range ps {
				repo := p.Repo
				if repo != "" {
					repo = "https://github.com/" + repo
				}
				rows[i] = projectRow{Name: p.Name, Repo: repo, Path: p.Path}
			}
			tableOutput(rows)
			return nil
		},
	}

	add := &cobra.Command{
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
			name := addName
			if name == "" {
				name = filepath.Base(path)
			}
			repo, err := git.Origin(path)
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			if _, ok := st.Projects[name]; ok {
				return fmt.Errorf("project %q already exists", name)
			}
			if addIssuePrefix != "" {
				if _, err := regexp.Compile(addIssuePrefix); err != nil {
					return fmt.Errorf("invalid --issue-prefix %q: %v", addIssuePrefix, err)
				}
			}
			if flagDryRun {
				fmt.Printf("would register project %s (%s) at %s\n", name, repo, path)
				if addIssuePrefix != "" {
					fmt.Printf("issue_prefix: %s\n", addIssuePrefix)
				}
				return nil
			}
			st.Projects[name] = &config.Project{Path: path, Repo: repo, IssuePrefix: addIssuePrefix}
			if err := config.Save(st); err != nil {
				return err
			}
			fmt.Printf("registered project %s (%s) at %s\n", name, repo, path)
			return nil
		},
	}
	add.Flags().StringVar(&addName, "name", "", "project name (defaults to directory name)")
	add.Flags().StringVar(&addIssuePrefix, "issue-prefix", "", "regex matching this project's Linear issue IDs, e.g. ^(ENG|TAW)-")

	get := &cobra.Command{
		Use:   "get <project>",
		Short: "Show one project with its worktrees",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := config.Load()
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
				_ = projectGetText.Execute(os.Stdout, view{p, wts})
			})
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <project>",
		Short: "Unregister a project (worktrees stay; use --purge to delete them too)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := config.Load()
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
			if len(wts) > 0 && !removePurge {
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
				if err := mux.WorkspaceClose(p.WorkspaceID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				}
			}
			delete(st.Projects, p.Name)
			delete(s.Projects, p.Name)
			if err := config.Save(st); err != nil {
				return err
			}
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("unregistered project %s\n", p.Name)
			return nil
		},
	}
	remove.Flags().BoolVar(&removePurge, "purge", false, "also tear down and delete all its worktrees")

	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage registered projects (stored in .assembly/settings.json)",
	}
	cmd.AddCommand(list, add, get, remove)
	return cmd
}

type projectRow struct {
	Name string `json:"name"`
	Repo string `json:"repo"`
	Path string `json:"path"`
}

type projView struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Repo        string `json:"repo"`
	IssuePrefix string `json:"issue_prefix,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

func projectViews(s *store.State, st *config.Settings) map[string]*projView {
	out := map[string]*projView{}
	for name, p := range st.Projects {
		v := &projView{Name: name, Path: p.Path, Repo: p.Repo, IssuePrefix: p.IssuePrefix}
		if ps, ok := s.Projects[name]; ok && ps != nil {
			v.WorkspaceID = ps.WorkspaceID
		}
		out[name] = v
	}
	return out
}

func sortedProjectViews(s *store.State, st *config.Settings) []*projView {
	m := projectViews(s, st)
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*projView, 0, len(names))
	for _, n := range names {
		out = append(out, m[n])
	}
	return out
}

func resolveProjectView(s *store.State, st *config.Settings, ref string) (*projView, error) {
	m := projectViews(s, st)
	if v, ok := m[ref]; ok {
		return v, nil
	}
	for _, v := range m {
		if v.Repo == ref || v.Path == ref {
			return v, nil
		}
	}
	for _, v := range m {
		if v.Repo != "" && filepath.Base(v.Repo) == ref {
			return v, nil
		}
	}
	return nil, fmt.Errorf("project %q not found", ref)
}

func resolveTargetProject(s *store.State, st *config.Settings, name string) (*projView, error) {
	if name != "" {
		return resolveProjectView(s, st, name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, v := range projectViews(s, st) {
		if cwd == v.Path || strings.HasPrefix(cwd, v.Path+string(filepath.Separator)) {
			return v, nil
		}
	}
	return nil, fmt.Errorf("cwd is not inside a registered project; pass --project")
}

func projectsByIssuePrefix(st *config.Settings, issueID string) ([]string, error) {
	var names []string
	for name, p := range st.Projects {
		if p.IssuePrefix == "" {
			continue
		}
		re, err := regexp.Compile(p.IssuePrefix)
		if err != nil {
			return nil, fmt.Errorf("project %s has invalid issue_prefix %q: %v", name, p.IssuePrefix, err)
		}
		if re.MatchString(issueID) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func checkIssuePrefix(st *config.Settings, p *projView, issueID string) error {
	proj, ok := st.Projects[p.Name]
	if !ok || proj.IssuePrefix == "" {
		return nil
	}
	re, err := regexp.Compile(proj.IssuePrefix)
	if err != nil {
		return fmt.Errorf("project %s has invalid issue_prefix %q: %v", p.Name, proj.IssuePrefix, err)
	}
	if !re.MatchString(issueID) {
		return fmt.Errorf("issue %s does not match project %s issue_prefix %q", issueID, p.Name, proj.IssuePrefix)
	}
	return nil
}

func setProjectWorkspace(s *store.State, name, id string) error {
	if s.Projects[name] == nil {
		s.Projects[name] = &store.ProjectState{}
	}
	s.Projects[name].WorkspaceID = id
	return store.Save(s)
}
