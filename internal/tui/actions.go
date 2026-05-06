package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/agustinfranchetti/circleci-tui/internal/cci"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

type actionKind int

const (
	actionRerun actionKind = iota
	actionRerunFromFailed
	actionCancel
	actionApproveHold
	actionOpenInBrowser
)

func (a actionKind) label() string {
	switch a {
	case actionRerun:
		return "Rerun workflow"
	case actionRerunFromFailed:
		return "Rerun from failed"
	case actionCancel:
		return "Cancel running workflow"
	case actionApproveHold:
		return "Approve hold"
	case actionOpenInBrowser:
		return "Open in browser"
	}
	return "?"
}

func (a actionKind) destructive() bool {
	return a == actionCancel
}

type actionItem struct {
	kind actionKind
	// Resolved targets for this action — kept on the item so we don't have
	// to recompute them when the user finally picks something.
	workflowID   string
	approvalJob  string
	browserURL   string
}

type actionsMenu struct {
	open      bool
	confirm   bool
	cursor    int
	items     []actionItem
	rowSummary string
	target     actionTarget
}

type actionTarget struct {
	project  model.Project
	pipeline model.Pipeline
	job      model.Job
	hasJob   bool
}

// buildActionMenu inspects the selected row and produces the relevant action
// items. Items that require a workflow ID we don't have are skipped — better
// to omit a row than show an action that will fail.
func buildActionMenu(t *model.Tree) (actionsMenu, bool) {
	r, ok := t.Current()
	if !ok || r.PipeIdx < 0 {
		return actionsMenu{}, false
	}
	proj := t.Projects[r.ProjIdx]
	pipe := proj.Pipelines[r.PipeIdx]
	target := actionTarget{project: proj, pipeline: pipe}

	var workflowID string
	var approvalJobID string
	var summary string

	if r.JobIdx >= 0 && r.JobIdx < len(pipe.Jobs) {
		job := pipe.Jobs[r.JobIdx]
		target.job = job
		target.hasJob = true
		workflowID = job.WorkflowID
		summary = fmt.Sprintf("%s · #%d · %s", proj.Name, pipe.Number, job.Name)
		if job.Status == model.StatusOnHold {
			approvalJobID = job.ID
		}
	} else {
		// Pipeline row — find a workflow ID by looking at any of its jobs.
		for _, j := range pipe.Jobs {
			if j.WorkflowID != "" {
				workflowID = j.WorkflowID
				break
			}
		}
		summary = fmt.Sprintf("%s · #%d · %s", proj.Name, pipe.Number, pipe.Branch)
	}

	browserURL := buildBrowserURL(proj.Slug, pipe.Number, workflowID, target.job.ID)

	var items []actionItem
	switch {
	case approvalJobID != "":
		items = append(items, actionItem{kind: actionApproveHold, workflowID: workflowID, approvalJob: approvalJobID})
	case pipe.Status == model.StatusRunning, target.hasJob && target.job.Status == model.StatusRunning:
		if workflowID != "" {
			items = append(items, actionItem{kind: actionCancel, workflowID: workflowID})
		}
	}
	if workflowID != "" {
		items = append(items, actionItem{kind: actionRerunFromFailed, workflowID: workflowID})
		items = append(items, actionItem{kind: actionRerun, workflowID: workflowID})
	}
	items = append(items, actionItem{kind: actionOpenInBrowser, browserURL: browserURL})

	if len(items) == 0 {
		return actionsMenu{}, false
	}
	return actionsMenu{
		open:       true,
		items:      items,
		rowSummary: summary,
		target:     target,
	}, true
}

func buildBrowserURL(slug string, pipelineNumber int, workflowID, jobID string) string {
	base := "https://app.circleci.com/pipelines/" + slug
	if pipelineNumber > 0 {
		base = fmt.Sprintf("%s/%d", base, pipelineNumber)
	}
	if workflowID != "" {
		base += "/workflows/" + workflowID
	}
	if jobID != "" {
		base += "/jobs/" + jobID
	}
	return base
}

func (a *actionsMenu) up() {
	if a.cursor > 0 {
		a.cursor--
	}
}
func (a *actionsMenu) down() {
	if a.cursor < len(a.items)-1 {
		a.cursor++
	}
}
func (a *actionsMenu) current() (actionItem, bool) {
	if a.cursor < 0 || a.cursor >= len(a.items) {
		return actionItem{}, false
	}
	return a.items[a.cursor], true
}

// actionResultMsg is what an action's tea.Cmd returns once the call completes.
type actionResultMsg struct {
	kind   actionKind
	target string
	err    error
}

// runActionCmd dispatches the chosen action to the Service. open-in-browser
// is local-only and short-circuits without hitting the Service.
func runActionCmd(svc *cci.Service, target actionTarget, item actionItem) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		summary := fmt.Sprintf("%s · #%d", target.project.Name, target.pipeline.Number)
		switch item.kind {
		case actionOpenInBrowser:
			if err := openInBrowser(item.browserURL); err != nil {
				return actionResultMsg{kind: item.kind, target: summary, err: err}
			}
			return actionResultMsg{kind: item.kind, target: summary}
		}
		if svc == nil {
			return actionResultMsg{
				kind:   item.kind,
				target: summary,
				err:    errors.New("demo mode — no Service available"),
			}
		}
		var err error
		switch item.kind {
		case actionRerun:
			err = svc.RerunWorkflow(ctx, item.workflowID, false)
		case actionRerunFromFailed:
			err = svc.RerunWorkflow(ctx, item.workflowID, true)
		case actionCancel:
			err = svc.CancelWorkflow(ctx, item.workflowID)
		case actionApproveHold:
			err = svc.ApproveJob(ctx, item.workflowID, item.approvalJob)
		}
		return actionResultMsg{kind: item.kind, target: summary, err: err}
	}
}

func openInBrowser(url string) error {
	if url == "" {
		return errors.New("no URL to open")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("don't know how to open URLs on %s", runtime.GOOS)
	}
	return cmd.Start()
}

// renderActionsMenu draws the contextual menu (or its confirmation prompt).
func (m Model) renderActionsMenu() string {
	t := m.Theme
	var b strings.Builder
	b.WriteString(t.Header.Render(" actions "))
	b.WriteString("\n")
	b.WriteString(t.Muted.Render(m.actions.rowSummary))
	b.WriteString("\n\n")

	if m.actions.confirm {
		item, _ := m.actions.current()
		warn := t.Failed.Render(fmt.Sprintf(" %s? ", item.kind.label()))
		b.WriteString(warn)
		b.WriteString("\n\n")
		b.WriteString(t.StatusKey.Render("y") + t.StatusInfo.Render(" yes  "))
		b.WriteString(t.StatusKey.Render("N/esc") + t.StatusInfo.Render(" cancel"))
	} else {
		for i, it := range m.actions.items {
			marker := "  "
			if i == m.actions.cursor {
				marker = "▸ "
			}
			line := marker + it.kind.label()
			if it.kind.destructive() {
				line += t.Muted.Render("  (will prompt)")
			}
			if i == m.actions.cursor {
				line = t.Selected.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		BorderForeground(lipgloss.AdaptiveColor{Dark: "#a78bfa", Light: "#7c3aed"}).
		Render(b.String())
	return box
}
