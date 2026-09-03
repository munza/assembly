package main

import (
	"fmt"

	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var (
	aliasGeneral  bool
	aliasThread   bool
	aliasTaskRef  string
	aliasIssueRef string
	aliasWtRef    string
)

func init() {
	for _, verb := range taskTypes {
		rootCmd.AddCommand(newAliasCmd(verb))
	}
}

func newAliasCmd(verb string) *cobra.Command {
	c := &cobra.Command{
		Use:   verb + " <note>",
		Short: "shortcut for: task add --type " + verb,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlias(verb, args[0])
		},
	}
	c.Flags().BoolVar(&aliasGeneral, "general", false, "note is a general note")
	c.Flags().BoolVar(&aliasThread, "thread", false, "note is tied to a review thread")
	c.Flags().StringVar(&aliasTaskRef, "task", "", "related task (uses its worktree, referenced in the note)")
	c.Flags().StringVar(&aliasIssueRef, "issue", "", "Linear issue ID (uses its worktree, referenced in the note)")
	c.Flags().StringVar(&aliasWtRef, "worktree", "", "target worktree")
	return c
}

func runAlias(verb, note string) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	target := aliasWtRef
	noteParts := []string{note}
	if aliasTaskRef != "" {
		t, err := store.ResolveTask(s, aliasTaskRef)
		if err != nil {
			return err
		}
		if target == "" {
			target = t.Worktree
		}
		noteParts = append(noteParts, fmt.Sprintf("[task %s]", t.ID))
	}
	if aliasIssueRef != "" {
		wt, err := store.ResolveWorktree(s, aliasIssueRef)
		if err != nil {
			return fmt.Errorf("no worktree for issue %s: %w", aliasIssueRef, err)
		}
		if target == "" {
			target = wt.Slug
		}
		noteParts = append(noteParts, fmt.Sprintf("[issue %s]", aliasIssueRef))
	}
	noteKind := ""
	if aliasThread {
		noteKind = "thread"
	} else if aliasGeneral {
		noteKind = "general"
	}
	t, err := addTask(verb, joinNote(noteParts), "", target, noteKind)
	if err != nil {
		return err
	}
	if flagDryRun {
		return nil
	}
	output(t, func() {
		fmt.Printf("created task %s (%s) in worktree %s — %s\n", t.ID, t.Type, t.Worktree, oneLine(t.Note))
	})
	return nil
}

func joinNote(parts []string) string {
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += " "
		}
		out += p
	}
	return out
}
