package main

import (
	"fmt"
	"strconv"
	"strings"

	"assembly/internal/config"
	"assembly/internal/git"
	"assembly/internal/linear"
	"assembly/internal/store"

	"github.com/spf13/cobra"
)

func newPRCmd() *cobra.Command {
	var (
		createTitle, createBase string
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
				if issue, err := linear.GetIssue(wt.IssueID, config.LinearAPIKey()); err == nil {
					title = issue.Title
					body = fmt.Sprintf("[%s](%s)\n\n%s", issue.Identifier, issue.URL, issue.Description)
				} else {
					fmt.Printf("warning: could not fetch issue %s: %v\n", wt.IssueID, err)
				}
			}
			if title == "" {
				title = wt.Branch
			}
			if !git.GhAvailable() {
				return fmt.Errorf("gh not found in PATH")
			}
			if flagDryRun {
				ghArgs := []string{"gh", "pr", "create", "--title", title, "--head", wt.Branch, "--repo", p.Repo}
				if createBase != "" {
					ghArgs = append(ghArgs, "--base", createBase)
				}
				fmt.Println("would run: " + quoteAll(ghArgs...))
				fmt.Printf("would set worktree %s status %s -> %s\n", wt.Slug, wt.Status, store.WtPROpen)
				return nil
			}
			url, err := git.PrCreate(wt.Path, p.Repo, title, body, createBase, wt.Branch)
			if err != nil {
				return err
			}
			prNum := 0
			if i := strings.LastIndex(url, "/"); i >= 0 {
				prNum, _ = strconv.Atoi(url[i+1:])
			}
			wt.PR = prNum
			wt.Status = store.WtPROpen
			if err := store.Save(s); err != nil {
				return err
			}
			fmt.Printf("created PR %s for worktree %s\n", url, wt.Slug)
			return nil
		},
	}
	create.Flags().StringVar(&createTitle, "title", "", "PR title (defaults to Linear issue title, else branch name)")
	create.Flags().StringVar(&createBase, "base", "", "base branch (defaults to repo default)")

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
			output(v, func() { printPR(v) })
			return nil
		},
	}
	get.Flags().BoolVar(&getComments, "comments", false, "include comments and reviews")

	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Create and inspect GitHub pull requests",
	}
	cmd.AddCommand(create, get)
	return cmd
}

func quoteAll(args ...string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = quoteIfNeeded(a)
	}
	return strings.Join(out, " ")
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
}
