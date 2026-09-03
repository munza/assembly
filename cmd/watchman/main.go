package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"assembly/internal/watchman"

	"github.com/spf13/cobra"
)

var flagJSON bool

func newRootCmd() *cobra.Command {
	var (
		detached    bool
		interval    int
		prs         bool
		project     string
		foremanPane string
	)

	opts := func() watchman.Options {
		return watchman.Options{
			Interval:    interval,
			Project:     project,
			PRs:         prs,
			ForemanPane: paneOrDefault(foremanPane),
		}
	}
	run := func(cmd *cobra.Command, args []string) error {
		if detached {
			st, started, err := watchman.SpawnDetached(selfBin(), opts())
			if err != nil {
				return err
			}
			verb := "already running"
			if started {
				verb = "started"
			}
			output(map[string]any{"running": true, "started": started, "pid": st.PID}, func() {
				fmt.Printf("watchman %s (pid %d)\n", verb, st.PID)
			})
			return nil
		}
		return watchman.Run(opts())
	}

	root := &cobra.Command{
		Use:   "watchman",
		Short: "Foreman daemon: delivers mailbox events to the foreman tab and polls GitHub PRs",
		RunE:  run,
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable JSON output")
	root.PersistentFlags().BoolVar(&detached, "detached", false, "spawn a background instance instead of running in the foreground")
	root.PersistentFlags().IntVar(&interval, "interval", 60, "GitHub poll interval in seconds (0 disables)")
	root.PersistentFlags().BoolVar(&prs, "pr", true, "watch PRs (comments, reviews, review requests)")
	root.PersistentFlags().StringVar(&project, "project", "", "limit polling to one project")
	root.PersistentFlags().StringVar(&foremanPane, "foreman-pane", "", "foreman tab pane ID for delivery (default: $HERDR_PANE_ID)")

	start := &cobra.Command{
		Use:   "start",
		Short: "Run the watchman (foreground unless --detached)",
		RunE:  run,
	}

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop the detached watchman",
		RunE: func(cmd *cobra.Command, args []string) error {
			st := watchman.Running()
			if st == nil {
				output(map[string]any{"running": false}, func() { fmt.Println("watchman not running") })
				return nil
			}
			if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
				return err
			}
			for i := 0; i < 30; i++ {
				time.Sleep(100 * time.Millisecond)
				if watchman.Running() == nil {
					output(map[string]any{"running": false, "pid": st.PID}, func() {
						fmt.Printf("watchman stopped (pid %d)\n", st.PID)
					})
					return nil
				}
			}
			_ = syscall.Kill(st.PID, syscall.SIGKILL)
			_ = os.Remove(watchman.StatePath())
			output(map[string]any{"running": false, "pid": st.PID}, func() {
				fmt.Printf("watchman killed (pid %d)\n", st.PID)
			})
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show whether the watchman is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			st := watchman.Running()
			if st == nil {
				output(map[string]any{"running": false}, func() { fmt.Println("watchman not running") })
				return nil
			}
			output(map[string]any{
				"running":      true,
				"pid":          st.PID,
				"foreman_pane": st.ForemanPane,
				"started":      st.Started,
				"log":          watchman.LogPath(),
			}, func() {
				fmt.Printf("watchman running (pid %d, foreman pane %s, since %s)\nlog: %s\n",
					st.PID, st.ForemanPane, st.Started.Local().Format(time.RFC3339), watchman.LogPath())
			})
			return nil
		},
	}

	root.AddCommand(start, stop, status)
	return root
}

func paneOrDefault(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv("HERDR_PANE_ID")
}

func selfBin() string {
	bin, err := os.Executable()
	if err != nil {
		return "watchman"
	}
	return bin
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

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
