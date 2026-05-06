package cci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
			if pl.Status == model.StatusNotRun {
				pl.Status = jobStatus
			}
		}
		mp.Pipelines = append(mp.Pipelines, pl)
	}
	return mp, nil
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
		for _, j := range js {
			st := statusFromJobState(j.Status)
			jobs = append(jobs, model.Job{
				ID:           j.ID,
				Name:         j.Name,
				Status:       st,
				WorkflowID:   wf.ID,
				WorkflowName: wf.Name,
			})
			worst = combineStatus(worst, st)
		}
	}
	return jobs, worst, nil
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
