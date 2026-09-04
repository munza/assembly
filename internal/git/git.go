package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

var prTemplatePaths = []string{
	".github/PULL_REQUEST_TEMPLATE.md",
	"PULL_REQUEST_TEMPLATE.md",
	"docs/PULL_REQUEST_TEMPLATE.md",
}

var prTemplateDirs = []string{
	".github/PULL_REQUEST_TEMPLATE",
	"PULL_REQUEST_TEMPLATE",
	"docs/PULL_REQUEST_TEMPLATE",
}

func PRTemplate(dir string) (string, string) {
	for _, p := range prTemplatePaths {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err == nil {
			return p, string(b)
		}
	}
	for _, d := range prTemplateDirs {
		entries, err := os.ReadDir(filepath.Join(dir, d))
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		p := filepath.Join(d, names[0])
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err == nil {
			return p, string(b)
		}
	}
	return "", ""
}

func RemoveWorktreeCheckout(path string) error {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return fmt.Errorf("resolve repo root of %s: %w", path, err)
	}
	root := strings.TrimSpace(string(out))
	cmd := exec.Command("git", "-C", root, "worktree", "remove", "--force", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree remove %s: %s", path, msg)
	}
	return nil
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

// FetchPRHead fetches a PR's head into FETCH_HEAD without touching any
// branch: `git fetch origin pull/N/head` without a destination ref.
func FetchPRHead(dir string, pr int) error {
	return runGit(dir, "fetch", "origin", fmt.Sprintf("pull/%d/head", pr))
}

// PointBranchAtFetchHead creates or force-moves a branch to what the last
// FetchPRHead fetched. Fails when the branch is checked out in a worktree;
// use ResetToFetchHead there instead.
func PointBranchAtFetchHead(dir, branch string) error {
	return runGit(dir, "branch", "-f", branch, "FETCH_HEAD")
}

// ResetToFetchHead hard-resets the current branch of the checkout in dir to
// what the last FetchPRHead fetched. For disposable review checkouts only.
func ResetToFetchHead(dir string) error {
	return runGit(dir, "reset", "--hard", "FETCH_HEAD")
}

func DeleteBranch(dir, branch string) error {
	return runGit(dir, "branch", "-D", branch)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
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
