package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"assembly/internal/github"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var (
	watchInterval int
	watchIssues   bool
	watchPRs      bool
	watchProject  string
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Poll GitHub and report PR events into the mailbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		if watchInterval <= 0 {
			return fmt.Errorf("--interval must be positive")
		}
		s, err := store.Load()
		if err != nil {
			return err
		}
		if !github.Available() {
			return fmt.Errorf("gh not found in PATH")
		}
		seen := loadSeenCommentCounts(s)
		for {
			n, err := pollOnce(s, seen)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else if n > 0 && !flagJSON {
				fmt.Printf("%s: %d new event(s) recorded\n", time.Now().Format(time.Kitchen), n)
			}
			time.Sleep(time.Duration(watchInterval) * time.Second)
		}
	},
}

func init() {
	watchCmd.Flags().IntVar(&watchInterval, "interval", 300, "poll interval in seconds")
	watchCmd.Flags().BoolVar(&watchIssues, "issue", false, "watch Linear issues for updates")
	watchCmd.Flags().BoolVar(&watchPRs, "pr", true, "watch PRs (comments, reviews, review requests)")
	watchCmd.Flags().StringVar(&watchProject, "project", "", "limit to one project")
	rootCmd.AddCommand(watchCmd)
}

type pollState struct {
	comments map[string]int
}

func loadSeenCommentCounts(s *store.State) *pollState {
	ps := &pollState{comments: map[string]int{}}
	ms, err := store.LoadMessages()
	if err != nil {
		return ps
	}
	for _, m := range ms {
		if m.From == "watch" && m.TaskID != "" {
			ps.comments[m.TaskID]++
		}
	}
	return ps
}

func pollOnce(s *store.State, seen *pollState) (int, error) {
	events := 0
	projects := map[string]*store.Project{}
	if watchProject != "" {
		p, err := store.ResolveProject(s, watchProject)
		if err != nil {
			return 0, err
		}
		projects[p.Name] = p
	} else {
		for n, p := range s.Projects {
			projects[n] = p
		}
	}
	for _, p := range sortedProjects(projects) {
		wts := store.ProjectWorktrees(s, p.Name)
		for _, wt := range wts {
			if wt.PR == 0 {
				continue
			}
			v, err := github.PrView(p.Repo, wt.PR, true)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}
			key := fmt.Sprintf("%s#%d", wt.Slug, wt.PR)
			count := 0
			if comments, ok := v["comments"].([]any); ok {
				count = len(comments)
			}
			if reviews, ok := v["reviews"].([]any); ok {
				count += len(reviews)
			}
			if count > seen.comments[key] {
				if n := count - seen.comments[key]; n > 0 {
					body := fmt.Sprintf("%d new comment(s)/review(s) on PR #%d (%s)", n, wt.PR, key)
					appendWatchEvent(wt.Slug, body)
					events += n
				}
				seen.comments[key] = count
				updateWorktreeFromPR(s, wt, v)
			}
		}
		prs, err := github.ReviewRequested(p.Repo)
		if err == nil {
			for _, pr := range prs {
				num, _ := pr["number"].(float64)
				key := fmt.Sprintf("%s#%d-rr", p.Name, int(num))
				if seen.comments[key] == 0 {
					title, _ := pr["title"].(string)
					appendWatchEvent(p.Name, fmt.Sprintf("review requested: PR #%d %s", int(num), title))
					seen.comments[key] = 1
					events++
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

func updateWorktreeFromPR(s *store.State, wt *store.Worktree, v map[string]any) {
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
	case state == "OPEN" && wt.Status == store.WtPlanning || wt.Status == store.WtBuilding:
		next = store.WtAwaitingReview
	}
	if next != wt.Status {
		appendWatchEvent(wt.Slug, fmt.Sprintf("status %s -> %s (from GitHub)", wt.Status, next))
		wt.Status = next
	}
}

func appendWatchEvent(target, body string) {
	m := &store.Message{From: "watch", TaskID: target, Body: body}
	if err := store.AppendMessage(m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
}

func sortedProjects(m map[string]*store.Project) []*store.Project {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*store.Project, 0, len(names))
	for _, n := range names {
		out = append(out, m[n])
	}
	return out
}
