// Package watcher polls Linear and GitHub for changes, dedupes them, and
// turns each new event into mailbox mail — plus nudges the foreman pane
// when one is running. Also watches the mailbox tree itself so mail from
// agents reaches the foreman agent.
package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assembly/internal/config"
	"assembly/internal/github"
	"assembly/internal/herdr"
	"assembly/internal/linear"
	"assembly/internal/mailbox"
)

// Poller is anything that can be polled for events.
type Poller interface {
	Name() string
	Poll(ctx context.Context) ([]Event, error)
}

// Event is one new external change, rendered as mail.
type Event struct {
	Key  string // dedupe key
	From string
	Type string
	Body string
}

// State is the dedupe store on disk.
type State struct {
	Seen map[string]time.Time `json:"seen"`
}

func statePath() string { return filepath.Join(config.StateDir(), "watcher.json") }

func loadState() *State {
	s := &State{Seen: map[string]time.Time{}}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, s)
	}
	return s
}

func (s *State) save() error {
	if err := os.MkdirAll(config.StateDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), b, 0o644)
}

// IsNew records key if unseen; reports whether it was new.
func (s *State) IsNew(key string) bool {
	if _, ok := s.Seen[key]; ok {
		return false
	}
	s.Seen[key] = time.Now().UTC()
	return true
}

// --- Linear poller ---

type LinearPoller struct{ Client linear.Client }

func (p LinearPoller) Name() string { return "linear" }

func (p LinearPoller) Poll(ctx context.Context) ([]Event, error) {
	if !p.Client.Enabled() {
		return nil, nil
	}
	since := time.Now().UTC().Add(-24 * time.Hour)
	issues, err := p.Client.IssuesUpdatedSince(since, "")
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, is := range issues {
		events = append(events, Event{
			Key:  "linear:" + is.ID + ":" + is.UpdatedAt.Format(time.RFC3339),
			From: "linear",
			Type: mailbox.TypeStatus,
			Body: fmt.Sprintf("%s %q state=%s assignee=%s updated=%s",
				is.Ref(), is.Title, is.State.Name, is.Assignee.Name, is.UpdatedAt.Format(time.Kitchen)),
		})
	}
	return events, nil
}

// --- GitHub poller ---

type GitHubPoller struct{ Client github.Client }

func (p GitHubPoller) Name() string { return "github" }

// recentWindow limits how far back the GitHub poller surfaces comments,
// so first runs do not flood the mailbox with a repo's entire history.
const recentWindow = 48 * time.Hour

func (p GitHubPoller) Poll(ctx context.Context) ([]Event, error) {
	if !p.Client.Enabled() {
		return nil, nil
	}
	prs, err := p.Client.OpenPRs()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-recentWindow)
	var events []Event
	for _, pr := range prs {
		comments, err := p.Client.ReviewComments(pr.Number)
		if err != nil {
			continue
		}
		for _, c := range comments {
			if c.CreatedAt.Before(cutoff) {
				continue
			}
			events = append(events, Event{
				Key:  fmt.Sprintf("gh:comment:%d", c.ID),
				From: "github",
				Type: mailbox.TypeQuestion, // review comments usually want a reply
				Body: fmt.Sprintf("PR #%d %q — %s commented: %s",
					pr.Number, pr.Title, c.User, firstLine(c.Body, 200)),
			})
		}
	}
	return events, nil
}

// --- Mail poller: delivers mailbox files to the foreman pane ---

type MailPoller struct{}

func (p MailPoller) Name() string { return "mail" }

func (p MailPoller) Poll(ctx context.Context) ([]Event, error) {
	msgs, err := mailbox.List("foreman")
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, m := range msgs {
		events = append(events, Event{
			Key:  "mail:" + m.ID,
			From: m.From,
			Type: m.Type,
			Body: m.Body,
		})
	}
	return events, nil
}

// Once polls every source a single time and exits. Used by `foreman
// watch --once` and tests.
func Once(cfg *config.Config, log *slog.Logger) error {
	state := loadState()
	pollers := []Poller{
		LinearPoller{Client: linear.Client{Key: cfg.LinearAPIKey}},
		GitHubPoller{Client: github.Client{Repo: cfg.GitHubRepo}},
		MailPoller{},
	}
	var newCount int
	for _, p := range pollers {
		events, err := p.Poll(context.Background())
		if err != nil {
			log.Warn("poll failed", "source", p.Name(), "err", err)
			continue
		}
		for _, ev := range events {
			if !state.IsNew(ev.Key) {
				continue
			}
			if p.Name() == "mail" {
				log.Info("new mail", "from", ev.From, "body", firstLine(ev.Body, 80))
			} else {
				if _, err := mailbox.Send(ev.From, "foreman", ev.Type, ev.Body); err != nil {
					log.Warn("mail send failed", "err", err)
				} else {
					log.Info("event -> mail", "source", p.Name(), "body", firstLine(ev.Body, 80))
				}
			}
			newCount++
		}
	}
	if err := state.save(); err != nil {
		return err
	}
	log.Info("poll done", "new", newCount)
	return nil
}

// Run polls all sources on an interval, writes new events as mail to the
// foreman box, and nudges the foreman agent pane (if running) with each
// new item. Blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	state := loadState()
	pollers := []Poller{
		LinearPoller{Client: linear.Client{Key: cfg.LinearAPIKey}},
		GitHubPoller{Client: github.Client{Repo: cfg.GitHubRepo}},
		MailPoller{},
	}

	external := cfg.PollInterval
	if external <= 0 {
		external = 60 * time.Second
	}
	tickExt := time.NewTicker(external)
	tickMail := time.NewTicker(5 * time.Second)
	defer tickExt.Stop()
	defer tickMail.Stop()

	log.Info("watcher running", "external", external, "mail", 5*time.Second)

	pollExternal := func() {
		for i := range pollers[:2] { // linear + github
			events, err := pollers[i].Poll(ctx)
			if err != nil {
				log.Warn("poll failed", "source", pollers[i].Name(), "err", err)
				continue
			}
			for _, ev := range events {
				if !state.IsNew(ev.Key) {
					continue
				}
				if _, err := mailbox.Send(ev.From, "foreman", ev.Type, ev.Body); err != nil {
					log.Warn("mail send failed", "err", err)
				} else {
					log.Info("event -> mail", "source", pollers[i].Name(), "body", firstLine(ev.Body, 80))
				}
			}
		}
		if err := state.save(); err != nil {
			log.Warn("state save failed", "err", err)
		}
	}

	pollMail := func() {
		events, err := MailPoller{}.Poll(ctx)
		if err != nil {
			return
		}
		pane := foremanPane()
		if pane == "" {
			return
		}
		for _, ev := range events {
			if !state.IsNew(ev.Key) {
				continue
			}
			if err := herdr.AgentPrompt(pane, "New mail from "+ev.From+" ("+ev.Type+"): "+ev.Body, false, nil, 0); err != nil {
				log.Warn("nudge failed", "pane", pane, "err", err)
			} else {
				log.Info("mail -> nudge", "from", ev.From, "pane", pane)
			}
		}
		if err := state.save(); err != nil {
			log.Warn("state save failed", "err", err)
		}
	}

	pollExternal()
	pollMail()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tickExt.C:
			pollExternal()
		case <-tickMail.C:
			pollMail()
		}
	}
}

// foremanPane finds the pane of the foreman agent (named "foreman*").
func foremanPane() string {
	agents, err := herdr.ListAgents()
	if err != nil {
		return ""
	}
	for _, a := range agents {
		if strings.HasPrefix(a.Name, "foreman") {
			return a.PaneID
		}
	}
	return ""
}

func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
