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
	Workflows []Workflow
}

// Workflow is one execution of a pipeline's workflow definition. A pipeline
// can have multiple workflows when CircleCI's "Rerun" actions add new
// workflows under the same pipeline — IsRerun flags those so the renderer
// can label them visually.
type Workflow struct {
	ID        string
	Name      string
	Status    Status
	CreatedAt time.Time
	StoppedAt time.Time
	IsRerun   bool
	Jobs      []Job
}

func (w Workflow) Duration() time.Duration {
	if w.CreatedAt.IsZero() {
		return 0
	}
	end := w.StoppedAt
	if end.IsZero() {
		end = time.Now()
	}
	if end.Before(w.CreatedAt) {
		return 0
	}
	return end.Sub(w.CreatedAt)
}

// AllJobs flattens jobs from every workflow under this pipeline. Used by
// `pipelineSpan` and any consumer that doesn't care about workflow grouping.
func (p Pipeline) AllJobs() []Job {
	var out []Job
	for _, wf := range p.Workflows {
		out = append(out, wf.Jobs...)
	}
	return out
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
	Number       int // CircleCI job_number — required for v1.1 step lookups
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
	// TreePrefix is the precomputed unicode tree-drawing prefix that goes
	// before the job's status glyph (e.g. "├─ ", "│  └─ "). Reflects the
	// dependency DAG collapsed to a tree (each job's primary parent is the
	// first dependency that exists in the workflow). Empty for jobs whose
	// workflow context isn't loaded yet.
	TreePrefix string
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

// JobDetail is the per-job step breakdown the user drills into when they
// press `e` (or click) on a job row. It mirrors what CircleCI's web UI Steps
// tab shows.
type JobDetail struct {
	JobNumber int
	JobName   string
	Steps     []Step
}

// Step is one entry in a job's step list. A step with parallelism > 1 has
// multiple Actions; the rolled-up Status reflects the worst across all
// actions.
type Step struct {
	Name      string
	Status    Status
	Actions   []StepAction
}

// StepAction is one parallel run of a step. Index is the task index (0..N-1
// when parallelism is N); StepID is the v1 action.step value used to fetch
// the action's raw output.
type StepAction struct {
	Index     int
	StepID    int
	Status    Status
	StartedAt time.Time
	StoppedAt time.Time
}

func (s Step) Duration() time.Duration {
	if len(s.Actions) == 0 {
		return 0
	}
	var first, last time.Time
	for _, a := range s.Actions {
		if !a.StartedAt.IsZero() && (first.IsZero() || a.StartedAt.Before(first)) {
			first = a.StartedAt
		}
		end := a.StoppedAt
		if end.IsZero() {
			end = time.Now()
		}
		if end.After(last) {
			last = end
		}
	}
	if first.IsZero() {
		return 0
	}
	return last.Sub(first)
}

// IsParallel returns true when the step ran with parallelism > 1.
func (s Step) IsParallel() bool { return len(s.Actions) > 1 }

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
