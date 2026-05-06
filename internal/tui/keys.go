package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Toggle      key.Binding
	ExpandAll   key.Binding
	CollapseAll key.Binding
	Logs   key.Binding
	Filter key.Binding
	Mine        key.Binding
	Refresh     key.Binding
	Actions     key.Binding
	Picker      key.Binding
	Cancel      key.Binding
	Quit        key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("⏎", "expand"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("+", "="),
			key.WithHelp("+", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("-", "_"),
			key.WithHelp("-", "collapse all"),
		),
		Logs: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "logs"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Mine: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mine"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Actions: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "actions"),
		),
		Picker: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "focus project"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
