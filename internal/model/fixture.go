package model

import "time"

// Fixtures returns a hardcoded snapshot used during phase 1 development before
// the MCP client is wired. It is intentionally varied so the rendering paths
// (failed/running/success/onhold/cancelled, expanded/collapsed) all get hit.
func Fixtures() []Project {
	now := time.Now()
	return []Project{
		{
			Slug: "gh/form/form-translation",
			Name: "form-translation",
			Pipelines: []Pipeline{
				{
					ID: "p1", Number: 1234, Branch: "main", Ticket: "CWEB-291",
					Status: StatusFailed, Actor: "agustinfranchetti",
					CreatedAt: now.Add(-2 * time.Minute),
					Jobs: []Job{
						{ID: "j1", Name: "build", Status: StatusSuccess, WorkflowID: "wf-1234", WorkflowName: "ci"},
						{ID: "j2", Name: "test", Status: StatusFailed, Hint: "3 specs failed", WorkflowID: "wf-1234", WorkflowName: "ci"},
						{ID: "j3", Name: "deploy", Status: StatusNotRun, WorkflowID: "wf-1234", WorkflowName: "ci"},
					},
				},
				{
					ID: "p2", Number: 1233, Branch: "main", Ticket: "CWEB-290",
					Status: StatusSuccess, Actor: "agustinfranchetti",
					CreatedAt: now.Add(-12 * time.Minute),
					Jobs: []Job{
						{ID: "j4", Name: "build", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci"},
						{ID: "j5", Name: "test", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci"},
						{ID: "j6", Name: "deploy", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci"},
					},
				},
				{
					ID: "p3", Number: 1232, Branch: "CWEB-292", Ticket: "CWEB-292",
					Status: StatusRunning, Actor: "agustinfranchetti",
					CreatedAt: now.Add(-3 * time.Minute),
					Jobs: []Job{
						{ID: "j7", Name: "build", Status: StatusRunning, WorkflowID: "wf-1232", WorkflowName: "ci"},
					},
				},
			},
		},
		{
			Slug: "gh/form/gospotcheck",
			Name: "gospotcheck",
			Pipelines: []Pipeline{
				{
					ID: "p4", Number: 5678, Branch: "master", Status: StatusSuccess,
					Actor: "tealc", CreatedAt: now.Add(-3 * time.Minute),
					Jobs: []Job{
						{ID: "j8", Name: "lint", Status: StatusSuccess, WorkflowID: "wf-5678", WorkflowName: "build"},
						{ID: "j9", Name: "test", Status: StatusSuccess, WorkflowID: "wf-5678", WorkflowName: "build"},
					},
				},
				{
					ID: "p5", Number: 5677, Branch: "master", Status: StatusSuccess,
					Actor: "tealc", CreatedAt: now.Add(-18 * time.Minute),
				},
			},
		},
		{
			Slug: "gh/form/form-engine",
			Name: "form-engine",
			Pipelines: []Pipeline{
				{
					ID: "p6", Number: 9012, Branch: "release/2.4",
					Status: StatusRunning, Actor: "ci-bot",
					CreatedAt: now.Add(-1 * time.Minute),
				},
				{
					ID: "p7", Number: 9011, Branch: "main",
					Status: StatusOnHold, Actor: "agustinfranchetti",
					CreatedAt: now.Add(-30 * time.Minute),
				},
			},
		},
	}
}
