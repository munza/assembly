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

type Project struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Repo        string `json:"repo"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type Worktree struct {
	Slug        string `json:"slug"`
	Project     string `json:"project"`
	IssueID     string `json:"issue_id,omitempty"`
	Branch      string `json:"branch"`
	Path        string `json:"path"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Status      string `json:"status"`
	PR          int    `json:"pr,omitempty"`
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

type Message struct {
	ID     string    `json:"id"`
	TaskID string    `json:"task_id,omitempty"`
	From   string    `json:"from"`
	Body   string    `json:"body"`
	Status string    `json:"status,omitempty"`
	Time   time.Time `json:"time"`
	Read   bool      `json:"read"`
}

type State struct {
	Projects  map[string]*Project  `json:"projects"`
	Worktrees map[string]*Worktree `json:"worktrees"`
	Tasks     map[string]*Task     `json:"tasks"`
	Mailbox   []*Message           `json:"mailbox"`
	NextTask  int                  `json:"next_task"`
}
