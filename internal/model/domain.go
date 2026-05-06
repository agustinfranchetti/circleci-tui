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
	Jobs      []Job
}

type Job struct {
	ID           string
	Name         string
	Status       Status
	Hint         string
	WorkflowID   string
	WorkflowName string
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
