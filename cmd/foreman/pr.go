package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/issue"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

func newPRCmd() *cobra.Command {
	var (
		createTitle, createBase string
		createNoTemplate        bool
		getComments             bool
	)

	create := &cobra.Command{
		Use:   "create <worktree>",
		Short: "Open a PR for a worktree's branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			p, err := resolveProjectView(s, st, wt.Project)
			if err != nil {
				return err
			}
			title := createTitle
			body := ""
			if title == "" && wt.IssueID != "" {
				if issue, err := issue.GetIssue(wt.IssueID, config.LinearAPIKey()); err == nil {
					title = issue.Title
					body = fmt.Sprintf("[%s](%s)\n\n%s", issue.Identifier, issue.URL, issue.Description)
				} else {
					fmt.Printf("warning: could not fetch issue %s: %v\n", wt.IssueID, err)
				}
			}
			if title == "" {
				title = wt.Branch
			}
			tmplPath, tmpl := "", ""
			if !createNoTemplate {
				tmplPath, tmpl = git.PRTemplate(wt.Path)
			}
			if tmpl != "" && body != "" {
				body = body + "\n\n" + tmpl
			} else if tmpl != "" {
				body = tmpl
			}
			if !git.GhAvailable() {
				return fmt.Errorf("gh not found in PATH")
			}
			if wt.Path == "" {
				return fmt.Errorf("worktree %s has no recorded path; cannot push its branch", wt.Slug)
			}
			if flagDryRun {
				fmt.Println("would run: git -C " + wt.Path + " push -u origin " + wt.Branch)
				if tmplPath != "" {
					fmt.Println("would use PR template: " + tmplPath)
				}
				ghArgs := []string{"gh", "pr", "create", "--title", title, "--head", wt.Branch, "--repo", p.Repo}
				if createBase != "" {
					ghArgs = append(ghArgs, "--base", createBase)
				}
				fmt.Println("would run: " + quoteAll(ghArgs...))
				switch wt.Status {
				case store.WtPlanning, store.WtBuilding, store.WtBlocked, store.WtFailed:
					fmt.Printf("would set worktree %s status %s -> %s\n", wt.Slug, wt.Status, store.WtPROpen)
				}
				return nil
			}
			if err := git.Push(wt.Path, wt.Branch); err != nil {
				return err
			}
			url, existed, err := git.PrCreate(wt.Path, p.Repo, title, body, createBase, wt.Branch)
			if err != nil {
				return err
			}
			prNum := 0
			if i := strings.LastIndex(url, "/"); i >= 0 {
				prNum, _ = strconv.Atoi(url[i+1:])
			}
			wt.PR = prNum
			switch wt.Status {
			case store.WtPlanning, store.WtBuilding, store.WtBlocked, store.WtFailed:
				fmt.Printf("worktree %s status %s -> %s\n", wt.Slug, wt.Status, store.WtPROpen)
				wt.Status = store.WtPROpen
			}
			if err := store.Save(s); err != nil {
				return err
			}
			if existed {
				fmt.Printf("PR already open: %s (branch pushed)\n", url)
			} else {
				fmt.Printf("created PR %s for worktree %s\n", url, wt.Slug)
			}
			if tmplPath != "" {
				fmt.Printf("using PR template: %s\n", tmplPath)
			}
			return nil
		},
	}
	create.Flags().StringVar(&createTitle, "title", "", "PR title (defaults to Linear issue title, else branch name)")
	create.Flags().StringVar(&createBase, "base", "", "base branch (defaults to repo default)")
	create.Flags().BoolVar(&createNoTemplate, "no-template", false, "ignore the repo PR template")

	get := &cobra.Command{
		Use:   "get <pr|worktree>",
		Short: "Show a PR (by number or worktree)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, prNum, err := resolvePR(s, args[0])
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			p, err := resolveProjectView(s, st, wt.Project)
			if err != nil {
				return err
			}
			v, err := git.PrView(p.Repo, prNum, getComments)
			if err != nil {
				return err
			}
			if getComments {
				if inline, ierr := git.PRReviewComments(p.Repo, prNum); ierr == nil && len(inline) > 0 {
					v["inlineComments"] = inline
				}
			}
			output(v, func() { printPR(v) })
			return nil
		},
	}
	get.Flags().BoolVar(&getComments, "comments", false, "include comments and reviews")

	var commentBody string
	var commentReplyID int
	commentCmd := &cobra.Command{
		Use:   "comment <pr|worktree>",
		Short: "Post a comment on a PR (optionally as a thread reply)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(commentBody) == "" {
				return fmt.Errorf("--body is required")
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, prNum, err := resolvePR(s, args[0])
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			p, err := resolveProjectView(s, st, wt.Project)
			if err != nil {
				return err
			}
			if flagDryRun {
				if commentReplyID > 0 {
					fmt.Printf("would run: gh api repos/%s/pulls/%d/comments/%d/replies (body: %s)\n", p.Repo, prNum, commentReplyID, oneLine(commentBody))
				} else {
					fmt.Printf("would run: gh pr comment %d --repo %s (body: %s)\n", prNum, p.Repo, oneLine(commentBody))
				}
				return nil
			}
			if commentReplyID > 0 {
				id, err := git.PRReplyComment(p.Repo, prNum, commentReplyID, commentBody)
				if err != nil {
					return err
				}
				wt.SelfComments = append(wt.SelfComments, id)
				if err := store.Save(s); err != nil {
					return err
				}
				fmt.Printf("posted thread reply on PR #%d (comment %d)\n", prNum, id)
				return nil
			}
			url, id, err := git.PRComment(p.Repo, prNum, commentBody)
			if err != nil {
				return err
			}
			wt.SelfComments = append(wt.SelfComments, id)
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("commented on PR #%d: %s\n", prNum, url)
			return nil
		},
	}
	commentCmd.Flags().StringVar(&commentBody, "body", "", "comment text")
	commentCmd.Flags().IntVar(&commentReplyID, "reply", 0, "inline comment ID to reply to (thread reply)")

	var reviewVerdict, reviewBody, reviewRepo, reviewCommentsJSON string
	var reviewPending bool
	var reviewSubmit int
	reviewCmd := &cobra.Command{
		Use:   "review <pr-number>",
		Short: "Submit a PR review, or leave one pending for you to publish later",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prNum, err := strconv.Atoi(args[0])
			if err != nil || prNum <= 0 {
				return fmt.Errorf("%q is not a PR number", args[0])
			}
			verdicts := map[string]bool{"approve": true, "comment": true, "request-changes": true}
			s, err := store.Load()
			if err != nil {
				return err
			}
			st, err := config.Load()
			if err != nil {
				return err
			}
			repo := reviewRepo
			if repo == "" {
				if wt, ok := s.Worktrees[fmt.Sprintf("pr-%d", prNum)]; ok {
					repo = st.Projects[wt.Project].Repo
				} else if len(st.Projects) == 1 {
					for _, p := range st.Projects {
						repo = p.Repo
					}
				}
			}
			if repo == "" {
				return fmt.Errorf("cannot resolve repo; pass --repo owner/name (or --project context)")
			}
			var comments []git.ReviewComment
			if reviewCommentsJSON != "" {
				if err := json.Unmarshal([]byte(reviewCommentsJSON), &comments); err != nil {
					return fmt.Errorf("invalid --comments-json: %v", err)
				}
			}
			if reviewSubmit > 0 {
				if !verdicts[reviewVerdict] {
					return fmt.Errorf("--verdict is required: approve|comment|request-changes")
				}
				if flagDryRun {
					fmt.Printf("would run: gh api repos/%s/pulls/%d/reviews/%d/events -f event=<%s>\n", repo, prNum, reviewSubmit, reviewVerdict)
					return nil
				}
				if err := git.PRReviewSubmitPending(repo, prNum, reviewSubmit, reviewVerdict); err != nil {
					return err
				}
				registerWatchedPR(s, st, repo, prNum)
				if err := store.Save(s); err != nil {
					return err
				}
				fmt.Printf("submitted pending review %d on PR #%d as %s\n", reviewSubmit, prNum, reviewVerdict)
				return nil
			}
			if reviewPending {
				if flagDryRun {
					fmt.Printf("would run: gh api repos/%s/pulls/%d/reviews --input - (body=%s, %d inline comment(s), left pending)\n", repo, prNum, oneLine(reviewBody), len(comments))
					return nil
				}
				id, url, err := git.PRReviewPending(repo, prNum, reviewBody, comments)
				if err != nil {
					return err
				}
				registerWatchedPR(s, st, repo, prNum)
				if err := store.Save(s); err != nil {
					return err
				}
				fmt.Printf("left review %d pending on PR #%d (visible only to you): %s\n", id, prNum, url)
				fmt.Printf("submit later with: pr review %d --verdict approve|comment|request-changes --submit %d\n", prNum, id)
				fmt.Printf("watching PR #%d for replies to this review\n", prNum)
				return nil
			}
			if !verdicts[reviewVerdict] {
				return fmt.Errorf("--verdict is required: approve|comment|request-changes")
			}
			if flagDryRun {
				fmt.Printf("would run: gh api repos/%s/pulls/%d/reviews --input - (event=%s, body=%s, %d inline comment(s))\n", repo, prNum, reviewVerdict, oneLine(reviewBody), len(comments))
				return nil
			}
			if err := git.PRReview(repo, prNum, reviewVerdict, reviewBody, comments); err != nil {
				return err
			}
			registerWatchedPR(s, st, repo, prNum)
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("submitted %s review on PR #%d\n", reviewVerdict, prNum)
			fmt.Printf("watching PR #%d for replies to this review\n", prNum)
			return nil
		},
	}
	reviewCmd.Flags().StringVar(&reviewVerdict, "verdict", "", "approve|comment|request-changes")
	reviewCmd.Flags().StringVar(&reviewBody, "body", "", "review body (the findings)")
	reviewCmd.Flags().StringVar(&reviewRepo, "repo", "", "repo (defaults to the pr-N worktree's project, or the single registered project)")
	reviewCmd.Flags().BoolVar(&reviewPending, "pending", false, "leave the review pending on GitHub (visible only to you) instead of submitting it; publish later with --submit")
	reviewCmd.Flags().IntVar(&reviewSubmit, "submit", 0, "publish a previously created pending review by its ID, with --verdict")
	reviewCmd.Flags().StringVar(&reviewCommentsJSON, "comments-json", "", `inline comments, preferred over folding findings into --body: a JSON array like [{"path":"main.py","line":54,"body":"..."}]`)

	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Create and inspect GitHub pull requests",
	}
	cmd.AddCommand(create, get, commentCmd, reviewCmd)
	return cmd
}

func quoteAll(args ...string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = quoteIfNeeded(a)
	}
	return strings.Join(out, " ")
}

// registerWatchedPR starts tracking comment/review activity on a PR you
// just reviewed, so watchman notifies you when the author responds -- but
// only if no worktree already owns it (that's polled separately, via
// wt.PR, and would otherwise be double-counted).
func registerWatchedPR(s *store.State, st *config.Settings, repo string, pr int) {
	name := ""
	for pname, p := range st.Projects {
		if p.Repo == repo {
			name = pname
			break
		}
	}
	if name == "" {
		return
	}
	for _, wt := range s.Worktrees {
		if wt.Project == name && wt.PR == pr {
			return
		}
	}
	key := fmt.Sprintf("%s#%d", name, pr)
	if s.WatchedPRs[key] == nil {
		s.WatchedPRs[key] = &store.WatchedPR{Project: name, PR: pr}
	}
}

func resolvePR(s *store.State, ref string) (*store.Worktree, int, error) {
	if n, err := strconv.Atoi(ref); err == nil {
		for _, wt := range s.Worktrees {
			if wt.PR == n {
				return wt, n, nil
			}
		}
		return nil, 0, fmt.Errorf("no worktree tracks PR #%d", n)
	}
	wt, err := store.ResolveWorktree(s, ref)
	if err != nil {
		return nil, 0, err
	}
	if wt.PR == 0 {
		return nil, 0, fmt.Errorf("worktree %s has no PR yet", wt.Slug)
	}
	return wt, wt.PR, nil
}

func printPR(v map[string]any) {
	num, _ := v["number"].(float64)
	title, _ := v["title"].(string)
	state, _ := v["state"].(string)
	url, _ := v["url"].(string)
	head, _ := v["headRefName"].(string)
	fmt.Printf("#%d  %s  [%s]  (%s)\n", int(num), title, state, head)
	if url != "" {
		fmt.Printf("url:     %s\n", url)
	}
	if author, ok := v["author"].(map[string]any); ok {
		if login, _ := author["login"].(string); login != "" {
			fmt.Printf("author:  %s\n", login)
		}
	}
	if rd, _ := v["reviewDecision"].(string); rd != "" {
		fmt.Printf("review:  %s\n", rd)
	}
	if roll, ok := v["statusCheckRollup"].([]any); ok && len(roll) > 0 {
		pass, fail, pending := 0, 0, 0
		for _, c := range roll {
			cm, _ := c.(map[string]any)
			conc, _ := cm["conclusion"].(string)
			switch {
			case conc == "SUCCESS":
				pass++
			case conc != "":
				fail++
			default:
				pending++
			}
		}
		fmt.Printf("ci:      %d pass, %d fail, %d pending\n", pass, fail, pending)
	}
	if comments, ok := v["comments"].([]any); ok {
		if len(comments) > 0 {
			fmt.Printf("\ncomments (%d):\n", len(comments))
			for _, c := range comments {
				cm, _ := c.(map[string]any)
				author, _ := cm["author"].(map[string]any)
				login := ""
				if author != nil {
					login, _ = author["login"].(string)
				}
				body, _ := cm["body"].(string)
				fmt.Printf("  @%s: %s\n", login, oneLine(body))
			}
		}
	}
	if reviews, ok := v["reviews"].([]any); ok && len(reviews) > 0 {
		fmt.Printf("\nreviews (%d):\n", len(reviews))
		for _, r := range reviews {
			rm, _ := r.(map[string]any)
			author, _ := rm["author"].(map[string]any)
			login := ""
			if author != nil {
				login, _ = author["login"].(string)
			}
			state, _ := rm["state"].(string)
			body, _ := rm["body"].(string)
			fmt.Printf("  @%s [%s]: %s\n", login, state, oneLine(body))
		}
	}
	if inline, ok := v["inlineComments"].([]map[string]any); ok && len(inline) > 0 {
		fmt.Printf("\ninline review comments (%d):\n", len(inline))
		for _, ic := range inline {
			user, _ := ic["user"].(map[string]any)
			login := ""
			if user != nil {
				login, _ = user["login"].(string)
			}
			path, _ := ic["path"].(string)
			line, has := ic["line"].(float64)
			if !has || line == 0 {
				line, _ = ic["original_line"].(float64)
			}
			body, _ := ic["body"].(string)
			fmt.Printf("  @%s on %s:%d: %s\n", login, path, int(line), body)
		}
	}
}
