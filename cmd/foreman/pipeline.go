package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"assembly/internal/store"

	"github.com/spf13/cobra"
)

var pipelineHalves = []string{"plan", "build", "pr", "respond", "review", "done"}

type pipelineRow struct {
	Worktree string `json:"worktree"`
	IssueID  string `json:"issue,omitempty"`
	Half     string `json:"half"`
	Held     bool   `json:"held,omitempty"`
	Reports  int    `json:"reports"`
	Updated  string `json:"updated"`
}

func newPipelineCmd() *cobra.Command {
	var addHalf, updateHalf string

	list := &cobra.Command{
		Use:   "list",
		Short: "List pipelines (one per worktree)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			slugs := make([]string, 0, len(s.Pipelines))
			for slug := range s.Pipelines {
				slugs = append(slugs, slug)
			}
			sort.Strings(slugs)
			if len(slugs) == 0 {
				fmt.Println("no pipelines")
				return nil
			}
			rows := make([]pipelineRow, len(slugs))
			for i, slug := range slugs {
				p := s.Pipelines[slug]
				held := false
				if wt, ok := s.Worktrees[slug]; ok {
					held = wt.Hold != ""
				}
				rows[i] = pipelineRow{
					Worktree: p.Worktree, IssueID: p.IssueID, Half: p.Half,
					Held: held, Reports: len(p.Reports),
					Updated: p.Updated.Format("2006-01-02 15:04"),
				}
			}
			tableOutput(rows)
			return nil
		},
	}

	add := &cobra.Command{
		Use:   "add <worktree>",
		Short: "Register the worktree's pipeline (idempotent); starts at the plan half",
		Long: "Records the half-level cursor and document index for a worktree's gated\n" +
			"flow. Idempotent: an existing pipeline is returned unchanged. --half\n" +
			"starts elsewhere (e.g. review for a pr-<N> checkout).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if addHalf != "" && !validPipelineHalf(addHalf) {
				return fmt.Errorf("invalid half %q; valid: %s", addHalf, strings.Join(pipelineHalves, "|"))
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			wt, err := store.ResolveWorktree(s, args[0])
			if err != nil {
				return err
			}
			if p, ok := s.Pipelines[wt.Slug]; ok {
				output(p, func() { fmt.Printf("pipeline for %s already exists (half %s)\n", wt.Slug, p.Half) })
				return nil
			}
			half := addHalf
			if half == "" {
				half = "plan"
			}
			p := &store.Pipeline{Worktree: wt.Slug, IssueID: wt.IssueID, Half: half, Created: time.Now(), Updated: time.Now()}
			if flagDryRun {
				fmt.Printf("would add pipeline for %s (half %s)\n", wt.Slug, half)
				return nil
			}
			s.Pipelines[wt.Slug] = p
			if err := store.Save(s); err != nil {
				return err
			}
			output(p, func() { fmt.Printf("added pipeline for %s (half %s)\n", wt.Slug, half) })
			return nil
		},
	}
	add.Flags().StringVar(&addHalf, "half", "", "starting half: "+strings.Join(pipelineHalves, "|")+" (default plan)")

	get := &cobra.Command{
		Use:   "get <worktree>",
		Short: "Show a pipeline: half, recorded reports, and the worktree's tasks",
		Long: "The resume entrypoint: which half the flow is in, every output document\n" +
			"recorded so far (missing files marked), and the stage-level task list\n" +
			"(done tasks are done stages — never re-run them).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			p, err := resolvePipeline(s, args[0])
			if err != nil {
				return err
			}
			tasks := store.WorktreeTasks(s, p.Worktree)
			type reportView struct {
				Path   string `json:"path"`
				Exists bool   `json:"exists"`
			}
			type view struct {
				Worktree string        `json:"worktree"`
				IssueID  string        `json:"issue_id,omitempty"`
				Half     string        `json:"half"`
				Reports  []reportView  `json:"reports"`
				Tasks    []*store.Task `json:"tasks"`
			}
			v := view{Worktree: p.Worktree, IssueID: p.IssueID, Half: p.Half, Tasks: tasks}
			for _, r := range p.Reports {
				_, err := os.Stat(r)
				v.Reports = append(v.Reports, reportView{Path: r, Exists: err == nil})
			}
			output(v, func() {
				data := struct {
					V       view
					Updated string
				}{v, p.Updated.Format("2006-01-02 15:04")}
				_ = pipelineGetText.Execute(os.Stdout, data)
			})
			return nil
		},
	}

	update := &cobra.Command{
		Use:   "update <worktree> --half <h>",
		Short: "Move a pipeline to another half (the handover step)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if updateHalf == "" {
				return fmt.Errorf("--half is required: %s", strings.Join(pipelineHalves, "|"))
			}
			if !validPipelineHalf(updateHalf) {
				return fmt.Errorf("invalid half %q; valid: %s", updateHalf, strings.Join(pipelineHalves, "|"))
			}
			s, err := store.Load()
			if err != nil {
				return err
			}
			p, err := resolvePipeline(s, args[0])
			if err != nil {
				return err
			}
			old := p.Half
			if flagDryRun {
				fmt.Printf("would move pipeline %s: %s -> %s\n", p.Worktree, old, updateHalf)
				return nil
			}
			p.Half = updateHalf
			p.Updated = time.Now()
			if err := store.Save(s); err != nil {
				return err
			}
			output(p, func() { fmt.Printf("pipeline %s: %s -> %s\n", p.Worktree, old, updateHalf) })
			return nil
		},
	}
	update.Flags().StringVar(&updateHalf, "half", "", "new half: "+strings.Join(pipelineHalves, "|"))

	report := &cobra.Command{
		Use:   "report <worktree> <path>",
		Short: "Record an output document produced for the worktree's pipeline",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			p, err := resolvePipeline(s, args[0])
			if err != nil {
				return err
			}
			for _, r := range p.Reports {
				if r == args[1] {
					fmt.Printf("report already recorded: %s\n", r)
					return nil
				}
			}
			if flagDryRun {
				fmt.Printf("would record report for %s: %s\n", p.Worktree, args[1])
				return nil
			}
			p.Reports = append(p.Reports, args[1])
			p.Updated = time.Now()
			if err := store.Save(s); err != nil {
				return err
			}
			output(p, func() { fmt.Printf("recorded report for %s: %s\n", p.Worktree, args[1]) })
			return nil
		},
	}

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Track each worktree's pipeline: current half and output documents",
	}
	cmd.AddCommand(list, add, get, update, report)
	return cmd
}

func validPipelineHalf(h string) bool {
	for _, v := range pipelineHalves {
		if v == h {
			return true
		}
	}
	return false
}

func resolvePipeline(s *store.State, ref string) (*store.Pipeline, error) {
	wt, err := store.ResolveWorktree(s, ref)
	if err != nil {
		return nil, err
	}
	p, ok := s.Pipelines[wt.Slug]
	if !ok {
		return nil, fmt.Errorf("worktree %s has no pipeline; run `pipeline add %s`", wt.Slug, wt.Slug)
	}
	return p, nil
}

var pipelineGetText = template.Must(template.New("pipeline").Parse(`Worktree:  {{.V.Worktree}}
{{- if .V.IssueID}}
Issue:     {{.V.IssueID}}
{{- end}}
Half:      {{.V.Half}}
Updated:   {{.Updated}}
{{- if .V.Reports}}

Reports:
{{- range .V.Reports}}
  {{if .Exists}}[ok]{{else}}[missing]{{end}} {{.Path}}
{{- end}}
{{- end}}
{{- if .V.Tasks}}

Tasks:
{{- range .V.Tasks}}
  {{.ID}} {{.Type}} {{.Status}}{{if .Slug}} ({{.Slug}}){{end}}
{{- end}}
{{- end}}
`))
