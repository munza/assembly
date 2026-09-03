package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkReadRewritesSourceFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOREMAN_STATE_DIR", dir)
	mbox := filepath.Join(dir, "mailbox")
	if err := os.MkdirAll(mbox, 0o755); err != nil {
		t.Fatal(err)
	}
	// Regression: a message file whose name is not "<id>.json" (hand-written,
	// not from AppendMessage) must be marked read in place — rewriting a
	// derived name left the original unread and re-delivered it forever.
	path := filepath.Join(mbox, "handwritten.json")
	if err := os.WriteFile(path, []byte(`{"id":"t9","from":"worker","body":"x","read":false}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MarkRead("t9"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"read": true`) {
		t.Fatalf("source file not marked read: %s", b)
	}
	entries, err := os.ReadDir(mbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mailbox file, got %d", len(entries))
	}
}
