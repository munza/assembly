package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const DirName = ".assembly"
const FileName = "state.json"
const MailboxDirName = "mailbox"

func Dir() string {
	if d := os.Getenv("FOREMAN_STATE_DIR"); d != "" {
		return d
	}
	return DirName
}

func Path() string {
	return filepath.Join(Dir(), FileName)
}

func MailboxDir() string {
	return filepath.Join(Dir(), MailboxDirName)
}

func Empty() *State {
	return &State{
		Projects:  map[string]*ProjectState{},
		Worktrees: map[string]*Worktree{},
		Tasks:     map[string]*Task{},
	}
}

func Load() (*State, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	s := Empty()
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", Path(), err)
	}
	if s.Projects == nil {
		s.Projects = map[string]*ProjectState{}
	}
	if s.Worktrees == nil {
		s.Worktrees = map[string]*Worktree{}
	}
	if s.Tasks == nil {
		s.Tasks = map[string]*Task{}
	}
	return s, nil
}

func Save(s *State) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

func NewTaskID(s *State) string {
	max := 0
	for id := range s.Tasks {
		n, err := strconv.Atoi(strings.TrimPrefix(id, "t"))
		if err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("t%d", max+1)
}

func ResolveWorktree(s *State, ref string) (*Worktree, error) {
	if wt, ok := s.Worktrees[ref]; ok {
		return wt, nil
	}
	for _, wt := range s.Worktrees {
		if strings.EqualFold(wt.IssueID, ref) || wt.Branch == ref {
			return wt, nil
		}
	}
	return nil, fmt.Errorf("worktree %q not found", ref)
}

func ResolveTask(s *State, ref string) (*Task, error) {
	if t, ok := s.Tasks[ref]; ok {
		return t, nil
	}
	norm := strings.TrimPrefix(strings.ToLower(ref), "t")
	for _, t := range s.Tasks {
		if strings.TrimPrefix(t.ID, "t") == norm {
			return t, nil
		}
	}
	for _, t := range s.Tasks {
		if t.Slug != "" && strings.EqualFold(t.Slug, ref) {
			return t, nil
		}
	}
	return nil, fmt.Errorf("task %q not found", ref)
}

func WorktreeTasks(s *State, slug string) []*Task {
	var ts []*Task
	for _, t := range s.Tasks {
		if t.Worktree == slug {
			ts = append(ts, t)
		}
	}
	slices.SortFunc(ts, func(a, b *Task) int {
		return strings.Compare(a.ID, b.ID)
	})
	return ts
}

func ProjectWorktrees(s *State, name string) []*Worktree {
	var wts []*Worktree
	for _, wt := range s.Worktrees {
		if wt.Project == name {
			wts = append(wts, wt)
		}
	}
	slices.SortFunc(wts, func(a, b *Worktree) int {
		return strings.Compare(a.Slug, b.Slug)
	})
	return wts
}

func UnreadCount() int {
	ms, err := UnreadMessages()
	if err != nil {
		return 0
	}
	return len(ms)
}

func ValidTaskStatus(v string) bool {
	return slices.Contains(TaskStatuses, v)
}

func ValidWorktreeStatus(v string) bool {
	return slices.Contains(WorktreeStatuses, v)
}
