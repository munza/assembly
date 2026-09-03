package main

import (
	"os"
	"strings"
	"text/template"

	"assembly/internal/config"
	"assembly/internal/linear"

	"github.com/spf13/cobra"
)

var issueText = template.Must(template.New("issue").Funcs(template.FuncMap{"join": strings.Join}).Parse(`ID:        {{.Identifier}}
Title:     {{.Title}}
State:     {{.State}}
{{- if .Assignee}}
Assignee:  {{.Assignee}}
{{- end}}
{{- if .Labels}}
Labels:    {{join .Labels ", "}}
{{- end}}
URL:       {{.URL}}
{{- if .Description}}
----------------------------------------
{{.Description}}
{{- end}}
`))

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
				_ = issueText.Execute(os.Stdout, issue)
			})
			return nil
		},
	})
	return cmd
}
