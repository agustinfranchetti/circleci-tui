package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

const (
	indentUnit = "  "
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.renderTitle())
	b.WriteString("\n")
	if m.filterEditing {
		b.WriteString(m.filterInput.View())
		b.WriteString("\n")
	}
	switch {
	case m.actions.open:
		b.WriteString(m.renderActionsMenu())
	case m.picker.open:
		b.WriteString(m.renderPicker())
	case m.svc != nil && len(m.Tree.Projects) == 0:
		b.WriteString(m.renderEmptyLiveState())
	default:
		b.WriteString(m.renderMainBody())
	}
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	return b.String()
}

// renderMainBody shows either the tree or the log panel — never both. The
// previous side-by-side split caused tree lines to word-wrap mid-job-name once
// the panel ate half the width; a full-screen panel keeps log reading roomy
// and the tree wrap-free. Press `e` to toggle.
func (m Model) renderMainBody() string {
	if m.logs.open {
		return m.logs.render(m.Theme)
	}
	tree := m.renderTree()
	if tree == "" && m.Tree.HasFilters() {
		tree = m.Theme.Muted.Render("no pipelines match the current filter")
	}
	return tree
}

func (m Model) renderTitle() string {
	title := m.Theme.Title.Render(" circleci-tui ")
	var badges []string
	if focus := m.Tree.Focus(); focus != "" {
		badges = append(badges, m.Theme.Ticket.Render(m.findProjectName(focus)))
	}
	if m.Tree.MineOnly() {
		actor := m.mineActor
		if actor == "" {
			actor = "?"
		}
		badges = append(badges, m.Theme.Ticket.Render("mine="+actor))
	}
	if f := m.Tree.TextFilter(); f != "" {
		badges = append(badges, m.Theme.Ticket.Render("filter:"+f))
	}
	if len(badges) > 0 {
		title += m.Theme.Muted.Render(" · ") + strings.Join(badges, m.Theme.Muted.Render(" · "))
		title += m.Theme.Muted.Render(" (esc to clear)")
	}
	return title
}

func (m Model) findProjectName(slug string) string {
	for _, p := range m.Tree.Projects {
		if p.Slug == slug {
			return p.Name
		}
	}
	return slug
}

func (m Model) renderEmptyLiveState() string {
	if m.loadErr != nil {
		return m.Theme.Failed.Render("✗ ") + m.Theme.Muted.Render(m.loadErr.Error())
	}
	if m.loading {
		return m.Theme.Running.Render("⟳ ") + m.Theme.Muted.Render("loading pipelines…")
	}
	return m.Theme.Muted.Render("no projects to watch — run `circleci-tui config`")
}

func (m Model) renderTree() string {
	rows := m.Tree.Visible()
	cursor := m.Tree.Cursor()
	start, end := m.treeWindow(len(rows), cursor)
	var b strings.Builder
	for i := start; i < end; i++ {
		line := m.renderRow(rows[i])
		if i == cursor {
			line = m.Theme.Selected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// treeWindow picks a [start,end) slice of the visible rows that fits in the
// space available for the tree, biased to keep the cursor visible. Without
// this, View() emits more rows than the terminal can show and the user is
// stuck looking at the bottom of the list with the cursor scrolled off-screen.
//
// When m.Height is 0 (tests, or before the first WindowSizeMsg) we render
// everything — viewport math only kicks in once we know the terminal size.
func (m Model) treeWindow(total, cursor int) (int, int) {
	if m.Height <= 0 {
		return 0, total
	}
	avail := m.treeHeight()
	if total <= avail {
		return 0, total
	}
	half := avail / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	end := start + avail
	if end > total {
		end = total
		start = end - avail
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

// treeHeight returns how many rows we can devote to the tree given the current
// terminal size, after subtracting fixed chrome (title, status bar, filter
// input when active).
func (m Model) treeHeight() int {
	chrome := 3 // title line + blank separator + status bar
	if m.filterEditing {
		chrome++
	}
	avail := m.Height - chrome
	if avail < 5 {
		avail = 5
	}
	return avail
}

func (m Model) renderRow(r model.Row) string {
	indent := strings.Repeat(indentUnit, r.Indent)
	switch r.Kind {
	case model.RowProject:
		return indent + m.renderProject(r)
	case model.RowPipeline:
		return indent + m.renderPipeline(r)
	case model.RowJob:
		return indent + m.renderJob(r)
	}
	return ""
}

func (m Model) renderProject(r model.Row) string {
	proj := m.Tree.Projects[r.ProjIdx]
	chevron := "▶"
	if !m.Tree.IsCollapsed(r.Key) {
		chevron = "▼"
	}
	count := projectCount(proj)
	countStyle := m.Theme.StatusStyle(proj.WorstStatus())
	header := m.Theme.Header.Render(proj.Name)
	return fmt.Sprintf("%s %s %s",
		chevron,
		header,
		countStyle.Render(count),
	)
}

func projectCount(p model.Project) string {
	switch {
	case p.FailedCount() > 0:
		return fmt.Sprintf("(%d failed)", p.FailedCount())
	case p.RunningCount() > 0:
		return fmt.Sprintf("(%d running)", p.RunningCount())
	default:
		return fmt.Sprintf("(%d ok)", len(p.Pipelines))
	}
}

func (m Model) renderPipeline(r model.Row) string {
	pl, ok := m.Tree.PipelineAt(r)
	if !ok {
		return ""
	}
	chevron := " "
	if len(pl.Jobs) > 0 {
		if m.Tree.IsCollapsed(r.Key) {
			chevron = "▶"
		} else {
			chevron = "▼"
		}
	}
	statusStyle := m.Theme.StatusStyle(pl.Status)
	glyph := statusStyle.Render(pl.Status.Glyph())
	number := m.Theme.Muted.Render(fmt.Sprintf("#%d", pl.Number))
	branch := m.Theme.Branch.Render(pl.Branch)

	parts := []string{chevron, glyph, number, branch}
	if pl.Ticket != "" && pl.Ticket != pl.Branch {
		parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Ticket.Render(pl.Ticket))
	}
	if pl.Actor != "" {
		parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Muted.Render("by "+pl.Actor))
	}
	parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Muted.Render(pipelineTime(pl)))
	return strings.Join(parts, " ")
}

// pipelineTime renders "running for 2m · started 5m ago", "took 4m 30s · 2h
// ago", or just "5m ago" when we have no duration data yet.
func pipelineTime(pl model.Pipeline) string {
	ago := humanizeAgo(pl.CreatedAt)
	dur := pl.Duration()
	if dur <= 0 {
		return ago
	}
	if pl.Status == model.StatusRunning {
		return "running for " + humanizeDuration(dur) + " · " + ago
	}
	return "took " + humanizeDuration(dur) + " · " + ago
}

func (m Model) renderJob(r model.Row) string {
	job, ok := m.Tree.JobAt(r)
	if !ok {
		return ""
	}
	statusStyle := m.Theme.StatusStyle(job.Status)
	glyph := statusStyle.Render(job.Status.Glyph())
	parts := []string{glyph, job.Name}
	if d := jobTime(job); d != "" {
		parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Muted.Render(d))
	}
	if job.Hint != "" {
		parts = append(parts, m.Theme.Muted.Render(job.Hint))
	}
	return strings.Join(parts, " ")
}

// jobTime returns "2m 14s", "running 1m 30s", or empty if no timing yet.
func jobTime(job model.Job) string {
	dur := job.Duration()
	if dur <= 0 {
		return ""
	}
	if job.Status == model.StatusRunning {
		return "running " + humanizeDuration(dur)
	}
	return humanizeDuration(dur)
}

// humanizeDuration formats a duration as "1h 23m", "4m 12s", or "37s" — short
// enough for status lines, lossy enough that we don't worry about ms.
func humanizeDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	mm := int(d.Minutes()) - h*60
	if mm == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, mm)
}

func (m Model) renderStatusBar() string {
	if m.filterEditing {
		keys := []struct{ k, label string }{
			{"type", "filter"},
			{"⏎", "apply"},
			{"esc", "clear"},
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts,
				m.Theme.StatusKey.Render(k.k)+" "+m.Theme.StatusInfo.Render(k.label))
		}
		return m.Theme.StatusBar.Render(strings.Join(parts, "  "))
	}
	if m.picker.open {
		keys := []struct{ k, label string }{
			{"↑↓", "nav"},
			{"⏎", "select"},
			{"esc", "cancel"},
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts,
				m.Theme.StatusKey.Render(k.k)+" "+m.Theme.StatusInfo.Render(k.label))
		}
		return m.Theme.StatusBar.Render(strings.Join(parts, "  "))
	}
	keys := []struct{ k, label string }{
		{"↑↓", "nav"},
		{"⏎", "expand"},
		{"+/-", "all"},
		{"tab", "focus"},
		{"/", "filter"},
		{"m", "mine"},
		{"e", "logs"},
		{"a", "actions"},
		{"r", "refresh"},
		{"q", "quit"},
	}
	if m.logs.open {
		keys = append(keys[:len(keys)-1],
			struct{ k, label string }{"PgUp/Dn", "scroll"},
			struct{ k, label string }{"q", "quit"},
		)
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts,
			m.Theme.StatusKey.Render(k.k)+" "+m.Theme.StatusInfo.Render(k.label))
	}
	left := m.Theme.StatusBar.Render(strings.Join(parts, "  "))
	right := m.renderRefreshIndicator()
	if toast := m.activeToast(); toast != "" {
		// Toast trumps the refresh indicator while it's active — both are
		// transient, ephemeral status, no point fighting for the same slot.
		right = toast
	}
	if right == "" {
		return left
	}
	return left + "   " + right
}

func (m Model) activeToast() string {
	if m.toast == "" || time.Now().After(m.toastUntil) {
		return ""
	}
	if strings.HasPrefix(m.toast, "✓") {
		return m.Theme.Success.Render(m.toast)
	}
	if strings.HasPrefix(m.toast, "✗") {
		return m.Theme.Failed.Render(m.toast)
	}
	return m.Theme.Muted.Render(m.toast)
}

func (m Model) renderRefreshIndicator() string {
	if m.svc == nil {
		return ""
	}
	if m.loading {
		return m.spinner.View() + m.Theme.Muted.Render(" refreshing…")
	}
	if m.loadErr != nil {
		return m.Theme.Failed.Render("✗ " + m.loadErr.Error())
	}
	if m.lastLoad.IsZero() {
		return ""
	}
	return m.Theme.Muted.Render("updated " + humanizeAgo(m.lastLoad))
}

func humanizeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
