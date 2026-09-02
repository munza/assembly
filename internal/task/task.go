// Package task tracks foreman tasks on disk: one task = one pane.
// A task belongs to an issue worktree (1 herdr worktree per issue) and
// has exactly one labeled pane running a pi agent.
package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"assembly/internal/config"
)

// Types a task can have. The type doubles as the CLI alias.
const (
	TypePlan     = "plan"
	TypeResearch = "research"
	TypeWork     = "work"
	TypeReview   = "review"
)

// ValidTypes is the closed set of task types.
var ValidTypes = []string{TypePlan, TypeResearch, TypeWork, TypeReview}

// Task is one unit of work: one issue, one type, one tab.
type Task struct {
	ID         string    `json:"id"`           // issue id or local tNNN
	Type       string    `json:"type"`         // plan | research | work | review
	Slug       string    `json:"slug"`         // 3-4 words from the title
	Title      string    `json:"title"`
	State      string    `json:"state"`        // picked | working | done | blocked | failed
	Branch     string    `json:"branch"`       // <prefix><id>-<slug>
	WorkspaceID string   `json:"workspace_id"`  // the issue worktree's herdr workspace
	TabID      string    `json:"tab_id"`       // the task's tab in the worktree workspace
	TabLabel   string    `json:"tab_label"`    // <type>-<slug>
	PaneID     string    `json:"pane_id"`      // the tab's pane running pi
	Message    string    `json:"message,omitempty"` // initial prompt, if any
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Dir returns the tasks directory.
func Dir() string { return filepath.Join(config.StateDir(), "tasks") }

// File returns the task's file path: <id>-<type>-<slug>.json.
func (t *Task) File() string {
	return filepath.Join(Dir(), fmt.Sprintf("%s-%s-%s.json", t.ID, t.Type, t.Slug))
}

// Save writes the task file.
func (t *Task) Save() error {
	t.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.File(), b, 0o644)
}

// Load reads all task files.
func Load() ([]*Task, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []*Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir(), e.Name()))
		if err != nil {
			return nil, err
		}
		var t Task
		if err := json.Unmarshal(b, &t); err != nil {
			continue // skip foreign files
		}
		tasks = append(tasks, &t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.Before(tasks[j].CreatedAt) })
	return tasks, nil
}

// Find resolves a task by "<id>", "<id>-<type>", or full filename stem.
func Find(ref string) (*Task, error) {
	tasks, err := Load()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		stem := fmt.Sprintf("%s-%s-%s", t.ID, t.Type, t.Slug)
		switch ref {
		case t.ID, fmt.Sprintf("%s-%s", t.ID, t.Type), stem, t.File():
			return t, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", ref)
}

// OpenForWorktree returns open tasks sharing a worktree workspace.
func OpenForWorkspace(tasks []*Task, workspaceID string) []*Task {
	var out []*Task
	for _, t := range tasks {
		if t.WorkspaceID == workspaceID && t.State != StateDone && t.State != StateFailed {
			out = append(out, t)
		}
	}
	return out
}

// Slug turns a title into a 3-4 word slug.
func Slug(title string) string {
	nonWord := regexp.MustCompile(`[^a-z0-9]+`)
	words := strings.Fields(strings.ToLower(title))
	var kept []string
	for _, w := range words {
		w = strings.Trim(nonWord.ReplaceAllString(w, "-"), "-")
		if w != "" {
			kept = append(kept, w)
		}
	}
	if len(kept) > 4 {
		kept = kept[:4]
	}
	return strings.Join(kept, "-")
}

// NextLocalID returns the next tNNN id based on existing tasks.
func NextLocalID(tasks []*Task) string {
	max := 0
	for _, t := range tasks {
		if strings.HasPrefix(t.ID, "t") {
			if n, err := strconv.Atoi(t.ID[1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("t%03d", max+1)
}

// ValidType reports whether s is a known task type.
func ValidType(s string) bool {
	for _, t := range ValidTypes {
		if s == t {
			return true
		}
	}
	return false
}
