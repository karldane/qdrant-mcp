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
// WhatDoIKnowTool
// ---------------------------------------------------------------------------

func TestWhatDoIKnowTool_Name(t *testing.T) {
	assert.Equal(t, "what_do_i_know", NewWhatDoIKnowTool(nil, rwCfg, nil).Name())
}

func TestWhatDoIKnowTool_NoEmbed(t *testing.T) {
	// what_do_i_know must NOT call embed — all reads are via Count/Scroll.
	ep := &mockEmbedProvider{}
	mc := &mockClient{
		countResult: 5,
		scrollRes:   []client.ScrollResult{},
	}
	out, err := NewWhatDoIKnowTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	// Embed must not have been called.
	assert.Equal(t, 0, ep.called)
	// All memory type sections present.
	assert.Contains(t, out, "semantic_memory")
	assert.Contains(t, out, "episodic_memory")
	assert.Contains(t, out, "procedures")
	assert.Contains(t, out, "active_tasks")
	assert.Contains(t, out, "cache")
}

func TestWhatDoIKnowTool_CountsPopulated(t *testing.T) {
	mc := &mockClient{
		countResult: 42,
		scrollRes:   []client.ScrollResult{},
	}
	out, err := NewWhatDoIKnowTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, out, "42")
}

func TestWhatDoIKnowTool_ProcedureNames(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "p1", Payload: map[string]interface{}{
				"memory_type": "procedural",
				"name":        "deploy-k8s",
			}},
		},
	}
	out, err := NewWhatDoIKnowTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, out, "deploy-k8s")
}

func TestWhatDoIKnowTool_ActiveTasksExcludeComplete(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "t1", Payload: map[string]interface{}{
				"memory_type": "task",
				"title":       "active work",
				"status":      "in_progress",
			}},
			{ID: "t2", Payload: map[string]interface{}{
				"memory_type": "task",
				"title":       "finished work",
				"status":      "complete",
			}},
		},
	}
	out, err := NewWhatDoIKnowTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, out, "active work")
	assert.NotContains(t, out, "finished work")
}

// ---------------------------------------------------------------------------
// MemoryStatsTool
// ---------------------------------------------------------------------------

func TestMemoryStatsTool_Name(t *testing.T) {
	assert.Equal(t, "memory_stats", NewMemoryStatsTool(nil, rwCfg).Name())
}

func TestMemoryStatsTool_Success(t *testing.T) {
	mc := &mockClient{
		countResult: 10,
		collectionInfoRes: map[string]interface{}{
			"vectors_count": int64(50),
			"status":        "ready",
		},
	}
	out, err := NewMemoryStatsTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, out, "total_points")
	assert.Contains(t, out, "by_type")
	assert.Contains(t, out, "vector_count")
	assert.Contains(t, out, "index_status")
}

func TestMemoryStatsTool_CollectionInfoError(t *testing.T) {
	mc := &mockClient{
		collectionInfoErr: errors.New("connection failed"),
	}
	_, err := NewMemoryStatsTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory_stats")
}

func TestMemoryStatsTool_ByTypeKeys(t *testing.T) {
	mc := &mockClient{
		countResult:       3,
		collectionInfoRes: map[string]interface{}{"status": "ready"},
	}
	out, err := NewMemoryStatsTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	// All expected memory types should appear in by_type.
	assert.Contains(t, out, "semantic")
	assert.Contains(t, out, "episodic")
	assert.Contains(t, out, "procedural")
	assert.Contains(t, out, "task")
	assert.Contains(t, out, "cache")
}
