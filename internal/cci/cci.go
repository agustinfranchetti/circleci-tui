package cci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agustinfranchetti/circleci-tui/internal/config"
	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

// configLoad is a small indirection so tests can stub out the on-disk read.
var configLoad = config.Load

// Service is the only thing the TUI talks to. All calls go through the REST
// client — v2 for the bulk of the API, v1.1 for per-job step metadata,
// /api/private for the followed-projects listing and step output. Same
// Circle-Token works across all three.
type Service struct {
	rest *restClient
}

// New constructs a Service. Returns an error if no token is supplied. Cheap —
// no subprocesses, no network calls; safe to invoke at startup.
func New(ctx context.Context, token string) (*Service, error) {
	_ = ctx
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing CircleCI token (set CIRCLECI_TOKEN or run `circleci-tui login`)")
	}
	return &Service{rest: newREST(token, "")}, nil
}

// TokenFromEnv returns the token from CIRCLECI_TOKEN, or empty if unset.
func TokenFromEnv() string { return os.Getenv("CIRCLECI_TOKEN") }

// LoadToken returns the best available token: env var first (so CI / scripted
// invocations can override anything on disk), then the persisted config file.
// Returns empty string if neither has a token — the caller should print the
// "run `circleci-tui login`" nudge.
func LoadToken() string {
	if v := strings.TrimSpace(TokenFromEnv()); v != "" {
		return v
	}
	cfg, err := configLoad()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Token)
}

// Close is a no-op kept for source compatibility with callers that used to
// shut down the MCP subprocess.
func (s *Service) Close() error { return nil }

// ListFollowedProjects walks /api/private/me/followed-projects and returns
// the user's followed projects with a normalised slug ("gh/org/repo").
func (s *Service) ListFollowedProjects(ctx context.Context) ([]Project, error) {
	raw, err := s.rest.listFollowedProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(raw))
	for _, p := range raw {
		out = append(out, Project{Slug: p.Slug, Name: p.Name})
	}
	return out, nil
}

// LoadProject fans out: list recent pipelines via REST, then for each pipeline
// list workflows + jobs. Returns a fully populated model.Project tree ready to
// hand to the TUI. limit caps the number of pipelines.
func (s *Service) LoadProject(ctx context.Context, slug, name string, limit int) (model.Project, error) {
	pipes, err := s.rest.listRecentPipelines(ctx, slug, "", limit)
	if err != nil {
		return model.Project{}, fmt.Errorf("list pipelines for %s: %w", slug, err)
	}
	mp := model.Project{Slug: slug, Name: name}
	for _, ap := range pipes {
		pl := model.Pipeline{
			ID:        ap.ID,
			Number:    ap.Number,
			Branch:    ap.VCS.Branch,
			Ticket:    extractTicket(ap.VCS.Branch),
			Status:    statusFromPipelineState(ap.State),
			Actor:     ap.Trigger.Actor.Login,
			CreatedAt: ap.CreatedAt,
		}
		wfs, jobStatus, err := s.loadPipelineWorkflows(ctx, ap.ID)
		if err != nil {
			// Don't fail the whole load — log to stderr and keep going so the
			// TUI still shows the rest of the project.
			fmt.Fprintf(os.Stderr, "warn: load workflows for pipeline %s: %v\n", ap.ID, err)
		} else {
			pl.Workflows = wfs
			pl.StartedAt, pl.StoppedAt = pipelineSpan(pl.AllJobs())
			if pl.Status == model.StatusNotRun {
				pl.Status = jobStatus
			}
		}
		mp.Pipelines = append(mp.Pipelines, pl)
	}
	return mp, nil
}

// pipelineSpan returns the earliest StartedAt and latest StoppedAt across all
// jobs. If any job is still running (zero StoppedAt), the pipeline's
// StoppedAt is left zero so model.Pipeline.Duration() falls back to "now".
func pipelineSpan(jobs []model.Job) (time.Time, time.Time) {
	var start, stop time.Time
	hasRunning := false
	for _, j := range jobs {
		if !j.StartedAt.IsZero() && (start.IsZero() || j.StartedAt.Before(start)) {
			start = j.StartedAt
		}
		if j.StoppedAt.IsZero() && !j.StartedAt.IsZero() {
			hasRunning = true
		}
		if !j.StoppedAt.IsZero() && j.StoppedAt.After(stop) {
			stop = j.StoppedAt
		}
	}
	if hasRunning {
		stop = time.Time{}
	}
	return start, stop
}

// loadPipelineWorkflows fetches every workflow under a pipeline plus their
// jobs and returns them as []model.Workflow with each workflow's jobs already
// DAG-sorted. The second+ workflow with the same name in a pipeline is
// flagged IsRerun=true — that's how CircleCI surfaces a "Rerun from failed".
func (s *Service) loadPipelineWorkflows(ctx context.Context, pipelineID string) ([]model.Workflow, model.Status, error) {
	wfs, err := s.rest.listWorkflows(ctx, pipelineID)
	if err != nil {
		return nil, model.StatusNotRun, err
	}
	worst := model.StatusSuccess
	out := make([]model.Workflow, 0, len(wfs))
	seenNames := make(map[string]int)
	for _, wf := range wfs {
		js, err := s.rest.listJobs(ctx, wf.ID)
		if err != nil {
			return nil, model.StatusNotRun, err
		}
		wfJobs := make([]model.Job, 0, len(js))
		wfStart, wfStop := time.Time{}, time.Time{}
		hasRunning := false
		for _, j := range js {
			st := statusFromJobState(j.Status)
			wfJobs = append(wfJobs, model.Job{
				ID:           j.ID,
				Number:       j.JobNumber,
				Name:         j.Name,
				Status:       st,
				WorkflowID:   wf.ID,
				WorkflowName: wf.Name,
				StartedAt:    j.StartedAt,
				StoppedAt:    j.StoppedAt,
				Dependencies: j.Dependencies,
			})
			worst = combineStatus(worst, st)
			if !j.StartedAt.IsZero() && (wfStart.IsZero() || j.StartedAt.Before(wfStart)) {
				wfStart = j.StartedAt
			}
			if j.StoppedAt.IsZero() && !j.StartedAt.IsZero() {
				hasRunning = true
			}
			if !j.StoppedAt.IsZero() && j.StoppedAt.After(wfStop) {
				wfStop = j.StoppedAt
			}
		}
		if hasRunning {
			wfStop = time.Time{}
		}
		seenNames[wf.Name]++
		ordered := sortByDAGDepth(computeJobDepths(wfJobs))
		ordered = applyTreePrefixes(ordered)
		out = append(out, model.Workflow{
			ID:        wf.ID,
			Name:      wf.Name,
			Status:    workflowStatus(ordered),
			CreatedAt: wfStart,
			StoppedAt: wfStop,
			IsRerun:   seenNames[wf.Name] > 1,
			Jobs:      ordered,
		})
	}
	return out, worst, nil
}

// workflowStatus rolls up a workflow's jobs into a single status: failed
// trumps running trumps on-hold trumps success.
func workflowStatus(jobs []model.Job) model.Status {
	hasRun, hasFailed, hasHold := false, false, false
	worst := model.StatusSuccess
	for _, j := range jobs {
		switch j.Status {
		case model.StatusFailed:
			hasFailed = true
		case model.StatusRunning:
			hasRun = true
		case model.StatusOnHold:
			hasHold = true
		}
		worst = combineStatus(worst, j.Status)
	}
	switch {
	case hasFailed:
		return model.StatusFailed
	case hasRun:
		return model.StatusRunning
	case hasHold:
		return model.StatusOnHold
	}
	return worst
}

// computeJobDepths sets Job.Depth to the topological depth of each job in the
// dependency DAG. A job with no deps (or only deps outside this workflow) has
// Depth=0; a job depending on jobs of max depth N has Depth N+1. Cycles are
// broken silently — any job in a cycle gets Depth=0.
func computeJobDepths(jobs []model.Job) []model.Job {
	idx := make(map[string]int, len(jobs))
	for i, j := range jobs {
		idx[j.ID] = i
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	state := make([]byte, len(jobs))
	depth := make([]int, len(jobs))
	var visit func(i int) int
	visit = func(i int) int {
		if state[i] == black {
			return depth[i]
		}
		if state[i] == gray {
			return 0
		}
		state[i] = gray
		d := 0
		for _, dep := range jobs[i].Dependencies {
			j, ok := idx[dep]
			if !ok {
				continue
			}
			if c := visit(j) + 1; c > d {
				d = c
			}
		}
		depth[i] = d
		state[i] = black
		return d
	}
	for i := range jobs {
		visit(i)
	}
	for i := range jobs {
		jobs[i].Depth = depth[i]
	}
	return jobs
}

// sortByDAGDepth orders jobs by depth, then by start time, then by name. So
// the rendered tree reads naturally top-to-bottom in dependency order, with
// stable ties.
func sortByDAGDepth(jobs []model.Job) []model.Job {
	sort.SliceStable(jobs, func(a, b int) bool {
		if jobs[a].Depth != jobs[b].Depth {
			return jobs[a].Depth < jobs[b].Depth
		}
		if !jobs[a].StartedAt.Equal(jobs[b].StartedAt) {
			return jobs[a].StartedAt.Before(jobs[b].StartedAt)
		}
		return jobs[a].Name < jobs[b].Name
	})
	return jobs
}

// applyTreePrefixes collapses the dependency DAG to a display tree (first
// existing dep becomes the primary parent), reorders the jobs in DFS order
// so children appear directly under their parent, and assigns a tree-drawing
// prefix (├─ / └─ / │  / "   ") to each. After this pass, renderers can just
// concatenate prefix + glyph + name and the result reads like a filetree.
func applyTreePrefixes(jobs []model.Job) []model.Job {
	type node struct {
		job      model.Job
		children []*node
	}
	nodes := make(map[string]*node, len(jobs))
	order := make([]*node, 0, len(jobs))
	for _, j := range jobs {
		n := &node{job: j}
		nodes[j.ID] = n
		order = append(order, n)
	}
	var roots []*node
	attached := make(map[string]bool, len(jobs))
	for _, n := range order {
		if attached[n.job.ID] {
			continue
		}
		var parent *node
		for _, dep := range n.job.Dependencies {
			if pn, ok := nodes[dep]; ok {
				parent = pn
				break
			}
		}
		if parent == nil {
			roots = append(roots, n)
		} else {
			parent.children = append(parent.children, n)
		}
		attached[n.job.ID] = true
	}

	out := make([]model.Job, 0, len(jobs))
	var walk func(siblings []*node, prefix string)
	walk = func(siblings []*node, prefix string) {
		for i, n := range siblings {
			isLast := i == len(siblings)-1
			connector := "├─ "
			childPrefix := prefix + "│  "
			if isLast {
				connector = "└─ "
				childPrefix = prefix + "   "
			}
			n.job.TreePrefix = prefix + connector
			out = append(out, n.job)
			walk(n.children, childPrefix)
		}
	}
	walk(roots, "")
	return out
}

func (s *Service) RerunWorkflow(ctx context.Context, workflowID string, fromFailed bool) error {
	return s.rest.rerunWorkflow(ctx, workflowID, fromFailed)
}

func (s *Service) CancelWorkflow(ctx context.Context, workflowID string) error {
	return s.rest.cancelWorkflow(ctx, workflowID)
}

func (s *Service) ApproveJob(ctx context.Context, workflowID, approvalRequestID string) error {
	return s.rest.approveJob(ctx, workflowID, approvalRequestID)
}

// GetJobDetail returns the per-step breakdown for a job. Steps preserve their
// order from CircleCI's v1.1 response; each step's actions correspond to
// parallel runs.
func (s *Service) GetJobDetail(ctx context.Context, slug string, jobNumber int) (model.JobDetail, error) {
	raw, err := s.rest.getJobDetailsV1(ctx, slug, jobNumber)
	if err != nil {
		return model.JobDetail{}, err
	}
	steps := make([]model.Step, 0, len(raw.Steps))
	for _, st := range raw.Steps {
		acts := make([]model.StepAction, 0, len(st.Actions))
		for _, a := range st.Actions {
			actStatus := statusFromJobState(a.Status)
			if a.Failed != nil && *a.Failed {
				actStatus = model.StatusFailed
			}
			acts = append(acts, model.StepAction{
				Index:     a.Index,
				StepID:    a.Step,
				Status:    actStatus,
				StartedAt: a.StartTime,
				StoppedAt: a.EndTime,
			})
		}
		steps = append(steps, model.Step{
			Name:    st.Name,
			Status:  rollupActionStatus(acts),
			Actions: acts,
		})
	}
	return model.JobDetail{JobNumber: raw.BuildNum, Steps: steps}, nil
}

// GetStepOutput fetches a single parallel run's stdout. action.StepID is the
// v1 action.step value; action.Index is the parallel-run index.
func (s *Service) GetStepOutput(ctx context.Context, slug string, jobNumber, taskIndex, stepID int) (string, error) {
	return s.rest.getStepOutput(ctx, slug, jobNumber, taskIndex, stepID, "output")
}

func rollupActionStatus(actions []model.StepAction) model.Status {
	hasFailed, hasRun := false, false
	worst := model.StatusSuccess
	for _, a := range actions {
		switch a.Status {
		case model.StatusFailed:
			hasFailed = true
		case model.StatusRunning:
			hasRun = true
		}
		worst = combineStatus(worst, a.Status)
	}
	switch {
	case hasFailed:
		return model.StatusFailed
	case hasRun:
		return model.StatusRunning
	}
	return worst
}
