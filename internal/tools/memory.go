package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/normalize"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// UpsertMemoryTool
// ---------------------------------------------------------------------------

type UpsertMemoryTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewUpsertMemoryTool(c QdrantClient, cfg readonly.ReadOnlyChecker, opts ...embed.Provider) *UpsertMemoryTool {
	var ep embed.Provider
	if len(opts) > 0 && opts[0] != nil {
		ep = opts[0]
	}
	return &UpsertMemoryTool{client: c, cfg: cfg, embedder: ep}
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
				"description": "Vector embedding (optional — auto-generated from content when omitted and provider is configured)",
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

	// Build vector: use supplied embedding if present, otherwise auto-embed.
	var vector []float64
	if v, ok := args["embedding"].([]interface{}); ok && len(v) > 0 {
		vector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				vector = append(vector, f)
			}
		}
	} else if t.embedder != nil && content != "" {
		var err error
		vector, err = t.embedder.Embed(ctx, content)
		if err != nil {
			return "", fmt.Errorf("auto-embed content: %w", err)
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
	metadata["created"] = time.Now().Format(time.RFC3339)

	if ttl, ok := args["ttl_seconds"].(float64); ok && ttl > 0 {
		expires := time.Now().Add(time.Duration(ttl) * time.Second)
		metadata["ttl"] = expires.Format(time.RFC3339)
	}

	id := fmt.Sprintf("mem_%d", time.Now().UnixNano())

	if err := t.client.UpsertPoint(ctx, id, vector, metadata); err != nil {
		return "", fmt.Errorf("upsert memory: %w", err)
	}

	return fmt.Sprintf(`{"success": true, "id": "%s"}`, id), nil
}

func (t *UpsertMemoryTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// SearchMemoryTool
// ---------------------------------------------------------------------------

type SearchMemoryTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewSearchMemoryTool(c QdrantClient, cfg readonly.ReadOnlyChecker, opts ...embed.Provider) *SearchMemoryTool {
	var ep embed.Provider
	if len(opts) > 0 && opts[0] != nil {
		ep = opts[0]
	}
	return &SearchMemoryTool{client: c, cfg: cfg, embedder: ep}
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
				"description": "Pre-computed query vector (optional — auto-generated when omitted and provider is configured)",
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
	query, _ := args["query"].(string)

	// Build query vector: use supplied embedding if present, otherwise auto-embed.
	var vector []float64
	if v, ok := args["query_embedding"].([]interface{}); ok && len(v) > 0 {
		vector = make([]float64, 0, len(v))
		for _, f := range v {
			if f, ok := f.(float64); ok {
				vector = append(vector, f)
			}
		}
	} else if t.embedder != nil && query != "" {
		var err error
		vector, err = t.embedder.Embed(ctx, query)
		if err != nil {
			return "", fmt.Errorf("auto-embed query: %w", err)
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

func (t *SearchMemoryTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithResourceCost(2),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// DeleteMemoryTool
// ---------------------------------------------------------------------------

type DeleteMemoryTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewDeleteMemoryTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *DeleteMemoryTool {
	return &DeleteMemoryTool{client: c, cfg: cfg}
}

func (t *DeleteMemoryTool) Name() string { return "delete_memory" }

func (t *DeleteMemoryTool) Description() string {
	return "Delete memories by ID list or by tag filter."
}

func (t *DeleteMemoryTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"ids": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Memory point IDs to delete",
			},
			"tag": map[string]interface{}{
				"type":        "string",
				"description": "Delete all memories with this tag",
			},
		},
	}
}

func (t *DeleteMemoryTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	var ids []string
	if v, ok := args["ids"].([]interface{}); ok {
		for _, id := range v {
			if s, ok := id.(string); ok {
				ids = append(ids, s)
			}
		}
	}

	tag, _ := args["tag"].(string)

	if len(ids) == 0 && tag == "" {
		return "", fmt.Errorf("must provide either ids or tag")
	}

	if len(ids) > 0 {
		if err := t.client.DeletePoints(ctx, ids, nil); err != nil {
			return "", fmt.Errorf("delete memory: %w", err)
		}
	} else {
		filter := map[string]interface{}{"type": "memory", "tags": tag}
		if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
			return "", fmt.Errorf("delete memory: %w", err)
		}
	}

	return `{"success": true}`, nil
}

func (t *DeleteMemoryTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskHigh),
		framework.WithImpact(framework.ImpactDelete),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}
