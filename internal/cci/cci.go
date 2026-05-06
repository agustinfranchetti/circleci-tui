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

// Service is the only thing the TUI talks to. Internally it dispatches each
// call to either the MCP subprocess or the REST client (or both, for calls
// that fan out — e.g. listing pipelines then their workflows + jobs).
type Service struct {
	rest *restClient
	mcp  *mcpClient
}

// New wires up both transports. Spawning the MCP subprocess is the slow part
// (npx cold start is several seconds on first run, ~1s warm). Callers should
// invoke this once at startup and reuse the Service for the lifetime of the
// program.
func New(ctx context.Context, token string) (*Service, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing CircleCI token (set CIRCLECI_TOKEN or run `circleci-tui login`)")
	}
	mcpC, err := newMCP(ctx, token)
	if err != nil {
		return nil, err
	}
	return &Service{
		rest: newREST(token, ""),
		mcp:  mcpC,
	}, nil
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

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	return s.mcp.Close()
}

func (s *Service) ListFollowedProjects(ctx context.Context) ([]Project, error) {
	raw, err := s.mcp.ListFollowedProjects(ctx)
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
		jobs, jobStatus, err := s.loadPipelineJobs(ctx, ap.ID)
		if err != nil {
			// Don't fail the whole load — log to stderr and keep going so the
			// TUI still shows the rest of the project.
			fmt.Fprintf(os.Stderr, "warn: load jobs for pipeline %s: %v\n", ap.ID, err)
		} else {
			pl.Jobs = jobs
			pl.StartedAt, pl.StoppedAt = pipelineSpan(jobs)
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

func (s *Service) loadPipelineJobs(ctx context.Context, pipelineID string) ([]model.Job, model.Status, error) {
	wfs, err := s.rest.listWorkflows(ctx, pipelineID)
	if err != nil {
		return nil, model.StatusNotRun, err
	}
	var jobs []model.Job
	worst := model.StatusSuccess
	for _, wf := range wfs {
		js, err := s.rest.listJobs(ctx, wf.ID)
		if err != nil {
			return nil, model.StatusNotRun, err
		}
		wfJobs := make([]model.Job, 0, len(js))
		for _, j := range js {
			st := statusFromJobState(j.Status)
			wfJobs = append(wfJobs, model.Job{
				ID:           j.ID,
				Name:         j.Name,
				Status:       st,
				WorkflowID:   wf.ID,
				WorkflowName: wf.Name,
				StartedAt:    j.StartedAt,
				StoppedAt:    j.StoppedAt,
				Dependencies: j.Dependencies,
			})
			worst = combineStatus(worst, st)
		}
		jobs = append(jobs, sortByDAGDepth(computeJobDepths(wfJobs))...)
	}
	return jobs, worst, nil
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

func (s *Service) RerunWorkflow(ctx context.Context, workflowID string, fromFailed bool) error {
	return s.mcp.RerunWorkflow(ctx, workflowID, fromFailed)
}

func (s *Service) CancelWorkflow(ctx context.Context, workflowID string) error {
	return s.rest.cancelWorkflow(ctx, workflowID)
}

func (s *Service) ApproveJob(ctx context.Context, workflowID, approvalRequestID string) error {
	return s.rest.approveJob(ctx, workflowID, approvalRequestID)
}


func (s *Service) GetFailureLogs(ctx context.Context, slug, branch string) (string, error) {
	return s.mcp.GetBuildFailureLogs(ctx, slug, branch)
}
