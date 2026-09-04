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
			p, wt, err := resolvePipeline(s, args[0])
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
				Hold     string        `json:"hold,omitempty"`
				Reports  []reportView  `json:"reports"`
				Tasks    []*store.Task `json:"tasks"`
			}
			v := view{Worktree: p.Worktree, IssueID: p.IssueID, Half: p.Half, Hold: wt.Hold, Tasks: tasks}
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
			p, _, err := resolvePipeline(s, args[0])
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
			p, _, err := resolvePipeline(s, args[0])
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

	status := &cobra.Command{
		Use:   "status <worktree>",
		Short: "Render the pipeline progress lines (plan/build/pr) from state",
		Long: "The progress view, derived — not hand-drawn: done stages (●), the\n" +
			"running stage with its task id and round suffix (◉), blocked or\n" +
			"failed (✗), not started (○), per half. Halves the cursor has passed\n" +
			"are shown complete; respond and review annotate their line. Show\n" +
			"this output verbatim on every pipeline update instead of\n" +
			"recomposing it by hand.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := store.Load()
			if err != nil {
				return err
			}
			p, wt, err := resolvePipeline(s, args[0])
			if err != nil {
				return err
			}
			lines := renderPipelineStatus(s, wt, p)
			output(struct {
				Worktree string   `json:"worktree"`
				Half     string   `json:"half"`
				Hold     string   `json:"hold,omitempty"`
				Lines    []string `json:"lines"`
			}{p.Worktree, p.Half, wt.Hold, lines}, func() {
				for _, l := range lines {
					fmt.Println(l)
				}
			})
			return nil
		},
	}

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Track each worktree's pipeline: current half and output documents",
	}
	cmd.AddCommand(list, add, get, status, update, report)
	return cmd
}

// renderPipelineStatus derives the progress lines from state: the pipeline
// cursor (half), the worktree (PR, status), and tasks grouped by stage (the
// Stage tag, falling back to slug stems for tasks created before it
// existed). Rounds come from the auto-appended -rN slug suffix.
func renderPipelineStatus(s *store.State, wt *store.Worktree, p *store.Pipeline) []string {
	tasks := store.WorktreeTasks(s, wt.Slug)
	byStage := func(match func(*store.Task) bool) *store.Task {
		var latest *store.Task
		for _, t := range tasks {
			if match(t) && (latest == nil || t.ID > latest.ID) {
				latest = t
			}
		}
		return latest
	}
	is := func(stage string) func(*store.Task) bool {
		return func(t *store.Task) bool { return t.Stage == stage }
	}
	// slug-stem fallbacks cover tasks created before --stage existed
	doc := byStage(is("doc"))
	if doc == nil {
		doc = byStage(func(t *store.Task) bool {
			return t.Stage == "" && t.Type == "build" && strings.HasPrefix(t.Slug, "doc-")
		})
	}
	lint := byStage(is("lint"))
	if lint == nil {
		lint = byStage(func(t *store.Task) bool {
			return t.Stage == "" && t.Type == "test" && strings.HasPrefix(t.Slug, "lint")
		})
	}
	noStage := func(typ string) func(*store.Task) bool {
		return func(t *store.Task) bool { return t.Stage == "" && t.Type == typ }
	}
	pick := func(stage, typ string, tagged *store.Task) *store.Task {
		if tagged != nil {
			return tagged
		}
		return byStage(noStage(typ))
	}
	stages := map[string]*store.Task{
		"plan": pick("plan", "plan", byStage(is("plan"))),
		"build": byStage(func(t *store.Task) bool {
			return t.Stage == "build" || (t.Stage == "" && t.Type == "build" && !strings.HasPrefix(t.Slug, "doc-"))
		}),
		"test": byStage(func(t *store.Task) bool {
			return t.Stage == "test" || (t.Stage == "" && t.Type == "test" && !strings.HasPrefix(t.Slug, "lint"))
		}),
		"review":  pick("review", "review", byStage(is("review"))),
		"fix":     byStage(is("fix")),
		"doc":     doc,
		"lint":    lint,
		"respond": pick("respond", "respond", byStage(is("respond"))),
	}

	researchRunning := false
	researchDone := false
	for _, t := range tasks {
		if t.Type != "research" {
			continue
		}
		if t.Status == store.TaskDone || t.Status == store.TaskFailed {
			researchDone = true
		} else {
			researchRunning = true
		}
	}

	mark := func(t *store.Task) string {
		if t == nil {
			return "○"
		}
		switch t.Status {
		case store.TaskDone:
			return "●"
		case store.TaskBlocked, store.TaskFailed:
			return "✗"
		default:
			return "◉"
		}
	}
	detail := func(t *store.Task) string {
		if t == nil {
			return ""
		}
		d := " (" + t.ID
		if r := roundSuffix(t.Slug); r != "" {
			d += " " + r
		}
		switch t.Status {
		case store.TaskDone:
			d += " ✓"
		case store.TaskBlocked:
			d += " blocked"
		case store.TaskFailed:
			d += " failed"
		}
		return d + ")"
	}
	// doneHalf: handovers imply completion of every earlier half
	doneHalf := func(h string) bool {
		cur := p.Half
		if cur == "respond" {
			cur = "pr"
		}
		if cur == "review" {
			return false
		}
		if cur == "done" {
			return true
		}
		return (cur == "pr" && h != "pr") || (cur == "build" && h == "plan")
	}

	issueMark := "○"
	if wt.IssueID != "" || wt.Path != "" {
		issueMark = "●"
	}
	wtMark := "○"
	if wt.Path != "" {
		wtMark = "●"
	}
	resMark := "○"
	if researchRunning {
		resMark = "◉"
	} else if researchDone {
		resMark = "●"
	}
	if doneHalf("plan") {
		issueMark, wtMark, resMark = "●", "●", "●"
	}
	planMark := mark(stages["plan"])
	if doneHalf("plan") {
		planMark = "●"
	}
	planLine := "plan:   " + issueMark + " ISSUE ── " + wtMark + " WORKTREE ── " + resMark + " RESEARCH ── " + planMark + " PLAN" + detail(stages["plan"])

	buildMark, testMark, reviewMark := mark(stages["build"]), mark(stages["test"]), mark(stages["review"])
	if doneHalf("build") {
		buildMark, testMark, reviewMark = "●", "●", "●"
	}
	buildLine := "build:  " + buildMark + " BUILD" + detail(stages["build"]) +
		" ── " + testMark + " TEST" + detail(stages["test"]) +
		" ── " + reviewMark + " REVIEW" + detail(stages["review"])

	prMark, prName := "○", "PR"
	if wt.PR > 0 {
		prMark, prName = "●", fmt.Sprintf("PR#%d", wt.PR)
	}
	ciMark := "○"
	if wt.Status == store.WtReadyForMerge || wt.Status == store.WtDone {
		ciMark = "●"
	}
	watchMark := "○"
	if wt.PR > 0 && wt.Status != store.WtDone {
		watchMark = "◉"
	}
	mergedMark := "○"
	if wt.Status == store.WtDone {
		mergedMark = "●"
		watchMark = "●"
	}
	if p.Half == "done" {
		ciMark, watchMark, mergedMark = "●", "●", "●"
	}
	prLine := "pr:     " + mark(stages["doc"]) + " DOC" + detail(stages["doc"]) +
		" ── " + mark(stages["lint"]) + " LINT" + detail(stages["lint"]) +
		" ── " + prMark + " " + prName +
		" ── " + ciMark + " CI ── " + watchMark + " WATCH ── " + mergedMark + " MERGED"

	lines := []string{planLine, buildLine, prLine}
	if p.Half == "respond" {
		lines = append(lines, "respond: ◉ RESPOND"+detail(stages["respond"])+" (pr half's WATCH loop)")
	}
	if p.Half == "review" {
		lines = append(lines, "review: ◉ REVIEW"+detail(stages["review"])+" — confirm/post/cleanup are agent-side (references/review.md)")
	}
	return lines
}

func roundSuffix(slug string) string {
	i := strings.LastIndex(slug, "-r")
	if i < 0 || i+2 >= len(slug) {
		return ""
	}
	for _, c := range slug[i+2:] {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return "r" + slug[i+2:]
}

func validPipelineHalf(h string) bool {
	for _, v := range pipelineHalves {
		if v == h {
			return true
		}
	}
	return false
}

func resolvePipeline(s *store.State, ref string) (*store.Pipeline, *store.Worktree, error) {
	wt, err := store.ResolveWorktree(s, ref)
	if err != nil {
		return nil, nil, err
	}
	p, ok := s.Pipelines[wt.Slug]
	if !ok {
		return nil, nil, fmt.Errorf("worktree %s has no pipeline; run `pipeline add %s`", wt.Slug, wt.Slug)
	}
	return p, wt, nil
}

var pipelineGetText = template.Must(template.New("pipeline").Parse(`Worktree:  {{.V.Worktree}}
{{- if .V.IssueID}}
Issue:     {{.V.IssueID}}
{{- end}}
Half:      {{.V.Half}}
Updated:   {{.Updated}}
{{- if .V.Hold}}
Hold:      {{.V.Hold}}
{{- end}}
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
