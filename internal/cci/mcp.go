package cci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The CircleCI MCP server's list_followed_projects tool returns LLM-friendly
// prose, not JSON: `<n>. <name> (projectSlug: <slug>)` per line, prefaced by
// "Projects followed:" and tailed with usage instructions for the model. We
// parse it by matching this line shape exactly.
var followedProjectLine = regexp.MustCompile(`^\d+\.\s+(.+?)\s+\(projectSlug:\s+(.+?)\)\s*$`)

type mcpClient struct {
	cmd     *exec.Cmd
	session *mcp.ClientSession
}

func newMCP(ctx context.Context, token string) (*mcpClient, error) {
	cmd := exec.CommandContext(ctx, "npx", "-y", "@circleci/mcp-server-circleci")
	cmd.Env = append(os.Environ(), "CIRCLECI_TOKEN="+token)
	cmd.Stderr = os.Stderr // surface any server-side errors to the user

	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "circleci-tui",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to circleci mcp server: %w", err)
	}
	return &mcpClient{cmd: cmd, session: session}, nil
}

func (m *mcpClient) Close() error {
	if m == nil || m.session == nil {
		return nil
	}
	return m.session.Close()
}

// callJSON invokes a tool and decodes the first text-content result as JSON
// into out. If the result is structured (the SDK exposes a StructuredContent
// field), we prefer that.
//
// args is the inner parameter map — we wrap it in `{params: {...}}` to match
// the CircleCI MCP server's input schema (every tool nests args under a single
// `params` object; sending args flat returns a "params is required" error).
func (m *mcpClient) callJSON(ctx context.Context, tool string, args map[string]any, out any) error {
	res, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: map[string]any{"params": args},
	})
	if err != nil {
		return fmt.Errorf("call %s: %w", tool, err)
	}
	if res.IsError {
		return fmt.Errorf("call %s: server returned error: %s", tool, summarizeContent(res.Content))
	}
	if out == nil {
		return nil
	}
	if res.StructuredContent != nil {
		// The SDK already unmarshals structured content into a map[string]any
		// or marshals it as raw JSON depending on the server. Re-encode then
		// decode so it lands in the caller's typed struct.
		buf, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return fmt.Errorf("marshal structured content: %w", err)
		}
		return json.Unmarshal(buf, out)
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return json.Unmarshal([]byte(tc.Text), out)
		}
	}
	return fmt.Errorf("call %s: no decodable content in response", tool)
}

// callText invokes a tool and returns the concatenated text content. Useful
// for tools whose payload is human-readable (e.g. log dumps). args is wrapped
// in `{params: {...}}` for the same reason as callJSON.
func (m *mcpClient) callText(ctx context.Context, tool string, args map[string]any) (string, error) {
	res, err := m.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: map[string]any{"params": args},
	})
	if err != nil {
		return "", fmt.Errorf("call %s: %w", tool, err)
	}
	if res.IsError {
		return "", fmt.Errorf("call %s: server returned error: %s", tool, summarizeContent(res.Content))
	}
	var s string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			s += tc.Text
		}
	}
	return s, nil
}

func summarizeContent(cs []mcp.Content) string {
	for _, c := range cs {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "(no message)"
}

// ListFollowedProjects calls the MCP `list_followed_projects` tool and parses
// its prose response (see followedProjectLine for the format).
func (m *mcpClient) ListFollowedProjects(ctx context.Context) ([]apiFollowedProject, error) {
	text, err := m.callText(ctx, "list_followed_projects", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseFollowedProjects(text), nil
}

func parseFollowedProjects(text string) []apiFollowedProject {
	var projects []apiFollowedProject
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		match := followedProjectLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		projects = append(projects, apiFollowedProject{
			Name: strings.TrimSpace(match[1]),
			Slug: strings.TrimSpace(match[2]),
		})
	}
	return projects
}

func (m *mcpClient) GetBuildFailureLogs(ctx context.Context, slug, branch string) (string, error) {
	return m.callText(ctx, "get_build_failure_logs", map[string]any{
		"projectSlug": slug,
		"branch":      branch,
	})
}

func (m *mcpClient) RerunWorkflow(ctx context.Context, workflowID string, fromFailed bool) error {
	args := map[string]any{"workflowId": workflowID}
	if fromFailed {
		args["fromFailed"] = true
	}
	_, err := m.callText(ctx, "rerun_workflow", args)
	return err
}

// Rollback intentionally not exposed yet: the MCP server's run_rollback_pipeline
// requires environmentName / componentName / currentVersion / targetVersion /
// namespace as required fields. That's a structured rollback workflow, not a
// one-keystroke action — wiring it up needs a multi-step prompt UI we haven't
// built. Phase 6 will revisit.
