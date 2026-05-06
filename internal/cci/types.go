package cci

import "time"

type apiPipeline struct {
	ID      string `json:"id"`
	Number  int    `json:"number"`
	State   string `json:"state"`
	VCS     struct {
		Branch string `json:"branch"`
	} `json:"vcs"`
	Trigger struct {
		Actor struct {
			Login string `json:"login"`
		} `json:"actor"`
	} `json:"trigger"`
	CreatedAt time.Time `json:"created_at"`
}

type pipelineList struct {
	Items         []apiPipeline `json:"items"`
	NextPageToken string        `json:"next_page_token"`
}

type apiWorkflow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	PipelineID     string `json:"pipeline_id"`
	PipelineNumber int    `json:"pipeline_number"`
}

type workflowList struct {
	Items         []apiWorkflow `json:"items"`
	NextPageToken string        `json:"next_page_token"`
}

type apiJob struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	JobNumber    int       `json:"job_number"`
	Type         string    `json:"type"`
	StartedAt    time.Time `json:"started_at"`
	StoppedAt    time.Time `json:"stopped_at"`
	Dependencies []string  `json:"dependencies"`
}

type jobList struct {
	Items         []apiJob `json:"items"`
	NextPageToken string   `json:"next_page_token"`
}

// apiFollowedProject mirrors the FollowedProjectSchema returned by
// /api/private/me/followed-projects: name, slug (e.g. "gh/org/repo"), vcs_type.
type apiFollowedProject struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	VCSType string `json:"vcs_type"`
}

type followedProjectsList struct {
	Items         []apiFollowedProject `json:"items"`
	NextPageToken string               `json:"next_page_token"`
}

// apiV1JobDetails is what /api/v1.1/project/{slug}/{jobNumber} returns. We
// only deserialise the fields the step view actually consumes; CircleCI's v1
// response is sprawling.
type apiV1JobDetails struct {
	BuildNum  int               `json:"build_num"`
	Status    string            `json:"status"`
	StartTime time.Time         `json:"start_time"`
	StopTime  time.Time         `json:"stop_time"`
	Steps     []apiV1Step       `json:"steps"`
}

type apiV1Step struct {
	Name    string        `json:"name"`
	Actions []apiV1Action `json:"actions"`
}

type apiV1Action struct {
	Index     int       `json:"index"`        // taskIndex — which parallel run this is
	Step      int       `json:"step"`         // stepId — what to pass to /output/raw
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	RunTimeMS int       `json:"run_time_millis"`
	Failed    *bool     `json:"failed"` // pointer because v1 sends null
	HasOutput bool      `json:"has_output"`
}
