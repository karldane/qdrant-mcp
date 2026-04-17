package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// RememberTool — semantic memory write
// ---------------------------------------------------------------------------

type RememberTool struct {
	client         QdrantClient
	cfg            readonly.ReadOnlyChecker
	embedder       embed.Provider
	dedupThreshold float64
}

func NewRememberTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider, dedupThreshold float64) *RememberTool {
	if dedupThreshold <= 0 {
		dedupThreshold = 0.95
	}
	return &RememberTool{client: c, cfg: cfg, embedder: ep, dedupThreshold: dedupThreshold}
}

func (t *RememberTool) Name() string { return "remember" }

func (t *RememberTool) Description() string {
	return "Store a fact, preference, or piece of knowledge for later recall. Automatically deduplicates against existing memories."
}

func (t *RememberTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The fact or preference to remember",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Topic labels for filtering",
			},
			"confidence": map[string]interface{}{
				"type":        "number",
				"description": "Certainty 0.0–1.0 (default: 1.0)",
				"default":     1.0,
			},
			"ttl_days": map[string]interface{}{
				"type":        "integer",
				"description": "Auto-expire after N days (default: no expiry)",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Where this fact came from",
			},
		},
		Required: []string{"content"},
	}
}

func (t *RememberTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	content, _ := args["content"].(string)
	if content == "" {
		return framework.TextResult(""), errors.New("content is required")
	}

	// Embed the content.
	var vector []float64
	var embedErr error
	if t.embedder != nil {
		vector, embedErr = t.embedder.Embed(ctx, content)
		if embedErr != nil {
			return framework.TextResult(""), fmt.Errorf("embed content: %w", embedErr)
		}
	}

	// Deduplication: search for near-identical semantic memories.
	action := "created"
	id := uuid.New().String()

	if len(vector) > 0 {
		dupes, err := t.client.Search(ctx, vector, 1, map[string]interface{}{"memory_type": "semantic"})
		if err == nil && len(dupes) > 0 && float64(dupes[0].Score) >= t.dedupThreshold {
			// Update existing instead of creating new.
			id = dupes[0].ID
			action = "updated"
			updatePayload := map[string]interface{}{
				"content": content,
				"updated": timestampf(),
			}
			if conf, ok := args["confidence"].(float64); ok {
				updatePayload["confidence"] = conf
			}
			if err := t.client.SetPayload(ctx, id, updatePayload); err != nil {
				return framework.TextResult(""), fmt.Errorf("update memory: %w", err)
			}
			out := map[string]interface{}{"id": id, "action": action, "content": content}
			b, _ := json.Marshal(out)
			return framework.TextResult(string(b)), nil
		}
	}

	// Build new point payload.
	payload := map[string]interface{}{
		"memory_type": "semantic",
		"content":     content,
		"created":     timestampf(),
		"updated":     timestampf(),
		"confidence":  1.0,
	}

	if conf, ok := args["confidence"].(float64); ok {
		payload["confidence"] = conf
	}
	if src, ok := args["source"].(string); ok && src != "" {
		payload["source"] = src
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		payload["tags"] = tagsToIfaces(tags)
	}
	if ttlDays, ok := args["ttl_days"].(float64); ok && ttlDays > 0 {
		payload["ttl"] = ttlFromDays(ttlDays)
	}

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("store memory: %w", err)
	}

	out := map[string]interface{}{"id": id, "action": action, "content": content}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *RememberTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// RecallTool — semantic memory read
// ---------------------------------------------------------------------------

type RecallTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewRecallTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *RecallTool {
	return &RecallTool{client: c, cfg: cfg, embedder: ep}
}

func (t *RecallTool) Name() string { return "recall" }

func (t *RecallTool) Description() string {
	return "Retrieve facts semantically relevant to a query. Filters expired memories, ranks by relevance."
}

func (t *RecallTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "What to search for",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional tag filter",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default: 5, max: 20)",
				"default":     5,
			},
			"min_score": map[string]interface{}{
				"type":        "number",
				"description": "Minimum similarity threshold (default: 0.0)",
				"default":     0.0,
			},
			"recency_bias": map[string]interface{}{
				"type":        "boolean",
				"description": "Weight recent memories higher (default: false)",
				"default":     false,
			},
		},
		Required: []string{"query"},
	}
}

func (t *RecallTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return framework.TextResult(""), errors.New("query is required")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 20 {
			limit = 20
		}
	}
	minScore := 0.0
	if ms, ok := args["min_score"].(float64); ok {
		minScore = ms
	}
	recencyBias, _ := args["recency_bias"].(bool)

	// Embed query.
	var vector []float64
	if t.embedder != nil {
		var err error
		vector, err = t.embedder.Embed(ctx, query)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed query: %w", err)
		}
	}

	filter := map[string]interface{}{"memory_type": "semantic"}
	// Tag filter: if exactly one tag provided via flat string, use it.
	// (Full multi-tag filtering needs richer Qdrant filter support, handled via scroll.)

	results, err := t.client.Search(ctx, vector, limit*2, filter) // over-fetch to allow TTL filtering
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("recall: %w", err)
	}

	// Tag filter post-processing.
	var tagFilter []string
	if tags, ok := args["tags"].([]interface{}); ok {
		tagFilter = ifacesToStrings(tags)
	}

	type memoryOut struct {
		ID         string   `json:"id"`
		Content    string   `json:"content"`
		Tags       []string `json:"tags,omitempty"`
		Confidence float64  `json:"confidence"`
		Score      float32  `json:"score"`
		AgeDays    float64  `json:"age_days"`
	}

	now := time.Now().UTC()
	memories := make([]memoryOut, 0, len(results))
	for _, r := range results {
		if float64(r.Score) < minScore {
			continue
		}
		// TTL check.
		if ttl := payloadString(r.Payload, "ttl"); ttl != "" {
			if t2, err := time.Parse(time.RFC3339, ttl); err == nil && now.After(t2) {
				continue
			}
		}
		// Tag filter.
		if len(tagFilter) > 0 {
			tags := ifacesToStrings(r.Payload["tags"])
			if !hasAnyTag(tags, tagFilter) {
				continue
			}
		}

		created := payloadString(r.Payload, "created")
		m := memoryOut{
			ID:         r.ID,
			Content:    payloadString(r.Payload, "content"),
			Tags:       ifacesToStrings(r.Payload["tags"]),
			Confidence: payloadFloat(r.Payload, "confidence", 1.0),
			Score:      r.Score,
			AgeDays:    ageDays(created),
		}
		memories = append(memories, m)
		if len(memories) >= limit {
			break
		}
	}

	// Recency bias: re-sort by score * recency_factor.
	if recencyBias && len(memories) > 1 {
		sort.Slice(memories, func(i, j int) bool {
			wi := float64(memories[i].Score) / (1 + memories[i].AgeDays)
			wj := float64(memories[j].Score) / (1 + memories[j].AgeDays)
			return wi > wj
		})
	}

	out := map[string]interface{}{"memories": memories, "count": len(memories)}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *RecallTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithResourceCost(2),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// ForgetTool — semantic memory delete
// ---------------------------------------------------------------------------

type ForgetTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewForgetTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *ForgetTool {
	return &ForgetTool{client: c, cfg: cfg, embedder: ep}
}

func (t *ForgetTool) Name() string { return "forget" }

func (t *ForgetTool) Description() string {
	return "Delete one or more memories by ID, by tag, or by semantic match."
}

func (t *ForgetTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Delete a specific memory by UUID",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Semantic search to find memories to forget",
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Delete all memories with these tags",
			},
			"confirm": map[string]interface{}{
				"type":        "boolean",
				"description": "Skip confirmation gate — delete immediately (default: false)",
				"default":     false,
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max memories to delete when using query (default: 5)",
				"default":     5,
			},
		},
	}
}

func (t *ForgetTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	id, _ := args["id"].(string)
	query, _ := args["query"].(string)
	tags, _ := args["tags"].([]interface{})
	confirm, _ := args["confirm"].(bool)
	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if id == "" && query == "" && len(tags) == 0 {
		return framework.TextResult(""), errors.New("must provide id, query, or tags")
	}

	var deletedIDs []string

	// Direct ID delete.
	if id != "" {
		if err := t.client.DeletePoints(ctx, []string{id}, nil); err != nil {
			return framework.TextResult(""), fmt.Errorf("forget: %w", err)
		}
		deletedIDs = []string{id}
	}

	// Tag-based delete.
	if len(tags) > 0 {
		filter := map[string]interface{}{"memory_type": "semantic", "tags": tags[0]}
		if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
			return framework.TextResult(""), fmt.Errorf("forget by tag: %w", err)
		}
	}

	// Semantic query delete.
	if query != "" && t.embedder != nil {
		vector, err := t.embedder.Embed(ctx, query)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed query: %w", err)
		}
		results, err := t.client.Search(ctx, vector, limit, map[string]interface{}{"memory_type": "semantic"})
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("search for forget: %w", err)
		}
		if !confirm {
			// Safety gate: return matches without deleting.
			previews := make([]map[string]interface{}, 0, len(results))
			for _, r := range results {
				previews = append(previews, map[string]interface{}{
					"id":      r.ID,
					"content": payloadString(r.Payload, "content"),
					"score":   r.Score,
				})
			}
			b, _ := json.Marshal(map[string]interface{}{
				"pending_delete": previews,
				"message":        "Call forget again with confirm=true to delete these memories",
			})
			return framework.TextResult(string(b)), nil
		}
		ids := make([]string, 0, len(results))
		for _, r := range results {
			ids = append(ids, r.ID)
		}
		if len(ids) > 0 {
			if err := t.client.DeletePoints(ctx, ids, nil); err != nil {
				return framework.TextResult(""), fmt.Errorf("forget: %w", err)
			}
			deletedIDs = append(deletedIDs, ids...)
		}
	}

	out := map[string]interface{}{"deleted": len(deletedIDs), "ids": deletedIDs}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *ForgetTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskHigh),
		framework.WithImpact(framework.ImpactDelete),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// ReflectTool — semantic memory synthesis
// ---------------------------------------------------------------------------

type ReflectTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewReflectTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *ReflectTool {
	return &ReflectTool{client: c, cfg: cfg, embedder: ep}
}

func (t *ReflectTool) Name() string { return "reflect" }

func (t *ReflectTool) Description() string {
	return "Synthesise a prose summary of what is known about a topic from semantic memory. Read-only."
}

func (t *ReflectTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Subject to reflect on",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max memories to draw from (default: 10)",
				"default":     10,
			},
			"tags": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Constrain to tagged memories",
			},
		},
		Required: []string{"topic"},
	}
}

func (t *ReflectTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	topic, _ := args["topic"].(string)
	if topic == "" {
		return framework.TextResult(""), errors.New("topic is required")
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var vector []float64
	if t.embedder != nil {
		var err error
		vector, err = t.embedder.Embed(ctx, topic)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed topic: %w", err)
		}
	}

	results, err := t.client.Search(ctx, vector, limit, map[string]interface{}{"memory_type": "semantic"})
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("reflect search: %w", err)
	}

	// Tag filter.
	var tagFilter []string
	if tags, ok := args["tags"].([]interface{}); ok {
		tagFilter = ifacesToStrings(tags)
	}

	now := time.Now().UTC()
	var sourceIDs []string
	var facts []string
	for i, r := range results {
		if ttl := payloadString(r.Payload, "ttl"); ttl != "" {
			if t2, err := time.Parse(time.RFC3339, ttl); err == nil && now.After(t2) {
				continue
			}
		}
		if len(tagFilter) > 0 {
			tags := ifacesToStrings(r.Payload["tags"])
			if !hasAnyTag(tags, tagFilter) {
				continue
			}
		}
		content := payloadString(r.Payload, "content")
		if content == "" {
			continue
		}
		conf := payloadFloat(r.Payload, "confidence", 1.0)
		age := humanAge(payloadString(r.Payload, "created"))
		facts = append(facts, fmt.Sprintf("%d. %s [confidence: %.2f, %s]", i+1, content, conf, age))
		sourceIDs = append(sourceIDs, r.ID)
	}

	summary := fmt.Sprintf("Reflecting on \"%s\" — %d relevant facts known:\n\n%s",
		topic, len(facts), strings.Join(facts, "\n"))

	out := map[string]interface{}{
		"summary": summary,
		"sources": sourceIDs,
		"count":   len(sourceIDs),
	}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *ReflectTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hasAnyTag returns true if any element of want appears in have.
func hasAnyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if h == w {
				return true
			}
		}
	}
	return false
}
