package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

// filterPalette is the pop-up dropdown opened with `/`. Three sections in
// vertical order: Status (single-select radio), Search (free text), Mine
// only (checkbox). Up/down moves between sections; left/right cycles status
// values when the cursor is on the Status row; typing flows into the search
// box when the cursor is on Search; space toggles the Mine row. Enter
// applies and closes; Esc cancels.
type filterPalette struct {
	open   bool
	cursor int // 0 = status, 1 = search, 2 = mine

	statusValues []model.Status
	statusIdx    int

	search    textinput.Model
	searchAt  bool // true when textinput owns key input
	mineOn    bool
	mineActor string
}

var defaultStatusOptions = []model.Status{
	"", // "any"
	model.StatusFailed,
	model.StatusRunning,
	model.StatusOnHold,
	model.StatusSuccess,
	model.StatusCancelled,
}

func newFilterPalette() filterPalette {
	ti := textinput.New()
	ti.Placeholder = "branch / repo / ticket"
	ti.Prompt = ""
	ti.CharLimit = 64
	return filterPalette{
		statusValues: defaultStatusOptions,
		search:       ti,
	}
}

// openWith seeds the palette from the tree's current filter state so the
// modal reflects what's already applied — closing it without a change leaves
// the tree untouched.
func (p *filterPalette) openWith(t *model.Tree, mineActor string) {
	p.open = true
	p.cursor = 0
	p.searchAt = false
	p.mineActor = mineActor
	// Seed status from tree
	p.statusIdx = 0
	cur := t.StatusFilter()
	for i, s := range p.statusValues {
		if s == cur {
			p.statusIdx = i
			break
		}
	}
	p.search.SetValue(t.TextFilter())
	p.search.Blur()
	p.mineOn = t.MineOnly()
}

func (p *filterPalette) close() {
	p.open = false
	p.searchAt = false
	p.search.Blur()
}

// apply pushes the palette's current selections to the tree.
func (p *filterPalette) apply(t *model.Tree, mineActor string) {
	t.SetStatusFilter(p.statusValues[p.statusIdx])
	t.SetTextFilter(p.search.Value())
	t.SetMineOnly(p.mineOn, mineActor)
}

func (p *filterPalette) up() {
	if p.cursor > 0 {
		p.cursor--
		p.searchAt = (p.cursor == 1)
		if p.searchAt {
			p.search.Focus()
		} else {
			p.search.Blur()
		}
	}
}

func (p *filterPalette) down() {
	if p.cursor < 2 {
		p.cursor++
		p.searchAt = (p.cursor == 1)
		if p.searchAt {
			p.search.Focus()
		} else {
			p.search.Blur()
		}
	}
}

// updateKey consumes a key event. Returns the (possibly mutated) palette
// plus a tea.Cmd from the textinput when the search section is focused.
func (p filterPalette) updateKey(msg tea.KeyMsg, keys KeyMap) (filterPalette, tea.Cmd, bool /*apply*/, bool /*close*/) {
	// Esc always cancels regardless of section.
	if key.Matches(msg, keys.Cancel) {
		return p, nil, false, true
	}
	// Enter applies and closes.
	if key.Matches(msg, keys.Toggle) {
		return p, nil, true, true
	}
	// Up/down change row, taking focus out of the textinput.
	if key.Matches(msg, keys.Up) {
		p.up()
		return p, nil, false, false
	}
	if key.Matches(msg, keys.Down) {
		p.down()
		return p, nil, false, false
	}

	switch p.cursor {
	case 0: // status
		switch msg.String() {
		case "right", "l":
			p.statusIdx = (p.statusIdx + 1) % len(p.statusValues)
		case "left", "h":
			p.statusIdx = (p.statusIdx - 1 + len(p.statusValues)) % len(p.statusValues)
		}
	case 1: // search
		var cmd tea.Cmd
		p.search, cmd = p.search.Update(msg)
		return p, cmd, false, false
	case 2: // mine
		switch msg.String() {
		case " ", "x":
			p.mineOn = !p.mineOn
		}
	}
	return p, nil, false, false
}

// updateMouse handles a left-click inside the palette. dx/dy are the click's
// position relative to the palette's top-left.
func (p filterPalette) updateMouse(dx, dy int) (filterPalette, bool) {
	// Approximate row mapping: rows inside the box are
	//   0: blank
	//   1: status
	//   2: blank
	//   3: search
	//   4: blank
	//   5: mine
	//   6: blank
	//   7: hint
	switch dy {
	case 1: // status
		p.cursor = 0
		// Clicked option — figure out which index. Each option ~10 cells
		// wide-ish; just bump to next.
		p.statusIdx = (p.statusIdx + 1) % len(p.statusValues)
	case 3: // search
		p.cursor = 1
		p.searchAt = true
		p.search.Focus()
	case 5: // mine
		p.cursor = 2
		p.mineOn = !p.mineOn
	default:
		return p, false
	}
	return p, true
}

func (p *filterPalette) render(theme Theme, screenW, screenH int) string {
	const innerW = 56
	width := innerW
	if screenW > 0 && screenW-4 < width {
		width = screenW - 4
	}
	if width < 30 {
		width = 30
	}

	statusRow := p.renderStatus(theme, width)
	searchRow := p.renderSearch(theme, width)
	mineRow := p.renderMine(theme, width)
	hintRow := theme.Muted.Render("↑↓ row · ←→ status · ⏎ apply · esc cancel")

	body := strings.Join([]string{
		"",
		statusRow,
		"",
		searchRow,
		"",
		mineRow,
		"",
		hintRow,
	}, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Dark: "#a78bfa", Light: "#7c3aed"}).
		Padding(0, 2).
		Width(width).
		Render(theme.Title.Render(" filter ") + "\n" + body)
	return box
}

func (p *filterPalette) renderStatus(theme Theme, width int) string {
	label := theme.Muted.Render("Status")
	var pills []string
	for i, s := range p.statusValues {
		name := string(s)
		if name == "" {
			name = "any"
		}
		pill := " " + name + " "
		if i == p.statusIdx {
			style := theme.StatusStyle(s)
			if name == "any" {
				style = theme.Header
			}
			pill = style.Reverse(true).Render(pill)
		} else {
			pill = theme.Muted.Render(pill)
		}
		pills = append(pills, pill)
	}
	value := strings.Join(pills, " ")
	caret := "  "
	if p.cursor == 0 {
		caret = theme.Title.Render("▸ ")
	}
	_ = width
	return caret + label + ":  " + value
}

func (p *filterPalette) renderSearch(theme Theme, width int) string {
	label := theme.Muted.Render("Search")
	caret := "  "
	if p.cursor == 1 {
		caret = theme.Title.Render("▸ ")
		p.search.Focus()
	} else {
		p.search.Blur()
	}
	value := p.search.View()
	if !p.searchAt && p.search.Value() == "" {
		value = theme.Muted.Render(p.search.Placeholder)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Dark: "#374151", Light: "#d1d5db"}).
		Padding(0, 1).
		Width(width - 18).
		Render(value)
	return caret + label + ":  " + box
}

func (p *filterPalette) renderMine(theme Theme, width int) string {
	label := theme.Muted.Render("Mine only")
	check := theme.Muted.Render("[ ]")
	if p.mineOn {
		check = theme.Success.Render("[x]")
	}
	caret := "  "
	if p.cursor == 2 {
		caret = theme.Title.Render("▸ ")
	}
	actor := theme.Muted.Render(p.mineActor)
	_ = width
	return caret + label + ":  " + check + "  " + actor
}
