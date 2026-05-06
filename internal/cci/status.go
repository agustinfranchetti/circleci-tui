package cci

import (
	"regexp"
	"strings"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

type Project struct {
	Slug string
	Name string
}

// statusFromPipelineState maps CircleCI pipeline states to our Status enum.
// Pipeline states: created, errored, setup-pending, setup, pending.
// They mostly describe "is the pipeline still computing its workflows", not
// the success/fail status — we derive overall status from the workflows.
func statusFromPipelineState(s string) model.Status {
	switch strings.ToLower(s) {
	case "errored":
		return model.StatusFailed
	case "setup-pending", "setup", "pending", "created":
		return model.StatusNotRun
	default:
		return model.StatusNotRun
	}
}

// statusFromJobState maps CircleCI job statuses to our Status enum.
// Possible values: success, running, not_run, failed, retried, queued,
// not_running, infrastructure_fail, timedout, on_hold, terminated-unknown,
// blocked, canceled, unauthorized.
func statusFromJobState(s string) model.Status {
	switch strings.ToLower(s) {
	case "success":
		return model.StatusSuccess
	case "failed", "infrastructure_fail", "timedout", "terminated-unknown", "unauthorized":
		return model.StatusFailed
	case "running", "queued":
		return model.StatusRunning
	case "on_hold", "blocked":
		return model.StatusOnHold
	case "canceled", "cancelled":
		return model.StatusCancelled
	default:
		return model.StatusNotRun
	}
}

func combineStatus(a, b model.Status) model.Status {
	rank := func(s model.Status) int {
		switch s {
		case model.StatusFailed:
			return 5
		case model.StatusOnHold:
			return 4
		case model.StatusRunning:
			return 3
		case model.StatusCancelled:
			return 2
		case model.StatusSuccess:
			return 1
		default:
			return 0
		}
	}
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

var ticketRE = regexp.MustCompile(`^[A-Z]{2,8}-\d+`)

// extractTicket pulls a JIRA-style ticket key out of a branch name like
// "CWEB-291-foo" → "CWEB-291". Returns empty if no match.
func extractTicket(branch string) string {
	branch = strings.TrimPrefix(strings.ToUpper(branch), "FEATURE/")
	branch = strings.TrimPrefix(branch, "FIX/")
	return ticketRE.FindString(strings.ToUpper(branch))
}
