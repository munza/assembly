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
				fmt.Printf("ID:       %s\n", issue.Identifier)
				fmt.Printf("Title:    %s\n", issue.Title)
				fmt.Printf("State:    %s\n", issue.State)
				if issue.Assignee != "" {
					fmt.Printf("Assignee: %s\n", issue.Assignee)
				}
				if len(issue.Labels) > 0 {
					fmt.Printf("Labels:   %s\n", strings.Join(issue.Labels, ", "))
				}
				fmt.Printf("URL:      %s\n", issue.URL)
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
