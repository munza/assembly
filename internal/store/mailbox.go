package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func NewMessageID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}

func AppendMessage(m *Message) error {
	m.Time = time.Now().UTC()
	if m.ID == "" {
		m.ID = NewMessageID()
	}
	if err := os.MkdirAll(MailboxDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	name := filepath.Join(MailboxDir(), m.ID+".json")
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

func LoadMessages() ([]*Message, error) {
	entries, err := os.ReadDir(MailboxDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ms []*Message
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(MailboxDir(), e.Name()))
		if err != nil {
			continue
		}
		var m Message
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		ms = append(ms, &m)
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	return ms, nil
}

func MarkRead(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	entries, err := os.ReadDir(MailboxDir())
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(MailboxDir(), e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m Message
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if !want[m.ID] || m.Read {
			continue
		}
		m.Read = true
		b, err = json.MarshalIndent(&m, "", "  ")
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func UnreadMessages() ([]*Message, error) {
	ms, err := LoadMessages()
	if err != nil {
		return nil, err
	}
	var out []*Message
	for _, m := range ms {
		if !m.Read {
			out = append(out, m)
		}
	}
	return out, nil
}

func SenderLabel(paneID string) string {
	if env := os.Getenv("HERDR_PANE_ID"); env != "" && paneID != "" && env == paneID {
		return "worker"
	}
	return "foreman"
}
