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

// logsLoadedMsg is what the log fetch returns once it lands.
type logsLoadedMsg struct {
	jobKey  string
	content string
	err     error
}

const (
	// minPanelWidth is the smallest right-pane width we'll render at. Below
	// this, we hide the tree entirely and let the panel take the full screen.
	minPanelWidth = 60
)

// logPanel is the right-side Warp-style panel. One panel per Model — picking
// `e` on a different job swaps the contents in place rather than stacking.
type logPanel struct {
	open       bool
	loading    bool
	err        error
	jobKey     string
	project    model.Project
	pipeline   model.Pipeline
	job        model.Job
	rawContent string // ANSI preserved
	viewport   viewport.Model
	width      int
	height     int
}

// open swaps the panel onto a job. We keep the previous content visible until
// the new fetch lands so the panel doesn't flash empty between jobs.
func (p *logPanel) openOn(jobKey string, project model.Project, pipeline model.Pipeline, job model.Job) {
	p.open = true
	p.loading = true
	p.err = nil
	p.jobKey = jobKey
	p.project = project
	p.pipeline = pipeline
	p.job = job
}

func (p *logPanel) close() {
	p.open = false
	p.loading = false
	p.err = nil
	p.jobKey = ""
	p.rawContent = ""
}

func (p *logPanel) setContent(jobKey, body string, err error) {
	if jobKey != p.jobKey {
		// Result for a stale fetch — the user has moved on. Drop it.
		return
	}
	p.loading = false
	p.err = err
	p.rawContent = body
	p.refreshViewport()
}

// resize recomputes the inner content viewport whenever the terminal or
// layout changes. The viewport is what lets us scroll within the panel —
// without it, long logs would just truncate.
func (p *logPanel) resize(w, h int) {
	p.width = w
	p.height = h
	innerW := w - 4 // border (2) + horizontal padding (2)
	if innerW < 20 {
		innerW = 20
	}
	innerH := h - 3 // border (2) + header (1)
	if innerH < 3 {
		innerH = 3
	}
	if p.viewport.Width != innerW || p.viewport.Height != innerH {
		p.viewport = viewport.New(innerW, innerH)
	}
	p.refreshViewport()
}

func (p *logPanel) refreshViewport() {
	if p.viewport.Width == 0 {
		return
	}
	p.viewport.SetContent(p.formattedBody())
	// Default to the bottom — failures live at the end of the log, and
	// auto-tailing matches what every CI dashboard does.
	p.viewport.GotoBottom()
}

func (p *logPanel) formattedBody() string {
	switch {
	case p.loading && p.rawContent == "":
		return "⟳ fetching logs…"
	case p.err != nil:
		return "error: " + p.err.Error()
	case p.rawContent == "":
		return "(no log content yet)"
	}
	// Wrap each line individually with ANSI awareness so the colour escape
	// sequences keep matching open/close pairs.
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(p.rawContent, "\n"), "\n") {
		b.WriteString(ansi.Hardwrap(line, p.viewport.Width, true))
		if i < strings.Count(p.rawContent, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// scroll hooks the panel up to vim-ish nav while it has focus. We avoid
// j/k/↑↓ on purpose — those still belong to the tree; the panel uses
// keys that don't conflict (PgUp/PgDn, Ctrl-U/D, g/G).
func (p *logPanel) scroll(msg tea.KeyMsg) {
	switch msg.String() {
	case "ctrl+d", "pgdown":
		p.viewport.HalfViewDown()
	case "ctrl+u", "pgup":
		p.viewport.HalfViewUp()
	case "g":
		p.viewport.GotoTop()
	case "G":
		p.viewport.GotoBottom()
	}
}

// isPanelScrollKey reports whether the keystroke is one we forward to the
// log panel before the tree-navigation handlers see it.
func isPanelScrollKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d", "g", "G":
		return true
	}
	return false
}

// render produces the bordered right-side panel. The border colour reflects
// the job's status so a quick glance at the panel edge tells you "good vs.
// bad" without reading the header.
func (p *logPanel) render(theme Theme) string {
	if !p.open {
		return ""
	}
	statusStyle := theme.StatusStyle(p.job.Status)
	header := fmt.Sprintf(" %s %s ", statusStyle.Render(p.job.Status.Glyph()), p.job.Name)
	switch {
	case p.loading:
		header += theme.Muted.Render("· loading…")
	case p.err != nil:
		header += theme.Failed.Render("· error")
	case p.pipeline.Number > 0:
		header += theme.Muted.Render(fmt.Sprintf("· #%d · %s", p.pipeline.Number, p.pipeline.Branch))
	}
	body := p.viewport.View()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColorFor(p.job.Status)).
		Padding(0, 1).
		Width(p.width - 2). // account for the border itself
		Height(p.height - 2)

	return box.Render(strings.TrimRight(header, " ") + "\n" + body)
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

// fetchLogsCmd is the async fetch — when complete it produces a
// logsLoadedMsg, which the Model reconciles into its log panel.
func fetchLogsCmd(svc *cci.Service, project model.Project, pipeline model.Pipeline, job model.Job, jobKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		body, err := fetchLogs(ctx, svc, project, pipeline, job)
		return logsLoadedMsg{jobKey: jobKey, content: body, err: err}
	}
}

func fetchLogs(ctx context.Context, svc *cci.Service, project model.Project, pipeline model.Pipeline, job model.Job) (string, error) {
	if svc == nil {
		// Demo / fixture mode — emit something with ANSI colour codes so the
		// panel visibly demonstrates ANSI passthrough.
		return demoLogs(project, pipeline, job), nil
	}
	body, err := svc.GetFailureLogs(ctx, project.Slug, pipeline.Branch)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		body = "(no failure logs returned for this branch)"
	}
	return body, nil
}

func demoLogs(project model.Project, pipeline model.Pipeline, job model.Job) string {
	const (
		red    = "\x1b[31m"
		green  = "\x1b[32m"
		yellow = "\x1b[33m"
		grey   = "\x1b[90m"
		bold   = "\x1b[1m"
		reset  = "\x1b[0m"
	)
	header := fmt.Sprintf("%s$ run %s · %s · pipeline #%d%s\n",
		grey, project.Slug, job.Name, pipeline.Number, reset)
	body := fmt.Sprintf(`%sStarting job %s%s
%s✓%s 02:14  yarn install --frozen-lockfile  ... ok
%s✓%s 02:31  yarn lint                       ... ok
%s✓%s 02:45  yarn typecheck                  ... ok
%s⟳%s 02:48  yarn test
   PASS  src/utils/format.test.ts
   PASS  src/components/Button.test.tsx
   %sFAIL  src/api/login.test.ts%s
     ● POST /login › returns 200 on valid credentials
       %sExpected: 200%s
       %sReceived: 401%s
       at Object.<anonymous> (src/api/login.test.ts:27:24)
%s✗%s 03:12  yarn test                       ... 3 specs failed
%sFAILED %s exit code 1%s
`,
		bold, job.Name, reset,
		green, reset,
		green, reset,
		green, reset,
		yellow, reset,
		red, reset,
		green, reset,
		red, reset,
		red, reset,
		red, bold, reset,
	)
	return header + body
}
