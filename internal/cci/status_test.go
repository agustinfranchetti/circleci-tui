package cci

import (
	"testing"

	"github.com/agustinfranchetti/circleci-tui/internal/model"
)

func TestExtractTicket(t *testing.T) {
	cases := map[string]string{
		"CWEB-291":                 "CWEB-291",
		"CWEB-291-add-translations": "CWEB-291",
		"feature/CWEB-292-form":     "CWEB-292",
		"main":                      "",
		"release/2.4":               "",
		"hotfix/foo":                "",
	}
	for in, want := range cases {
		if got := extractTicket(in); got != want {
			t.Errorf("extractTicket(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCombineStatusFavoursWorst(t *testing.T) {
	if got := combineStatus(model.StatusSuccess, model.StatusFailed); got != model.StatusFailed {
		t.Errorf("combine(success, failed) = %v, want failed", got)
	}
	if got := combineStatus(model.StatusRunning, model.StatusSuccess); got != model.StatusRunning {
		t.Errorf("combine(running, success) = %v, want running", got)
	}
	if got := combineStatus(model.StatusFailed, model.StatusOnHold); got != model.StatusFailed {
		t.Errorf("combine(failed, onhold) = %v, want failed", got)
	}
}

func TestStatusFromJobStateCommonValues(t *testing.T) {
	cases := map[string]model.Status{
		"success":             model.StatusSuccess,
		"failed":              model.StatusFailed,
		"infrastructure_fail": model.StatusFailed,
		"running":             model.StatusRunning,
		"queued":              model.StatusRunning,
		"on_hold":             model.StatusOnHold,
		"canceled":            model.StatusCancelled,
		"weird_unknown":       model.StatusNotRun,
	}
	for in, want := range cases {
		if got := statusFromJobState(in); got != want {
			t.Errorf("statusFromJobState(%q) = %v, want %v", in, got, want)
		}
	}
}
