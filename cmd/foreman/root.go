package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"assembly/internal/config"

	"github.com/spf13/cobra"
)

var flagJSON bool
var flagDryRun bool

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "foreman",
		Short:        "Orchestrate projects, worktrees, and pi agents through herdr",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !flagDryRun {
				for c := cmd; c != nil; c = c.Parent() {
					switch c.Name() {
					case "help", "completion", "clean":
						return nil
					}
				}
				ensureWatchman()
			}
			return nil
		},
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
		newStatusCmd(),
		newCleanCmd(),
	)
	root.AddCommand(newAliasCmds()...)
	return root
}

func Execute() error {
	config.LoadDotEnv()
	return newRootCmd().Execute()
}

func output(v any, text func()) {
	if flagJSON {
		emitJSON(v)
		return
	}
	text()
}

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// tableOutput renders a slice of view structs as an aligned table in text mode
// and as JSON in --json mode. Columns come from struct fields in order; the
// header is the json tag uppercased; a field tagged omitempty is hidden when
// every row has its zero value.
func tableOutput(rows any) {
	if flagJSON {
		emitJSON(rows)
		return
	}
	rv := reflect.ValueOf(rows)
	if rv.Kind() != reflect.Slice || rv.Len() == 0 {
		return
	}
	elem := rv.Type().Elem()
	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return
	}
	type col struct {
		name string
		hide bool
		vals []string
	}
	var cols []col
	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		c := col{name: strings.ToUpper(name), hide: strings.Contains(tag, ",omitempty")}
		for j := 0; j < rv.Len(); j++ {
			v := rv.Index(j)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			c.vals = append(c.vals, fmt.Sprintf("%v", v.Field(i).Interface()))
		}
		if c.hide {
			zero := true
			for _, s := range c.vals {
				if s != "" && s != "0" && s != "<nil>" && s != "false" {
					zero = false
					break
				}
			}
			c.hide = zero
		}
		cols = append(cols, c)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 3, ' ', 0)
	header := make([]string, 0, len(cols))
	for _, c := range cols {
		if !c.hide {
			header = append(header, c.name)
		}
	}
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for r := 0; r < rv.Len(); r++ {
		cells := make([]string, 0, len(cols))
		for _, c := range cols {
			if !c.hide {
				cells = append(cells, c.vals[r])
			}
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	w.Flush()
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
