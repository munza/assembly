package watchman

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"assembly/internal/git"
	"assembly/internal/herdr"
	"assembly/internal/store"
)

const (
	stateFile = "watchman.json"
	logFile   = "watchman.log"
)

type Options struct {
	Interval    int    // GitHub poll interval in seconds; 0 disables polling
	Project     string // limit polling to one project
	PRs         bool   // poll PRs (comments, reviews, review requests)
	ForemanPane string // foreman tab pane ID; empty disables delivery and liveness exit
}

type State struct {
	PID         int       `json:"pid"`
	ForemanPane string    `json:"foreman_pane,omitempty"`
	Started     time.Time `json:"started"`
}

func StatePath() string { return filepath.Join(store.Dir(), stateFile) }
func LogPath() string   { return filepath.Join(store.Dir(), logFile) }

func ReadState() (*State, error) {
	b, err := os.ReadFile(StatePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, nil
	}
	return &st, nil
}

// Running returns the state of a live watchman, or nil. A state file left
// behind by a dead watchman is removed.
func Running() *State {
	st, err := ReadState()
	if err != nil || st == nil {
		return nil
	}
	if st.PID <= 0 || syscall.Kill(st.PID, 0) != nil {
		_ = os.Remove(StatePath())
		return nil
	}
	return st
}

func writeState(st *State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	path := StatePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func logf(format string, args ...any) {
	log.Printf("[watchman] "+format, args...)
}

// Run is the daemon loop. It blocks until SIGTERM/SIGINT, or until the
// foreman pane it is bound to disappears.
func Run(opts Options) error {
	if err := os.MkdirAll(store.MailboxDir(), 0o755); err != nil {
		return err
	}
	if opts.Interval > 0 && !git.GhAvailable() {
		logf("gh not found in PATH; GitHub polling disabled")
		opts.Interval = 0
	}
	st := &State{PID: os.Getpid(), ForemanPane: opts.ForemanPane, Started: time.Now().UTC()}
	if err := writeState(st); err != nil {
		return err
	}
	defer os.Remove(StatePath())
	logf("started pid %d, foreman pane %q, poll interval %ds", st.PID, opts.ForemanPane, opts.Interval)

	w, err := New()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.AddDir(store.MailboxDir()); err != nil {
		return err
	}

	seen := NewSeenComments()
	poll := func() {
		n, err := PollGitHub(opts, seen)
		if err != nil {
			logf("poll: %v", err)
		} else if n > 0 {
			logf("poll: %d new event(s)", n)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	mailTick := time.NewTicker(30 * time.Second)
	defer mailTick.Stop()

	var pollC <-chan time.Time
	if opts.Interval > 0 {
		pollTick := time.NewTicker(time.Duration(opts.Interval) * time.Second)
		pollC = pollTick.C
		defer pollTick.Stop()
		poll()
	}

	var liveC <-chan time.Time
	if opts.ForemanPane != "" {
		liveTick := time.NewTicker(5 * time.Second)
		liveC = liveTick.C
		defer liveTick.Stop()
	}

	deliver(opts.ForemanPane)
	misses := 0
	for {
		select {
		case <-sig:
			logf("stopping")
			return nil
		case <-w.Events:
			deliver(opts.ForemanPane)
		case <-mailTick.C:
			deliver(opts.ForemanPane)
		case <-pollC:
			poll()
		case <-liveC:
			ok, err := herdr.PaneAgentAlive(opts.ForemanPane)
			if err != nil {
				logf("liveness: %v", err)
				continue
			}
			if ok {
				misses = 0
				continue
			}
			misses++
			logf("foreman agent on %s gone (%d/3)", opts.ForemanPane, misses)
			if misses >= 3 {
				logf("foreman agent gone; stopping")
				return nil
			}
		case err := <-w.Errors:
			logf("mailbox watch: %v", err)
		}
	}
}

// deliver pushes unread worker and watch messages into the foreman tab and
// marks them read. Messages the foreman itself sent are skipped: mailbox send
// already delivered those into the worker's tab.
func deliver(pane string) {
	if pane == "" {
		return
	}
	ms, err := store.UnreadMessages()
	if err != nil {
		logf("mailbox: %v", err)
		return
	}
	for _, m := range ms {
		if m.From != "worker" && m.From != "watch" {
			continue
		}
		if err := herdr.AgentPrompt(pane, PromptText(m)); err != nil {
			logf("deliver: %v", err)
			continue
		}
		if err := store.MarkRead(m.ID); err != nil {
			logf("mark read %s: %v", m.ID, err)
		}
	}
}

func PromptText(m *store.Message) string {
	head := "github event on " + m.TaskID
	if m.From == "worker" {
		head = "mailbox from task " + m.TaskID
	}
	if m.Status != "" {
		head += " [" + m.Status + "]"
	}
	if m.Worktree != "" {
		head += " " + m.Worktree
		var inner []string
		if m.Project != "" {
			inner = append(inner, m.Project)
		}
		if m.IssueID != "" {
			inner = append(inner, m.IssueID)
		}
		if len(inner) > 0 {
			head += " (" + strings.Join(inner, ", ") + ")"
		}
		if m.TabLabel != "" {
			head += " tab " + m.TabLabel
		}
	}
	body := m.Body
	if len(body) > 4000 {
		body = body[:4000] + "\n...(truncated)"
	}
	return fmt.Sprintf("[watchman] %s:\n%s", head, body)
}
