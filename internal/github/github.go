// Package github polls GitHub via the gh CLI (auth handled by gh).
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// PR is an open pull request.
type PR struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Comment is a review or issue comment on a PR.
type Comment struct {
	ID        int64     `json:"id"`
	PR        int       `json:"-"` // filled by the poller
	User      string    `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Client wraps gh for one repo ("owner/name").
type Client struct{ Repo string }

// Enabled reports whether a repo is configured.
func (c Client) Enabled() bool { return c.Repo != "" }

type prJSON struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Author    struct{ Login string } `json:"author"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// OpenPRs lists open PRs for the repo.
func (c Client) OpenPRs() ([]PR, error) {
	var raws []prJSON
	err := gh(&raws, "pr", "list", "--repo", c.Repo, "--state", "open",
		"--json", "number,title,author,updatedAt", "--limit", "50")
	if err != nil {
		return nil, err
	}
	var prs []PR
	for _, r := range raws {
		prs = append(prs, PR{Number: r.Number, Title: r.Title, Author: r.Author.Login, UpdatedAt: r.UpdatedAt})
	}
	return prs, nil
}

// ReviewComments fetches review comments on one PR.
func (c Client) ReviewComments(prNumber int) ([]Comment, error) {
	type rawComment struct {
		ID        int64     `json:"id"`
		User      struct{ Login string } `json:"user"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	var raws []rawComment
	if err := gh(&raws, "api", fmt.Sprintf("repos/%s/pulls/%d/comments", c.Repo, prNumber)); err != nil {
		return nil, err
	}
	var out []Comment
	for _, r := range raws {
		out = append(out, Comment{ID: r.ID, PR: prNumber, User: r.User.Login, Body: r.Body, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func gh(out any, args ...string) error {
	cmd := exec.Command("gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh %v: %w: %s", args, err, truncate(stderr.String(), 300))
	}
	return json.Unmarshal(stdout.Bytes(), out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
