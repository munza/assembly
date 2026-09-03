package watchman

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"assembly/internal/store"
)

// SpawnDetached starts a background watchman (running `bin start` in the
// foreground under itself) unless one is already running. It returns the
// running state and whether this call started it. The child gets an absolute
// FOREMAN_STATE_DIR so it survives the caller's cwd.
func SpawnDetached(bin string, opts Options) (*State, bool, error) {
	if st := Running(); st != nil {
		return st, false, nil
	}
	if err := os.MkdirAll(store.Dir(), 0o755); err != nil {
		return nil, false, err
	}
	logF, err := os.OpenFile(LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, false, err
	}
	defer logF.Close()

	args := []string{"start", "--interval", strconv.Itoa(opts.Interval)}
	if !opts.PRs {
		args = append(args, "--pr=false")
	}
	if opts.Project != "" {
		args = append(args, "--project", opts.Project)
	}
	if opts.ForemanPane != "" {
		args = append(args, "--foreman-pane", opts.ForemanPane)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = envWithAbsStateDir()
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := Running(); st != nil {
			return st, true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, false, fmt.Errorf("watchman did not start; see %s", LogPath())
}

func envWithAbsStateDir() []string {
	env := os.Environ()
	if os.Getenv("FOREMAN_STATE_DIR") == "" {
		if abs, err := filepath.Abs(store.Dir()); err == nil {
			env = append(env, "FOREMAN_STATE_DIR="+abs)
		}
	}
	return env
}
