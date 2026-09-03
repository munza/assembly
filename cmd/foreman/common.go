package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"assembly/internal/settings"
	"assembly/internal/store"
)

type projView struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Repo        string `json:"repo"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

func projectViews(s *store.State, st *settings.Settings) map[string]*projView {
	out := map[string]*projView{}
	for name, p := range st.Projects {
		v := &projView{Name: name, Path: p.Path, Repo: p.Repo}
		if ps, ok := s.Projects[name]; ok && ps != nil {
			v.WorkspaceID = ps.WorkspaceID
		}
		out[name] = v
	}
	return out
}

func sortedProjectViews(s *store.State, st *settings.Settings) []*projView {
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

func resolveProjectView(s *store.State, st *settings.Settings, ref string) (*projView, error) {
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

func resolveTargetProject(s *store.State, st *settings.Settings, name string) (*projView, error) {
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

func setProjectWorkspace(s *store.State, name, id string) error {
	if s.Projects[name] == nil {
		s.Projects[name] = &store.ProjectState{}
	}
	s.Projects[name].WorkspaceID = id
	return store.Save(s)
}

func linearKey() string {
	st, err := settings.Load()
	if err != nil {
		return os.Getenv("LINEAR_API_KEY")
	}
	key := settings.Expand(st.Linear.APIKey)
	if key == "" {
		key = os.Getenv("LINEAR_API_KEY")
	}
	return key
}
