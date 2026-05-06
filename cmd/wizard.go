package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/config"
)

// Configure runs the first-launch wizard: list every project the user follows
// on CircleCI, let them check the ones they care about, persist the selection
// to ~/.config/circleci-tui/config.toml. Non-interactive friendly: with
// `--all` we just save every followed project without prompting.
func Configure(args []string) error {
	if !hasToken() {
		return fmt.Errorf("no token configured — run `circleci-tui login` first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stderr, "fetching followed projects from CircleCI…")
	svc, err := cci.New(ctx, cci.LoadToken())
	if err != nil {
		return err
	}
	defer svc.Close()
	projects, err := svc.ListFollowedProjects(ctx)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintln(os.Stderr, "no projects followed on CircleCI — nothing to choose from.")
		return nil
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })

	cfg, _ := config.Load()
	already := map[string]bool{}
	for _, slug := range cfg.WatchedProjects {
		already[slug] = true
	}

	if hasFlag(args, "--all") {
		var slugs []string
		for _, p := range projects {
			slugs = append(slugs, p.Slug)
		}
		cfg.WatchedProjects = slugs
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "✓ watching all %d projects\n", len(slugs))
		return nil
	}

	model := newWizard(projects, already)
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	w, ok := final.(wizardModel)
	if !ok || w.cancelled {
		fmt.Fprintln(os.Stderr, "cancelled — config not changed")
		return nil
	}
	cfg.WatchedProjects = w.selectedSlugs()
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ watching %d project(s)\n", len(cfg.WatchedProjects))
	return nil
}

func hasToken() bool { return strings.TrimSpace(cci.LoadToken()) != "" }

func hasFlag(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// --- wizard Bubble Tea model -----------------------------------------------

type wizardModel struct {
	projects  []cci.Project
	checked   []bool
	cursor    int
	keys      wizardKeyMap
	cancelled bool
	done      bool
}

type wizardKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding
	All    key.Binding
	None   key.Binding
	Accept key.Binding
	Cancel key.Binding
}

func defaultWizardKeys() wizardKeyMap {
	return wizardKeyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k")),
		Down:   key.NewBinding(key.WithKeys("down", "j")),
		Toggle: key.NewBinding(key.WithKeys(" ")),
		All:    key.NewBinding(key.WithKeys("a")),
		None:   key.NewBinding(key.WithKeys("n")),
		Accept: key.NewBinding(key.WithKeys("enter")),
		Cancel: key.NewBinding(key.WithKeys("esc", "q", "ctrl+c")),
	}
}

func newWizard(projects []cci.Project, alreadyChecked map[string]bool) wizardModel {
	checked := make([]bool, len(projects))
	for i, p := range projects {
		checked[i] = alreadyChecked[p.Slug] || len(alreadyChecked) == 0
		// First-time setup (no prior selection): default everything ON so the
		// user can opt-out rather than opt-in. Less work for the common case.
	}
	return wizardModel{
		projects: projects,
		checked:  checked,
		keys:     defaultWizardKeys(),
	}
}

func (m wizardModel) Init() tea.Cmd { return nil }

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(km, m.keys.Cancel):
		m.cancelled = true
		return m, tea.Quit
	case key.Matches(km, m.keys.Accept):
		m.done = true
		return m, tea.Quit
	case key.Matches(km, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(km, m.keys.Down):
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
	case key.Matches(km, m.keys.Toggle):
		m.checked[m.cursor] = !m.checked[m.cursor]
	case key.Matches(km, m.keys.All):
		for i := range m.checked {
			m.checked[i] = true
		}
	case key.Matches(km, m.keys.None):
		for i := range m.checked {
			m.checked[i] = false
		}
	}
	return m, nil
}

func (m wizardModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Dark: "#a78bfa", Light: "#7c3aed"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#9ca3af", Light: "#6b7280"})
	selected := lipgloss.NewStyle().Reverse(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(" pick projects to watch "))
	b.WriteString("\n\n")
	count := 0
	for i, p := range m.projects {
		mark := "[ ]"
		if m.checked[i] {
			mark = "[x]"
			count++
		}
		line := fmt.Sprintf("%s %-30s %s", mark, p.Name, muted.Render(p.Slug))
		if i == m.cursor {
			line = selected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(muted.Render(fmt.Sprintf("%d / %d selected", count, len(m.projects))))
	b.WriteString("\n\n")
	help := []string{
		"↑↓/jk nav",
		"space toggle",
		"a all",
		"n none",
		"⏎ accept",
		"esc cancel",
	}
	b.WriteString(muted.Render(strings.Join(help, " · ")))
	return b.String()
}

func (m wizardModel) selectedSlugs() []string {
	var out []string
	for i, p := range m.projects {
		if m.checked[i] {
			out = append(out, p.Slug)
		}
	}
	return out
}
