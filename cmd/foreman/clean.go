package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"assembly/internal/store"
	"assembly/internal/watchman"

	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Reset runtime state: stop the watchman, clear state.json and the mailbox (settings and binaries are kept)",
		RunE: func(cmd *cobra.Command, args []string) error {
			stopped := false
			if st := watchman.Running(); st != nil {
				if flagDryRun {
					fmt.Printf("would stop watchman (pid %d)\n", st.PID)
				} else {
					_ = syscall.Kill(st.PID, syscall.SIGTERM)
					for i := 0; i < 20; i++ {
						time.Sleep(100 * time.Millisecond)
						if watchman.Running() == nil {
							stopped = true
							break
						}
					}
					if !stopped {
						return fmt.Errorf("watchman (pid %d) did not stop; run `watchman stop` and retry", st.PID)
					}
				}
			}
			targets := []string{
				store.Path(),
				store.MailboxDir(),
				watchman.StatePath(),
				watchman.LogPath(),
				watchman.StatePath() + ".tmp",
			}
			var removed []string
			for _, t := range targets {
				if _, err := os.Stat(t); err != nil {
					continue
				}
				if flagDryRun {
					fmt.Printf("would remove %s\n", t)
					continue
				}
				if err := os.RemoveAll(t); err != nil {
					return err
				}
				removed = append(removed, t)
			}
			if flagDryRun {
				fmt.Println("settings.json, .env and bin/ are kept")
				return nil
			}
			output(map[string]any{"removed": removed, "watchman_stopped": stopped}, func() {
				if len(removed) == 0 {
					fmt.Println("nothing to clean")
				}
				for _, r := range removed {
					fmt.Printf("removed %s\n", r)
				}
				if stopped {
					fmt.Println("watchman stopped (restarts on the next foreman command)")
				}
				fmt.Printf("kept: settings.json, .env, bin/ under %s\n", filepath.Clean(store.Dir()))
			})
			return nil
		},
	}
	return cmd
}
