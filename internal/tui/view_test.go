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

func TestViewWithJobDetailOpen(t *testing.T) {
	m := New(model.Fixtures())
	m.Width = 160
	m.Height = 30
	// Pipelines start collapsed — expand to land the cursor on a job row.
	m.Tree.ExpandAll()
	// Walk to a job row inside the first pipeline.
	for i := 0; i < 3; i++ {
		m.Tree.Down()
	}
	if cmd := m.toggleDetailForCurrent(); cmd == nil {
		t.Fatal("expected step-detail fetch command for selected job")
	}
	// Synthesize a detail-loaded message as if v1.1 returned three steps.
	m.detail.setDetail(m.detail.jobKey, model.JobDetail{
		Steps: []model.Step{
			{Name: "Spin up environment", Status: model.StatusSuccess, Actions: []model.StepAction{{Index: 0, StepID: 100, Status: model.StatusSuccess}}},
			{Name: "yarn test", Status: model.StatusFailed, Actions: []model.StepAction{{Index: 0, StepID: 101, Status: model.StatusFailed}}},
		},
	}, nil)
	out := m.View()
	if !strings.Contains(out, "yarn test") {
		t.Errorf("rendered view should contain step name; got:\n%s", out)
	}
	if strings.Contains(out, "form-translation") {
		t.Errorf("tree should be hidden while step-detail panel is open; got:\n%s", out)
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
