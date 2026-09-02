package task

// Full task lifecycle states. Workers advance through them via
// `foreman task set --state`.
const (
	StatePicked            = "picked"
	StatePlanning          = "planning"
	StateCoding            = "coding"
	StateSelfReview        = "self-review"
	StatePROpen            = "pr-open"
	StateAwaitingReview    = "awaiting-review"
	StateAddressingCommnts = "addressing-comments"
	StateWorking           = "working"
	StateDone              = "done"
	StateBlocked           = "blocked"
	StateFailed            = "failed"
)

// ValidStates is the closed set of task states.
var ValidStates = []string{
	StatePicked, StatePlanning, StateCoding, StateSelfReview,
	StatePROpen, StateAwaitingReview, StateAddressingCommnts,
	StateWorking, StateDone, StateBlocked, StateFailed,
}

// ValidState reports whether s is a known state.
func ValidState(s string) bool {
	for _, v := range ValidStates {
		if s == v {
			return true
		}
	}
	return false
}

// Ref is the "<id>-<type>" short reference used in prompts and commands.
func (t *Task) Ref() string { return t.ID + "-" + t.Type }

// Prompt renders the initial message for the task's agent. foremanBin is
// the absolute path to the foreman CLI workers should call.
func (t *Task) Prompt(foremanBin string) string {
	return "You are a " + t.Type + " agent for assembly. Follow the worker skill.\n" +
		"task: " + t.Ref() + " — " + t.Title + "\n" +
		"issue: " + t.ID + "\n" +
		"branch: " + t.Branch + " (you are already in its worktree)\n" +
		"foreman CLI: " + foremanBin + "\n" +
		"When finished or blocked, always mail foreman and set your task state."
}
