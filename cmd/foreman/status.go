package main

import (
	"fmt"

	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "One-screen overview: worktrees, running tasks, unread mail",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Load()
		if err != nil {
			return err
		}
		wts := sortedWorktrees(s, "")
		type taskView struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			Running bool   `json:"running"`
			Note    string `json:"note"`
		}
		type wtView struct {
			Slug   string     `json:"slug"`
			Status string     `json:"status"`
			PR     int        `json:"pr,omitempty"`
			Tasks  []taskView `json:"tasks"`
		}
		var view []wtView
		for _, wt := range wts {
			v := wtView{Slug: wt.Slug, Status: wt.Status, PR: wt.PR}
			for _, t := range store.WorktreeTasks(s, wt.Slug) {
				v.Tasks = append(v.Tasks, taskView{t.ID, t.Type, t.Status, t.TabID != "", t.Note})
			}
			view = append(view, v)
		}
		unread := store.UnreadCount()
		output(struct {
			Worktrees []wtView `json:"worktrees"`
			Unread    int      `json:"unread"`
		}{view, unread}, func() {
			if len(view) == 0 {
				fmt.Println("no worktrees; unread mail: 0")
				return
			}
			for _, wt := range view {
				pr := ""
				if wt.PR > 0 {
					pr = fmt.Sprintf("  #%d", wt.PR)
				}
				fmt.Printf("%s  [%s]%s\n", wt.Slug, wt.Status, pr)
				for _, t := range wt.Tasks {
					mark := " "
					if t.Running {
						mark = "*"
					}
					fmt.Printf("  %s %s %-6s %-12s %s\n", mark, t.ID, t.Type, t.Status, oneLine(t.Note))
				}
			}
			fmt.Printf("unread mail: %d (foreman mailbox inbox)\n", unread)
		})
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
