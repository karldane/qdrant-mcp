package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SaveProgressTool
// ---------------------------------------------------------------------------

func TestSaveProgressTool_Name(t *testing.T) {
	assert.Equal(t, "save_progress", NewSaveProgressTool(nil, rwCfg, nil).Name())
}

func TestSaveProgressTool_Readonly(t *testing.T) {
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewSaveProgressTool(&mockClient{}, roCfg, ep).Handle(context.Background(), map[string]interface{}{
		"title": "some task",
	})
	require.Error(t, err)
}

func TestSaveProgressTool_CreateRequiresTitle(t *testing.T) {
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewSaveProgressTool(&mockClient{}, rwCfg, ep).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}

func TestSaveProgressTool_CreateSuccess(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	result, err := NewSaveProgressTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"title":   "Refactor auth module",
		"summary": "Moved JWT logic to separate package",
		"next_steps": []interface{}{
			"write tests",
			"update docs",
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "task_id")
	assert.Contains(t, result.Content[0].Text, "created")
	assert.Contains(t, result.Content[0].Text, "Refactor auth module")
}

func TestSaveProgressTool_UpdateNotFound(t *testing.T) {
	mc := &mockClient{getErr: errors.New("not found")}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewSaveProgressTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"task_id": "nonexistent-uuid",
		"status":  "complete",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSaveProgressTool_UpdateSuccess(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID: "task-1",
			Payload: map[string]interface{}{
				"title":   "existing task",
				"status":  "in_progress",
				"created": "2026-01-01T00:00:00Z",
			},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	result, err := NewSaveProgressTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"task_id": "task-1",
		"status":  "complete",
		"summary": "All done",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "updated")
}

// ---------------------------------------------------------------------------
// ResumeTaskTool
// ---------------------------------------------------------------------------

func TestResumeTaskTool_Name(t *testing.T) {
	assert.Equal(t, "resume_task", NewResumeTaskTool(nil, rwCfg, nil).Name())
}

func TestResumeTaskTool_RequiresIdOrQuery(t *testing.T) {
	_, err := NewResumeTaskTool(&mockClient{}, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task_id or query")
}

func TestResumeTaskTool_ByID(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID: "task-42",
			Payload: map[string]interface{}{
				"title":   "important task",
				"status":  "in_progress",
				"summary": "halfway done",
				"updated": "2026-01-01T00:00:00Z",
			},
		},
	}
	result, err := NewResumeTaskTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"task_id": "task-42",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "important task")
}

func TestResumeTaskTool_ByQueryExcludesComplete(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "t1", Score: 0.9, Payload: map[string]interface{}{
				"title":  "completed task",
				"status": "complete",
			}},
			{ID: "t2", Score: 0.8, Payload: map[string]interface{}{
				"title":  "active task",
				"status": "in_progress",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	result, err := NewResumeTaskTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"query": "find my work",
	})
	require.NoError(t, err)
	assert.NotContains(t, result.Content[0].Text, "completed task")
	assert.Contains(t, result.Content[0].Text, "active task")
}

// ---------------------------------------------------------------------------
// ListTasksTool
// ---------------------------------------------------------------------------

func TestListTasksTool_Name(t *testing.T) {
	assert.Equal(t, "list_tasks", NewListTasksTool(nil, rwCfg).Name())
}

func TestListTasksTool_ExcludesCompleteByDefault(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "t1", Payload: map[string]interface{}{"title": "active", "status": "in_progress"}},
			{ID: "t2", Payload: map[string]interface{}{"title": "done", "status": "complete"}},
			{ID: "t3", Payload: map[string]interface{}{"title": "gave up", "status": "abandoned"}},
		},
	}
	result, err := NewListTasksTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "active")
	assert.NotContains(t, result.Content[0].Text, "done")
	assert.NotContains(t, result.Content[0].Text, "gave up")
}

func TestListTasksTool_FilterByStatus(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "t1", Payload: map[string]interface{}{"title": "blocked one", "status": "blocked"}},
		},
	}
	result, err := NewListTasksTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{
		"status": "blocked",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "blocked one")
}

func TestListTasksTool_Count(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "t1", Payload: map[string]interface{}{"title": "task1", "status": "in_progress"}},
			{ID: "t2", Payload: map[string]interface{}{"title": "task2", "status": "blocked"}},
		},
	}
	result, err := NewListTasksTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, `"count":2`)
}

// ---------------------------------------------------------------------------
// AbandonTaskTool
// ---------------------------------------------------------------------------

func TestAbandonTaskTool_Name(t *testing.T) {
	assert.Equal(t, "abandon_task", NewAbandonTaskTool(nil, rwCfg).Name())
}

func TestAbandonTaskTool_Readonly(t *testing.T) {
	_, err := NewAbandonTaskTool(&mockClient{}, roCfg).Handle(context.Background(), map[string]interface{}{
		"task_id": "some-id",
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "readonly") || strings.Contains(err.Error(), "not permitted"))
}

func TestAbandonTaskTool_RequiresTaskID(t *testing.T) {
	_, err := NewAbandonTaskTool(&mockClient{}, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task_id")
}

func TestAbandonTaskTool_Success(t *testing.T) {
	mc := &mockClient{}
	result, err := NewAbandonTaskTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{
		"task_id": "task-99",
		"reason":  "decided not to pursue",
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "task-99")
	assert.Contains(t, result.Content[0].Text, "abandoned")
}

func TestAbandonTaskTool_SetPayloadError(t *testing.T) {
	mc := &mockClient{setPayloadErr: errors.New("set payload failed")}
	_, err := NewAbandonTaskTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{
		"task_id": "task-99",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "abandon_task")
}
