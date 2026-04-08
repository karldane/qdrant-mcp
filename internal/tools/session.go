package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/karldane/qdrant-mcp/internal/normalize"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// ListSessionsTool
// ---------------------------------------------------------------------------

type ListSessionsTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewListSessionsTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *ListSessionsTool {
	return &ListSessionsTool{client: c, cfg: cfg}
}

func (t *ListSessionsTool) Name() string { return "list_sessions" }

func (t *ListSessionsTool) Description() string {
	return "List active sessions stored in the collection."
}

func (t *ListSessionsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum sessions to return",
				"default":     20,
			},
		},
	}
}

func (t *ListSessionsTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	filter := map[string]interface{}{"type": "session"}

	results, _, err := t.client.Scroll(ctx, limit, filter, "")
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}

	sessions := make([]*normalize.Session, 0, len(results))
	for _, r := range results {
		s := &normalize.Session{}
		if name, ok := r.Payload["name"].(string); ok {
			s.Name = name
		}
		if state, ok := r.Payload["state"].(map[string]interface{}); ok {
			s.State = state
		}
		s.Active = true
		s.ID = r.ID
		sessions = append(sessions, s)
	}

	output := map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}

	b, _ := json.Marshal(output)
	return string(b), nil
}

func (t *ListSessionsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// LoadSessionTool
// ---------------------------------------------------------------------------

type LoadSessionTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewLoadSessionTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *LoadSessionTool {
	return &LoadSessionTool{client: c, cfg: cfg}
}

func (t *LoadSessionTool) Name() string { return "load_session" }

func (t *LoadSessionTool) Description() string {
	return "Load session state by ID."
}

func (t *LoadSessionTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID",
			},
		},
		Required: []string{"id"},
	}
}

func (t *LoadSessionTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)

	result, err := t.client.GetPoint(ctx, id)
	if err != nil {
		return "", fmt.Errorf("load session: %w", err)
	}

	s := &normalize.Session{
		ID: result.ID,
	}
	if name, ok := result.Payload["name"].(string); ok {
		s.Name = name
	}
	if state, ok := result.Payload["state"].(map[string]interface{}); ok {
		s.State = state
	}

	b, _ := json.Marshal(s)
	return string(b), nil
}

func (t *LoadSessionTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// SaveSessionTool
// ---------------------------------------------------------------------------

type SaveSessionTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewSaveSessionTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *SaveSessionTool {
	return &SaveSessionTool{client: c, cfg: cfg}
}

func (t *SaveSessionTool) Name() string { return "save_session" }

func (t *SaveSessionTool) Description() string {
	return "Persist current session state."
}

func (t *SaveSessionTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Session name",
			},
			"state": map[string]interface{}{
				"type":        "object",
				"description": "Session state to persist",
			},
		},
		Required: []string{"name"},
	}
}

func (t *SaveSessionTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	name, _ := args["name"].(string)
	state, _ := args["state"].(map[string]interface{})
	if state == nil {
		state = make(map[string]interface{})
	}

	state["type"] = "session"
	state["name"] = name
	state["created"] = time.Now().Format(time.RFC3339)

	id := fmt.Sprintf("session_%d", time.Now().UnixNano())

	if err := t.client.UpsertPoint(ctx, id, nil, state); err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}

	return fmt.Sprintf(`{"success": true, "id": "%s"}`, id), nil
}

func (t *SaveSessionTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// DeleteSessionTool
// ---------------------------------------------------------------------------

type DeleteSessionTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewDeleteSessionTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *DeleteSessionTool {
	return &DeleteSessionTool{client: c, cfg: cfg}
}

func (t *DeleteSessionTool) Name() string { return "delete_session" }

func (t *DeleteSessionTool) Description() string {
	return "Remove a session by ID."
}

func (t *DeleteSessionTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID to delete",
			},
		},
		Required: []string{"id"},
	}
}

func (t *DeleteSessionTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	if err := t.client.DeletePoints(ctx, []string{id}, nil); err != nil {
		return "", fmt.Errorf("delete session: %w", err)
	}

	return `{"success": true}`, nil
}

func (t *DeleteSessionTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactDelete),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}
