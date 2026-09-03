package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var flagJSON bool
var flagDryRun bool

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "foreman",
		Short:        "Orchestrate projects, worktrees, and pi agents through herdr",
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable JSON output")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "reads run normally; writes print what they would do")
	root.CompletionOptions.HiddenDefaultCmd = true
	root.AddCommand(
		newProjectCmd(),
		newIssueCmd(),
		newWorktreeCmd(),
		newTaskCmd(),
		newPRCmd(),
		newMailboxCmd(),
		newWatchCmd(),
		newStatusCmd(),
	)
	root.AddCommand(newAliasCmds()...)
	return root
}

func Execute() error {
	return newRootCmd().Execute()
}

func output(v any, text func()) {
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	text()
}

func planRun(cmd string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, cmd)
	for _, a := range args {
		parts = append(parts, quoteIfNeeded(a))
	}
	return strings.Join(parts, " ")
}

func quoteIfNeeded(a string) string {
	if len(a) == 0 || containsAny(a, " \t\"'") {
		return fmt.Sprintf("%q", a)
	}
	return a
}

func containsAny(s string, chars string) bool {
	for _, c := range chars {
		for _, r := range s {
			if r == c {
				return true
			}
		}
	}
	return false
}
