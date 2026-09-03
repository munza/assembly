package main

import (
	"fmt"
	"os"
	"time"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

type pollState struct {
	comments map[string]int
}

func newWatchCmd() *cobra.Command {
	var (
		interval int
		prs      bool
		project  string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Poll GitHub and report PR events into the mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return fmt.Errorf("--interval must be positive")
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			if !git.GhAvailable() {
				return fmt.Errorf("gh not found in PATH")
			}
			seen := loadSeenCommentCounts()
			for {
				n, err := pollOnce(st, s, seen, project, prs)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				} else if n > 0 && !flagJSON {
					fmt.Printf("%s: %d new event(s) recorded\n", time.Now().Format(time.Kitchen), n)
				}
				time.Sleep(time.Duration(interval) * time.Second)
			}
		},
	}
	cmd.Flags().IntVar(&interval, "interval", 300, "poll interval in seconds")
	cmd.Flags().BoolVar(&prs, "pr", true, "watch PRs (comments, reviews, review requests)")
	cmd.Flags().StringVar(&project, "project", "", "limit to one project")
	return cmd
}

func loadSeenCommentCounts() *pollState {
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

func pollOnce(st *config.Settings, s *store.State, seen *pollState, project string, prs bool) (int, error) {
	events := 0
	var projects []*projView
	if project != "" {
		p, err := resolveProjectView(s, st, project)
		if err != nil {
			return 0, err
		}
		projects = []*projView{p}
	} else {
		projects = sortedProjectViews(s, st)
	}
	for _, p := range projects {
		if prs {
			for _, wt := range store.ProjectWorktrees(s, p.Name) {
				if wt.PR == 0 {
					continue
				}
				v, err := git.PrView(p.Repo, wt.PR, true)
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
			prList, err := git.ReviewRequested(p.Repo)
			if err == nil {
				for _, pr := range prList {
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
