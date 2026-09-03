package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func Origin(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("no origin remote in %s", path)
	}
	repo := parseRepo(string(out))
	if repo == "" {
		return "", fmt.Errorf("cannot parse repo from origin url %q", strings.TrimSpace(string(out)))
	}
	return repo, nil
}

func IsRepo(path string) bool {
	return exec.Command("git", "-C", path, "rev-parse", "--git-dir").Run() == nil
}

func Push(dir, branch string) error {
	cmd := exec.Command("git", "-C", dir, "push", "-u", "origin", branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git push %s: %s", branch, msg)
	}
	return nil
}

func parseRepo(url string) string {
	u := strings.TrimSuffix(strings.TrimSpace(url), ".git")
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	} else if c := strings.Index(u, ":"); c >= 0 {
		if s := strings.Index(u, "/"); s < 0 || c < s {
			u = u[c+1:]
		}
	}
	parts := strings.Split(u, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}
