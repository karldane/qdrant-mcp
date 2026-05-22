package tools

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/karldane/qdrant-mcp/internal/normalize"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

type UpsertPointTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewUpsertPointTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *UpsertPointTool {
	return &UpsertPointTool{client: c, cfg: cfg}
}

func (t *UpsertPointTool) Name() string { return "upsert_point" }

func (t *UpsertPointTool) Description() string {
	return "Store a vector with payload data. Use for storing embeddings with metadata."
}

func (t *UpsertPointTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Unique identifier for the point",
			},
			"vector": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "number"},
				"description": "Vector embedding (e.g., from OpenAI text-embedding-3-small)",
			},
			"payload": map[string]interface{}{
				"type":        "object",
				"description": "Metadata to store with the vector",
			},
		},
		Required: []string{"id"},
	}
}

func (t *UpsertPointTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	id, _ := args["id"].(string)
	var vector []float64
	if v, ok := args["vector"].([]interface{}); ok {
		vector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				vector = append(vector, f)
			}
		}
	}
	payload, _ := args["payload"].(map[string]interface{})

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("upsert point: %w", err)
	}

	return framework.TextResult(fmt.Sprintf(`{"success": true, "id": "%s"}`, id)), nil
}

func (t *UpsertPointTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

func (t *UpsertPointTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

type SearchPointsTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewSearchPointsTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *SearchPointsTool {
	return &SearchPointsTool{client: c, cfg: cfg}
}

func (t *SearchPointsTool) Name() string { return "search_points" }

func (t *SearchPointsTool) Description() string {
	return "Search vectors using semantic similarity. Returns points sorted by relevance."
}

func (t *SearchPointsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query_vector": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "number"},
				"description": "Query vector for similarity search",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum results to return",
				"default":     5,
			},
			"filter": map[string]interface{}{
				"type":        "object",
				"description": "Filter by payload fields (e.g., {\"type\": \"memory\"})",
			},
		},
		Required: []string{"query_vector"},
	}
}

func (t *SearchPointsTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	var queryVector []float64
	if v, ok := args["query_vector"].([]interface{}); ok {
		queryVector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				queryVector = append(queryVector, f)
			}
		}
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	filter, _ := args["filter"].(map[string]interface{})

	results, err := t.client.Search(ctx, queryVector, limit, filter)
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("search points: %w", err)
	}

	points := make([]*normalize.Point, 0, len(results))
	for _, r := range results {
		points = append(points, &normalize.Point{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		})
	}

	output := map[string]interface{}{
		"results": points,
		"count":   len(points),
	}

	b, _ := json.Marshal(output)
	return framework.TextResult(string(b)), nil
}

func (t *SearchPointsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithResourceCost(2),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

func (t *SearchPointsTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

type ScrollPointsTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewScrollPointsTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *ScrollPointsTool {
	return &ScrollPointsTool{client: c, cfg: cfg}
}

func (t *ScrollPointsTool) Name() string { return "scroll_points" }

func (t *ScrollPointsTool) Description() string {
	return "List all points with optional filtering and pagination."
}

func (t *ScrollPointsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Number of points to return",
				"default":     20,
			},
			"filter": map[string]interface{}{
				"type":        "object",
				"description": "Filter by payload fields",
			},
			"offset": map[string]interface{}{
				"type":        "string",
				"description": "Pagination offset from previous response",
			},
		},
	}
}

func (t *ScrollPointsTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	filter, _ := args["filter"].(map[string]interface{})
	offset, _ := args["offset"].(string)

	results, nextOffset, err := t.client.Scroll(ctx, limit, filter, offset)
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("scroll points: %w", err)
	}

	points := make([]*normalize.Point, 0, len(results))
	for _, r := range results {
		points = append(points, &normalize.Point{
			ID:      r.ID,
			Payload: r.Payload,
		})
	}

	output := map[string]interface{}{
		"points":      points,
		"count":       len(points),
		"next_offset": nextOffset,
	}

	b, _ := json.Marshal(output)
	return framework.TextResult(string(b)), nil
}

func (t *ScrollPointsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

func (t *ScrollPointsTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

type GetPointTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewGetPointTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *GetPointTool {
	return &GetPointTool{client: c, cfg: cfg}
}

func (t *GetPointTool) Name() string { return "get_point" }

func (t *GetPointTool) Description() string {
	return "Retrieve a single point by its ID."
}

func (t *GetPointTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Point ID to retrieve",
			},
		},
		Required: []string{"id"},
	}
}

func (t *GetPointTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	id, _ := args["id"].(string)

	result, err := t.client.GetPoint(ctx, id)
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("get point: %w", err)
	}

	point := &normalize.Point{
		ID:      result.ID,
		Vector:  result.Vector,
		Payload: result.Payload,
	}

	b, _ := json.Marshal(point)
	return framework.TextResult(string(b)), nil
}

func (t *GetPointTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

func (t *GetPointTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

type DeletePointsTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewDeletePointsTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *DeletePointsTool {
	return &DeletePointsTool{client: c, cfg: cfg}
}

func (t *DeletePointsTool) Name() string { return "delete_points" }

func (t *DeletePointsTool) Description() string {
	return "Delete points by ID or by filter criteria."
}

func (t *DeletePointsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"ids": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Point IDs to delete",
			},
			"filter": map[string]interface{}{
				"type":        "object",
				"description": "Delete all points matching this filter",
			},
		},
	}
}

func (t *DeletePointsTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	var ids []string
	if v, ok := args["ids"].([]interface{}); ok {
		for _, id := range v {
			if id, ok := id.(string); ok {
				ids = append(ids, id)
			}
		}
	}

	filter, _ := args["filter"].(map[string]interface{})

	if len(ids) == 0 && filter == nil {
		return framework.TextResult(""), errors.New("must provide either ids or filter")
	}

	if err := t.client.DeletePoints(ctx, ids, filter); err != nil {
		return framework.TextResult(""), fmt.Errorf("delete points: %w", err)
	}

	return framework.TextResult(`{"success": true}`), nil
}

func (t *DeletePointsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskHigh),
		framework.WithImpact(framework.ImpactDelete),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

func (t *DeletePointsTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}
