package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var flagJSON bool
var flagDryRun bool

var rootCmd = &cobra.Command{
	Use:          "foreman",
	Short:        "Orchestrate projects, worktrees, and pi agents through herdr",
	SilenceUsage: true,
}

func Execute() error {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable JSON output")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "reads run normally; writes print what they would do")
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
	s := cmd
	for _, a := range args {
		s += " " + quoteIfNeeded(a)
	}
	return s
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
