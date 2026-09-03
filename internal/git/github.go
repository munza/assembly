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

func PrCreate(dir, repo, title, body, base, head string) (string, error) {
	args := []string{"pr", "create", "--title", title, "--body", body, "--head", head}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if base != "" {
		args = append(args, "--base", base)
	}
	out, err := run(dir, args...)
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(out))
	if i := strings.IndexAny(url, "\r\n"); i >= 0 {
		url = url[:i]
	}
	return url, nil
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
