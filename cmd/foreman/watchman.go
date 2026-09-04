package main

import (
	"fmt"
	"os"
	"path/filepath"

	"assembly/internal/store"
	"assembly/internal/watchman"
)

// ensureWatchman guarantees a running watchman before any foreman command
// proceeds, when run inside the foreman tab: a herdr pane of the assembly
// repo itself. It starts the detached daemon if none is running and fails
// the command otherwise -- a foreman tab without its daemon silently loses
// every worker report and PR event. Worker tabs (FOREMAN_STATE_DIR set),
// plain terminals, and FOREMAN_NO_WATCHMAN=1 are exempt.
func ensureWatchman() error {
	if os.Getenv("FOREMAN_NO_WATCHMAN") != "" || os.Getenv("FOREMAN_STATE_DIR") != "" {
		return nil
	}
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "settings.json")); err != nil {
		return nil
	}
	if watchman.Running() != nil {
		return nil
	}
	bin := resolveWatchmanBin()
	if bin == "" {
		return fmt.Errorf("watchman is not running and no binary was found; run `foreman setup` and retry")
	}
	if _, _, err := watchman.SpawnDetached(bin, watchman.Options{Interval: 60, PRs: true, ForemanPane: pane}); err != nil {
		return fmt.Errorf("watchman is not running: %v", err)
	}
	return nil
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
