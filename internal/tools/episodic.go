package tools

import (
	"context"
	"encoding/json"
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
// LogEventTool — episodic memory write (immutable event record)
// ---------------------------------------------------------------------------

type LogEventTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewLogEventTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *LogEventTool {
	return &LogEventTool{client: c, cfg: cfg, embedder: ep}
}

func (t *LogEventTool) Name() string { return "log_event" }

func (t *LogEventTool) Description() string {
	return "Record that something happened. Events are immutable once written — use for decisions, actions, observations, errors, and outcomes."
}

func (t *LogEventTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"event": map[string]interface{}{
				"type":        "string",
				"description": "What happened",
			},
			"event_type": map[string]interface{}{
				"type":        "string",
				"description": "decision | action | observation | error | outcome",
				"enum":        []string{"decision", "action", "observation", "error", "outcome"},
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Surrounding context (optional)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"metadata": map[string]interface{}{
				"type":        "object",
				"description": "Arbitrary structured data (optional)",
			},
		},
		Required: []string{"event"},
	}
}

func (t *LogEventTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	event, _ := args["event"].(string)
	if event == "" {
		return "", fmt.Errorf("event is required")
	}
	eventType, _ := args["event_type"].(string)
	eventContext, _ := args["context"].(string)

	// Embed event + context together.
	embedText := event
	if eventContext != "" {
		embedText = event + " " + eventContext
	}

	var vector []float64
	if t.embedder != nil {
		var err error
		vector, err = t.embedder.Embed(ctx, embedText)
		if err != nil {
			return "", fmt.Errorf("embed event: %w", err)
		}
	}

	id := uuid.New().String()
	now := timestampf()

	// Events are write-once: only "created", never "updated".
	payload := map[string]interface{}{
		"memory_type": "episodic",
		"content":     event,
		"created":     now,
	}
	if eventType != "" {
		payload["event_type"] = eventType
	}
	if eventContext != "" {
		payload["context"] = eventContext
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		payload["tags"] = tagsToIfaces(tags)
	}
	if metadata, ok := args["metadata"].(map[string]interface{}); ok {
		payload["metadata"] = metadata
	}

	if err := t.client.UpsertPoint(ctx, id, vector, payload); err != nil {
		return "", fmt.Errorf("log_event: %w", err)
	}

	out := map[string]interface{}{"id": id, "timestamp": now}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *LogEventTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// RecallEventsTool — episodic memory read
// ---------------------------------------------------------------------------

type RecallEventsTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewRecallEventsTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *RecallEventsTool {
	return &RecallEventsTool{client: c, cfg: cfg, embedder: ep}
}

func (t *RecallEventsTool) Name() string { return "recall_events" }

func (t *RecallEventsTool) Description() string {
	return "Retrieve past events by time range, topic, or type. Supports semantic search, time filtering, and event type filtering."
}

func (t *RecallEventsTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Semantic search over event text (optional)",
			},
			"event_type": map[string]interface{}{
				"type":        "string",
				"description": "Filter by event type (optional)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"since": map[string]interface{}{
				"type":        "string",
				"description": "ISO8601 or relative: '7d', '2h', '1w'",
			},
			"until": map[string]interface{}{
				"type":        "string",
				"description": "ISO8601 or relative (optional, default: now)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default: 10, max: 50)",
				"default":     10,
			},
			"order": map[string]interface{}{
				"type":        "string",
				"description": "asc | desc (default: desc — most recent first)",
				"enum":        []string{"asc", "desc"},
				"default":     "desc",
			},
		},
	}
}

func (t *RecallEventsTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 50 {
			limit = 50
		}
	}
	order, _ := args["order"].(string)
	if order == "" {
		order = "desc"
	}

	query, _ := args["query"].(string)
	eventType, _ := args["event_type"].(string)

	// Parse time bounds.
	var sinceTime, untilTime time.Time
	if s, ok := args["since"].(string); ok && s != "" {
		var err error
		sinceTime, err = parseRelativeTime(s)
		if err != nil {
			return "", fmt.Errorf("invalid time 'since': %v", err)
		}
	}
	if u, ok := args["until"].(string); ok && u != "" {
		var err error
		untilTime, err = parseRelativeTime(u)
		if err != nil {
			return "", fmt.Errorf("invalid time 'until': %v", err)
		}
	}

	// Build filter map to pass to Search/Scroll.
	// Our client interface uses a flat filter map; compose what we need.
	filter := map[string]interface{}{"memory_type": "episodic"}
	if eventType != "" {
		filter["event_type"] = eventType
	}
	if tags, ok := args["tags"].([]interface{}); ok && len(tags) > 0 {
		filter["tags"] = tags[0] // primary tag filter (post-process for multi-tag)
	}

	type eventOut struct {
		ID        string   `json:"id"`
		Event     string   `json:"event"`
		EventType string   `json:"event_type,omitempty"`
		Tags      []string `json:"tags,omitempty"`
		Timestamp string   `json:"timestamp"`
		Age       string   `json:"age"`
	}

	var events []eventOut

	if query != "" && t.embedder != nil {
		// Semantic path.
		vector, err := t.embedder.Embed(ctx, query)
		if err != nil {
			return "", fmt.Errorf("embed query: %w", err)
		}
		results, err := t.client.Search(ctx, vector, limit*2, filter)
		if err != nil {
			return "", fmt.Errorf("recall_events search: %w", err)
		}
		for _, r := range results {
			created := payloadString(r.Payload, "created")
			if !inTimeRange(created, sinceTime, untilTime) {
				continue
			}
			events = append(events, eventOut{
				ID:        r.ID,
				Event:     payloadString(r.Payload, "content"),
				EventType: payloadString(r.Payload, "event_type"),
				Tags:      ifacesToStrings(r.Payload["tags"]),
				Timestamp: created,
				Age:       humanAge(created),
			})
			if len(events) >= limit {
				break
			}
		}
	} else {
		// Scroll path (no query).
		results, _, err := t.client.Scroll(ctx, limit*2, filter, "")
		if err != nil {
			return "", fmt.Errorf("recall_events scroll: %w", err)
		}
		for _, r := range results {
			created := payloadString(r.Payload, "created")
			if !inTimeRange(created, sinceTime, untilTime) {
				continue
			}
			events = append(events, eventOut{
				ID:        r.ID,
				Event:     payloadString(r.Payload, "content"),
				EventType: payloadString(r.Payload, "event_type"),
				Tags:      ifacesToStrings(r.Payload["tags"]),
				Timestamp: created,
				Age:       humanAge(created),
			})
			if len(events) >= limit {
				break
			}
		}
		// Sort by timestamp.
		sort.Slice(events, func(i, j int) bool {
			if order == "asc" {
				return events[i].Timestamp < events[j].Timestamp
			}
			return events[i].Timestamp > events[j].Timestamp
		})
	}

	out := map[string]interface{}{"events": events, "count": len(events)}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *RecallEventsTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(true),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// SummarisePeriodTool — episodic prose narrative (no LLM)
// ---------------------------------------------------------------------------

type SummarisePeriodTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewSummarisePeriodTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *SummarisePeriodTool {
	return &SummarisePeriodTool{client: c, cfg: cfg, embedder: ep}
}

func (t *SummarisePeriodTool) Name() string { return "summarise_period" }

func (t *SummarisePeriodTool) Description() string {
	return "Produce a chronological prose narrative of events over a time period. Useful for handoff notes, daily summaries, or session recaps. No LLM call — template-based."
}

func (t *SummarisePeriodTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"since": map[string]interface{}{
				"type":        "string",
				"description": "ISO8601 or relative string (e.g. '24h', '7d')",
			},
			"until": map[string]interface{}{
				"type":        "string",
				"description": "ISO8601 or relative (optional, default: now)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"focus": map[string]interface{}{
				"type":        "string",
				"description": "Topic to emphasise in the summary (optional)",
			},
		},
		Required: []string{"since"},
	}
}

func (t *SummarisePeriodTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	sinceStr, _ := args["since"].(string)
	if sinceStr == "" {
		return "", fmt.Errorf("since is required")
	}
	sinceTime, err := parseRelativeTime(sinceStr)
	if err != nil {
		return "", fmt.Errorf("invalid time 'since': %v", err)
	}

	var untilTime time.Time
	if u, ok := args["until"].(string); ok && u != "" {
		untilTime, err = parseRelativeTime(u)
		if err != nil {
			return "", fmt.Errorf("invalid time 'until': %v", err)
		}
	}

	filter := map[string]interface{}{"memory_type": "episodic"}
	if tags, ok := args["tags"].([]interface{}); ok && len(tags) > 0 {
		filter["tags"] = tags[0]
	}

	results, _, err := t.client.Scroll(ctx, 50, filter, "")
	if err != nil {
		return "", fmt.Errorf("summarise_period scroll: %w", err)
	}

	type eventEntry struct {
		timestamp string
		eventType string
		content   string
	}
	var entries []eventEntry

	for _, r := range results {
		created := payloadString(r.Payload, "created")
		if !inTimeRange(created, sinceTime, untilTime) {
			continue
		}
		entries = append(entries, eventEntry{
			timestamp: created,
			eventType: payloadString(r.Payload, "event_type"),
			content:   payloadString(r.Payload, "content"),
		})
	}

	// Sort ascending for narrative flow.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp < entries[j].timestamp
	})

	// Build period label.
	periodLabel := fmt.Sprintf("From %s", sinceTime.UTC().Format("2006-01-02 15:04 UTC"))
	if !untilTime.IsZero() {
		periodLabel += fmt.Sprintf(" to %s", untilTime.UTC().Format("2006-01-02 15:04 UTC"))
	} else {
		periodLabel += " to now"
	}

	// Template-based prose narrative.
	var lines []string
	for _, e := range entries {
		ts := e.timestamp
		if t2, err2 := time.Parse(time.RFC3339, ts); err2 == nil {
			ts = t2.UTC().Format("15:04")
		}
		line := fmt.Sprintf("  %s", ts)
		if e.eventType != "" {
			line += fmt.Sprintf(" [%s]", e.eventType)
		}
		line += fmt.Sprintf(": %s", e.content)
		lines = append(lines, line)
	}

	focus, _ := args["focus"].(string)
	header := fmt.Sprintf("During %s, the following occurred:", periodLabel)
	if focus != "" {
		header = fmt.Sprintf("During %s, focusing on \"%s\":", periodLabel, focus)
	}

	var summary string
	if len(lines) == 0 {
		summary = fmt.Sprintf("%s\n\n  (no events recorded)", header)
	} else {
		summary = header + "\n\n" + strings.Join(lines, "\n")
	}

	out := map[string]interface{}{
		"summary":     summary,
		"event_count": len(entries),
		"period":      periodLabel,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func (t *SummarisePeriodTool) GetEnforcerProfile() *framework.EnforcerProfile {
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

// inTimeRange returns true if the RFC3339 ts string falls within [since, until].
// Zero-value since/until means unbounded on that end.
func inTimeRange(ts string, since, until time.Time) bool {
	if ts == "" {
		return true // no timestamp — include
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true
	}
	if !since.IsZero() && t.Before(since) {
		return false
	}
	if !until.IsZero() && t.After(until) {
		return false
	}
	return true
}
