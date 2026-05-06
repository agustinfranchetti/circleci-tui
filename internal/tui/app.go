package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

const defaultRefreshInterval = 30 * time.Second

// LiveOptions carries optional settings the live TUI honours when present
// in the user's config file. Zero values fall back to the defaults.
type LiveOptions struct {
	// RefreshInterval overrides the 30s auto-refresh ticker.
	RefreshInterval time.Duration
	// MineOnAtStartup, when true, applies the mine-only filter on launch
	// (matches BranchFilter = "mine" in the config).
	MineOnAtStartup bool
}

type Model struct {
	Tree   *model.Tree
	Theme  Theme
	Keys   KeyMap
	Width  int
	Height int

	// Live mode (nil when running on fixtures)
	svc      *cci.Service
	watch    []cci.Project
	loading  bool
	lastLoad time.Time
	loadErr  error

	picker     projectPicker
	filter     filterPalette
	mineActor  string
	actions    actionsMenu
	detail     jobDetailPanel
	toast      string
	toastUntil time.Time
	spinner    spinner.Model
	refreshEvery  time.Duration
	// loadFrame ticks once per spinner.TickMsg. Drives the multi-row
	// "loading pipelines…" wave animation in renderEmptyLiveState.
	loadFrame int

	// Mouse-drag → auto-copy state. dragStartX/Y is captured on
	// MouseActionPress; dragging flips on the first MouseActionMotion past a
	// 1-cell threshold; on Release with dragging=true, we extract the text
	// between press and release coordinates from the freshly-rendered View()
	// and write it to the system clipboard.
	dragStartX, dragStartY int
	dragging               bool
}

func New(projects []model.Project) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = DefaultTheme().Running
	return Model{
		Tree:      model.NewTree(projects),
		Theme:     DefaultTheme(),
		Keys:      DefaultKeyMap(),
		filter:    newFilterPalette(),
		mineActor: cci.CurrentActor(),
		spinner:   sp,
		detail:    newJobDetailPanel(),
	}
}

// NewLive constructs a Model in live mode — Init() will trigger an initial
// load and start the refresh ticker.
func NewLive(svc *cci.Service, watch []cci.Project, opts LiveOptions) Model {
	m := New(nil)
	m.svc = svc
	m.watch = watch
	m.loading = true
	m.refreshEvery = opts.RefreshInterval
	if m.refreshEvery <= 0 {
		m.refreshEvery = defaultRefreshInterval
	}
	if opts.MineOnAtStartup {
		m.Tree.SetMineOnly(true, m.mineActor)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.svc == nil {
		return m.spinner.Tick
	}
	return tea.Batch(
		loadAllCmd(m.svc, m.watch),
		tickCmd(m.refreshEvery),
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		if m.detail.open {
			pw, ph := m.panelDimensions()
			m.detail.resize(pw, ph)
		}
		return m, nil

	case detailLoadedMsg:
		m.detail.setDetail(msg.jobKey, msg.detail, msg.err)
		return m, nil

	case stepOutputLoadedMsg:
		m.detail.setStepOutput(msg.jobKey, msg.stepID, msg.output, msg.err)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.loadFrame++
		return m, cmd

	case loadedMsg:
		m.loading = false
		m.lastLoad = msg.at
		m.loadErr = msg.err
		if msg.err == nil || len(msg.projects) > 0 {
			m.replaceProjects(msg.projects)
		}
		return m, nil

	case tickMsg:
		if m.svc == nil {
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(
			loadAllCmd(m.svc, m.watch),
			tickCmd(m.refreshEvery),
		)

	case actionResultMsg:
		if msg.err != nil {
			m.flash("✗ " + msg.kind.label() + ": " + msg.err.Error())
		} else {
			m.flash("✓ " + msg.kind.label() + " — " + msg.target)
			// Trigger an immediate refresh in live mode so the new state
			// (rerun queued, workflow cancelled, etc.) shows up faster than
			// the next ticker would.
			if m.svc != nil && !m.loading {
				m.loading = true
				return m, loadAllCmd(m.svc, m.watch)
			}
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if m.actions.open {
			return m.updateActions(msg)
		}
		if m.picker.open {
			return m.updatePicker(msg)
		}
		if m.filter.open {
			next, cmd, doApply, doClose := m.filter.updateKey(msg, m.Keys)
			m.filter = next
			if doApply {
				m.filter.apply(m.Tree, m.mineActor)
			}
			if doClose {
				m.filter.close()
			}
			return m, cmd
		}
		// When the step-detail panel is open, tree-nav keys move through the
		// step list and PgUp/Dn / Ctrl-U/D / g / G scroll the viewport.
		// Enter expands the cursor's step (lazy-fetches its output); esc
		// closes the panel.
		if m.detail.open {
			if isPanelScrollKey(msg) {
				m.detail.scroll(msg)
				return m, nil
			}
			switch {
			case key.Matches(msg, m.Keys.Up):
				m.detail.up()
				return m, nil
			case key.Matches(msg, m.Keys.Down):
				m.detail.down()
				return m, nil
			case key.Matches(msg, m.Keys.Toggle):
				if cmd := m.detail.toggleExpand(m.svc); cmd != nil {
					return m, cmd
				}
				return m, nil
			case key.Matches(msg, m.Keys.Cancel), key.Matches(msg, m.Keys.Logs):
				m.detail.close()
				return m, nil
			case key.Matches(msg, m.Keys.Quit):
				return m, tea.Quit
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, m.Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.Keys.Up):
			m.Tree.Up()
		case key.Matches(msg, m.Keys.Down):
			m.Tree.Down()
		case msg.String() == "pgdown" || msg.String() == "ctrl+d":
			m.Tree.PageDown(m.treeHeight() / 2)
		case msg.String() == "pgup" || msg.String() == "ctrl+u":
			m.Tree.PageUp(m.treeHeight() / 2)
		case msg.String() == "home" || msg.String() == "g":
			m.Tree.GotoTop()
		case msg.String() == "end" || msg.String() == "G":
			m.Tree.GotoBottom()
		case key.Matches(msg, m.Keys.Toggle):
			// On a job row (a leaf — no expand/collapse), Enter / Space
			// open the step-detail panel instead. Otherwise toggle the
			// project/pipeline/workflow row.
			if r, ok := m.Tree.Current(); ok && r.Kind == model.RowJob {
				if cmd := m.toggleDetailForCurrent(); cmd != nil {
					return m, cmd
				}
			} else {
				m.Tree.ToggleCurrent()
			}
		case key.Matches(msg, m.Keys.ExpandAll):
			m.Tree.ExpandAll()
		case key.Matches(msg, m.Keys.CollapseAll):
			m.Tree.CollapseAll()
		case key.Matches(msg, m.Keys.Refresh):
			if m.svc != nil && !m.loading {
				m.loading = true
				return m, loadAllCmd(m.svc, m.watch)
			}
		case key.Matches(msg, m.Keys.Picker):
			m.picker.build(m.Tree.Projects, m.Tree.Focus())
			m.picker.open = true
		case key.Matches(msg, m.Keys.Filter):
			m.filter.openWith(m.Tree, m.mineActor)
		case key.Matches(msg, m.Keys.Mine):
			actor := m.mineActor
			m.Tree.SetMineOnly(!m.Tree.MineOnly(), actor)
		case key.Matches(msg, m.Keys.Logs):
			if cmd := m.toggleDetailForCurrent(); cmd != nil {
				return m, cmd
			}
		case key.Matches(msg, m.Keys.Actions):
			if menu, ok := buildActionMenu(m.Tree); ok {
				m.actions = menu
			}
		case key.Matches(msg, m.Keys.Cancel):
			switch {
			case m.detail.open:
				m.detail.close()
			case m.Tree.TextFilter() != "":
				m.Tree.SetTextFilter("")
			case m.Tree.MineOnly():
				m.Tree.SetMineOnly(false, m.mineActor)
			case m.Tree.Focus() != "":
				m.Tree.SetFocus("")
			}
		}
	}
	return m, nil
}

func (m Model) updateActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.actions.confirm {
		switch msg.String() {
		case "y", "Y":
			item, ok := m.actions.current()
			if !ok {
				m.actions = actionsMenu{}
				return m, nil
			}
			target := m.actions.target
			m.actions = actionsMenu{}
			return m, runActionCmd(m.svc, target, item)
		case "n", "N", "esc", "ctrl+c":
			m.actions.confirm = false
			return m, nil
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, m.Keys.Cancel):
		m.actions = actionsMenu{}
	case key.Matches(msg, m.Keys.Up):
		m.actions.up()
	case key.Matches(msg, m.Keys.Down):
		m.actions.down()
	case key.Matches(msg, m.Keys.Toggle):
		item, ok := m.actions.current()
		if !ok {
			return m, nil
		}
		if item.kind.destructive() {
			m.actions.confirm = true
			return m, nil
		}
		target := m.actions.target
		m.actions = actionsMenu{}
		return m, runActionCmd(m.svc, target, item)
	case key.Matches(msg, m.Keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) flash(s string) {
	m.toast = s
	m.toastUntil = time.Now().Add(4 * time.Second)
}


func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Cancel), key.Matches(msg, m.Keys.Picker):
		m.picker.open = false
	case key.Matches(msg, m.Keys.Up):
		m.picker.up()
	case key.Matches(msg, m.Keys.Down):
		m.picker.down()
	case key.Matches(msg, m.Keys.Toggle):
		m.Tree.SetFocus(m.picker.selected())
		m.picker.open = false
	case key.Matches(msg, m.Keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}

// replaceProjects swaps in a freshly loaded project list while doing its best
// to preserve the user's expand/collapse state, current focus, and selected
// row across the refresh. Row keys are based on project slug and CircleCI's
// own pipeline/job UUIDs, so the carry-over is stable.
func (m *Model) replaceProjects(fresh []model.Project) {
	prev := m.Tree
	prevKey := ""
	if prev != nil {
		if r, ok := prev.Current(); ok {
			prevKey = r.Key
		}
	}
	next := model.NewTree(fresh)
	next.AdoptCollapsedFrom(prev)
	next.AdoptFiltersFrom(prev)
	if prev != nil {
		next.SetFocus(prev.Focus())
	}
	m.Tree = next
	if prevKey == "" {
		return
	}
	for i, r := range m.Tree.Visible() {
		if r.Key == prevKey {
			m.setCursor(i)
			return
		}
	}
}

func (m *Model) setCursor(target int) {
	cur := m.Tree.Cursor()
	for cur < target {
		m.Tree.Down()
		if m.Tree.Cursor() == cur {
			break
		}
		cur = m.Tree.Cursor()
	}
	for cur > target {
		m.Tree.Up()
		if m.Tree.Cursor() == cur {
			break
		}
		cur = m.Tree.Cursor()
	}
}

// toggleDetailForCurrent resolves the cursor's row to a (project, pipeline,
// workflow, job) tuple and opens the step-detail panel for that job. On a
// project/pipeline/workflow row, picks the most recent failed job under the
// cursor (or the first job if none failed) so `e` does something useful even
// when the user hasn't drilled into a job. Returns nil if there's no job to
// show.
func (m *Model) toggleDetailForCurrent() tea.Cmd {
	r, ok := m.Tree.Current()
	if !ok || r.PipeIdx < 0 {
		return nil
	}
	jobKey := r.Key
	if m.detail.open && m.detail.jobKey == jobKey {
		m.detail.close()
		return nil
	}
	proj := m.Tree.Projects[r.ProjIdx]
	pipe := proj.Pipelines[r.PipeIdx]
	var wf model.Workflow
	var job model.Job
	switch {
	case r.WfIdx >= 0 && r.JobIdx >= 0 && r.WfIdx < len(pipe.Workflows) && r.JobIdx < len(pipe.Workflows[r.WfIdx].Jobs):
		wf = pipe.Workflows[r.WfIdx]
		job = wf.Jobs[r.JobIdx]
	case r.WfIdx >= 0 && r.WfIdx < len(pipe.Workflows):
		wf = pipe.Workflows[r.WfIdx]
		if j, found := pickInterestingJob(wf); found {
			job = j
		} else {
			return nil
		}
	default:
		// Pipeline-row: search across all workflows for a failed/running job.
		for _, w := range pipe.Workflows {
			if j, found := pickInterestingJob(w); found {
				wf = w
				job = j
				break
			}
		}
		if job.ID == "" {
			return nil
		}
	}
	m.detail.openOn(jobKey, proj, pipe, wf, job)
	pw, ph := m.panelDimensions()
	m.detail.resize(pw, ph)
	return fetchJobDetailCmd(m.svc, proj, job, jobKey)
}

// pickInterestingJob returns the first failed job in a workflow, or the first
// running one, or the first job at all — whatever the user is most likely to
// want to inspect.
func pickInterestingJob(wf model.Workflow) (model.Job, bool) {
	if len(wf.Jobs) == 0 {
		return model.Job{}, false
	}
	for _, j := range wf.Jobs {
		if j.Status == model.StatusFailed {
			return j, true
		}
	}
	for _, j := range wf.Jobs {
		if j.Status == model.StatusRunning {
			return j, true
		}
	}
	return wf.Jobs[0], true
}

// panelDimensions returns the (width, height) the step-detail panel renders
// at. Since the v0.1.6 layout switch, the panel takes the whole screen
// rather than splitting with the tree.
func (m Model) panelDimensions() (int, int) {
	if m.Width == 0 {
		return 80, 20 // sane fallback before the first WindowSizeMsg
	}
	ph := m.Height - 3 // title + status bar
	if ph < 5 {
		ph = 5
	}
	return m.Width, ph
}

// handleMouse routes mouse events. Behaviours:
//   - Wheel up/down: move cursor one row (or scroll the step panel).
//   - Left-press: stash drag origin; defer the click action to release.
//   - Left-motion past 1 cell: enter "dragging" mode for text selection.
//   - Left-release with dragging: extract the text between origin and
//     release from a freshly-rendered View(), write it to the clipboard,
//     toast on success.
//   - Left-release without dragging: treat as a single click — select row,
//     toggle chevron, drill into job, etc.
//   - Right-press: open the actions menu for the row under the cursor.
//
// Clicks while a modal (picker/actions/filter) is open are ignored.
func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.actions.open || m.picker.open || m.filter.open {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.detail.open {
			m.detail.viewport.LineUp(2)
		} else {
			m.Tree.Up()
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if m.detail.open {
			m.detail.viewport.LineDown(2)
		} else {
			m.Tree.Down()
		}
		return m, nil
	}

	// Right-click is press-driven (no drag semantics).
	if msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
		(&m).clickToCursor(msg.Y)
		if menu, ok := buildActionMenu(m.Tree); ok {
			m.actions = menu
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	switch msg.Action {
	case tea.MouseActionPress:
		m.dragStartX = msg.X
		m.dragStartY = msg.Y
		m.dragging = false
		return m, nil
	case tea.MouseActionMotion:
		if !m.dragging && (abs(msg.X-m.dragStartX) > 1 || abs(msg.Y-m.dragStartY) > 0) {
			m.dragging = true
		}
		return m, nil
	case tea.MouseActionRelease:
		if m.dragging {
			text := extractSelection(m.View(), m.dragStartX, m.dragStartY, msg.X, msg.Y)
			m.dragging = false
			if strings.TrimSpace(text) == "" {
				return m, nil
			}
			if err := clipboard.WriteAll(text); err != nil {
				m.flash("✗ copy failed: " + err.Error())
			} else {
				m.flash("✓ copied " + countSummary(text) + " to clipboard")
			}
			return m, nil
		}
	}

	// No drag → treat as a single click. Use the release coords (or the
	// press coords for press-only delivery — they're the same when nothing
	// moved).
	return m.handleLeftClick(msg)
}

func (m Model) handleLeftClick(msg tea.MouseMsg) (Model, tea.Cmd) {
	// In the step-detail panel, click on a step row selects + expands it.
	if m.detail.open {
		return m.handleDetailClick(msg)
	}

	const treeY = 1
	if msg.Y < treeY {
		return m, nil
	}
	rows := m.Tree.Visible()
	start, end := m.treeWindow(len(rows), m.Tree.Cursor())
	idx := start + (msg.Y - treeY)
	if idx < start || idx >= end {
		return m, nil
	}
	(&m).setCursor(idx)
	r := rows[idx]
	chevronX := r.Indent * 2
	if msg.X >= chevronX && msg.X <= chevronX+1 {
		switch r.Kind {
		case model.RowProject, model.RowPipeline, model.RowWorkflow:
			m.Tree.ToggleCurrent()
			return m, nil
		}
	}
	if r.Kind == model.RowJob {
		if cmd := (&m).toggleDetailForCurrent(); cmd != nil {
			return m, cmd
		}
	}
	return m, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// extractSelection returns the visible (ANSI-stripped) text between two
// screen coordinates from a rendered View() string. The selection follows
// natural line flow: characters from x1 to end-of-line on the start row,
// full lines in between, and start-of-line through x2 on the end row.
func extractSelection(view string, x1, y1, x2, y2 int) string {
	if y1 > y2 || (y1 == y2 && x1 > x2) {
		x1, x2 = x2, x1
		y1, y2 = y2, y1
	}
	lines := strings.Split(view, "\n")
	var b strings.Builder
	for y := y1; y <= y2 && y < len(lines); y++ {
		plain := ansi.Strip(lines[y])
		runes := []rune(plain)
		a := 0
		c := len(runes)
		if y == y1 {
			a = x1
		}
		if y == y2 {
			c = x2 + 1
		}
		if a < 0 {
			a = 0
		}
		if c > len(runes) {
			c = len(runes)
		}
		if a < c {
			b.WriteString(string(runes[a:c]))
		}
		if y < y2 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func countSummary(s string) string {
	n := strings.Count(s, "\n") + 1
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// clickToCursor walks the tree cursor to the row at screen y. No-op when
// the click is above the tree or past the visible window.
func (m *Model) clickToCursor(y int) {
	const treeY = 1
	if y < treeY {
		return
	}
	rows := m.Tree.Visible()
	start, end := m.treeWindow(len(rows), m.Tree.Cursor())
	idx := start + (y - treeY)
	if idx < start || idx >= end {
		return
	}
	m.setCursor(idx)
}

// handleDetailClick maps a left-click inside the step-detail panel to a step
// row, moving the panel's cursor and expanding the clicked step.
func (m Model) handleDetailClick(msg tea.MouseMsg) (Model, tea.Cmd) {
	// Screen layout in detail mode: title row (1) + panel rounded-border top
	// (1) + panel header (1) + header rule (1) = 4 rows of chrome before the
	// viewport. The viewport itself can also be scrolled, so add YOffset.
	const stepListY = 4
	if msg.Y < stepListY {
		return m, nil
	}
	clicked := msg.Y - stepListY + m.detail.viewport.YOffset
	idx := stepIndexForRow(&m.detail, clicked)
	if idx < 0 || idx >= len(m.detail.detail.Steps) {
		return m, nil
	}
	m.detail.cursor = idx
	if cmd := m.detail.toggleExpand(m.svc); cmd != nil {
		return m, cmd
	}
	return m, nil
}

// stepIndexForRow walks the panel's step list accumulating the same row
// counts as cursorRowOffset, returning the step index that owns the given
// row in viewport space.
func stepIndexForRow(p *jobDetailPanel, row int) int {
	y := 0
	for i, step := range p.detail.Steps {
		if i > 0 {
			if row == y {
				// User clicked the divider rule between steps — treat as
				// click on the next step.
				return i
			}
			y++
		}
		if row == y {
			return i
		}
		y++
		if i == p.expandedStepIdx {
			rows := p.expansionRowCount(step)
			if row < y+rows {
				return i // click inside the expansion still belongs to its step
			}
			y += rows
		}
	}
	return -1
}

// Run launches the TUI in fixture mode (used by `circleci-tui tui` when no
// token / live data is available — keeps the demo path runnable).
func Run(projects []model.Project) error {
	p := tea.NewProgram(New(projects), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// RunLive launches the TUI against a real Service. opts honours config-file
// settings — refresh interval, default branch filter — when provided.
func RunLive(svc *cci.Service, watch []cci.Project, opts LiveOptions) error {
	p := tea.NewProgram(NewLive(svc, watch, opts), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
