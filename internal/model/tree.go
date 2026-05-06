package model

import (
	"fmt"
	"strings"
)

type RowKind int

const (
	RowProject RowKind = iota
	RowPipeline
	RowJob
)

type Row struct {
	Kind    RowKind
	Indent  int
	Key     string
	ProjIdx int
	PipeIdx int
	JobIdx  int
}

type Tree struct {
	Projects  []Project
	collapsed map[string]bool
	cursor    int
	// focus is empty for "show all projects". When set to a project slug,
	// Visible() returns rows only for that project.
	focus string
	// textFilter is a case-insensitive substring matched against branch,
	// ticket, status, project name, and pipeline number. Empty = match all.
	textFilter string
	// mineOnly + mineActor restrict to pipelines whose actor login matches.
	mineOnly  bool
	mineActor string
}

func NewTree(projects []Project) *Tree {
	return &Tree{
		Projects:  projects,
		collapsed: make(map[string]bool),
	}
}

// AdoptCollapsedFrom copies the previous tree's collapsed state into this one.
// Used after a refresh to preserve the user's expand/collapse choices —
// without this, every 30s tick blows away their carefully curated view.
// Stable keys (project slug, pipeline/job ID) make this safe across refreshes
// even when pipelines reorder.
func (t *Tree) AdoptCollapsedFrom(prev *Tree) {
	if prev == nil {
		return
	}
	for k, v := range prev.collapsed {
		t.collapsed[k] = v
	}
}

// ExpandAll clears the collapsed state for every row — the default is now
// "everything expanded", so this just resets back to the open view.
func (t *Tree) ExpandAll() {
	t.collapsed = make(map[string]bool)
}

// CollapseAll folds every project and pipeline. Jobs aren't collapsible.
func (t *Tree) CollapseAll() {
	for pi, proj := range t.Projects {
		t.collapsed[projectKey(proj)] = true
		for _, pl := range proj.Pipelines {
			t.collapsed[pipelineKey(pl)] = true
		}
		_ = pi
	}
}

// Focus returns the currently focused project slug, or empty if none.
func (t *Tree) Focus() string { return t.focus }

// TextFilter returns the active substring filter, or empty.
func (t *Tree) TextFilter() string { return t.textFilter }

// MineOnly reports whether the "show only my pipelines" filter is active.
func (t *Tree) MineOnly() bool { return t.mineOnly }

// SetTextFilter installs a case-insensitive substring filter and resets the
// cursor to the top of the visible set so the user doesn't end up pointed at
// nothing.
func (t *Tree) SetTextFilter(s string) {
	if t.textFilter == s {
		return
	}
	t.textFilter = s
	t.cursor = 0
}

// SetMineOnly toggles the "mine only" filter. actor is the login string we
// match against pipeline.Actor — typically derived from CIRCLECI_TUI_MINE,
// $USER, or git config user.email's local-part.
func (t *Tree) SetMineOnly(on bool, actor string) {
	if t.mineOnly == on && t.mineActor == actor {
		return
	}
	t.mineOnly = on
	t.mineActor = actor
	t.cursor = 0
}

// HasFilters reports whether any pipeline-narrowing filter is active. Used by
// the view to decide whether to show "no matches" empty-state copy.
func (t *Tree) HasFilters() bool {
	return t.textFilter != "" || t.mineOnly
}

// pipelineMatches returns true when the pipeline survives the current
// filters. A pipeline with no matches is hidden along with its jobs; a project
// with zero surviving pipelines is also hidden.
func (t *Tree) pipelineMatches(p Pipeline, projectName string) bool {
	if t.mineOnly && !equalActor(p.Actor, t.mineActor) {
		return false
	}
	if t.textFilter == "" {
		return true
	}
	q := strings.ToLower(t.textFilter)
	hay := []string{
		p.Branch, p.Ticket, p.Actor, projectName, string(p.Status),
		fmt.Sprintf("#%d", p.Number),
	}
	for _, h := range hay {
		if strings.Contains(strings.ToLower(h), q) {
			return true
		}
	}
	return false
}

func equalActor(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// SetFocus restricts the tree to rows from a single project (by slug). Pass
// "" to clear focus and show all projects again. Cursor is reset to the top
// of the visible list.
func (t *Tree) SetFocus(slug string) {
	if t.focus == slug {
		return
	}
	t.focus = slug
	t.cursor = 0
}

func (t *Tree) IsCollapsed(key string) bool { return t.collapsed[key] }

func projectKey(p Project) string  { return "P:" + p.Slug }
func pipelineKey(p Pipeline) string { return "L:" + p.ID }
func jobKey(j Job) string           { return "J:" + j.ID }

func (t *Tree) Cursor() int { return t.cursor }

func (t *Tree) Visible() []Row {
	rows := make([]Row, 0, 32)
	for pi, proj := range t.Projects {
		if t.focus != "" && proj.Slug != t.focus {
			continue
		}
		// Pre-compute matching pipelines so we can skip the project header
		// when nothing inside it survives the filters.
		matching := make([]int, 0, len(proj.Pipelines))
		for li, pl := range proj.Pipelines {
			if t.pipelineMatches(pl, proj.Name) {
				matching = append(matching, li)
			}
		}
		if t.HasFilters() && len(matching) == 0 {
			continue
		}
		pKey := projectKey(proj)
		rows = append(rows, Row{
			Kind: RowProject, Indent: 0, Key: pKey, ProjIdx: pi, PipeIdx: -1, JobIdx: -1,
		})
		if t.collapsed[pKey] {
			continue
		}
		for _, li := range matching {
			pl := proj.Pipelines[li]
			lKey := pipelineKey(pl)
			rows = append(rows, Row{
				Kind: RowPipeline, Indent: 1, Key: lKey,
				ProjIdx: pi, PipeIdx: li, JobIdx: -1,
			})
			if t.collapsed[lKey] {
				continue
			}
			for ji, job := range pl.Jobs {
				rows = append(rows, Row{
					Kind: RowJob, Indent: 2 + job.Depth, Key: jobKey(job),
					ProjIdx: pi, PipeIdx: li, JobIdx: ji,
				})
			}
		}
	}
	return rows
}

func (t *Tree) Up() {
	if t.cursor > 0 {
		t.cursor--
	}
}

func (t *Tree) Down() {
	if t.cursor < len(t.Visible())-1 {
		t.cursor++
	}
}

func (t *Tree) ToggleCurrent() {
	rows := t.Visible()
	if len(rows) == 0 {
		return
	}
	row := rows[t.cursor]
	if row.Kind == RowJob {
		return
	}
	t.collapsed[row.Key] = !t.collapsed[row.Key]
}

func (t *Tree) Current() (Row, bool) {
	rows := t.Visible()
	if len(rows) == 0 {
		return Row{}, false
	}
	if t.cursor >= len(rows) {
		t.cursor = len(rows) - 1
	}
	return rows[t.cursor], true
}

func (t *Tree) PipelineAt(r Row) (Pipeline, bool) {
	if r.PipeIdx < 0 {
		return Pipeline{}, false
	}
	return t.Projects[r.ProjIdx].Pipelines[r.PipeIdx], true
}

func (t *Tree) JobAt(r Row) (Job, bool) {
	if r.JobIdx < 0 {
		return Job{}, false
	}
	return t.Projects[r.ProjIdx].Pipelines[r.PipeIdx].Jobs[r.JobIdx], true
}
