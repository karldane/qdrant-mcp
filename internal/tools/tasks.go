package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// SaveProgressTool — working memory write
// ---------------------------------------------------------------------------

type SaveProgressTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewSaveProgressTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *SaveProgressTool {
	return &SaveProgressTool{client: c, cfg: cfg, embedder: ep}
}

func (t *SaveProgressTool) Name() string { return "save_progress" }

func (t *SaveProgressTool) Description() string {
	return "Persist the current state of an in-progress task. Creates or updates a task record. Call at natural checkpoints so work can be resumed after interruption."
}

func (t *SaveProgressTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "UUID — update existing task (omit to create new)",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Short task description (required on create)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "in_progress | blocked | awaiting_input | complete",
				"enum":        []string{"in_progress", "blocked", "awaiting_input", "complete"},
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "What has been done so far (optional)",
			},
			"next_steps": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"context": map[string]interface{}{
				"type":        "object",
				"description": "Arbitrary structured state to restore later",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
	}
}

func (t *SaveProgressTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	taskID, _ := args["task_id"].(string)
	title, _ := args["title"].(string)
	status, _ := args["status"].(string)
	if status == "" {
		status = "in_progress"
	}

	action := "created"
	createdAt := timestampf()

	if taskID != "" {
		// Update path: verify task exists.
		existing, err := t.client.GetPoint(ctx, taskID)
		if err != nil || existing == nil {
			return framework.TextResult(""), fmt.Errorf("not found: no task with id %v", taskID)
		}
		action = "updated"
		// Preserve original created timestamp.
		createdAt = payloadString(existing.Payload, "created")
		if createdAt == "" {
			createdAt = timestampf()
		}
		// Preserve title if not provided.
		if title == "" {
			title = payloadString(existing.Payload, "title")
		}
	} else {
		taskID = uuid.New().String()
		if title == "" {
			return framework.TextResult(""), errors.New("title is required when creating a new task")
		}
	}

	// Build embed text from title + summary.
	summary, _ := args["summary"].(string)
	embedText := title
	if summary != "" {
		embedText = title + " " + summary
	}

	var vector []float64
	if t.embedder != nil {
		var err error
		vector, err = t.embedder.Embed(ctx, embedText)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("embed task: %v", err)
		}
	}

	payload := map[string]interface{}{
		"memory_type": "task",
		"title":       title,
		"status":      status,
		"content":     embedText,
		"created":     createdAt,
		"updated":     timestampf(),
	}
	if summary != "" {
		payload["summary"] = summary
	}
	if rawSteps, ok := args["next_steps"].([]interface{}); ok {
		payload["next_steps"] = tagsToIfaces(rawSteps)
	}
	if taskCtx, ok := args["context"].(map[string]interface{}); ok {
		payload["task_context"] = taskCtx
	}
	if tags, ok := args["tags"].([]interface{}); ok {
		payload["tags"] = tagsToIfaces(tags)
	}

	if err := t.client.UpsertPoint(ctx, taskID, vector, payload); err != nil {
		return framework.TextResult(""), fmt.Errorf("save_progress: %v", err)
	}

	out := map[string]interface{}{"task_id": taskID, "action": action, "title": title}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *SaveProgressTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(false),
	)
}

// ---------------------------------------------------------------------------
// ResumeTaskTool — working memory read
// ---------------------------------------------------------------------------

type ResumeTaskTool struct {
	client   QdrantClient
	cfg      readonly.ReadOnlyChecker
	embedder embed.Provider
}

func NewResumeTaskTool(c QdrantClient, cfg readonly.ReadOnlyChecker, ep embed.Provider) *ResumeTaskTool {
	return &ResumeTaskTool{client: c, cfg: cfg, embedder: ep}
}

func (t *ResumeTaskTool) Name() string { return "resume_task" }

func (t *ResumeTaskTool) Description() string {
	return "Find and load a task by description or ID. Returns full task state formatted for context injection."
}

func (t *ResumeTaskTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "Load by exact UUID (optional)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Describe the task to search for (optional)",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Filter by status (optional)",
			},
			"limit": map[string]interface{}{
				"type":    "integer",
				"default": 3,
			},
		},
	}
}

func (t *ResumeTaskTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	taskID, _ := args["task_id"].(string)
	query, _ := args["query"].(string)
	status, _ := args["status"].(string)
	limit := 3
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	if taskID == "" && query == "" {
		return framework.TextResult(""), errors.New("resume_task: supply either task_id or query")
	}

	type taskOut struct {
		TaskID    string      `json:"task_id"`
		Title     string      `json:"title"`
		Status    string      `json:"status"`
		Summary   string      `json:"summary,omitempty"`
		NextSteps []string    `json:"next_steps,omitempty"`
		Context   interface{} `json:"context,omitempty"`
		Updated   string      `json:"updated"`
		Age       string      `json:"age"`
	}

	var tasks []taskOut

	if taskID != "" {
		// Exact ID lookup.
		result, err := t.client.GetPoint(ctx, taskID)
		if err != nil || result == nil {
			return framework.TextResult(""), fmt.Errorf("not found: no task with id %v", taskID)
		}
		tasks = append(tasks, taskOut{
			TaskID:    taskID,
			Title:     payloadString(result.Payload, "title"),
			Status:    payloadString(result.Payload, "status"),
			Summary:   payloadString(result.Payload, "summary"),
			NextSteps: ifacesToStrings(result.Payload["next_steps"]),
			Context:   result.Payload["task_context"],
			Updated:   payloadString(result.Payload, "updated"),
			Age:       humanAge(payloadString(result.Payload, "updated")),
		})
	} else {
		// Semantic search.
		var vector []float64
		if t.embedder != nil {
			var err error
			vector, err = t.embedder.Embed(ctx, query)
			if err != nil {
				return framework.TextResult(""), fmt.Errorf("embed query: %v", err)
			}
		}
		filter := map[string]interface{}{"memory_type": "task"}
		if status != "" {
			filter["status"] = status
		}
		results, err := t.client.Search(ctx, vector, limit, filter)
		if err != nil {
			return framework.TextResult(""), fmt.Errorf("resume_task search: %v", err)
		}
		for _, r := range results {
			taskStatus := payloadString(r.Payload, "status")
			// Default: exclude complete and abandoned.
			if status == "" && (taskStatus == "complete" || taskStatus == "abandoned") {
				continue
			}
			tasks = append(tasks, taskOut{
				TaskID:    r.ID,
				Title:     payloadString(r.Payload, "title"),
				Status:    taskStatus,
				Summary:   payloadString(r.Payload, "summary"),
				NextSteps: ifacesToStrings(r.Payload["next_steps"]),
				Context:   r.Payload["task_context"],
				Updated:   payloadString(r.Payload, "updated"),
				Age:       humanAge(payloadString(r.Payload, "updated")),
			})
		}
	}

	out := map[string]interface{}{"tasks": tasks}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *ResumeTaskTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// ListTasksTool — working memory list
// ---------------------------------------------------------------------------

type ListTasksTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewListTasksTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *ListTasksTool {
	return &ListTasksTool{client: c, cfg: cfg}
}

func (t *ListTasksTool) Name() string { return "list_tasks" }

func (t *ListTasksTool) Description() string {
	return "List all active tasks with their current status. By default excludes complete and abandoned tasks."
}

func (t *ListTasksTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Filter by status (optional, default: all non-complete)",
			},
			"tags": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"limit": map[string]interface{}{
				"type":    "integer",
				"default": 20,
			},
		},
	}
}

func (t *ListTasksTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	status, _ := args["status"].(string)

	filter := map[string]interface{}{"memory_type": "task"}
	if status != "" {
		filter["status"] = status
	}
	if tags, ok := args["tags"].([]interface{}); ok && len(tags) > 0 {
		filter["tags"] = tags[0]
	}

	results, _, err := t.client.Scroll(ctx, limit*3, filter, "")
	if err != nil {
		return framework.TextResult(""), fmt.Errorf("list_tasks: %v", err)
	}

	type taskSummary struct {
		TaskID  string `json:"task_id"`
		Title   string `json:"title"`
		Status  string `json:"status"`
		Updated string `json:"updated"`
		Age     string `json:"age"`
	}

	var tasks []taskSummary
	for _, r := range results {
		taskStatus := payloadString(r.Payload, "status")
		// Default: exclude complete and abandoned.
		if status == "" && (taskStatus == "complete" || taskStatus == "abandoned") {
			continue
		}
		tasks = append(tasks, taskSummary{
			TaskID:  r.ID,
			Title:   payloadString(r.Payload, "title"),
			Status:  taskStatus,
			Updated: payloadString(r.Payload, "updated"),
			Age:     humanAge(payloadString(r.Payload, "updated")),
		})
		if len(tasks) >= limit {
			break
		}
	}

	out := map[string]interface{}{"tasks": tasks, "count": len(tasks)}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *ListTasksTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// AbandonTaskTool — working memory status update
// ---------------------------------------------------------------------------

type AbandonTaskTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewAbandonTaskTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *AbandonTaskTool {
	return &AbandonTaskTool{client: c, cfg: cfg}
}

func (t *AbandonTaskTool) Name() string { return "abandon_task" }

func (t *AbandonTaskTool) Description() string {
	return "Mark a task as abandoned with an optional reason. Does not delete — kept for episodic record."
}

func (t *AbandonTaskTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "UUID of the task to abandon",
			},
			"reason": map[string]interface{}{
				"type":        "string",
				"description": "Why it was abandoned (optional)",
			},
		},
		Required: []string{"task_id"},
	}
}

func (t *AbandonTaskTool) Handle(ctx context.Context, args map[string]interface{}) (framework.ToolResult, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return framework.TextResult(""), err
	}

	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return framework.TextResult(""), errors.New("task_id is required")
	}
	reason, _ := args["reason"].(string)

	update := map[string]interface{}{
		"status":  "abandoned",
		"updated": timestampf(),
	}
	if reason != "" {
		update["abandon_reason"] = reason
	}

	if err := t.client.SetPayload(ctx, taskID, update); err != nil {
		return framework.TextResult(""), fmt.Errorf("abandon_task: %v", err)
	}

	out := map[string]interface{}{"task_id": taskID, "status": "abandoned"}
	b, _ := json.Marshal(out)
	return framework.TextResult(string(b)), nil
}

func (t *AbandonTaskTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(false),
	)
}
