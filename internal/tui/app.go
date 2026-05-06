package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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

	picker        projectPicker
	filterInput   textinput.Model
	filterEditing bool
	mineActor     string
	actions       actionsMenu
	logs          logPanel
	toast         string
	toastUntil    time.Time
	spinner       spinner.Model
	refreshEvery  time.Duration
}

func New(projects []model.Project) Model {
	ti := textinput.New()
	ti.Placeholder = "branch / repo / ticket / status"
	ti.Prompt = "/ "
	ti.CharLimit = 64
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = DefaultTheme().Running
	return Model{
		Tree:        model.NewTree(projects),
		Theme:       DefaultTheme(),
		Keys:        DefaultKeyMap(),
		filterInput: ti,
		mineActor:   cci.CurrentActor(),
		spinner:     sp,
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
		if m.logs.open {
			pw, ph := m.panelDimensions()
			m.logs.resize(pw, ph)
		}
		return m, nil

	case logsLoadedMsg:
		m.logs.setContent(msg.jobKey, msg.content, msg.err)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
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
		return m.handleMouse(msg), nil

	case tea.KeyMsg:
		if m.actions.open {
			return m.updateActions(msg)
		}
		if m.picker.open {
			return m.updatePicker(msg)
		}
		if m.filterEditing {
			return m.updateFilterEditing(msg)
		}
		// Panel scroll claims its own keys before tree nav so the user can
		// page through logs without nudging the tree cursor. We keep j/k/↑↓
		// for tree nav — the panel uses pgup/pgdown/ctrl-u/ctrl-d/g/G.
		if m.logs.open && isPanelScrollKey(msg) {
			m.logs.scroll(msg)
			return m, nil
		}
		switch {
		case key.Matches(msg, m.Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.Keys.Up):
			m.Tree.Up()
		case key.Matches(msg, m.Keys.Down):
			m.Tree.Down()
		case key.Matches(msg, m.Keys.Toggle):
			m.Tree.ToggleCurrent()
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
			m.filterInput.SetValue(m.Tree.TextFilter())
			m.filterInput.Focus()
			m.filterEditing = true
		case key.Matches(msg, m.Keys.Mine):
			actor := m.mineActor
			m.Tree.SetMineOnly(!m.Tree.MineOnly(), actor)
		case key.Matches(msg, m.Keys.Logs):
			if cmd := m.toggleLogsForCurrent(); cmd != nil {
				return m, cmd
			}
		case key.Matches(msg, m.Keys.Actions):
			if menu, ok := buildActionMenu(m.Tree); ok {
				m.actions = menu
			}
		case key.Matches(msg, m.Keys.Cancel):
			switch {
			case m.logs.open:
				m.logs.close()
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

func (m Model) updateFilterEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Cancel):
		m.filterEditing = false
		m.filterInput.Blur()
		m.Tree.SetTextFilter("")
		m.filterInput.SetValue("")
		return m, nil
	case key.Matches(msg, m.Keys.Toggle):
		m.filterEditing = false
		m.filterInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.Tree.SetTextFilter(m.filterInput.Value())
	return m, cmd
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

// toggleLogsForCurrent resolves the currently selected row to a (project,
// pipeline, job) triple and toggles the right-side log panel. Pressing `e`
// on the same job again closes the panel; on a different job, swaps content.
// Returns nil when the cursor isn't on something log-able.
func (m *Model) toggleLogsForCurrent() tea.Cmd {
	r, ok := m.Tree.Current()
	if !ok || r.PipeIdx < 0 {
		return nil
	}
	jobKey := r.Key
	// Same row already open → close.
	if m.logs.open && m.logs.jobKey == jobKey {
		m.logs.close()
		return nil
	}
	proj := m.Tree.Projects[r.ProjIdx]
	pipe := proj.Pipelines[r.PipeIdx]
	var job model.Job
	if r.JobIdx >= 0 && r.JobIdx < len(pipe.Jobs) {
		job = pipe.Jobs[r.JobIdx]
	} else {
		// Pipeline-row: synthesize a "pipeline-level" pseudo-job so the
		// panel header still has something useful to show. Status comes from
		// the pipeline; name from the branch.
		job = model.Job{Name: pipe.Branch, Status: pipe.Status}
	}
	m.logs.openOn(jobKey, proj, pipe, job)
	pw, ph := m.panelDimensions()
	m.logs.resize(pw, ph)
	return fetchLogsCmd(m.svc, proj, pipe, job, jobKey)
}

// panelDimensions returns the right-pane (width, height) given the current
// terminal size. We carve off ~half the width for the panel, with a floor
// of minPanelWidth — below that we'd rather drop the tree entirely.
func (m Model) panelDimensions() (int, int) {
	if m.Width == 0 {
		return 80, 20 // sane fallback before the first WindowSizeMsg
	}
	pw := m.Width / 2
	if pw < minPanelWidth {
		pw = m.Width
	}
	ph := m.Height - 3 // title + status bar
	if ph < 5 {
		ph = 5
	}
	return pw, ph
}

// handleMouse routes mouse events. Left-click on a tree row moves the cursor
// to that row; wheel up/down scrolls one row at a time. Clicks outside the
// tree area, or while a modal (picker/actions/filter) is open, are ignored —
// modals own the input focus.
func (m Model) handleMouse(msg tea.MouseMsg) Model {
	if m.actions.open || m.picker.open || m.filterEditing {
		return m
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.Tree.Up()
		return m
	case tea.MouseButtonWheelDown:
		m.Tree.Down()
		return m
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m
	}
	// Tree starts at y=1 (after the title); +1 more when the filter input is
	// drawn — but filter editing is handled by the early-return above so we
	// don't worry about it here.
	const treeY = 1
	if msg.Y < treeY {
		return m
	}
	rows := m.Tree.Visible()
	start, end := m.treeWindow(len(rows), m.Tree.Cursor())
	idx := start + (msg.Y - treeY)
	if idx < start || idx >= end {
		return m
	}
	(&m).setCursor(idx)
	return m
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
