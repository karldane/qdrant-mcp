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

type UpsertMemoryTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewUpsertMemoryTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *UpsertMemoryTool {
	return &UpsertMemoryTool{client: c, cfg: cfg}
}

func (t *UpsertMemoryTool) Name() string { return "upsert_memory" }

func (t *UpsertMemoryTool) Description() string {
	return "Store a fact, observation, or session data with optional TTL and tags."
}

func (t *UpsertMemoryTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Text content to store",
			},
			"embedding": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "number"},
				"description": "Vector embedding (optional, for semantic search)",
			},
			"metadata": map[string]interface{}{
				"type":        "object",
				"description": "Additional metadata",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Tags for categorization",
			},
			"ttl_seconds": map[string]interface{}{
				"type":        "number",
				"description": "Time-to-live in seconds (optional)",
			},
		},
		Required: []string{"content"},
	}
}

func (t *UpsertMemoryTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	content, _ := args["content"].(string)
	var vector []float64
	if v, ok := args["embedding"].([]interface{}); ok {
		vector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				vector = append(vector, f)
			}
		}
	}

	metadata, _ := args["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	if tags, ok := args["tags"].([]interface{}); ok {
		tagStrs := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag, ok := tag.(string); ok {
				tagStrs = append(tagStrs, tag)
			}
		}
		metadata["tags"] = tagStrs
	}

	metadata["content"] = content
	metadata["type"] = "memory"
	metadata["created_at"] = time.Now().Format(time.RFC3339)

	if ttl, ok := args["ttl_seconds"].(float64); ok && ttl > 0 {
		expires := time.Now().Add(time.Duration(ttl) * time.Second)
		metadata["expires_at"] = expires.Format(time.RFC3339)
	}

	id := fmt.Sprintf("mem_%d", time.Now().UnixNano())

	if err := t.client.UpsertPoint(ctx, id, vector, metadata); err != nil {
		return "", fmt.Errorf("upsert memory: %w", err)
	}

	return fmt.Sprintf(`{"success": true, "id": "%s"}`, id), nil
}

func (t *UpsertMemoryTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

type SearchMemoryTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewSearchMemoryTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *SearchMemoryTool {
	return &SearchMemoryTool{client: c, cfg: cfg}
}

func (t *SearchMemoryTool) Name() string { return "search_memory" }

func (t *SearchMemoryTool) Description() string {
	return "Search recent or related facts using semantic similarity."
}

func (t *SearchMemoryTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query text",
			},
			"query_embedding": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "number"},
				"description": "Pre-computed query vector",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum results",
				"default":     5,
			},
			"filter": map[string]interface{}{
				"type":        "object",
				"description": "Filter by metadata fields",
			},
		},
		Required: []string{"query"},
	}
}

func (t *SearchMemoryTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	var vector []float64
	if v, ok := args["query_embedding"].([]interface{}); ok {
		vector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				vector = append(vector, f)
			}
		}
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	filter, _ := args["filter"].(map[string]interface{})
	if filter == nil {
		filter = make(map[string]interface{})
	}
	filter["type"] = "memory"

	results, err := t.client.Search(ctx, vector, limit, filter)
	if err != nil {
		return "", fmt.Errorf("search memory: %w", err)
	}

	memories := make([]*normalize.Memory, 0, len(results))
	for _, r := range results {
		m := &normalize.Memory{
			Metadata: r.Payload,
		}
		if content, ok := r.Payload["content"].(string); ok {
			m.Content = content
		}
		if tags, ok := r.Payload["tags"].([]interface{}); ok {
			tagStrs := make([]string, 0, len(tags))
			for _, tag := range tags {
				if tag, ok := tag.(string); ok {
					tagStrs = append(tagStrs, tag)
				}
			}
			m.Tags = tagStrs
		}
		m.ID = r.ID
		m.Score = r.Score
		memories = append(memories, m)
	}

	output := map[string]interface{}{
		"memories": memories,
		"count":    len(memories),
	}

	b, _ := json.Marshal(output)
	return string(b), nil
}

func (t *SearchMemoryTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithResourceCost(2),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

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

func (t *ListSessionsTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

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

func (t *LoadSessionTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

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
	state["created_at"] = time.Now().Format(time.RFC3339)

	id := fmt.Sprintf("session_%d", time.Now().UnixNano())

	if err := t.client.UpsertPoint(ctx, id, nil, state); err != nil {
		return "", fmt.Errorf("save session: %w", err)
	}

	return fmt.Sprintf(`{"success": true, "id": "%s"}`, id), nil
}

func (t *SaveSessionTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

type InvalidateCacheTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewInvalidateCacheTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *InvalidateCacheTool {
	return &InvalidateCacheTool{client: c, cfg: cfg}
}

func (t *InvalidateCacheTool) Name() string { return "invalidate_cache" }

func (t *InvalidateCacheTool) Description() string {
	return "Clear stale cache entries by key prefix or all."
}

func (t *InvalidateCacheTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"prefix": map[string]interface{}{
				"type":        "string",
				"description": "Key prefix to invalidate (optional, clears all if empty)",
			},
		},
	}
}

func (t *InvalidateCacheTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	prefix, _ := args["prefix"].(string)

	var filter map[string]interface{}
	if prefix != "" {
		filter = map[string]interface{}{"key": prefix}
	} else {
		filter = map[string]interface{}{"type": "cache"}
	}

	if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
		return "", fmt.Errorf("invalidate cache: %w", err)
	}

	return `{"success": true}`, nil
}

func (t *InvalidateCacheTool) GetEnforcerProfile() framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}
