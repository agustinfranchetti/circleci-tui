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

type apiFollowedProject struct {
	Slug string `json:"project_slug"`
	Name string `json:"name"`
}
