package tui

import (
	"strings"
	"testing"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

func TestViewRendersAllProjects(t *testing.T) {
	m := New(model.Fixtures())
	out := m.View()
	for _, p := range m.Tree.Projects {
		if !strings.Contains(out, p.Name) {
			t.Errorf("View() missing project name %q", p.Name)
		}
	}
}

func TestViewShowsFailedPipelineDetails(t *testing.T) {
	m := New(model.Fixtures())
	// Pipelines start collapsed; expand them so the job hints (e.g. "3 specs
	// failed") become reachable.
	m.Tree.ExpandAll()
	out := m.View()
	if !strings.Contains(out, "CWEB-291") {
		t.Errorf("View() should surface CWEB-291 ticket on the failed pipeline:\n%s", out)
	}
	if !strings.Contains(out, "3 specs failed") {
		t.Errorf("View() should surface the failed-job hint; got:\n%s", out)
	}
}

func TestViewWithActiveFilters(t *testing.T) {
	m := New(model.Fixtures())
	m.Tree.SetTextFilter("CWEB-292")
	m.Tree.SetMineOnly(true, "agustinfranchetti")
	out := m.View()
	if !strings.Contains(out, "filter:CWEB-292") {
		t.Errorf("active text filter should appear in title badge:\n%s", out)
	}
	if !strings.Contains(out, "mine=") {
		t.Errorf("mine-only should appear in title badge:\n%s", out)
	}
}

func TestViewWithLogPanelOpen(t *testing.T) {
	m := New(model.Fixtures())
	m.Width = 160
	m.Height = 30
	// Pipelines start collapsed — expand the failed pipeline so we can land
	// the cursor on its failing test job.
	m.Tree.ExpandAll()
	// Walk to the failed test job (project 0 → pipeline 0 → job 1).
	for i := 0; i < 3; i++ {
		m.Tree.Down()
	}
	if cmd := m.toggleLogsForCurrent(); cmd == nil {
		t.Fatal("expected log fetch command for selected job")
	}
	m.logs.setContent(m.logs.jobKey, "test failure log\nline2\n", nil)
	out := m.View()
	if !strings.Contains(out, "test failure log") {
		t.Errorf("rendered view should contain log body; got:\n%s", out)
	}
	// With the panel open we now hide the tree to give logs the full width;
	// project names should NOT appear in the rendered view.
	if strings.Contains(out, "form-translation") {
		t.Errorf("tree should be hidden while log panel is open; got:\n%s", out)
	}
}

func TestViewWithActionsMenuOpen(t *testing.T) {
	m := New(model.Fixtures())
	// Cursor is on the first project header by default; nudge to the first
	// failed pipeline (form-translation #1234).
	m.Tree.Down()
	menu, ok := buildActionMenu(m.Tree)
	if !ok {
		t.Fatal("expected action menu to build for pipeline row")
	}
	m.actions = menu
	out := m.View()
	if !strings.Contains(out, "Rerun") {
		t.Errorf("actions menu should list rerun options:\n%s", out)
	}
}
