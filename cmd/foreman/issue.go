package main

import (
	"fmt"
	"strings"

	"assembly/internal/config"
	"assembly/internal/linear"

	"github.com/spf13/cobra"
)

func newIssueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Fetch issue details from Linear",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "get <issue-id>",
		Short: "Fetch a Linear issue by ID (e.g. ENG-123)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			issue, err := linear.GetIssue(args[0], config.LinearAPIKey())
			if err != nil {
				return err
			}
			output(issue, func() {
				kv("ID", "%s", issue.Identifier)
				kv("Title", "%s", issue.Title)
				kv("State", "%s", issue.State)
				if issue.Assignee != "" {
					kv("Assignee", "%s", issue.Assignee)
				}
				if len(issue.Labels) > 0 {
					kv("Labels", "%s", strings.Join(issue.Labels, ", "))
				}
				kv("URL", "%s", issue.URL)
				if issue.Description != "" {
					fmt.Println(strings.Repeat("-", 40))
					fmt.Println(issue.Description)
				}
			})
			return nil
		},
	})
	return cmd
}
