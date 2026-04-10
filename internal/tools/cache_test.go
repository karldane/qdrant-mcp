package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// StoreResultTool
// ---------------------------------------------------------------------------

func TestStoreResultTool_Name(t *testing.T) {
	assert.Equal(t, "store_result", NewStoreResultTool(nil, rwCfg, nil).Name())
}

func TestStoreResultTool_Readonly(t *testing.T) {
	ep := &mockEmbedProvider{result: []float64{0.1}}
	_, err := NewStoreResultTool(&mockClient{}, roCfg, ep).Handle(context.Background(), map[string]interface{}{
		"result": "some value",
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "readonly") || strings.Contains(err.Error(), "not permitted"))
}

func TestStoreResultTool_RequiresResult(t *testing.T) {
	_, err := NewStoreResultTool(&mockClient{}, rwCfg, nil).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "result")
}

func TestStoreResultTool_CreateNew(t *testing.T) {
	mc := &mockClient{scrollRes: []client.ScrollResult{}} // no existing
	ep := &mockEmbedProvider{result: []float64{0.1, 0.2}}
	out, err := NewStoreResultTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"input":     "what is 2+2?",
		"result":    "4",
		"ttl_hours": float64(12),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "key")
	assert.Contains(t, out, "created")
	assert.Contains(t, out, "expires")
	// Embed should have been called once for the input.
	assert.Equal(t, 1, ep.called)
}

func TestStoreResultTool_UpdateExisting(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "cache-1", Payload: map[string]interface{}{
				"cache_key": "abc123",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewStoreResultTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"key":    "abc123",
		"result": "updated value",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "updated")
	// Embed should NOT be called on update path (SetPayload only).
	assert.Equal(t, 0, ep.called)
}

func TestStoreResultTool_KeyDerivedFromInput(t *testing.T) {
	mc := &mockClient{scrollRes: []client.ScrollResult{}}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewStoreResultTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"input":  "derive key from this",
		"result": "the answer",
	})
	require.NoError(t, err)
	// Key should be present and non-empty.
	assert.Contains(t, out, "key")
	assert.NotContains(t, out, `"key":""`)
}

// ---------------------------------------------------------------------------
// LookupResultTool
// ---------------------------------------------------------------------------

func TestLookupResultTool_Name(t *testing.T) {
	assert.Equal(t, "lookup_result", NewLookupResultTool(nil, rwCfg, nil).Name())
}

func TestLookupResultTool_ExactHit(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "cache-1", Payload: map[string]interface{}{
				"cache_key":   "abc123",
				"result":      "the answer",
				"result_type": "text",
				"ttl":         "2099-01-01T00:00:00Z",
				"created":     "2026-01-01T00:00:00Z",
			}},
		},
	}
	out, err := NewLookupResultTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"key": "abc123",
	})
	require.NoError(t, err)
	assert.Contains(t, out, `"hit":true`)
	assert.Contains(t, out, "the answer")
}

func TestLookupResultTool_Miss_NoResults(t *testing.T) {
	mc := &mockClient{scrollRes: []client.ScrollResult{}}
	out, err := NewLookupResultTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"key": "nonexistent",
	})
	require.NoError(t, err)
	assert.Contains(t, out, `"hit":false`)
}

func TestLookupResultTool_Miss_Expired(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "cache-1", Payload: map[string]interface{}{
				"cache_key": "abc",
				"result":    "old value",
				"ttl":       "2000-01-01T00:00:00Z", // expired
			}},
		},
	}
	out, err := NewLookupResultTool(mc, rwCfg, nil).Handle(context.Background(), map[string]interface{}{
		"key": "abc",
	})
	require.NoError(t, err)
	assert.Contains(t, out, `"hit":false`)
}

func TestLookupResultTool_SemanticHit(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "cache-2", Score: 0.92, Payload: map[string]interface{}{
				"cache_key":   "sem-key",
				"result":      "semantic result",
				"result_type": "text",
				"ttl":         "2099-01-01T00:00:00Z",
				"created":     "2026-01-01T00:00:00Z",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewLookupResultTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"query":     "something similar",
		"min_score": float64(0.85),
	})
	require.NoError(t, err)
	assert.Contains(t, out, `"hit":true`)
	assert.Contains(t, out, "semantic result")
}

func TestLookupResultTool_SemanticBelowThreshold(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "c1", Score: 0.50, Payload: map[string]interface{}{
				"cache_key": "k1",
				"result":    "low score",
				"ttl":       "2099-01-01T00:00:00Z",
			}},
		},
	}
	ep := &mockEmbedProvider{result: []float64{0.1}}
	out, err := NewLookupResultTool(mc, rwCfg, ep).Handle(context.Background(), map[string]interface{}{
		"query":     "something",
		"min_score": float64(0.85),
	})
	require.NoError(t, err)
	assert.Contains(t, out, `"hit":false`)
}

// ---------------------------------------------------------------------------
// InvalidateResultTool
// ---------------------------------------------------------------------------

func TestInvalidateResultTool_Name(t *testing.T) {
	assert.Equal(t, "invalidate_result", NewInvalidateResultTool(nil, rwCfg).Name())
}

func TestInvalidateResultTool_Readonly(t *testing.T) {
	_, err := NewInvalidateResultTool(&mockClient{}, roCfg).Handle(context.Background(), map[string]interface{}{
		"key": "some-key",
	})
	require.Error(t, err)
}

func TestInvalidateResultTool_RequiresKeyOrTags(t *testing.T) {
	_, err := NewInvalidateResultTool(&mockClient{}, rwCfg).Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key, tags, or query")
}

func TestInvalidateResultTool_ByKey(t *testing.T) {
	mc := &mockClient{}
	out, err := NewInvalidateResultTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{
		"key": "cache-key-1",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "invalidated")
}

func TestInvalidateResultTool_ByTags(t *testing.T) {
	mc := &mockClient{}
	out, err := NewInvalidateResultTool(mc, rwCfg).Handle(context.Background(), map[string]interface{}{
		"tags": []interface{}{"stale", "old"},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "invalidated")
}
