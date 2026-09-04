package main

import (
	"fmt"

	"assembly/internal/store"

	"github.com/spf13/cobra"
)

type aliasOpts struct {
	general  bool
	thread   bool
	slug     string
	stage    string
	task     string
	issue    string
	worktree string
}

func newAliasCmds() []*cobra.Command {
	var cs []*cobra.Command
	for _, verb := range taskTypes {
		cs = append(cs, newAliasCmd(verb))
	}
	return cs
}

func newAliasCmd(verb string) *cobra.Command {
	var o aliasOpts
	c := &cobra.Command{
		Use:   verb + " <note>",
		Short: "shortcut for: task add --type " + verb,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlias(verb, args[0], o)
		},
	}
	c.Flags().BoolVar(&o.general, "general", false, "note is a general note")
	c.Flags().BoolVar(&o.thread, "thread", false, "note is tied to a review thread")
	c.Flags().StringVar(&o.slug, "slug", "", "short unique slug (used as tab/agent label)")
	c.Flags().StringVar(&o.stage, "stage", "", "pipeline stage tag (see task add)")
	c.Flags().StringVar(&o.task, "task", "", "related task (uses its worktree, referenced in the note)")
	c.Flags().StringVar(&o.issue, "issue", "", "Linear issue ID (uses its worktree, referenced in the note)")
	c.Flags().StringVar(&o.worktree, "worktree", "", "target worktree")
	return c
}

func runAlias(verb, note string, o aliasOpts) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	target := o.worktree
	noteParts := []string{note}
	if o.task != "" {
		t, err := store.ResolveTask(s, o.task)
		if err != nil {
			return err
		}
		if target == "" {
			target = t.Worktree
		}
		noteParts = append(noteParts, fmt.Sprintf("[task %s]", t.ID))
	}
	if o.issue != "" {
		wt, err := store.ResolveWorktree(s, o.issue)
		if err != nil {
			return fmt.Errorf("no worktree for issue %s: %w", o.issue, err)
		}
		if target == "" {
			target = wt.Slug
		}
		noteParts = append(noteParts, fmt.Sprintf("[issue %s]", o.issue))
	}
	noteKind := ""
	if o.thread {
		noteKind = "thread"
	} else if o.general {
		noteKind = "general"
	}
	t, err := addTask(verb, joinNote(noteParts), o.slug, target, o.stage, noteKind)
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
