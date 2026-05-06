package cci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://circleci.com/api/v2"

type restClient struct {
	http    *http.Client
	baseURL string
	token   string
}

func newREST(token, baseURL string) *restClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &restClient{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}

func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body io.Reader, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Circle-Token", c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("circleci %s %s: %s — %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *restClient) listRecentPipelines(ctx context.Context, slug string, branch string, limit int) ([]apiPipeline, error) {
	q := url.Values{}
	if branch != "" {
		q.Set("branch", branch)
	}
	var all []apiPipeline
	pageToken := ""
	for {
		if pageToken != "" {
			q.Set("page-token", pageToken)
		}
		var page pipelineList
		if err := c.do(ctx, http.MethodGet, "/project/"+slug+"/pipeline", q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(all) >= limit || page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (c *restClient) listWorkflows(ctx context.Context, pipelineID string) ([]apiWorkflow, error) {
	var out workflowList
	if err := c.do(ctx, http.MethodGet, "/pipeline/"+pipelineID+"/workflow", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *restClient) listJobs(ctx context.Context, workflowID string) ([]apiJob, error) {
	var out jobList
	if err := c.do(ctx, http.MethodGet, "/workflow/"+workflowID+"/job", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *restClient) cancelWorkflow(ctx context.Context, workflowID string) error {
	return c.do(ctx, http.MethodPost, "/workflow/"+workflowID+"/cancel", nil, nil, nil)
}

func (c *restClient) approveJob(ctx context.Context, workflowID, approvalRequestID string) error {
	return c.do(ctx, http.MethodPost, "/workflow/"+workflowID+"/approve/"+approvalRequestID, nil, nil, nil)
}
