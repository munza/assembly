package store

import "time"

const (
	TaskPending    = "pending"
	TaskInProgress = "in-progress"
	TaskSelfReview = "self-review"
	TaskDone       = "done"
	TaskBlocked    = "blocked"
	TaskFailed     = "failed"

	WtPlanning           = "planning"
	WtBuilding           = "building"
	WtPROpen             = "pr-open"
	WtAwaitingReview     = "awaiting-review"
	WtAddressingComments = "addressing-comments"
	WtReadyForMerge      = "ready-for-merge"
	WtDone               = "done"
	WtBlocked            = "blocked"
	WtFailed             = "failed"
)

var TaskStatuses = []string{TaskPending, TaskInProgress, TaskSelfReview, TaskDone, TaskBlocked, TaskFailed}
var WorktreeStatuses = []string{WtPlanning, WtBuilding, WtPROpen, WtAwaitingReview, WtAddressingComments, WtReadyForMerge, WtDone, WtBlocked, WtFailed}

type ProjectState struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type Worktree struct {
	Slug         string `json:"slug"`
	Project      string `json:"project"`
	IssueID      string `json:"issue_id,omitempty"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	RootTabID    string `json:"root_tab_id,omitempty"`
	SeenComments int    `json:"seen_comments,omitempty"`
	SelfComments []int  `json:"self_comments,omitempty"`
	Hold         string `json:"hold,omitempty"`
	Status       string `json:"status"`
	PR           int    `json:"pr,omitempty"`
}

type Task struct {
	ID        string `json:"id"`
	Slug      string `json:"slug,omitempty"`
	Worktree  string `json:"worktree"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Note      string `json:"note,omitempty"`
	NoteKind  string `json:"note_kind,omitempty"`
	TabID     string `json:"tab_id,omitempty"`
	PaneID    string `json:"pane_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

// Pipeline is the half-level cursor for a worktree's gated flow
// (plan → build → pr, with respond re-entering pr, and review for other
// people's PRs). Stage-level truth stays in Tasks; this record only says
// where the flow stands and which output documents it has produced, so any
// half can resume from state instead of re-deriving it.
type Pipeline struct {
	Worktree string    `json:"worktree"`
	IssueID  string    `json:"issue_id,omitempty"`
	Half     string    `json:"half"`
	Reports  []string  `json:"reports,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

// WatchedPR tracks comment/review activity on a PR you reviewed but don't
// own -- no worktree survives pipeline review's CLEANUP to hang this off of,
// so it gets its own minimal record. Removed once the PR is merged or
// closed.
type WatchedPR struct {
	Project      string `json:"project"`
	PR           int    `json:"pr"`
	SeenComments int    `json:"seen_comments,omitempty"`
	SelfComments []int  `json:"self_comments,omitempty"`
}

type Message struct {
	ID       string    `json:"id"`
	TaskID   string    `json:"task_id,omitempty"`
	From     string    `json:"from"`
	Body     string    `json:"body"`
	Status   string    `json:"status,omitempty"`
	Project  string    `json:"project,omitempty"`
	Worktree string    `json:"worktree,omitempty"`
	IssueID  string    `json:"issue_id,omitempty"`
	Label    string    `json:"label,omitempty"`
	Time     time.Time `json:"time"`
	Read     bool      `json:"read"`
}

type State struct {
	Projects   map[string]*ProjectState `json:"projects"`
	Worktrees  map[string]*Worktree     `json:"worktrees"`
	Tasks      map[string]*Task         `json:"tasks"`
	Pipelines  map[string]*Pipeline     `json:"pipelines,omitempty"`
	WatchedPRs map[string]*WatchedPR    `json:"watched_prs,omitempty"`
}
