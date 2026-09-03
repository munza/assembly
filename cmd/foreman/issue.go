package main

import (
	"fmt"
	"strings"

	"assembly/internal/linear"

	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Fetch issue details from Linear",
}

func init() {
	issueCmd.AddCommand(issueGetCmd)
	rootCmd.AddCommand(issueCmd)
}

var issueGetCmd = &cobra.Command{
	Use:   "get <issue-id>",
	Short: "Fetch a Linear issue by ID (e.g. ENG-123)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := linearKey()
		issue, err := linear.GetIssue(args[0], key)
		if err != nil {
			return err
		}
		output(issue, func() {
			fmt.Printf("%s  %s\n", issue.Identifier, issue.Title)
			fmt.Printf("state:    %s\n", issue.State)
			if issue.Assignee != "" {
				fmt.Printf("assignee: %s\n", issue.Assignee)
			}
			if len(issue.Labels) > 0 {
				fmt.Printf("labels:   %s\n", strings.Join(issue.Labels, ", "))
			}
			fmt.Printf("url:      %s\n", issue.URL)
			if issue.Description != "" {
				fmt.Println(strings.Repeat("-", 40))
				fmt.Println(issue.Description)
			}
		})
		return nil
	},
}
