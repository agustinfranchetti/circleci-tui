package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

// detailLoadedMsg lands when the v1.1 job-details fetch completes.
type detailLoadedMsg struct {
	jobKey string
	detail model.JobDetail
	err    error
}

// stepOutputLoadedMsg lands when an expanded step's raw output fetch
// completes.
type stepOutputLoadedMsg struct {
	jobKey string
	stepID int
	output string
	err    error
}

// jobDetailPanel renders the per-job step list with status icons, durations,
// and on-demand step-output expansion. Replaces the v0.1.x prose log panel —
// that one called the MCP server's `get_build_failure_logs`, which returned
// LLM-friendly text useless for a structured TUI.
type jobDetailPanel struct {
	open    bool
	loading bool
	err     error

	jobKey   string
	project  model.Project
	pipeline model.Pipeline
	workflow model.Workflow
	job      model.Job
	detail   model.JobDetail

	cursor          int
	expandedStepIdx int                    // -1 when no step is expanded
	expandedOutput  map[int]string         // stepID -> output text
	expandedErr     map[int]error          // stepID -> fetch error
	expandedLoading map[int]bool           // stepID -> in-flight flag
	viewport        viewport.Model
	width           int
	height          int
}

func newJobDetailPanel() jobDetailPanel {
	return jobDetailPanel{
		expandedStepIdx: -1,
		expandedOutput:  map[int]string{},
		expandedErr:     map[int]error{},
		expandedLoading: map[int]bool{},
	}
}

func (p *jobDetailPanel) openOn(jobKey string, project model.Project, pipeline model.Pipeline, workflow model.Workflow, job model.Job) {
	p.open = true
	p.loading = true
	p.err = nil
	p.jobKey = jobKey
	p.project = project
	p.pipeline = pipeline
	p.workflow = workflow
	p.job = job
	p.detail = model.JobDetail{}
	p.cursor = 0
	p.expandedStepIdx = -1
	p.expandedOutput = map[int]string{}
	p.expandedErr = map[int]error{}
	p.expandedLoading = map[int]bool{}
}

func (p *jobDetailPanel) close() {
	p.open = false
	p.loading = false
	p.err = nil
	p.jobKey = ""
}

func (p *jobDetailPanel) setDetail(jobKey string, detail model.JobDetail, err error) {
	if jobKey != p.jobKey {
		return
	}
	p.loading = false
	p.err = err
	p.detail = detail
}

func (p *jobDetailPanel) setStepOutput(jobKey string, stepID int, out string, err error) {
	if jobKey != p.jobKey {
		return
	}
	p.expandedLoading[stepID] = false
	if err != nil {
		p.expandedErr[stepID] = err
		return
	}
	p.expandedOutput[stepID] = out
}

func (p *jobDetailPanel) resize(w, h int) {
	p.width = w
	p.height = h
	innerW := w - 4 // border (2) + horizontal padding (2)
	if innerW < 20 {
		innerW = 20
	}
	// Reserve two header lines + one blank gap. The rest belongs to the
	// viewport that scrolls the step list and any expanded output.
	innerH := h - 5
	if innerH < 5 {
		innerH = 5
	}
	if p.viewport.Width != innerW || p.viewport.Height != innerH {
		p.viewport = viewport.New(innerW, innerH)
	}
}

// scroll forwards a key event to the viewport. Matches both the high-level
// msg.Type (more reliable across terminals) and the legacy msg.String() so
// we don't miss keystrokes on Mac/iTerm/Ghostty/etc.
func (p *jobDetailPanel) scroll(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyPgDown:
		p.viewport.PageDown()
		return
	case tea.KeyPgUp:
		p.viewport.PageUp()
		return
	case tea.KeyHome:
		p.viewport.GotoTop()
		return
	case tea.KeyEnd:
		p.viewport.GotoBottom()
		return
	}
	switch msg.String() {
	case "pgdown", " ":
		p.viewport.PageDown()
	case "pgup", "b":
		p.viewport.PageUp()
	case "ctrl+d":
		p.viewport.HalfViewDown()
	case "ctrl+u":
		p.viewport.HalfViewUp()
	case "g":
		p.viewport.GotoTop()
	case "G":
		p.viewport.GotoBottom()
	}
}

// isPanelScrollKey reports whether the keystroke should be consumed by the
// step-detail viewport instead of triggering step navigation.
func isPanelScrollKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
		return true
	}
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d", "g", "G", "b", " ":
		return true
	}
	return false
}

// ensureCursorVisible scrolls the viewport so the cursor's step row is in
// view. Called after every up/down/expand to keep the user oriented.
func (p *jobDetailPanel) ensureCursorVisible() {
	y := p.cursorRowOffset()
	if y < p.viewport.YOffset {
		p.viewport.SetYOffset(y)
		return
	}
	if y >= p.viewport.YOffset+p.viewport.Height {
		p.viewport.SetYOffset(y - p.viewport.Height + 1)
	}
}

// cursorRowOffset returns the row index of the cursor's step within the
// viewport's content. Accounts for the inter-step horizontal rules and any
// expanded step's output rows.
func (p *jobDetailPanel) cursorRowOffset() int {
	y := 0
	for i, step := range p.detail.Steps {
		if i > 0 {
			y++ // inter-step horizontal rule
		}
		if i == p.cursor {
			return y
		}
		y++ // step header row
		if i == p.expandedStepIdx && len(step.Actions) > 0 {
			y += p.expansionRowCount(step)
		}
	}
	return y
}

// expansionRowCount returns the number of visible rows the expansion of
// `step` contributes to the body — used to keep cursorRowOffset accurate
// when scrolling around expanded steps. Counts the same wrapped lines that
// renderExpansion emits so the math stays consistent.
func (p *jobDetailPanel) expansionRowCount(step model.Step) int {
	return len(p.expansionLines(step))
}

// stepKey returns a stable identifier for the step at the given index. Used
// to track which step is expanded across re-renders.
func (p *jobDetailPanel) stepKeyAt(i int) int {
	if i < 0 || i >= len(p.detail.Steps) || len(p.detail.Steps[i].Actions) == 0 {
		return -1
	}
	return p.detail.Steps[i].Actions[0].StepID
}

func (p *jobDetailPanel) up() {
	if p.cursor > 0 {
		p.cursor--
		p.ensureCursorVisible()
	}
}

func (p *jobDetailPanel) down() {
	if p.cursor < len(p.detail.Steps)-1 {
		p.cursor++
		p.ensureCursorVisible()
	}
}

// toggleExpand returns a tea.Cmd to fetch the cursor's step output if it's
// not already loaded and the step is expandable. Returns nil when nothing
// needs fetching (e.g. cursor on a step with no actions).
func (p *jobDetailPanel) toggleExpand(svc *cci.Service) tea.Cmd {
	if p.cursor < 0 || p.cursor >= len(p.detail.Steps) {
		return nil
	}
	step := p.detail.Steps[p.cursor]
	if len(step.Actions) == 0 {
		return nil
	}
	stepID := step.Actions[0].StepID
	if p.expandedStepIdx == p.cursor {
		// Toggle off — collapse.
		p.expandedStepIdx = -1
		p.ensureCursorVisible()
		return nil
	}
	p.expandedStepIdx = p.cursor
	p.ensureCursorVisible()
	if _, have := p.expandedOutput[stepID]; have {
		return nil
	}
	if p.expandedLoading[stepID] {
		return nil
	}
	p.expandedLoading[stepID] = true
	taskIndex := step.Actions[0].Index
	jobNumber := p.job.Number
	if jobNumber == 0 {
		jobNumber = p.detail.JobNumber
	}
	jobKey := p.jobKey
	slug := p.project.Slug
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		out, err := svc.GetStepOutput(ctx, slug, jobNumber, taskIndex, stepID)
		return stepOutputLoadedMsg{jobKey: jobKey, stepID: stepID, output: out, err: err}
	}
}

func (p *jobDetailPanel) render(theme Theme, statusGlyph func(model.Status) string) string {
	if !p.open {
		return ""
	}
	header := p.renderHeader(theme, statusGlyph(p.job.Status))

	var bodyContent string
	switch {
	case p.loading:
		bodyContent = theme.Muted.Render("⟳ fetching steps…")
	case p.err != nil:
		bodyContent = theme.Failed.Render("error: ") + theme.Muted.Render(p.err.Error())
	case len(p.detail.Steps) == 0:
		bodyContent = theme.Muted.Render("(no steps reported for this job — it may not have started yet, or it's an approval gate)")
	default:
		bodyContent = p.renderSteps(theme, statusGlyph)
	}
	p.viewport.SetContent(bodyContent)

	// Scroll-position indicator on the header rule. Lets the user see at a
	// glance whether there's more content below the visible viewport.
	headerRule := p.renderHeaderRule(theme)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColorFor(p.job.Status)).
		Padding(0, 1).
		Width(p.width - 2).
		Height(p.height - 2).
		Render(header + "\n" + headerRule + "\n" + p.viewport.View())
	return box
}

func (p *jobDetailPanel) renderHeaderRule(theme Theme) string {
	w := p.viewport.Width
	indicator := ""
	if p.viewport.TotalLineCount() > p.viewport.Height {
		percent := int(p.viewport.ScrollPercent() * 100)
		indicator = fmt.Sprintf(" %d%% ", percent)
	}
	indW := lipgloss.Width(indicator)
	if indW >= w {
		return theme.HRule.Render(strings.Repeat("─", w))
	}
	return theme.HRule.Render(strings.Repeat("─", w-indW)) + theme.Muted.Render(indicator)
}

func (p *jobDetailPanel) renderHeader(theme Theme, statusGlyph string) string {
	title := statusGlyph + " " + theme.Header.Render(p.job.Name)
	stats := []string{
		"#" + intToStr(p.pipeline.Number),
		p.workflow.Name,
		p.pipeline.Branch,
	}
	if p.workflow.IsRerun {
		stats = append(stats, "rerun")
	}
	if p.pipeline.Actor != "" {
		stats = append(stats, "by "+p.pipeline.Actor)
	}
	if d := p.job.Duration(); d > 0 {
		stats = append(stats, humanizeDuration(d))
	}
	sep := theme.Divider.Render(" · ")
	statLine := theme.Muted.Render(stats[0])
	for _, s := range stats[1:] {
		statLine += sep + theme.Muted.Render(s)
	}
	combined := title + "  " + statLine
	// The viewport stays a fixed inner width (p.width - 4); anything beyond
	// that wraps and bumps every step row down by one — which then breaks
	// click hit-testing. Hard-truncate so the header is always exactly one
	// visual row regardless of how long the workflow name + branch is.
	maxW := p.viewport.Width
	if maxW <= 0 {
		maxW = p.width - 4
	}
	return ansi.Truncate(combined, maxW, "…")
}

func (p *jobDetailPanel) renderSteps(theme Theme, statusGlyph func(model.Status) string) string {
	rule := theme.HRule.Render(strings.Repeat("─", p.viewport.Width))
	var b strings.Builder
	for i, step := range p.detail.Steps {
		if i > 0 {
			b.WriteString(rule)
			b.WriteString("\n")
		}
		marker := "  "
		if i == p.cursor {
			marker = "▸ "
		}
		glyph := statusGlyph(step.Status)
		dur := ""
		if d := step.Duration(); d > 0 {
			dur = " · " + humanizeDuration(d)
		}
		parallel := ""
		if step.IsParallel() {
			parallel = theme.Muted.Render(fmt.Sprintf(" · %d×", len(step.Actions)))
		}
		row := marker + glyph + " " + step.Name + theme.Muted.Render(dur) + parallel
		// Hard-truncate so a long step name can't push the row past the
		// viewport edge — that's what was breaking the panel border.
		row = ansi.Truncate(row, p.viewport.Width, "…")
		if i == p.cursor {
			row = theme.Selected.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
		if i == p.expandedStepIdx {
			b.WriteString(p.renderExpansion(step, theme))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *jobDetailPanel) renderExpansion(step model.Step, theme Theme) string {
	lines := p.expansionLines(step)
	if len(lines) == 0 {
		return ""
	}
	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(borderColorFor(step.Status)).
		PaddingLeft(2).
		Render(body)
	return box + "\n"
}

// expansionLines is the single source of truth for the rendered rows of an
// expanded step's body — used both by renderExpansion (to draw the block)
// and by expansionRowCount (to keep cursorRowOffset and viewport bookkeeping
// honest). Each returned line is guaranteed to fit inside the viewport's
// inner width minus the left border + padding (3 cells).
func (p *jobDetailPanel) expansionLines(step model.Step) []string {
	if len(step.Actions) == 0 {
		return nil
	}
	stepID := step.Actions[0].StepID
	switch {
	case p.expandedLoading[stepID]:
		return []string{"⟳ fetching output…"}
	case p.expandedErr[stepID] != nil:
		return []string{"error: " + p.expandedErr[stepID].Error()}
	}
	body, ok := p.expandedOutput[stepID]
	if !ok {
		return nil
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return []string{"(empty)"}
	}
	maxLine := p.viewport.Width - 3 // border (1) + padding (2)
	if maxLine < 10 {
		maxLine = 10
	}
	var out []string
	for _, raw := range strings.Split(body, "\n") {
		wrapped := ansi.Hardwrap(raw, maxLine, true)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return out
}

func borderColorFor(s model.Status) lipgloss.AdaptiveColor {
	switch s {
	case model.StatusFailed:
		return lipgloss.AdaptiveColor{Dark: "#ef4444", Light: "#b91c1c"}
	case model.StatusRunning:
		return lipgloss.AdaptiveColor{Dark: "#06b6d4", Light: "#0e7490"}
	case model.StatusOnHold:
		return lipgloss.AdaptiveColor{Dark: "#eab308", Light: "#a16207"}
	case model.StatusSuccess:
		return lipgloss.AdaptiveColor{Dark: "#22c55e", Light: "#15803d"}
	default:
		return lipgloss.AdaptiveColor{Dark: "#6b7280", Light: "#9ca3af"}
	}
}

// fetchJobDetailCmd kicks off a v1.1 job-details fetch and routes the result
// through detailLoadedMsg.
func fetchJobDetailCmd(svc *cci.Service, project model.Project, job model.Job, jobKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if svc == nil {
			return detailLoadedMsg{jobKey: jobKey, detail: demoDetail(job)}
		}
		if job.Number == 0 {
			return detailLoadedMsg{
				jobKey: jobKey,
				err:    fmt.Errorf("job hasn't started yet (no job number)"),
			}
		}
		detail, err := svc.GetJobDetail(ctx, project.Slug, job.Number)
		return detailLoadedMsg{jobKey: jobKey, detail: detail, err: err}
	}
}

// demoDetail returns a fixture step list for `circleci-tui demo` so the
// step-detail view is exercisable without a token.
func demoDetail(job model.Job) model.JobDetail {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	steps := []model.Step{
		{Name: "Spin up environment", Status: model.StatusSuccess, Actions: []model.StepAction{{Index: 0, StepID: 100, Status: model.StatusSuccess, StartedAt: ago(3 * time.Minute), StoppedAt: ago(2*time.Minute - 45*time.Second)}}},
		{Name: "Checkout code", Status: model.StatusSuccess, Actions: []model.StepAction{{Index: 0, StepID: 101, Status: model.StatusSuccess, StartedAt: ago(2*time.Minute - 45*time.Second), StoppedAt: ago(2*time.Minute - 40*time.Second)}}},
		{Name: "Restore cache", Status: model.StatusSuccess, Actions: []model.StepAction{{Index: 0, StepID: 102, Status: model.StatusSuccess, StartedAt: ago(2*time.Minute - 40*time.Second), StoppedAt: ago(60 * time.Second)}}},
		{Name: "yarn test", Status: job.Status, Actions: []model.StepAction{{Index: 0, StepID: 103, Status: job.Status, StartedAt: ago(60 * time.Second), StoppedAt: ago(5 * time.Second)}}},
	}
	return model.JobDetail{JobNumber: job.Number, JobName: job.Name, Steps: steps}
}

func intToStr(i int) string { return fmt.Sprintf("%d", i) }
