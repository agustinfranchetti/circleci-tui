package tui

import (
	"fmt"
	"math"
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
	switch {
	case m.filter.open:
		b.WriteString(m.filter.render(m.Theme, m.Width, m.Height))
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

// renderMainBody shows either the tree or the step-detail panel — never both.
// `e` toggles. Full-screen panel avoids the wrap problems that the
// side-by-side split had.
func (m Model) renderMainBody() string {
	if m.detail.open {
		return m.detail.render(m.Theme, m.statusGlyph)
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
	if s := m.Tree.StatusFilter(); s != "" {
		badges = append(badges, m.Theme.StatusStyle(s).Render("status:"+string(s)))
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
		return m.renderLoadingAnim()
	}
	return m.Theme.Muted.Render("no projects to watch — run `circleci-tui config`")
}

// renderLoadingAnim draws a centred ocean animation: one wavy surface line
// rippling left-to-right, with a fixed top-to-bottom cyan-to-teal gradient
// behind it. The compound-sine surface mixes two waves at different speeds
// so the ripple pattern doesn't repeat obviously, while every row below the
// surface stays a solid colour — that combination ends up reading like
// water far better than overlapping translucent layers did in the terminal.
func (m Model) renderLoadingAnim() string {
	if m.Width <= 0 {
		return m.Theme.Running.Render("⟳ ") + m.Theme.Muted.Render("loading pipelines…")
	}
	width := 60
	if avail := m.Width - 4; avail < width {
		width = avail
	}
	if width < 20 {
		width = 20
	}
	const animHeight = 9
	// One colour per row, top → bottom. The wave's surface only rides
	// rows ~2–4; everything below is a solid block in the row's colour.
	rowColors := []lipgloss.AdaptiveColor{
		{Dark: "#67e8f9", Light: "#22d3ee"},
		{Dark: "#22d3ee", Light: "#06b6d4"},
		{Dark: "#06b6d4", Light: "#0891b2"},
		{Dark: "#0891b2", Light: "#0e7490"},
		{Dark: "#0e7490", Light: "#155e75"},
		{Dark: "#155e75", Light: "#164e63"},
		{Dark: "#164e63", Light: "#0c4a6e"},
		{Dark: "#0c4a6e", Light: "#0c4a6e"},
		{Dark: "#082f49", Light: "#082f49"},
	}

	bars := []rune("▁▂▃▄▅▆▇█")
	frame := float64(m.loadFrame)

	leftPad := (m.Width - width) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// Surface position: compound sine for a non-repeating ripple.
	surface := func(x int) float64 {
		fx := float64(x)
		return 3.0 +
			1.0*math.Sin((fx+frame*0.55)*0.16) +
			0.5*math.Sin((fx+frame*0.27)*0.31)
	}

	var b strings.Builder
	b.WriteString("\n\n")
	for y := 0; y < animHeight; y++ {
		b.WriteString(strings.Repeat(" ", leftPad))
		col := rowColors[y]
		for x := 0; x < width; x++ {
			yWave := surface(x)
			cellTop := float64(y)
			cellBottom := cellTop + 1
			if cellBottom <= yWave {
				b.WriteString(" ") // sky
				continue
			}
			if cellTop >= yWave {
				b.WriteString(lipgloss.NewStyle().Foreground(col).Render("█"))
				continue
			}
			filled := cellBottom - yWave
			idx := int(math.Round(filled * 8))
			if idx < 1 {
				idx = 1
			} else if idx > 8 {
				idx = 8
			}
			b.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(bars[idx-1])))
		}
		b.WriteString("\n")
	}

	// Caption + rotating bright-dot indicator, centred.
	b.WriteString("\n")
	caption := m.Theme.Muted.Render("fetching pipelines from CircleCI")
	dots := renderLoadingDots(m.loadFrame, m.Theme)
	captionLine := caption + "  " + dots
	capPad := (m.Width - lipgloss.Width(captionLine)) / 2
	if capPad < 0 {
		capPad = 0
	}
	b.WriteString(strings.Repeat(" ", capPad))
	b.WriteString(captionLine)
	return b.String()
}

// renderLoadingDots draws three pellets where one is brighter than the
// others, with the highlight rotating left-to-right every few frames. Mirrors
// the reference mockup's blinking-dots feel without timing-based opacity.
func renderLoadingDots(frame int, theme Theme) string {
	pos := (frame / 4) % 3
	var b strings.Builder
	for i := 0; i < 3; i++ {
		if i == pos {
			b.WriteString(theme.Running.Render("●"))
		} else {
			b.WriteString(theme.Muted.Render("○"))
		}
		if i < 2 {
			b.WriteString(" ")
		}
	}
	return b.String()
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

// treeHeight returns how many rows we can devote to the tree given the
// current terminal size, after subtracting fixed chrome (title + blank
// separator + status bar).
func (m Model) treeHeight() int {
	chrome := 3
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
	case model.RowWorkflow:
		return indent + m.renderWorkflow(r)
	case model.RowJob:
		return indent + m.renderJob(r)
	}
	return ""
}

// renderWorkflow draws a workflow header. Only shown when a pipeline has more
// than one workflow (the rerun case) — otherwise the renderer skips this row
// entirely to avoid noise.
func (m Model) renderWorkflow(r model.Row) string {
	wf, ok := m.Tree.WorkflowAt(r)
	if !ok {
		return ""
	}
	chevron := " "
	if len(wf.Jobs) > 0 {
		if m.Tree.IsCollapsed(r.Key) {
			chevron = "▶"
		} else {
			chevron = "▼"
		}
	}
	glyph := m.statusGlyph(wf.Status)
	nameStyle := m.Theme.Header
	switch wf.Status {
	case model.StatusFailed, model.StatusRunning, model.StatusOnHold:
		nameStyle = m.Theme.StatusStyle(wf.Status).Bold(true)
	}
	name := nameStyle.Render(wf.Name)
	parts := []string{chevron, glyph, name}
	if wf.IsRerun {
		parts = append(parts, m.Theme.Ticket.Render("(rerun)"))
	}
	parts = append(parts, m.Theme.Muted.Render("·"), m.Theme.Muted.Render(workflowTime(wf)))
	return strings.Join(parts, " ")
}

func workflowTime(wf model.Workflow) string {
	dur := wf.Duration()
	if dur <= 0 {
		return humanizeAgo(wf.CreatedAt)
	}
	if wf.Status == model.StatusRunning {
		return "running " + humanizeDuration(dur)
	}
	return "took " + humanizeDuration(dur)
}

// statusGlyph returns the visual marker for a status — animated when the
// status is Running (the bubbles spinner ticks every frame), static for
// finished statuses. All renderers go through this so a single global
// spinner state animates every running row in lock-step.
func (m Model) statusGlyph(s model.Status) string {
	if s == model.StatusRunning {
		return m.spinner.View()
	}
	return m.Theme.StatusStyle(s).Render(s.Glyph())
}

// emphasizeNumber renders a pipeline number with status-tinted bold when the
// status is "interesting" (failed/running/on-hold) — makes those rows pop in
// a long mixed list.
func (m Model) emphasizeNumber(s model.Status, n int) string {
	switch s {
	case model.StatusFailed, model.StatusRunning, model.StatusOnHold:
		return m.Theme.StatusStyle(s).Bold(true).Render(fmt.Sprintf("#%d", n))
	}
	return m.Theme.Muted.Render(fmt.Sprintf("#%d", n))
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
	if len(pl.Workflows) > 0 {
		if m.Tree.IsCollapsed(r.Key) {
			chevron = "▶"
		} else {
			chevron = "▼"
		}
	}
	glyph := m.statusGlyph(pl.Status)
	number := m.emphasizeNumber(pl.Status, pl.Number)
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
	glyph := m.statusGlyph(job.Status)
	nameStyle := lipgloss.NewStyle()
	switch job.Status {
	case model.StatusFailed:
		nameStyle = m.Theme.Failed
	case model.StatusRunning:
		nameStyle = m.Theme.Running
	}
	tree := m.Theme.Divider.Render(job.TreePrefix)
	parts := []string{tree + glyph, nameStyle.Render(job.Name)}
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
	// During the initial load the user has nothing to do but wait, so we
	// strip the chrome down to just the quit hint instead of advertising
	// keys that won't do anything yet.
	if m.svc != nil && len(m.Tree.Projects) == 0 && m.loading {
		return m.Theme.StatusBar.Render(
			m.Theme.StatusKey.Render("q") + " " + m.Theme.StatusInfo.Render("quit"),
		)
	}
	if m.filter.open {
		keys := []struct{ k, label string }{
			{"↑↓", "row"},
			{"←→", "status"},
			{"⏎", "apply"},
			{"esc", "cancel"},
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts,
				m.Theme.StatusKey.Render(k.k)+" "+m.Theme.StatusInfo.Render(k.label))
		}
		sep := m.Theme.Divider.Render(" │ ")
		return m.Theme.StatusBar.Render(strings.Join(parts, sep))
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
	if m.detail.open {
		keys = []struct{ k, label string }{
			{"↑↓", "step"},
			{"⏎", "expand"},
			{"PgUp/Dn", "scroll"},
			{"esc/e", "close"},
			{"q", "quit"},
		}
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts,
			m.Theme.StatusKey.Render(k.k)+" "+m.Theme.StatusInfo.Render(k.label))
	}
	sep := m.Theme.Divider.Render(" │ ")
	left := m.Theme.StatusBar.Render(strings.Join(parts, sep))
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
