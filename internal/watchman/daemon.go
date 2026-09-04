package watchman

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"assembly/internal/git"
	"assembly/internal/mux"
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
	ForemanPane string // foreman tab pane ID, watched only for liveness (empty disables the exit-when-gone check); watchman does not deliver messages into it -- see `foreman mailbox inbox --follow`
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
// foreman pane it is bound to disappears. It polls GitHub and writes results
// into the mailbox; it does not deliver messages itself (see package doc).
func Run(opts Options) error {
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

	misses := 0
	for {
		select {
		case <-sig:
			logf("stopping")
			return nil
		case <-pollC:
			poll()
		case <-liveC:
			ok, err := mux.PaneAgentAlive(opts.ForemanPane)
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
		}
	}
}
