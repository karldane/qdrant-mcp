package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// WhatDoIKnowTool — introspection / orientation
// ---------------------------------------------------------------------------

type WhatDoIKnowTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewWhatDoIKnowTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *WhatDoIKnowTool {
	return &WhatDoIKnowTool{client: c, cfg: cfg, embedder: ep}
}

func (t *WhatDoIKnowTool) Name() string { return "what_do_i_know" }

func (t *WhatDoIKnowTool) Description() string {
	return "High-level summary of what is stored across all memory types. Useful at session start to orient the agent without loading everything."
}

func (t *WhatDoIKnowTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"topic": map[string]interface{}{
				"type":        "string",
				"description": "Constrain summary to a subject area (optional)",
			},
		},
	}
}

func (t *WhatDoIKnowTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	memTypes := []string{"semantic", "episodic", "procedural", "task", "cache"}

	counts := make(map[string]int64)
	for _, mt := range memTypes {
		n, err := t.client.Count(ctx, map[string]interface{}{"memory_type": mt})
		if err != nil {
			n = -1 // indicate unavailable
		}
		counts[mt] = n
	}

	// Collect names of procedures.
	var procNames []string
	procResults, _, _ := t.client.Scroll(ctx, 20, map[string]interface{}{"memory_type": "procedural"}, "")
	for _, r := range procResults {
		if n := payloadString(r.Payload, "name"); n != "" {
			procNames = append(procNames, n)
		}
	}

	// Collect active task titles (non-complete, non-abandoned).
	var taskTitles []string
	taskResults, _, _ := t.client.Scroll(ctx, 20, map[string]interface{}{"memory_type": "task"}, "")
	for _, r := range taskResults {
		status := payloadString(r.Payload, "status")
		if status == "complete" || status == "abandoned" {
			continue
		}
		if title := payloadString(r.Payload, "title"); title != "" {
			taskTitles = append(taskTitles, title)
		}
	}

	// Collect unique semantic tags.
	var semanticTags []string
	seenTags := map[string]bool{}
	semResults, _, _ := t.client.Scroll(ctx, 50, map[string]interface{}{"memory_type": "semantic"}, "")
	oldest, newest := "", ""
	for _, r := range semResults {
		for _, tag := range ifacesToStrings(r.Payload["tags"]) {
			if !seenTags[tag] {
				seenTags[tag] = true
				semanticTags = append(semanticTags, tag)
			}
		}
		if ts := payloadString(r.Payload, "created"); ts != "" {
			if oldest == "" || ts < oldest {
				oldest = ts
			}
			if newest == "" || ts > newest {
				newest = ts
			}
		}
	}

	// Collect event type distribution.
	eventTypes := map[string]int{}
	lastEvent := ""
	epResults, _, _ := t.client.Scroll(ctx, 100, map[string]interface{}{"memory_type": "episodic"}, "")
	for _, r := range epResults {
		et := payloadString(r.Payload, "event_type")
		if et != "" {
			eventTypes[et]++
		}
		if ts := payloadString(r.Payload, "created"); ts != "" {
			if lastEvent == "" || ts > lastEvent {
				lastEvent = ts
			}
		}
	}

	// Oldest cache expiry.
	oldestExpiry := ""
	cacheResults, _, _ := t.client.Scroll(ctx, 50, map[string]interface{}{"memory_type": "cache"}, "")
	for _, r := range cacheResults {
		if exp := payloadString(r.Payload, "ttl"); exp != "" {
			if oldestExpiry == "" || exp < oldestExpiry {
				oldestExpiry = exp
			}
		}
	}

	activeTasks := int64(0)
	for _, r := range taskResults {
		status := payloadString(r.Payload, "status")
		if status != "complete" && status != "abandoned" {
			activeTasks++
		}
	}

	out := map[string]interface{}{
		"semantic_memory": map[string]interface{}{
			"count":  counts["semantic"],
			"tags":   semanticTags,
			"oldest": humanAge(oldest),
			"newest": humanAge(newest),
		},
		"episodic_memory": map[string]interface{}{
			"count":       counts["episodic"],
			"last_event":  humanAge(lastEvent),
			"event_types": eventTypes,
		},
		"procedures": map[string]interface{}{
			"count": counts["procedural"],
			"names": procNames,
		},
		"active_tasks": map[string]interface{}{
			"count":  activeTasks,
			"titles": taskTitles,
		},
		"cache": map[string]interface{}{
			"count":         counts["cache"],
			"oldest_expiry": oldestExpiry,
		},
	}

	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *WhatDoIKnowTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// MemoryStatsTool — raw collection diagnostics
// ---------------------------------------------------------------------------

type MemoryStatsTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewMemoryStatsTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *MemoryStatsTool {
	return &MemoryStatsTool{client: c, cfg: cfg}
}

func (t *MemoryStatsTool) Name() string { return "memory_stats" }

func (t *MemoryStatsTool) Description() string {
	return "Raw collection statistics for diagnostics — total points, per-type counts, vector count, and collection health."
}

func (t *MemoryStatsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type:       "object",
		Properties: map[string]interface{}{},
	}
}

func (t *MemoryStatsTool) Handle(ctx context.Context, _ map[string]interface{}) (framework.ToolResult, error) {
	memTypes := []string{"semantic", "episodic", "procedural", "task", "cache"}

	byType := make(map[string]int64)
	total := int64(0)
	for _, mt := range memTypes {
		n, err := t.client.Count(ctx, map[string]interface{}{"memory_type": mt})
		if err != nil {
			n = -1
		}
		byType[mt] = n
		if n > 0 {
			total += n
		}
	}

	// Get collection info for vector count and status.
	info, err := t.client.CollectionInfo(ctx)
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("memory_stats: collection_info: %w", err)
	}

	vectorCount := int64(0)
	if vc, ok := info["indexed_vectors_count"].(uint64); ok {
		vectorCount = int64(vc)
	} else if vc, ok := info["points_count"].(uint64); ok {
		vectorCount = int64(vc)
	}

	indexStatus := "ready"
	if s, ok := info["status"].(string); ok && s != "" {
		indexStatus = s
	}

	out := map[string]interface{}{
		"total_points": total,
		"by_type":      byType,
		"vector_count": vectorCount,
		"index_status": indexStatus,
	}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *MemoryStatsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}
