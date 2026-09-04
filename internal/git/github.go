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

// runStdin is run, but piping stdin bytes in -- for `gh api ... --input -`
// calls whose request body has structure (arrays, nesting) that -f/-F
// key=value flags can't express.
func runStdin(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = bytes.NewReader(stdin)
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

// CurrentUserLogin returns the authenticated gh user's login, for filtering
// your own activity out of "new comment" polling (comments you post
// yourself through means other than foreman -- e.g. GitHub's own UI --
// aren't in a SelfComments list, but are still never worth notifying
// yourself about).
func CurrentUserLogin() (string, error) {
	out, err := run("", "api", "user", "-q", ".login")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

// ReviewComment is one inline comment anchored to a file/line, submitted as
// part of a review. Preferred over folding everything into the review body:
// it puts the finding exactly where the reader is already looking.
type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

func reviewEvent(verdict string) (string, error) {
	event := map[string]string{
		"approve":          "APPROVE",
		"comment":          "COMMENT",
		"request-changes":  "REQUEST_CHANGES",
	}[verdict]
	if event == "" {
		return "", fmt.Errorf("invalid verdict %q; valid: approve|comment|request-changes", verdict)
	}
	return event, nil
}

// postReview creates a review via the API -- gh pr review can't attach
// inline comments at all, so both the immediate-submit and pending paths go
// through gh api directly. event == "" leaves it PENDING (see
// PRReviewPending); otherwise it's submitted immediately with that event.
func postReview(repo string, number int, body, event string, comments []ReviewComment) (id int, url string, err error) {
	payload := map[string]any{}
	if strings.TrimSpace(body) != "" {
		payload["body"] = body
	}
	if event != "" {
		payload["event"] = event
	}
	if len(comments) > 0 {
		payload["comments"] = comments
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	out, err := runStdin(b, "api", fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, number), "--input", "-")
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

// PRReview submits a review immediately (approve, comment, or
// request-changes), with an optional body and inline comments.
func PRReview(repo string, number int, verdict, body string, comments []ReviewComment) error {
	event, err := reviewEvent(verdict)
	if err != nil {
		return err
	}
	_, _, err = postReview(repo, number, body, event, comments)
	return err
}

// PRReviewPending creates a review left in GitHub's PENDING state --
// visible only to the author until published with PRReviewSubmitPending.
// `gh pr review` has no equivalent; it always submits.
func PRReviewPending(repo string, number int, body string, comments []ReviewComment) (id int, url string, err error) {
	return postReview(repo, number, body, "", comments)
}

// PRReviewSubmitPending publishes a review previously left pending by
// PRReviewPending, assigning it the given verdict. Any comments were
// already attached at creation time.
func PRReviewSubmitPending(repo string, number, reviewID int, verdict string) error {
	event, err := reviewEvent(verdict)
	if err != nil {
		return err
	}
	_, err = run("", "api", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/events", repo, number, reviewID), "-X", "POST", "-f", "event="+event)
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
