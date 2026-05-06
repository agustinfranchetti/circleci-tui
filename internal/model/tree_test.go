package model

import "testing"

func TestNewTreeStartsWithPipelinesCollapsed(t *testing.T) {
	tr := NewTree(Fixtures())
	for _, r := range tr.Visible() {
		if r.Kind == RowJob {
			t.Errorf("expected no job rows in default view (pipelines collapsed); got %+v", r)
		}
	}
}

func TestExpandAllAndCollapseAll(t *testing.T) {
	tr := NewTree(Fixtures())
	tr.ExpandAll()
	allOpen := len(tr.Visible())
	tr.CollapseAll()
	collapsed := len(tr.Visible())
	if collapsed >= allOpen {
		t.Errorf("CollapseAll should reduce visible rows; got %d before, %d after", allOpen, collapsed)
	}
	if collapsed != len(tr.Projects) {
		t.Errorf("CollapseAll should leave only project headers visible (%d); got %d", len(tr.Projects), collapsed)
	}
	tr.ExpandAll()
	if got := len(tr.Visible()); got != allOpen {
		t.Errorf("ExpandAll should restore full row set; want %d, got %d", allOpen, got)
	}
}

func TestAdoptCollapsedFromPreservesAcrossRefresh(t *testing.T) {
	old := NewTree(Fixtures())
	old.CollapseAll()
	collapsedCount := len(old.Visible())

	// Simulate a refresh: same data, brand-new tree (this is what loadedMsg does).
	fresh := NewTree(Fixtures())
	fresh.AdoptCollapsedFrom(old)
	if got := len(fresh.Visible()); got != collapsedCount {
		t.Errorf("AdoptCollapsedFrom didn't preserve state; want %d, got %d", collapsedCount, got)
	}
}

func TestToggleExpandsAndCollapses(t *testing.T) {
	tr := NewTree(Fixtures())
	before := len(tr.Visible())
	tr.ToggleCurrent() // cursor starts at 0 = first project header
	after := len(tr.Visible())
	if after == before {
		t.Errorf("toggle on project header changed visible rows from %d to %d (no change)", before, after)
	}
}

func TestTextFilterMatchesBranchAndTicket(t *testing.T) {
	tr := NewTree(Fixtures())
	tr.SetTextFilter("CWEB-292")
	rows := tr.Visible()
	if len(rows) == 0 {
		t.Fatal("expected at least one match for CWEB-292")
	}
	for _, r := range rows {
		if r.Kind != RowPipeline {
			continue
		}
		pl := tr.Projects[r.ProjIdx].Pipelines[r.PipeIdx]
		if pl.Branch != "CWEB-292" && pl.Ticket != "CWEB-292" {
			t.Errorf("filter leaked through: pipeline branch=%q ticket=%q", pl.Branch, pl.Ticket)
		}
	}
}

func TestMineOnlyHidesOthers(t *testing.T) {
	tr := NewTree(Fixtures())
	tr.SetMineOnly(true, "tealc")
	for _, r := range tr.Visible() {
		if r.Kind != RowPipeline {
			continue
		}
		pl := tr.Projects[r.ProjIdx].Pipelines[r.PipeIdx]
		if pl.Actor != "tealc" {
			t.Errorf("mine=tealc leaked pipeline by %q", pl.Actor)
		}
	}
}

func TestFiltersHideEmptyProjectHeaders(t *testing.T) {
	tr := NewTree(Fixtures())
	tr.SetTextFilter("nothing-matches-this-string")
	rows := tr.Visible()
	if len(rows) != 0 {
		t.Errorf("expected zero rows when no pipeline matches; got %d", len(rows))
	}
}

func TestSetFocusFiltersToOneProject(t *testing.T) {
	tr := NewTree(Fixtures())
	allRows := len(tr.Visible())
	tr.SetFocus("gh/form/form-translation")
	rows := tr.Visible()
	if len(rows) >= allRows {
		t.Errorf("focus should reduce visible rows; got %d before, %d after", allRows, len(rows))
	}
	for _, r := range rows {
		if tr.Projects[r.ProjIdx].Slug != "gh/form/form-translation" {
			t.Errorf("focused tree showed project %q which is not the focus", tr.Projects[r.ProjIdx].Slug)
		}
	}
	tr.SetFocus("")
	if got := len(tr.Visible()); got != allRows {
		t.Errorf("clearing focus should restore full row set; want %d, got %d", allRows, got)
	}
}

func TestNavigation(t *testing.T) {
	tr := NewTree(Fixtures())
	if tr.Cursor() != 0 {
		t.Fatalf("cursor starts at %d, want 0", tr.Cursor())
	}
	tr.Down()
	if tr.Cursor() != 1 {
		t.Errorf("after Down cursor=%d, want 1", tr.Cursor())
	}
	tr.Up()
	if tr.Cursor() != 0 {
		t.Errorf("after Up cursor=%d, want 0", tr.Cursor())
	}
	tr.Up() // already at top — should clamp
	if tr.Cursor() != 0 {
		t.Errorf("Up at top should clamp; cursor=%d", tr.Cursor())
	}
}
