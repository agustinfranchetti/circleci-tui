package cci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHost = "https://circleci.com"

// restClient hits CircleCI's REST APIs directly. Three flavours live behind
// the same client:
//
//   - /api/v2/...      — the public, documented API (pipelines, workflows,
//                        jobs, cancel, approve, rerun).
//   - /api/v1.1/...    — older but still functional; only place that
//                        exposes per-job step metadata.
//   - /api/private/... — undocumented endpoints the web UI uses for
//                        followed-projects listings and raw step output.
//                        Same Circle-Token works for all three.
type restClient struct {
	http  *http.Client
	host  string
	token string
}

func newREST(token, host string) *restClient {
	if host == "" {
		host = defaultHost
	}
	return &restClient{
		http:  &http.Client{Timeout: 30 * time.Second},
		host:  strings.TrimRight(host, "/"),
		token: token,
	}
}

// do issues a JSON request and decodes the response into out. Path is the
// absolute URL path including the API-version prefix (e.g. "/api/v2/...").
func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	u := c.host + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Circle-Token", c.token)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
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

// doText fetches a path and returns the response body as text. Used for the
// /api/private/output/raw/... endpoints which serve raw step stdout/stderr.
func (c *restClient) doText(ctx context.Context, method, path string) (string, error) {
	u := c.host + path
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Circle-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("circleci %s %s: %s — %s", method, path, resp.Status, strings.TrimSpace(string(body)))
	}
	return string(body), nil
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
		if err := c.do(ctx, http.MethodGet, "/api/v2/project/"+slug+"/pipeline", q, nil, &page); err != nil {
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
	if err := c.do(ctx, http.MethodGet, "/api/v2/pipeline/"+pipelineID+"/workflow", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *restClient) listJobs(ctx context.Context, workflowID string) ([]apiJob, error) {
	var out jobList
	if err := c.do(ctx, http.MethodGet, "/api/v2/workflow/"+workflowID+"/job", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *restClient) cancelWorkflow(ctx context.Context, workflowID string) error {
	return c.do(ctx, http.MethodPost, "/api/v2/workflow/"+workflowID+"/cancel", nil, nil, nil)
}

func (c *restClient) approveJob(ctx context.Context, workflowID, approvalRequestID string) error {
	return c.do(ctx, http.MethodPost, "/api/v2/workflow/"+workflowID+"/approve/"+approvalRequestID, nil, nil, nil)
}

// rerunWorkflow triggers a CircleCI workflow rerun. fromFailed=true reruns
// only the failed jobs (and their downstream); false reruns the whole thing
// from scratch.
func (c *restClient) rerunWorkflow(ctx context.Context, workflowID string, fromFailed bool) error {
	body := map[string]any{}
	if fromFailed {
		body["from_failed"] = true
	}
	return c.do(ctx, http.MethodPost, "/api/v2/workflow/"+workflowID+"/rerun", nil, body, nil)
}

// getJobDetailsV1 fetches per-step metadata for a single job. v1.1 is the
// only CircleCI API that exposes step-level structure; v2 stops at the job
// header.
func (c *restClient) getJobDetailsV1(ctx context.Context, slug string, jobNumber int) (*apiV1JobDetails, error) {
	var out apiV1JobDetails
	path := fmt.Sprintf("/api/v1.1/project/%s/%d", slug, jobNumber)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// getStepOutput fetches the raw stdout (`output`) or stderr (`error`) for a
// single step's parallel run. taskIndex is which parallel run we want;
// stepID is the action.step value from the v1 job details.
func (c *restClient) getStepOutput(ctx context.Context, slug string, jobNumber, taskIndex, stepID int, stream string) (string, error) {
	if stream != "output" && stream != "error" {
		return "", fmt.Errorf("getStepOutput: stream must be \"output\" or \"error\", got %q", stream)
	}
	path := fmt.Sprintf("/api/private/output/raw/%s/%d/%s/%d/%d", slug, jobNumber, stream, taskIndex, stepID)
	return c.doText(ctx, http.MethodGet, path)
}

// listFollowedProjects walks /api/private/me/followed-projects with token
// pagination. Caps at 20 pages so a misbehaving response can't loop forever.
func (c *restClient) listFollowedProjects(ctx context.Context) ([]apiFollowedProject, error) {
	var all []apiFollowedProject
	q := url.Values{}
	prevToken := ""
	for page := 0; page < 20; page++ {
		var resp followedProjectsList
		if err := c.do(ctx, http.MethodGet, "/api/private/me/followed-projects", q, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if resp.NextPageToken == "" || resp.NextPageToken == prevToken {
			break
		}
		prevToken = resp.NextPageToken
		q.Set("page-token", resp.NextPageToken)
	}
	return all, nil
}
