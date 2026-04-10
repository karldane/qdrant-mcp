package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// remember
// ---------------------------------------------------------------------------

func TestRememberHandle_CreatesNewMemory(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"content": "Paris is the capital of France",
		"tags":    []interface{}{"geography"},
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "created", result["action"])
	assert.NotEmpty(t, result["id"])
	assert.Equal(t, 1, ep.called)
}

func TestRememberHandle_DeduplicatesNearMatch(t *testing.T) {
	// Search returns a result with score above threshold.
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "existing-uuid", Score: 0.97, Payload: map[string]interface{}{"memory_type": "semantic", "content": "old text"}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"content": "Paris is the capital of France (updated)",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "updated", result["action"])
	assert.Equal(t, "existing-uuid", result["id"])
}

func TestRememberHandle_DoesNotDeduplicateLowScore(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "other-uuid", Score: 0.80, Payload: map[string]interface{}{"memory_type": "semantic"}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"content": "something different",
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "created", result["action"])
}

func TestRememberHandle_ReadonlyBlocked(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRememberTool(mc, roCfg, ep, 0.95)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"content": "x"})
	require.Error(t, err)
}

func TestRememberHandle_EmptyContentError(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"content": ""})
	require.Error(t, err)
}

func TestRememberHandle_EmbedError(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{err: errors.New("embed down")}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"content": "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed")
}

func TestRememberHandle_UpsertError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("qdrant down")}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"content": "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store memory")
}

func TestRememberHandle_IDIsUUID(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRememberTool(mc, rwCfg, ep, 0.95)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"content": "uuid test"})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	id, ok := result["id"].(string)
	require.True(t, ok)
	assert.Len(t, id, 36)
	assert.Equal(t, 4, strings.Count(id, "-"))
}

func TestRememberTool_Schema(t *testing.T) {
	tool := NewRememberTool(nil, rwCfg, nil, 0.95)
	assert.Equal(t, "object", tool.Schema().Type)
	assert.Contains(t, tool.Schema().Required, "content")
}

// ---------------------------------------------------------------------------
// recall
// ---------------------------------------------------------------------------

func TestRecallHandle_ReturnsMemories(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.88, Payload: map[string]interface{}{
				"memory_type": "semantic",
				"content":     "France facts",
				"confidence":  float64(1.0),
				"created":     "2026-01-01T00:00:00Z",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRecallTool(mc, rwCfg, ep)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"query": "France",
		"limit": float64(5),
	})
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(1), result["count"])
}

func TestRecallHandle_FiltersExpired(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.9, Payload: map[string]interface{}{
				"memory_type": "semantic",
				"content":     "expired fact",
				"ttl":         "2000-01-01T00:00:00Z", // expired
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRecallTool(mc, rwCfg, ep)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"query": "test"})
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(0), result["count"])
}

func TestRecallHandle_EmbedError(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{err: errors.New("embed down")}
	tool := NewRecallTool(mc, rwCfg, ep)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"query": "test"})
	require.Error(t, err)
}

func TestRecallHandle_SearchError(t *testing.T) {
	mc := &mockClient{searchErr: errors.New("search fail")}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewRecallTool(mc, rwCfg, ep)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"query": "test"})
	require.Error(t, err)
}

func TestRecallTool_Schema(t *testing.T) {
	tool := NewRecallTool(nil, rwCfg, nil)
	assert.Equal(t, "object", tool.Schema().Type)
	assert.Contains(t, tool.Schema().Required, "query")
}

// ---------------------------------------------------------------------------
// forget
// ---------------------------------------------------------------------------

func TestForgetHandle_ByID(t *testing.T) {
	mc := &mockClient{}
	tool := NewForgetTool(mc, rwCfg, nil)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"id": "some-uuid-1234-5678",
	})
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(1), result["deleted"])
}

func TestForgetHandle_ByQueryWithoutConfirm_ReturnsPending(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.9, Payload: map[string]interface{}{"content": "test fact"}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewForgetTool(mc, rwCfg, ep)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"query":   "test",
		"confirm": false,
	})
	require.NoError(t, err)
	assert.Contains(t, out, "pending_delete")
	assert.Contains(t, out, "confirm=true")
}

func TestForgetHandle_ByQueryWithConfirm_Deletes(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.9, Payload: map[string]interface{}{"content": "test fact"}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewForgetTool(mc, rwCfg, ep)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"query":   "test",
		"confirm": true,
	})
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, float64(1), result["deleted"])
}

func TestForgetHandle_NoArgsError(t *testing.T) {
	mc := &mockClient{}
	tool := NewForgetTool(mc, rwCfg, nil)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
}

func TestForgetHandle_ReadonlyBlocked(t *testing.T) {
	mc := &mockClient{}
	tool := NewForgetTool(mc, roCfg, nil)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"id": "x"})
	require.Error(t, err)
}

func TestForgetTool_Schema(t *testing.T) {
	tool := NewForgetTool(nil, rwCfg, nil)
	assert.Equal(t, "object", tool.Schema().Type)
	assert.Empty(t, tool.Schema().Required)
}

// ---------------------------------------------------------------------------
// reflect
// ---------------------------------------------------------------------------

func TestReflectHandle_ReturnsSummary(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.9, Payload: map[string]interface{}{
				"memory_type": "semantic",
				"content":     "France is in Europe",
				"confidence":  float64(0.9),
				"created":     "2026-04-01T00:00:00Z",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	tool := NewReflectTool(mc, rwCfg, ep)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"topic": "France"})
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.NotEmpty(t, result["summary"])
	assert.Equal(t, float64(1), result["count"])
}

func TestReflectHandle_EmptyTopicError(t *testing.T) {
	tool := NewReflectTool(&mockClient{}, rwCfg, &mockEmbedProvider{result: []float64{0.1}})
	_, err := tool.Handle(context.Background(), map[string]interface{}{"topic": ""})
	require.Error(t, err)
}

func TestReflectHandle_EmbedError(t *testing.T) {
	mc := &mockClient{}
	ep := &mockEmbedProvider{err: errors.New("embed down")}
	tool := NewReflectTool(mc, rwCfg, ep)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"topic": "test"})
	require.Error(t, err)
}

func TestReflectTool_Schema(t *testing.T) {
	tool := NewReflectTool(nil, rwCfg, nil)
	assert.Equal(t, "object", tool.Schema().Type)
	assert.Contains(t, tool.Schema().Required, "topic")
}
