package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

type Theme struct {
	Success   lipgloss.Style
	Failed    lipgloss.Style
	Running   lipgloss.Style
	OnHold    lipgloss.Style
	Cancelled lipgloss.Style
	NotRun    lipgloss.Style

	Header     lipgloss.Style
	Branch     lipgloss.Style
	Ticket     lipgloss.Style
	Muted      lipgloss.Style
	Selected   lipgloss.Style
	StatusBar  lipgloss.Style
	StatusKey  lipgloss.Style
	StatusInfo lipgloss.Style
	Title      lipgloss.Style
}

func DefaultTheme() Theme {
	col := func(dark, light string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Dark: dark, Light: light}
	}
	success := lipgloss.NewStyle().Foreground(col("#22c55e", "#15803d"))
	failed := lipgloss.NewStyle().Foreground(col("#ef4444", "#b91c1c")).Bold(true)
	running := lipgloss.NewStyle().Foreground(col("#06b6d4", "#0e7490"))
	onhold := lipgloss.NewStyle().Foreground(col("#eab308", "#a16207"))
	cancelled := lipgloss.NewStyle().Foreground(col("#6b7280", "#4b5563"))
	notrun := lipgloss.NewStyle().Foreground(col("#4b5563", "#9ca3af"))
	muted := lipgloss.NewStyle().Foreground(col("#9ca3af", "#6b7280"))

	return Theme{
		Success:    success,
		Failed:     failed,
		Running:    running,
		OnHold:     onhold,
		Cancelled:  cancelled,
		NotRun:     notrun,
		Header:     lipgloss.NewStyle().Bold(true),
		Branch:     muted,
		Ticket:     lipgloss.NewStyle().Foreground(col("#a78bfa", "#7c3aed")),
		Muted:      muted,
		Selected:   lipgloss.NewStyle().Reverse(true),
		StatusBar:  muted,
		StatusKey:  lipgloss.NewStyle().Foreground(col("#a78bfa", "#7c3aed")).Bold(true),
		StatusInfo: muted,
		Title:      lipgloss.NewStyle().Bold(true).Foreground(col("#a78bfa", "#7c3aed")),
	}
}

func (t Theme) StatusStyle(s model.Status) lipgloss.Style {
	switch s {
	case model.StatusSuccess:
		return t.Success
	case model.StatusFailed:
		return t.Failed
	case model.StatusRunning:
		return t.Running
	case model.StatusOnHold:
		return t.OnHold
	case model.StatusCancelled:
		return t.Cancelled
	default:
		return t.NotRun
	}
}
