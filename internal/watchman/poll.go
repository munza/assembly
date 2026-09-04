package watchman

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/mux"
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
					for _, c := range comments {
						cm, _ := c.(map[string]any)
						if !isSelfComment(cm["databaseId"], wt.SelfComments) {
							count++
						}
					}
				}
				if reviews, ok := v["reviews"].([]any); ok {
					for _, r := range reviews {
						rm, _ := r.(map[string]any)
						body, _ := rm["body"].(string)
						if strings.TrimSpace(body) != "" {
							count++
						}
					}
				}
				var inline []map[string]any
				if ic, ierr := git.PRReviewComments(repo, wt.PR); ierr == nil {
					for _, c := range ic {
						if !isSelfComment(c["id"], wt.SelfComments) {
							inline = append(inline, c)
						}
					}
					count += len(inline)
				}
				if count > wt.SeenComments {
					n := count - wt.SeenComments
					body := fmt.Sprintf("%d new comment(s)/review(s) on PR #%d", n, wt.PR)
					if detail := commentDetail(v, inline); detail != "" {
						body += "\n" + detail
					}
					appendWorktreeEvent(wt, body)
					events += n
					wt.SeenComments = count
				}
				if updateWorktreeFromPR(s, wt, v) {
					events++
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
		msg := fmt.Sprintf("status %s -> %s (from GitHub)", wt.Status, next)
		if next == store.WtReadyForMerge {
			if who := approvalSummary(v); who != "" {
				msg += " — " + who
			}
		}
		appendWorktreeEvent(wt, msg)
		wt.Status = next
		if state == "MERGED" {
			for _, t := range store.WorktreeTasks(s, wt.Slug) {
				if t.TabID != "" {
					mux.TabCloseDetached(t.TabID)
					t.TabID, t.PaneID, t.AgentName = "", "", ""
				}
				if t.Status != store.TaskDone && t.Status != store.TaskFailed {
					t.Status = store.TaskDone
				}
			}
		}
		return true
	}
	return false
}

func approvalSummary(v map[string]any) string {
	reviews, _ := v["reviews"].([]any)
	latest := map[string]string{}
	for _, r := range reviews {
		rm, _ := r.(map[string]any)
		login := authorLogin(rm)
		state, _ := rm["state"].(string)
		if login != "" && (state == "APPROVED" || state == "CHANGES_REQUESTED") {
			latest[login] = state
		}
	}
	var parts []string
	for login, state := range latest {
		parts = append(parts, fmt.Sprintf("@%s %s", login, strings.ToLower(state)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func isSelfComment(idAny any, self []int) bool {
	id, ok := idAny.(float64)
	if !ok || id == 0 {
		return false
	}
	for _, s := range self {
		if s == int(id) {
			return true
		}
	}
	return false
}

func commentDetail(v map[string]any, inline []map[string]any) string {
	var lines []string
	if comments, ok := v["comments"].([]any); ok {
		for _, c := range comments {
			cm, _ := c.(map[string]any)
			author := authorLogin(cm)
			body, _ := cm["body"].(string)
			if strings.TrimSpace(body) != "" {
				lines = append(lines, fmt.Sprintf("- @%s (comment): %s", author, body))
			}
		}
	}
	if reviews, ok := v["reviews"].([]any); ok {
		for _, r := range reviews {
			rm, _ := r.(map[string]any)
			state, _ := rm["state"].(string)
			body, _ := rm["body"].(string)
			if strings.TrimSpace(body) != "" {
				lines = append(lines, fmt.Sprintf("- @%s (review %s): %s", authorLogin(rm), state, body))
			}
		}
	}
	if inline != nil {
		for _, ic := range inline {
			author := userLogin(ic)
			path, _ := ic["path"].(string)
			loc := path
			if line, ok := ic["line"].(float64); ok && line > 0 {
				loc = fmt.Sprintf("%s:%d", path, int(line))
			} else if line, ok := ic["original_line"].(float64); ok && line > 0 {
				loc = fmt.Sprintf("%s:%d (original)", path, int(line))
			}
			body, _ := ic["body"].(string)
			lines = append(lines, fmt.Sprintf("- @%s on %s: %s", author, loc, body))
		}
	}
	return strings.Join(lines, "\n")
}

func authorLogin(m map[string]any) string {
	author, _ := m["author"].(map[string]any)
	if author == nil {
		return ""
	}
	login, _ := author["login"].(string)
	return login
}

func userLogin(m map[string]any) string {
	user, _ := m["user"].(map[string]any)
	if user == nil {
		return ""
	}
	login, _ := user["login"].(string)
	return login
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
