package watchman

import (
	"fmt"
	"os"
	"sort"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/store"
)

type seenComments map[string]int

func NewSeenComments() seenComments {
	ps := seenComments{}
	ms, err := store.LoadMessages()
	if err != nil {
		return ps
	}
	for _, m := range ms {
		if m.From == "watch" && m.TaskID != "" {
			ps[m.TaskID]++
		}
	}
	return ps
}

// PollGitHub loads fresh state on every call: the daemon outlives many
// state.json writes from other processes and must not clobber them.
func PollGitHub(opts Options, seen seenComments) (int, error) {
	s, err := store.Load()
	if err != nil {
		return 0, err
	}
	st, err := config.Load()
	if err != nil {
		return 0, err
	}
	var names []string
	if opts.Project != "" {
		if _, ok := st.Projects[opts.Project]; !ok {
			return 0, fmt.Errorf("unknown project %q", opts.Project)
		}
		names = []string{opts.Project}
	} else {
		for name := range st.Projects {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	events := 0
	for _, name := range names {
		repo := st.Projects[name].Repo
		if opts.PRs {
			for _, wt := range store.ProjectWorktrees(s, name) {
				if wt.PR == 0 {
					continue
				}
				v, err := git.PrView(repo, wt.PR, true)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
					continue
				}
				count := 0
				if comments, ok := v["comments"].([]any); ok {
					count = len(comments)
				}
				if reviews, ok := v["reviews"].([]any); ok {
					count += len(reviews)
				}
				if count > wt.SeenComments {
					n := count - wt.SeenComments
					appendWorktreeEvent(wt, fmt.Sprintf("%d new comment(s)/review(s) on PR #%d", n, wt.PR))
					events += n
					wt.SeenComments = count
					if updateWorktreeFromPR(s, wt, v) {
						events++
					}
				}
			}
			prList, err := git.ReviewRequested(repo)
			if err == nil {
				for _, pr := range prList {
					num, _ := pr["number"].(float64)
					key := fmt.Sprintf("%s#%d-rr", name, int(num))
					if seen[key] == 0 {
						title, _ := pr["title"].(string)
					appendEvent(name, fmt.Sprintf("review requested: PR #%d %s", int(num), title))
						seen[key] = 1
						events++
					}
				}
			}
		}
	}
	if events > 0 {
		if err := store.Save(s); err != nil {
			return events, err
		}
	}
	return events, nil
}

func updateWorktreeFromPR(s *store.State, wt *store.Worktree, v map[string]any) bool {
	state, _ := v["state"].(string)
	rd, _ := v["reviewDecision"].(string)
	next := wt.Status
	switch {
	case state == "MERGED":
		next = store.WtDone
	case state == "OPEN" && rd == "CHANGES_REQUESTED" && wt.Status != store.WtAddressingComments:
		next = store.WtAddressingComments
	case state == "OPEN" && rd == "APPROVED" && wt.Status != store.WtReadyForMerge:
		next = store.WtReadyForMerge
	case state == "OPEN" && (wt.Status == store.WtPlanning || wt.Status == store.WtBuilding):
		next = store.WtAwaitingReview
	}
	if next != wt.Status {
		appendWorktreeEvent(wt, fmt.Sprintf("status %s -> %s (from GitHub)", wt.Status, next))
		wt.Status = next
		return true
	}
	return false
}

func appendEvent(target, body string) {
	m := &store.Message{From: "watch", TaskID: target, Project: target, Body: body}
	if err := store.AppendMessage(m); err != nil {
		logf("append event: %v", err)
	}
}

func appendWorktreeEvent(wt *store.Worktree, body string) {
	m := &store.Message{From: "watch", TaskID: wt.Slug, Project: wt.Project, Worktree: wt.Slug, IssueID: wt.IssueID, Body: body}
	if err := store.AppendMessage(m); err != nil {
		logf("append event: %v", err)
	}
}
