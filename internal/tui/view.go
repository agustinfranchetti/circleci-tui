package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

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

// renderMainBody is the tree (left) plus the optional log panel (right). When
// the panel is closed it's just the tree at full width. When the panel's open
// and the terminal is wide enough we split 50/50; below the panel's minimum
// width we hide the tree and let the panel take the whole row.
func (m Model) renderMainBody() string {
	tree := m.renderTree()
	if tree == "" && m.Tree.HasFilters() {
		tree = m.Theme.Muted.Render("no pipelines match the current filter")
	}
	if !m.logs.open {
		return tree
	}
	pw, _ := m.panelDimensions()
	panel := m.logs.render(m.Theme)
	if pw == m.Width {
		// Narrow terminal — panel only.
		return panel
	}
	treeW := m.Width - pw - 1
	if treeW < 20 {
		return panel
	}
	left := lipgloss.NewStyle().Width(treeW).Render(tree)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", panel)
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
	var b strings.Builder
	for i, row := range rows {
		line := m.renderRow(row)
		if i == cursor {
			line = m.Theme.Selected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
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
	when := m.Theme.Muted.Render(humanizeAgo(pl.CreatedAt))

	parts := []string{chevron, glyph, number, branch}
	if pl.Ticket != "" && pl.Ticket != pl.Branch {
		parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Ticket.Render(pl.Ticket))
	}
	parts = append(parts, m.Theme.Muted.Render("·"), when)
	return strings.Join(parts, " ")
}

func (m Model) renderJob(r model.Row) string {
	job, ok := m.Tree.JobAt(r)
	if !ok {
		return ""
	}
	statusStyle := m.Theme.StatusStyle(job.Status)
	glyph := statusStyle.Render(job.Status.Glyph())
	name := lipgloss.NewStyle().Width(10).Render(job.Name)
	hint := ""
	if job.Hint != "" {
		hint = " " + m.Theme.Muted.Render(job.Hint)
	}
	return fmt.Sprintf("%s %s%s", glyph, name, hint)
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
