package main

import (
	"fmt"
	"os"
	"path/filepath"

	"assembly/internal/store"
	"assembly/internal/watchman"
)

// ensureWatchman lazily starts the detached watchman when a foreman command
// runs inside the foreman tab: a herdr pane of the assembly repo itself.
// Worker tabs (FOREMAN_STATE_DIR set) and plain terminals never trigger it.
// Best-effort: failures print one warning and never fail the command.
func ensureWatchman() {
	if os.Getenv("FOREMAN_NO_WATCHMAN") != "" || os.Getenv("FOREMAN_STATE_DIR") != "" {
		return
	}
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "settings.json")); err != nil {
		return
	}
	if watchman.Running() != nil {
		return
	}
	bin := resolveWatchmanBin()
	if bin == "" {
		fmt.Fprintln(os.Stderr, "watchman: no binary found; run `foreman setup`")
		return
	}
	_, _, err := watchman.SpawnDetached(bin, watchman.Options{Interval: 60, PRs: true, ForemanPane: pane})
	if err != nil {
		fmt.Fprintf(os.Stderr, "watchman: %v\n", err)
	}
}

func resolveWatchmanBin() string {
	if p := os.Getenv("FOREMAN_WATCHMAN_BIN"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
		return ""
	}
	candidates := []string{filepath.Join(store.Dir(), "bin", "watchman")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "watchman"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return c
		}
	}
	return ""
}
