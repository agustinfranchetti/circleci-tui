package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

// projectPicker is a small overlay that lets the user focus the tree on a
// single project. It is intentionally a plain struct (not a separate Bubble
// Tea component) — the parent Model owns its key handling and lifecycle.
type projectPicker struct {
	open    bool
	cursor  int
	options []pickerOption
}

type pickerOption struct {
	slug  string // empty for the "all projects" entry
	label string
	count string // e.g. "(2 failed)"
	worst model.Status
}

func (p *projectPicker) build(projects []model.Project, currentFocus string) {
	p.options = p.options[:0]
	p.options = append(p.options, pickerOption{label: "all projects"})
	for _, proj := range projects {
		count := projectCount(proj)
		p.options = append(p.options, pickerOption{
			slug:  proj.Slug,
			label: proj.Name,
			count: count,
			worst: proj.WorstStatus(),
		})
	}
	p.cursor = 0
	for i, opt := range p.options {
		if opt.slug == currentFocus {
			p.cursor = i
			break
		}
	}
}

func (p *projectPicker) up() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *projectPicker) down() {
	if p.cursor < len(p.options)-1 {
		p.cursor++
	}
}

func (p *projectPicker) selected() (slug string) {
	if p.cursor < 0 || p.cursor >= len(p.options) {
		return ""
	}
	return p.options[p.cursor].slug
}

func (m Model) renderPicker() string {
	t := m.Theme
	var b strings.Builder
	b.WriteString(t.Header.Render(" focus a project "))
	b.WriteString("\n\n")
	for i, opt := range m.picker.options {
		marker := "  "
		if i == m.picker.cursor {
			marker = "▸ "
		}
		var line string
		if opt.slug == "" {
			line = fmt.Sprintf("%s%s", marker, opt.label)
		} else {
			styledCount := t.StatusStyle(opt.worst).Render(opt.count)
			line = fmt.Sprintf("%s%-24s  %s", marker, opt.label, styledCount)
		}
		if i == m.picker.cursor {
			line = t.Selected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	body := b.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		BorderForeground(lipgloss.AdaptiveColor{Dark: "#a78bfa", Light: "#7c3aed"}).
		Render(body)
	return box
}
