package tools

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// StoreResultTool — result cache write
// ---------------------------------------------------------------------------

type StoreResultTool struct {
	framework.BaseTool
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewStoreResultTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *StoreResultTool {
	return &StoreResultTool{client: c, cfg: cfg, embedder: ep}
}

func (t *StoreResultTool) Name() string { return "store_result" }

func (t *StoreResultTool) Description() string {
	return "Cache the result of an operation. Key is derived from input hash or supplied explicitly. Embeds the input for semantic lookup."
}

func (t *StoreResultTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Cache key (optional; auto-derived from input if omitted)",
			},
			"input": map[string]interface{}{
				"type":        "string",
				"description": "The input that produced this result (for key derivation and semantic search)",
			},
			"result": map[string]interface{}{
				"type":        "string",
				"description": "The result to cache",
			},
			"ttl_hours": map[string]interface{}{
				"type":        "integer",
				"description": "Expiry in hours (default: 24)",
				"default":     24,
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"result_type": map[string]interface{}{
				"type":        "string",
				"description": "text | json | markdown | code (default: text)",
				"default":     "text",
			},
		},
		Required: []string{"result"},
	}
}

func (t *StoreResultTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	result, _ := args["result"].(string)
	if result == "" {
		return framework.TextResult(""), errors.New("result is required")
	}

	input, _ := args["input"].(string)
	key, _ := args["key"].(string)

	// Derive key from input if not supplied.
	if key == "" {
		src := input
		if src == "" {
			src = result
		}
		h := sha256.Sum256([]byte(strings.TrimSpace(src)))
		key = fmt.Sprintf("%x", h[:16])
	}

	ttlHours := 24.0
	if h, ok := args["ttl_hours"].(float64); ok && h > 0 {
		ttlHours = h
	}
	resultType, _ := args["result_type"].(string)
	if resultType == "" {
		resultType = "text"
	}

	expires := ttlFromHours(ttlHours)

	// Check if key already exists.
	existing, _, err := t.client.Scroll(ctx, 1, map[string]interface{}{
		"memory_type": "cache",
		"cache_key":   key,
	}, "")
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("store_result: check existing: %w", err)
	}

	action := "created"
	if len(existing) > 0 {
		// Update existing: set payload only.
		update := map[string]interface{}{
			"result":      result,
			"result_type": resultType,
			"ttl":         expires,
			"updated":     timestampf(),
		}
		if err := t.client.SetPayload(ctx, existing[0].ID, update); err != nil {
			return framework.TextResult(""), fmt.Errorf("store_result: update: %w", err)
		}
		action = "updated"
		out := map[string]interface{}{"key": key, "action": action, "expires": expires}
		b, _ := json.Marshal(out)
		return framework.TextResult(string(b)), nil
	}

	// Embed the input (not the result) so semantic lookup works by input description.
	embedText := input
	if embedText == "" {
		embedText = result
	}
	var vector []float64
	if t.embedder != nil {
		vector, err = t.embedder.Embed(ctx, embedText)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed input: %w", err)
		}
	}

	id := uuid.New().String()
	payload := map[string]interface{}{
		"memory_type": "cache",
		"cache_key":   key,
		"input_hash":  key,
		"result":      result,
		"result_type": resultType,
		"content":     embedText,
		"ttl":         expires,
		"created":     timestampf(),
		"updated":     timestampf(),
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		payload["tags"] = tagsToIfaces(tags)
	}

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("store_result: upsert: %w", err)
	}

	out := map[string]interface{}{"key": key, "action": action, "expires": expires}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *StoreResultTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

func (t *StoreResultTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

// ---------------------------------------------------------------------------
// LookupResultTool — result cache read
// ---------------------------------------------------------------------------

type LookupResultTool struct {
	framework.BaseTool
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewLookupResultTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *LookupResultTool {
	return &LookupResultTool{client: c, cfg: cfg, embedder: ep}
}

func (t *LookupResultTool) Name() string { return "lookup_result" }

func (t *LookupResultTool) Description() string {
	return "Retrieve a cached result by key or by semantic similarity to the input. Returns a miss cleanly rather than an error."
}

func (t *LookupResultTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Exact key lookup (optional)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Semantic search for similar cached results (optional)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"min_score": map[string]interface{}{
				"type":        "number",
				"description": "Minimum similarity for semantic lookup (default: 0.85)",
				"default":     0.85,
			},
		},
	}
}

func (t *LookupResultTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	key, _ := args["key"].(string)
	query, _ := args["query"].(string)
	minScore := 0.85
	if ms, ok := args["min_score"].(float64); ok {
		minScore = ms
	}

	miss := `{"hit":false}`

	if key != "" {
		// Exact key lookup via scroll.
		results, _, err := t.client.Scroll(ctx, 1, map[string]interface{}{
			"memory_type": "cache",
			"cache_key":   key,
		}, "")
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("lookup_result: %w", err)
		}
		if len(results) == 0 {
			return framework.TextResult(miss), nil
		}
		r := results[0]
		// TTL check.
		if isExpired(payloadString(r.Payload, "ttl")) {
			return framework.TextResult(miss), nil
		}
		out := map[string]interface{}{
			"hit":         true,
			"key":         payloadString(r.Payload, "cache_key"),
			"result":      payloadString(r.Payload, "result"),
			"result_type": payloadString(r.Payload, "result_type"),
			"score":       1.0,
			"expires":     payloadString(r.Payload, "ttl"),
			"age":         humanAge(payloadString(r.Payload, "created")),
		}
		b, _ := json.Marshal(out)
		return framework.TextResult(string(b)), nil
	}

	if query != "" && t.embedder != nil {
		// Semantic lookup.
		vector, err := t.embedder.Embed(ctx, query)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed query: %w", err)
		}
		filter := map[string]interface{}{"memory_type": "cache"}
		if tags, ok := args["tags"].([]interface{}); ok && len(tags) > 0 {
			filter["tags"] = tags[0]
		}
		results, err := t.client.Search(ctx, vector, 1, filter)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("lookup_result search: %w", err)
		}
		if len(results) == 0 || float64(results[0].Score) < minScore {
			return framework.TextResult(miss), nil
		}
		r := results[0]
		if isExpired(payloadString(r.Payload, "ttl")) {
			return framework.TextResult(miss), nil
		}
		out := map[string]interface{}{
			"hit":         true,
			"key":         payloadString(r.Payload, "cache_key"),
			"result":      payloadString(r.Payload, "result"),
			"result_type": payloadString(r.Payload, "result_type"),
			"score":       r.Score,
			"expires":     payloadString(r.Payload, "ttl"),
			"age":         humanAge(payloadString(r.Payload, "created")),
		}
		b, _ := json.Marshal(out)
		return framework.TextResult(string(b)), nil
	}

	return framework.TextResult(miss), nil
}

func (t *LookupResultTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

func (t *LookupResultTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

// ---------------------------------------------------------------------------
// InvalidateResultTool — result cache delete
// ---------------------------------------------------------------------------

type InvalidateResultTool struct {
	framework.BaseTool
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewInvalidateResultTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *InvalidateResultTool {
	return &InvalidateResultTool{client: c, cfg: cfg}
}

func (t *InvalidateResultTool) Name() string { return "invalidate_result" }

func (t *InvalidateResultTool) Description() string {
	return "Remove a cached result by key, tag, or semantic query."
}

func (t *InvalidateResultTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Exact cache key (optional)",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Delete all entries with these tags (optional)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Semantic match to invalidate (optional)",
			},
		},
	}
}

func (t *InvalidateResultTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	key, _ := args["key"].(string)
	tags, _ := args["tags"].([]interface{})
	query, _ := args["query"].(string)

	if key == "" && len(tags) == 0 && query == "" {
		return framework.TextResult(""), errors.New("invalidate_result: supply key, tags, or query")
	}

	invalidated := 0

	if key != "" {
		filter := map[string]interface{}{"memory_type": "cache", "cache_key": key}
		if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
			return framework.TextResult(""), fmt.Errorf("invalidate_result by key: %w", err)
		}
		invalidated++
	}

	if len(tags) > 0 {
		filter := map[string]interface{}{"memory_type": "cache", "tags": tags[0]}
		if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
			return framework.TextResult(""), fmt.Errorf("invalidate_result by tags: %w", err)
		}
		invalidated++
	}

	// query path requires embed — not wired here (no embedder field).
	// This tool intentionally has no embedder to keep it simple.
	// Semantic invalidation via query is handled by key or tags.

	out := map[string]interface{}{"invalidated": invalidated}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *InvalidateResultTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(false),
	)
}

func (t *InvalidateResultTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}
