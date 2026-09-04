package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"assembly/internal/store"

	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Build the foreman and watchman binaries into the state dir's bin/",
		Long: "Build the foreman and watchman binaries into <state-dir>/bin/.\n" +
			"Workers run in other repos and cannot `go run` this one, and the\n" +
			"watchman daemon needs a real binary to auto-start. Run this from the\n" +
			"assembly repo once (and after pulling changes that touch Go code).",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := store.Dir()
			bin := filepath.Join(dir, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				return err
			}
			if flagDryRun {
				fmt.Printf("would run: go build -o %s ./cmd/...\n", bin)
				return nil
			}
			fmt.Printf("building into %s\n", bin)
			build := exec.Command("go", "build", "-o", bin+string(filepath.Separator), "./cmd/...")
			build.Stdout = os.Stdout
			build.Stderr = os.Stderr
			if err := build.Run(); err != nil {
				return fmt.Errorf("go build failed (run from the assembly repo): %w", err)
			}
			fmt.Println("built foreman and watchman")
			return nil
		},
	}
}
