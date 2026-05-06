package model

import "time"

type Status string

const (
	StatusSuccess   Status = "success"
	StatusFailed    Status = "failed"
	StatusRunning   Status = "running"
	StatusOnHold    Status = "on_hold"
	StatusCancelled Status = "cancelled"
	StatusNotRun    Status = "not_run"
)

func (s Status) Glyph() string {
	switch s {
	case StatusSuccess:
		return "✓"
	case StatusFailed:
		return "✗"
	case StatusRunning:
		return "⟳"
	case StatusOnHold:
		return "⏸"
	case StatusCancelled:
		return "⊘"
	default:
		return "·"
	}
}

type Project struct {
	Slug      string
	Name      string
	Pipelines []Pipeline
}

type Pipeline struct {
	ID        string
	Number    int
	Branch    string
	Ticket    string
	Status    Status
	Actor     string
	CreatedAt time.Time
	StartedAt time.Time
	StoppedAt time.Time
	Jobs      []Job
}

// Duration returns the wall-clock span of the pipeline. For finished pipelines
// it's StoppedAt-StartedAt; for running ones it's now-StartedAt. Zero if we
// don't have any timing yet.
func (p Pipeline) Duration() time.Duration {
	if p.StartedAt.IsZero() {
		return 0
	}
	end := p.StoppedAt
	if end.IsZero() {
		end = time.Now()
	}
	if end.Before(p.StartedAt) {
		return 0
	}
	return end.Sub(p.StartedAt)
}

type Job struct {
	ID           string
	Name         string
	Status       Status
	Hint         string
	WorkflowID   string
	WorkflowName string
	StartedAt    time.Time
	StoppedAt    time.Time
	// Dependencies is the list of job IDs that must complete before this one
	// runs (CircleCI's `requires:` from the workflow YAML). Used to render the
	// DAG as a tree.
	Dependencies []string
	// Depth is the topological depth of this job within its workflow — the
	// length of the longest dependency chain from any root job. Set by
	// computeJobDepths. Roots have Depth=0.
	Depth int
}

func (j Job) Duration() time.Duration {
	if j.StartedAt.IsZero() {
		return 0
	}
	end := j.StoppedAt
	if end.IsZero() {
		end = time.Now()
	}
	if end.Before(j.StartedAt) {
		return 0
	}
	return end.Sub(j.StartedAt)
}

func (p Project) WorstStatus() Status {
	hasFailed, hasRunning := false, false
	for _, pl := range p.Pipelines {
		switch pl.Status {
		case StatusFailed:
			hasFailed = true
		case StatusRunning:
			hasRunning = true
		}
	}
	switch {
	case hasFailed:
		return StatusFailed
	case hasRunning:
		return StatusRunning
	default:
		return StatusSuccess
	}
}

func (p Project) FailedCount() int {
	n := 0
	for _, pl := range p.Pipelines {
		if pl.Status == StatusFailed {
			n++
		}
	}
	return n
}

func (p Project) RunningCount() int {
	n := 0
	for _, pl := range p.Pipelines {
		if pl.Status == StatusRunning {
			n++
		}
	}
	return n
}
