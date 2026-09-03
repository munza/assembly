package git

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
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
