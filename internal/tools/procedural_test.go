package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// LearnProcedureTool
// ---------------------------------------------------------------------------

func TestLearnProcedureTool_Name(t *testing.T) {
	assert.Equal(t, "learn_procedure", NewLearnProcedureTool(nil, rwCfg, nil).Name())
}

func TestLearnProcedureTool_Readonly(t *testing.T) {
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewLearnProcedureTool(&mockClient{}, roCfg, ep).Handle(context.Background(), map[string]interface{}{
		"name":        "deploy",
		"description": "how to deploy",
		"steps":       []interface{}{"step1"},
	})
	require.Error(t, err)
}

func TestLearnProcedureTool_RequiredFields(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewLearnProcedureTool(mc, rwCfg, ep)

	_, err := tool.Handle(context.Background(), map[string]interface{}{
		"description": "desc",
		"steps":       []interface{}{"s1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")

	_, err = tool.Handle(context.Background(), map[string]interface{}{
		"name":  "p",
		"steps": []interface{}{"s1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description")

	_, err = tool.Handle(context.Background(), map[string]interface{}{
		"name":        "p",
		"description": "d",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "steps")
}

func TestLearnProcedureTool_CreateNew(t *testing.T) {
	mc := &mockClient{scrollRes: []client.ScrollResult{}} // no existing
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	out, err := NewLearnProcedureTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"name":        "build-go",
		"description": "Build a Go project",
		"steps":       []interface{}{"go mod tidy", "go build ./..."},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "created")
	assert.Contains(t, out, "build-go")
}

func TestLearnProcedureTool_UpdateExisting(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "proc-1", Payload: map[string]interface{}{
				"name":     "build-go",
				"revision": float64(1),
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewLearnProcedureTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"name":        "build-go",
		"description": "Build a Go project (updated)",
		"steps":       []interface{}{"go mod tidy", "go build ./...", "go test ./..."},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "updated")
}

// ---------------------------------------------------------------------------
// RecallProcedureTool
// ---------------------------------------------------------------------------

func TestRecallProcedureTool_Name(t *testing.T) {
	assert.Equal(t, "recall_procedure", NewRecallProcedureTool(nil, rwCfg, nil).Name())
}

func TestRecallProcedureTool_RequiresNameOrQuery(t *testing.T) {
	_, err := NewRecallProcedureTool(&mockClient{}, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name or query")
}

func TestRecallProcedureTool_ByName(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "proc-1", Payload: map[string]interface{}{
				"name":        "deploy-k8s",
				"description": "Deploy to Kubernetes",
				"steps":       []interface{}{"kubectl apply", "kubectl rollout"},
			}},
		},
	}
	out, err := NewRecallProcedureTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"name": "deploy-k8s",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "deploy-k8s")
	assert.Contains(t, out, "procedures")
}

func TestRecallProcedureTool_ByQuery(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "proc-2", Score: 0.88, Payload: map[string]interface{}{
				"name":        "debug-oom",
				"description": "Debug OOM errors",
				"steps":       []interface{}{"check logs", "analyse heap"},
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewRecallProcedureTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"query": "how to debug memory issues",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "debug-oom")
}

// ---------------------------------------------------------------------------
// UpdateProcedureTool
// ---------------------------------------------------------------------------

func TestUpdateProcedureTool_Name(t *testing.T) {
	assert.Equal(t, "update_procedure", NewUpdateProcedureTool(nil, rwCfg, nil).Name())
}

func TestUpdateProcedureTool_Readonly(t *testing.T) {
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewUpdateProcedureTool(&mockClient{}, roCfg, ep).Handle(context.Background(), map[string]interface{}{
		"id": "some-id",
	})
	require.Error(t, err)
}

func TestUpdateProcedureTool_NotFound(t *testing.T) {
	mc := &mockClient{getErr: errors.New("not found")}
	_, err := NewUpdateProcedureTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"id": "missing-id",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateProcedureTool_Success(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID: "proc-1",
			Payload: map[string]interface{}{
				"name":        "build-go",
				"description": "Build a Go project",
				"steps":       []interface{}{"go build ./..."},
				"revision":    float64(1),
				"created":     "2026-01-01T00:00:00Z",
				"updated":     "2026-01-01T00:00:00Z",
			},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewUpdateProcedureTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"id":     "proc-1",
		"steps":  []interface{}{"go mod tidy", "go build ./...", "go test ./..."},
		"reason": "Added test step",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "revision")
	assert.Contains(t, out, "proc-1")
}

func TestUpdateProcedureTool_RequiresID(t *testing.T) {
	_, err := NewUpdateProcedureTool(&mockClient{}, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}
