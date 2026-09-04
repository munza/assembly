package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

func GhAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func PrCreate(dir, repo, title, body, base, head string) (string, bool, error) {
	args := []string{"pr", "create", "--title", title, "--body", body, "--head", head}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if base != "" {
		args = append(args, "--base", base)
	}
	out, err := run(dir, args...)
	if err == nil {
		return firstLine(out), false, nil
	}
	if !strings.Contains(err.Error(), "already exists") {
		return "", false, err
	}
	viewArgs := []string{"pr", "view", head, "--json", "url"}
	if repo != "" {
		viewArgs = append(viewArgs, "--repo", repo)
	}
	v, verr := run(dir, viewArgs...)
	if verr != nil {
		return "", false, err
	}
	var r struct {
		URL string `json:"url"`
	}
	if jerr := json.Unmarshal(v, &r); jerr != nil || r.URL == "" {
		return "", false, err
	}
	return r.URL, true, nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func PrView(repo string, number int, withComments bool) (map[string]any, error) {
	fields := "number,title,state,author,url,headRefName,reviewDecision,statusCheckRollup"
	if withComments {
		fields += ",comments,reviews"
	}
	out, err := run("", "pr", "view", fmt.Sprintf("%d", number), "--repo", repo, "--json", fields)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func PRReviewComments(repo string, number int) ([]map[string]any, error) {
	out, err := run("", "api", fmt.Sprintf("repos/%s/pulls/%d/comments", repo, number))
	if err != nil {
		return nil, err
	}
	var cs []map[string]any
	if err := json.Unmarshal(out, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func PRComment(repo string, number int, body string) (string, int, error) {
	out, err := run("", "pr", "comment", fmt.Sprintf("%d", number), "--repo", repo, "--body", body)
	if err != nil {
		return "", 0, err
	}
	url := firstLine(out)
	id := 0
	if i := strings.Index(url, "issuecomment-"); i >= 0 {
		id, _ = strconv.Atoi(strings.TrimPrefix(url[i:], "issuecomment-"))
	}
	return url, id, nil
}

func PRReplyComment(repo string, number, commentID int, body string) (int, error) {
	out, err := run("", "api", fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", repo, number, commentID), "-f", "body="+body)
	if err != nil {
		return 0, err
	}
	var r struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return 0, err
	}
	return r.ID, nil
}

func PRReview(repo string, number int, verdict, body string) error {
	flag := map[string]string{
		"approve": "--approve",
		"comment": "--comment",
		"request-changes": "--request-changes",
	}[verdict]
	if flag == "" {
		return fmt.Errorf("invalid verdict %q; valid: approve|comment|request-changes", verdict)
	}
	args := []string{"pr", "review", fmt.Sprintf("%d", number), "--repo", repo, flag}
	if strings.TrimSpace(body) != "" {
		args = append(args, "--body", body)
	}
	_, err := run("", args...)
	return err
}

// PRReviewPending creates a PR review via the API without an event, which
// GitHub leaves in the PENDING state: visible only to the author until
// published with PRReviewSubmitPending. `gh pr review` has no equivalent --
// it always submits.
func PRReviewPending(repo string, number int, body string) (id int, url string, err error) {
	args := []string{"api", fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, number)}
	if strings.TrimSpace(body) != "" {
		args = append(args, "-f", "body="+body)
	}
	out, err := run("", args...)
	if err != nil {
		return 0, "", err
	}
	var r struct {
		ID      int    `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return 0, "", err
	}
	return r.ID, r.HTMLURL, nil
}

// PRReviewSubmitPending publishes a review previously left pending by
// PRReviewPending, assigning it the given verdict.
func PRReviewSubmitPending(repo string, number, reviewID int, verdict string) error {
	event := map[string]string{
		"approve":          "APPROVE",
		"comment":          "COMMENT",
		"request-changes":  "REQUEST_CHANGES",
	}[verdict]
	if event == "" {
		return fmt.Errorf("invalid verdict %q; valid: approve|comment|request-changes", verdict)
	}
	_, err := run("", "api", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/events", repo, number, reviewID), "-X", "POST", "-f", "event="+event)
	return err
}

func ReviewRequested(repo string) ([]map[string]any, error) {
	out, err := run("", "pr", "list", "--repo", repo, "--search", "review-requested:@me", "--json", "number,title,author,url")
	if err != nil {
		return nil, err
	}
	var prs []map[string]any
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}
