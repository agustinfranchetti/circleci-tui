package model

import "time"

// Fixtures returns a hardcoded snapshot used during phase 1 development before
// the MCP client is wired. It is intentionally varied so the rendering paths
// (failed/running/success/onhold/cancelled, expanded/collapsed) all get hit.
//
// Job timings + Dependencies populate the same fields the live API loader
// fills in, so demo mode exercises duration display and DAG-depth indenting.
func Fixtures() []Project {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }

	return []Project{
		{
			Slug: "gh/form/form-translation",
			Name: "form-translation",
			Pipelines: []Pipeline{
				{
					ID: "p1", Number: 1234, Branch: "main", Ticket: "CWEB-291",
					Status: StatusFailed, Actor: "agustinfranchetti",
					CreatedAt: ago(5 * time.Minute),
					StartedAt: ago(5 * time.Minute),
					StoppedAt: ago(2 * time.Minute),
					Jobs: []Job{
						{ID: "j1", Name: "build", Status: StatusSuccess, WorkflowID: "wf-1234", WorkflowName: "ci",
							StartedAt: ago(5 * time.Minute), StoppedAt: ago(4 * time.Minute), Depth: 0},
						{ID: "j2", Name: "test", Status: StatusFailed, Hint: "3 specs failed",
							WorkflowID: "wf-1234", WorkflowName: "ci",
							StartedAt: ago(4 * time.Minute), StoppedAt: ago(2 * time.Minute),
							Dependencies: []string{"j1"}, Depth: 1},
						{ID: "j3", Name: "deploy", Status: StatusNotRun, WorkflowID: "wf-1234", WorkflowName: "ci",
							Dependencies: []string{"j2"}, Depth: 2},
					},
				},
				{
					ID: "p2", Number: 1233, Branch: "main", Ticket: "CWEB-290",
					Status: StatusSuccess, Actor: "agustinfranchetti",
					CreatedAt: ago(12 * time.Minute),
					StartedAt: ago(12 * time.Minute),
					StoppedAt: ago(8 * time.Minute),
					Jobs: []Job{
						{ID: "j4", Name: "build", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci",
							StartedAt: ago(12 * time.Minute), StoppedAt: ago(11 * time.Minute), Depth: 0},
						{ID: "j5", Name: "test", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci",
							StartedAt: ago(11 * time.Minute), StoppedAt: ago(9 * time.Minute),
							Dependencies: []string{"j4"}, Depth: 1},
						{ID: "j6", Name: "deploy", Status: StatusSuccess, WorkflowID: "wf-1233", WorkflowName: "ci",
							StartedAt: ago(9 * time.Minute), StoppedAt: ago(8 * time.Minute),
							Dependencies: []string{"j5"}, Depth: 2},
					},
				},
				{
					ID: "p3", Number: 1232, Branch: "CWEB-292", Ticket: "CWEB-292",
					Status: StatusRunning, Actor: "agustinfranchetti",
					CreatedAt: ago(3 * time.Minute),
					StartedAt: ago(3 * time.Minute),
					Jobs: []Job{
						{ID: "j7", Name: "build", Status: StatusRunning, WorkflowID: "wf-1232", WorkflowName: "ci",
							StartedAt: ago(3 * time.Minute), Depth: 0},
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
					Actor:     "tealc",
					CreatedAt: ago(3 * time.Minute),
					StartedAt: ago(3 * time.Minute),
					StoppedAt: ago(45 * time.Second),
					Jobs: []Job{
						{ID: "j8", Name: "lint", Status: StatusSuccess, WorkflowID: "wf-5678", WorkflowName: "build",
							StartedAt: ago(3 * time.Minute), StoppedAt: ago(2 * time.Minute), Depth: 0},
						{ID: "j9", Name: "test", Status: StatusSuccess, WorkflowID: "wf-5678", WorkflowName: "build",
							StartedAt: ago(2 * time.Minute), StoppedAt: ago(45 * time.Second),
							Dependencies: []string{"j8"}, Depth: 1},
					},
				},
				{
					ID: "p5", Number: 5677, Branch: "master", Status: StatusSuccess,
					Actor: "tealc", CreatedAt: ago(18 * time.Minute),
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
					CreatedAt: ago(1 * time.Minute),
					StartedAt: ago(1 * time.Minute),
				},
				{
					ID: "p7", Number: 9011, Branch: "main",
					Status: StatusOnHold, Actor: "agustinfranchetti",
					CreatedAt: ago(30 * time.Minute),
					StartedAt: ago(30 * time.Minute),
				},
			},
		},
	}
}
