package tools

import (
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
// LearnProcedureTool — procedural memory write
// ---------------------------------------------------------------------------

type LearnProcedureTool struct {
	framework.BaseTool
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewLearnProcedureTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *LearnProcedureTool {
	return &LearnProcedureTool{client: c, cfg: cfg, embedder: ep}
}

func (t *LearnProcedureTool) Name() string { return "learn_procedure" }

func (t *LearnProcedureTool) Description() string {
	return "Store a named procedure or workflow. If a procedure with the same name already exists it is updated (revision incremented)."
}

func (t *LearnProcedureTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Short identifier for the procedure",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "What this procedure achieves",
			},
			"steps": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Ordered steps",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "When to use this procedure (optional)",
			},
			"ttl_days": map[string]interface{}{
				"type":        "integer",
				"description": "Optional expiry in days",
			},
		},
		Required: []string{"name", "description", "steps"},
	}
}

func (t *LearnProcedureTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	if name == "" {
		return framework.TextResult(""), errors.New("name is required")
	}
	if description == "" {
		return framework.TextResult(""), errors.New("description is required")
	}

	rawSteps, _ := args["steps"].([]interface{})
	if len(rawSteps) == 0 {
		return framework.TextResult(""), errors.New("steps is required")
	}
	steps := ifacesToStrings(rawSteps)

	// Check if a procedure with this name already exists.
	existing, _, err := t.client.Scroll(ctx, 1, map[string]interface{}{
		"memory_type": "procedural",
		"name":        name,
	}, "")
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("learn_procedure: scroll check: %v", err)
	}

	id := uuid.New().String()
	revision := 1
	action := "created"
	if len(existing) > 0 {
		id = existing[0].ID
		revision = payloadInt(existing[0].Payload, "revision", 0) + 1
		action = "updated"
	}

	// Build embed text from description + steps.
	embedText := description + " " + strings.Join(steps, " ")
	var vector []float64
	if t.embedder != nil {
		vector, err = t.embedder.Embed(ctx, embedText)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed procedure: %v", err)
		}
	}

	payload := map[string]interface{}{
		"memory_type": "procedural",
		"name":        name,
		"description": description,
		"steps":       stringsToIfaces(steps),
		"revision":    revision,
		"content":     embedText,
		"created":     timestampf(),
		"updated":     timestampf(),
	}
	if ctx2, ok := args["context"].(string); ok && ctx2 != "" {
		payload["context"] = ctx2
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		payload["tags"] = tagsToIfaces(tags)
	}
	if ttlDays, ok := args["ttl_days"].(float64); ok && ttlDays > 0 {
		payload["ttl"] = ttlFromDays(ttlDays)
	}

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("learn_procedure: upsert: %v", err)
	}

	out := map[string]interface{}{"id": id, "name": name, "action": action}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *LearnProcedureTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(false),
	)
}

func (t *LearnProcedureTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

// ---------------------------------------------------------------------------
// RecallProcedureTool — procedural memory read
// ---------------------------------------------------------------------------

type RecallProcedureTool struct {
	framework.BaseTool
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewRecallProcedureTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *RecallProcedureTool {
	return &RecallProcedureTool{client: c, cfg: cfg, embedder: ep}
}

func (t *RecallProcedureTool) Name() string { return "recall_procedure" }

func (t *RecallProcedureTool) Description() string {
	return "Retrieve a procedure by name or by describing what you want to do. Supports exact name lookup and semantic search."
}

func (t *RecallProcedureTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Exact name lookup (optional)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Semantic search for a matching procedure (optional)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"limit": map[string]interface{}{
				"type":    "integer",
				"default": 3,
			},
		},
	}
}

func (t *RecallProcedureTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	name, _ := args["name"].(string)
	query, _ := args["query"].(string)
	limit := 3
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if name == "" && query == "" {
		return framework.TextResult(""), errors.New("recall_procedure: supply either name or query")
	}

	type procOut struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Steps       []string `json:"steps"`
		Context     string   `json:"context,omitempty"`
		Score       float32  `json:"score,omitempty"`
	}

	var procedures []procOut

	if name != "" {
		// Exact name scroll.
		results, _, err := t.client.Scroll(ctx, 1, map[string]interface{}{
			"memory_type": "procedural",
			"name":        name,
		}, "")
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("recall_procedure: %v", err)
		}
		for _, r := range results {
			procedures = append(procedures, procOut{
				ID:          r.ID,
				Name:        payloadString(r.Payload, "name"),
				Description: payloadString(r.Payload, "description"),
				Steps:       ifacesToStrings(r.Payload["steps"]),
				Context:     payloadString(r.Payload, "context"),
			})
		}
	} else {
		// Semantic search.
		vector, err := t.embedder.Embed(ctx, query)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed query: %v", err)
		}
		filter := map[string]interface{}{"memory_type": "procedural"}
		if tags, ok := args["tags"].([]interface{}); ok && len(tags) > 0 {
			filter["tags"] = tags[0]
		}
		results, err := t.client.Search(ctx, vector, limit, filter)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("recall_procedure search: %v", err)
		}
		for _, r := range results {
			procedures = append(procedures, procOut{
				ID:          r.ID,
				Name:        payloadString(r.Payload, "name"),
				Description: payloadString(r.Payload, "description"),
				Steps:       ifacesToStrings(r.Payload["steps"]),
				Context:     payloadString(r.Payload, "context"),
				Score:       r.Score,
			})
		}
	}

	out := map[string]interface{}{"procedures": procedures}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *RecallProcedureTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

func (t *RecallProcedureTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}

// ---------------------------------------------------------------------------
// UpdateProcedureTool — procedural memory update
// ---------------------------------------------------------------------------

type UpdateProcedureTool struct {
	framework.BaseTool
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewUpdateProcedureTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *UpdateProcedureTool {
	return &UpdateProcedureTool{client: c, cfg: cfg, embedder: ep}
}

func (t *UpdateProcedureTool) Name() string { return "update_procedure" }

func (t *UpdateProcedureTool) Description() string {
	return "Revise an existing procedure. Increments revision and archives the prior version in metadata for audit purposes."
}

func (t *UpdateProcedureTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "UUID of the procedure to update",
			},
			"steps": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "New step list (optional)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Updated description (optional)",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Why this procedure was revised (optional)",
			},
		},
		Required: []string{"id"},
	}
}

func (t *UpdateProcedureTool) Handle(ctx framework.CallContext, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	id, _ := args["id"].(string)
	if id == "" {
		return framework.TextResult(""), errors.New("id is required")
	}

	// Fetch existing.
	existing, err := t.client.GetPoint(ctx, id)
	if err != nil || existing == nil {
		return framework.TextResult(""), fmt.Errorf("not found: no procedure with id %v", id)
	}

	currentRevision := payloadInt(existing.Payload, "revision", 1)
	currentSteps := ifacesToStrings(existing.Payload["steps"])
	currentDescription := payloadString(existing.Payload, "description")
	existingName := payloadString(existing.Payload, "name")

	// Apply updates.
	newDescription := currentDescription
	if d, ok := args["description"].(string); ok && d != "" {
		newDescription = d
	}

	newSteps := currentSteps
	if rawSteps, ok := args["steps"].([]interface{}); ok && len(rawSteps) > 0 {
		newSteps = ifacesToStrings(rawSteps)
	}

	reason, _ := args["reason"].(string)

	// Archive prior revision.
	priorRevision := map[string]interface{}{
		"revision":    currentRevision,
		"steps":       stringsToIfaces(currentSteps),
		"description": currentDescription,
		"updated":     payloadString(existing.Payload, "updated"),
	}
	if reason != "" {
		priorRevision["reason"] = reason
	}

	// Load prior_revisions array.
	var priorRevisions []interface{}
	if pr, ok := existing.Payload["prior_revisions"].([]interface{}); ok {
		priorRevisions = pr
	}
	priorRevisions = append(priorRevisions, priorRevision)

	// Re-embed.
	embedText := newDescription + " " + strings.Join(newSteps, " ")
	var vector []float64
	if t.embedder != nil {
		vector, err = t.embedder.Embed(ctx, embedText)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed procedure: %v", err)
		}
	}

	payload := map[string]interface{}{
		"memory_type":     "procedural",
		"name":            existingName,
		"description":     newDescription,
		"steps":           stringsToIfaces(newSteps),
		"revision":        currentRevision + 1,
		"prior_revisions": priorRevisions,
		"content":         embedText,
		"created":         payloadString(existing.Payload, "created"),
		"updated":         timestampf(),
	}
	if reason != "" {
		payload["revision_reason"] = reason
	}
	// Preserve tags and context.
	if existing.Payload["tags"] != nil {
		payload["tags"] = existing.Payload["tags"]
	}
	if ctx2 := payloadString(existing.Payload, "context"); ctx2 != "" {
		payload["context"] = ctx2
	}

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("update_procedure: %v", err)
	}

	out := map[string]interface{}{"id": id, "revision": currentRevision + 1}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *UpdateProcedureTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(false),
	)
}

func (t *UpdateProcedureTool) EnforcerProfile(args map[string]interface{}) *framework.EnforcerProfile {
	return t.GetEnforcerProfile()
}
