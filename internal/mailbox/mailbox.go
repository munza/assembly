// Package mailbox is the on-disk message bus between agents.
// Messages are JSON files in .assembly/mailbox/<to>/, one file per message.
// A later watcher can fsnotify this tree and deliver mail automatically.
package mailbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"assembly/internal/config"
)

// Message types.
const (
	TypeQuestion = "question"
	TypeResult   = "result"
	TypeHandoff  = "handoff"
	TypeStatus   = "status"
)

// Message is one mail between agents (or user).
type Message struct {
	ID        string    `json:"id"` // <ts>-<from>-<type>
	From      string    `json:"from"`
	To        string    `json:"to"`
	Type      string    `json:"type"` // question | result | handoff | status
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Dir returns the mailbox root.
func Dir() string { return filepath.Join(config.StateDir(), "mailbox") }

func (m *Message) dir() string { return filepath.Join(Dir(), m.To) }

func (m *Message) path() string { return filepath.Join(m.dir(), m.ID+".json") }

// Send writes a message to the recipient's box.
func Send(from, to, typ, body string) (*Message, error) {
	ts := time.Now().UTC()
	m := &Message{
		ID:        fmt.Sprintf("%s-%s-%s", ts.Format("20060102T150405"), sanitize(from), sanitize(typ)),
		From:      from,
		To:        to,
		Type:      typ,
		Body:      body,
		CreatedAt: ts,
	}
	if err := os.MkdirAll(m.dir(), 0o755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(m.path(), b, 0o644); err != nil {
		return nil, err
	}
	return m, nil
}

// List returns messages, newest first. to == "" lists every box.
func List(to string) ([]*Message, error) {
	var boxes []string
	if to != "" {
		boxes = []string{to}
	} else {
		entries, err := os.ReadDir(Dir())
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				boxes = append(boxes, e.Name())
			}
		}
	}
	var msgs []*Message
	for _, b := range boxes {
		entries, err := os.ReadDir(filepath.Join(Dir(), b))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(Dir(), b, e.Name()))
			if err != nil {
				continue
			}
			var m Message
			if json.Unmarshal(raw, &m) == nil {
				msgs = append(msgs, &m)
			}
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].CreatedAt.After(msgs[j].CreatedAt) })
	return msgs, nil
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}
