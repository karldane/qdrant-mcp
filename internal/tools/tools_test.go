package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/karldane/mcp-framework/framework"
	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// readOnlyStub implements readonly.ReadOnlyChecker.
type readOnlyStub struct{ ro bool }

func (r *readOnlyStub) ReadOnly() bool { return r.ro }

var (
	rwCfg = &readOnlyStub{ro: false}
	roCfg = &readOnlyStub{ro: true}
)

// mockClient is a controllable implementation of QdrantClient.
type mockClient struct {
	upsertErr  error
	searchRes  []client.SearchResult
	searchErr  error
	scrollRes  []client.ScrollResult
	scrollNext string
	scrollErr  error
	getRes     *client.GetResult
	getErr     error
	deleteErr  error
}

func (m *mockClient) UpsertPoint(_ context.Context, _ string, _ []float64, _ map[string]interface{}) error {
	return m.upsertErr
}
func (m *mockClient) Search(_ context.Context, _ []float64, _ int, _ map[string]interface{}) ([]client.SearchResult, error) {
	return m.searchRes, m.searchErr
}
func (m *mockClient) Scroll(_ context.Context, _ int, _ map[string]interface{}, _ string) ([]client.ScrollResult, string, error) {
	return m.scrollRes, m.scrollNext, m.scrollErr
}
func (m *mockClient) GetPoint(_ context.Context, _ string) (*client.GetResult, error) {
	return m.getRes, m.getErr
}
func (m *mockClient) DeletePoints(_ context.Context, _ []string, _ map[string]interface{}) error {
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// Name / Description smoke tests
// ---------------------------------------------------------------------------

func TestToolNames(t *testing.T) {
	tools := []struct {
		name string
		got  string
	}{
		{"upsert_point", NewUpsertPointTool(nil, rwCfg).Name()},
		{"search_points", NewSearchPointsTool(nil, rwCfg).Name()},
		{"scroll_points", NewScrollPointsTool(nil, rwCfg).Name()},
		{"get_point", NewGetPointTool(nil, rwCfg).Name()},
		{"delete_points", NewDeletePointsTool(nil, rwCfg).Name()},
		{"upsert_memory", NewUpsertMemoryTool(nil, rwCfg).Name()},
		{"search_memory", NewSearchMemoryTool(nil, rwCfg).Name()},
		{"list_sessions", NewListSessionsTool(nil, rwCfg).Name()},
		{"load_session", NewLoadSessionTool(nil, rwCfg).Name()},
		{"save_session", NewSaveSessionTool(nil, rwCfg).Name()},
		{"invalidate_cache", NewInvalidateCacheTool(nil, rwCfg).Name()},
		{"upsert_cache", NewUpsertCacheTool(nil, rwCfg).Name()},
		{"get_cache", NewGetCacheTool(nil, rwCfg).Name()},
	}
	for _, tt := range tools {
		assert.Equal(t, tt.name, tt.got, "tool name mismatch")
	}
}

func TestToolDescriptionsNonEmpty(t *testing.T) {
	descs := []string{
		NewUpsertPointTool(nil, rwCfg).Description(),
		NewSearchPointsTool(nil, rwCfg).Description(),
		NewScrollPointsTool(nil, rwCfg).Description(),
		NewGetPointTool(nil, rwCfg).Description(),
		NewDeletePointsTool(nil, rwCfg).Description(),
		NewUpsertMemoryTool(nil, rwCfg).Description(),
		NewSearchMemoryTool(nil, rwCfg).Description(),
		NewListSessionsTool(nil, rwCfg).Description(),
		NewLoadSessionTool(nil, rwCfg).Description(),
		NewSaveSessionTool(nil, rwCfg).Description(),
		NewInvalidateCacheTool(nil, rwCfg).Description(),
		NewUpsertCacheTool(nil, rwCfg).Description(),
		NewGetCacheTool(nil, rwCfg).Description(),
	}
	for _, d := range descs {
		assert.NotEmpty(t, d)
	}
}

// ---------------------------------------------------------------------------
// Schema tests
// ---------------------------------------------------------------------------

func TestSchemaTypes(t *testing.T) {
	schemas := []struct {
		name string
		typ  string
	}{
		{"upsert_point", NewUpsertPointTool(nil, rwCfg).Schema().Type},
		{"search_points", NewSearchPointsTool(nil, rwCfg).Schema().Type},
		{"scroll_points", NewScrollPointsTool(nil, rwCfg).Schema().Type},
		{"get_point", NewGetPointTool(nil, rwCfg).Schema().Type},
		{"delete_points", NewDeletePointsTool(nil, rwCfg).Schema().Type},
		{"upsert_memory", NewUpsertMemoryTool(nil, rwCfg).Schema().Type},
		{"search_memory", NewSearchMemoryTool(nil, rwCfg).Schema().Type},
		{"list_sessions", NewListSessionsTool(nil, rwCfg).Schema().Type},
		{"load_session", NewLoadSessionTool(nil, rwCfg).Schema().Type},
		{"save_session", NewSaveSessionTool(nil, rwCfg).Schema().Type},
		{"invalidate_cache", NewInvalidateCacheTool(nil, rwCfg).Schema().Type},
		{"upsert_cache", NewUpsertCacheTool(nil, rwCfg).Schema().Type},
		{"get_cache", NewGetCacheTool(nil, rwCfg).Schema().Type},
	}
	for _, s := range schemas {
		assert.Equal(t, "object", s.typ, "schema type for %s", s.name)
	}
}

func TestRequiredFields(t *testing.T) {
	tests := []struct {
		toolName string
		required []string
		schema   func() []string
	}{
		{
			"upsert_point",
			[]string{"id"},
			func() []string { return NewUpsertPointTool(nil, rwCfg).Schema().Required },
		},
		{
			"search_points",
			[]string{"query_vector"},
			func() []string { return NewSearchPointsTool(nil, rwCfg).Schema().Required },
		},
		{
			"get_point",
			[]string{"id"},
			func() []string { return NewGetPointTool(nil, rwCfg).Schema().Required },
		},
		{
			"upsert_memory",
			[]string{"content"},
			func() []string { return NewUpsertMemoryTool(nil, rwCfg).Schema().Required },
		},
		{
			"search_memory",
			[]string{"query"},
			func() []string { return NewSearchMemoryTool(nil, rwCfg).Schema().Required },
		},
		{
			"load_session",
			[]string{"id"},
			func() []string { return NewLoadSessionTool(nil, rwCfg).Schema().Required },
		},
		{
			"save_session",
			[]string{"name"},
			func() []string { return NewSaveSessionTool(nil, rwCfg).Schema().Required },
		},
		{
			"upsert_cache",
			[]string{"key", "value"},
			func() []string { return NewUpsertCacheTool(nil, rwCfg).Schema().Required },
		},
		{
			"get_cache",
			[]string{"key"},
			func() []string { return NewGetCacheTool(nil, rwCfg).Schema().Required },
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := tt.schema()
			for _, req := range tt.required {
				assert.Contains(t, got, req, "%s: expected required field %q", tt.toolName, req)
			}
		})
	}
}

func TestOptionalToolsHaveNoRequiredFields(t *testing.T) {
	assert.Empty(t, NewScrollPointsTool(nil, rwCfg).Schema().Required)
	assert.Empty(t, NewListSessionsTool(nil, rwCfg).Schema().Required)
	assert.Empty(t, NewInvalidateCacheTool(nil, rwCfg).Schema().Required)
}

// ---------------------------------------------------------------------------
// EnforcerProfile tests
// ---------------------------------------------------------------------------

func TestEnforcerProfiles(t *testing.T) {
	tests := []struct {
		toolName    string
		profile     framework.EnforcerProfile
		wantRisk    framework.RiskLevel
		wantImpact  framework.ImpactScope
		wantPII     bool
		wantIdemp   bool
		wantResCost int // 0 means don't assert resource cost
	}{
		{
			toolName: "search_points", profile: NewSearchPointsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true, wantResCost: 2,
		},
		{
			toolName: "scroll_points", profile: NewScrollPointsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "get_point", profile: NewGetPointTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "search_memory", profile: NewSearchMemoryTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true, wantResCost: 2,
		},
		{
			toolName: "list_sessions", profile: NewListSessionsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "load_session", profile: NewLoadSessionTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "get_cache", profile: NewGetCacheTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: false, wantIdemp: true,
		},
		{
			toolName: "upsert_point", profile: NewUpsertPointTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "upsert_memory", profile: NewUpsertMemoryTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: true, wantIdemp: false,
		},
		{
			toolName: "save_session", profile: NewSaveSessionTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: true, wantIdemp: false,
		},
		{
			toolName: "invalidate_cache", profile: NewInvalidateCacheTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: true, wantIdemp: false,
		},
		{
			toolName: "upsert_cache", profile: NewUpsertCacheTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: false, wantIdemp: true,
		},
		{
			toolName: "delete_points", profile: NewDeletePointsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskHigh, wantImpact: framework.ImpactDelete, wantPII: true, wantIdemp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			assert.Equal(t, tt.wantRisk, tt.profile.RiskLevel, "RiskLevel")
			assert.Equal(t, tt.wantImpact, tt.profile.ImpactScope, "ImpactScope")
			assert.Equal(t, tt.wantPII, tt.profile.PIIExposure, "PIIExposure")
			assert.Equal(t, tt.wantIdemp, tt.profile.Idempotent, "Idempotent")
			if tt.wantResCost != 0 {
				assert.Equal(t, tt.wantResCost, tt.profile.ResourceCost, "ResourceCost")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Readonly enforcement — mutating tools must fail when ReadOnly() == true
// ---------------------------------------------------------------------------

func TestReadonlyEnforcement(t *testing.T) {
	ctx := context.Background()
	// pass a non-nil mock so we know the readonly check fires first
	mc := &mockClient{}

	mutatingTools := []struct {
		name   string
		handle func(args map[string]interface{}) (string, error)
	}{
		{"upsert_point", func(args map[string]interface{}) (string, error) {
			return NewUpsertPointTool(mc, roCfg).Handle(ctx, args)
		}},
		{"delete_points", func(args map[string]interface{}) (string, error) {
			return NewDeletePointsTool(mc, roCfg).Handle(ctx, args)
		}},
		{"upsert_memory", func(args map[string]interface{}) (string, error) {
			return NewUpsertMemoryTool(mc, roCfg).Handle(ctx, args)
		}},
		{"save_session", func(args map[string]interface{}) (string, error) {
			return NewSaveSessionTool(mc, roCfg).Handle(ctx, args)
		}},
		{"invalidate_cache", func(args map[string]interface{}) (string, error) {
			return NewInvalidateCacheTool(mc, roCfg).Handle(ctx, args)
		}},
		{"upsert_cache", func(args map[string]interface{}) (string, error) {
			return NewUpsertCacheTool(mc, roCfg).Handle(ctx, args)
		}},
	}

	for _, tt := range mutatingTools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.handle(map[string]interface{}{
				"id":      "test-id",
				"key":     "test-key",
				"value":   map[string]interface{}{"x": 1},
				"content": "hello",
				"name":    "session1",
				"ids":     []interface{}{"a"},
			})
			require.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), "readonly") || strings.Contains(err.Error(), "not permitted"),
				"expected readonly error, got: %v", err)
		})
	}
}

// ---------------------------------------------------------------------------
// Handle — validation (missing required args)
// ---------------------------------------------------------------------------

func TestDeletePointsRequiresIdsOrFilter(t *testing.T) {
	tool := NewDeletePointsTool(&mockClient{}, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filter")
}

// ---------------------------------------------------------------------------
// Handle — happy-path tests via mock client
// ---------------------------------------------------------------------------

func TestUpsertPointHandle_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewUpsertPointTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"id":      "abc-123",
		"vector":  []interface{}{0.1, 0.2, 0.3},
		"payload": map[string]interface{}{"text": "hello"},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "abc-123")
	assert.Contains(t, out, "true")
}

func TestUpsertPointHandle_ClientError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("upsert failed")}
	tool := NewUpsertPointTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"id": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert point")
}

func TestSearchPointsHandle_Success(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "p1", Score: 0.9, Payload: map[string]interface{}{"text": "hi"}},
		},
	}
	tool := NewSearchPointsTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"query_vector": []interface{}{0.1, 0.2},
		"limit":        float64(3),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "p1")
	assert.Contains(t, out, `"count":1`)
}

func TestSearchPointsHandle_ClientError(t *testing.T) {
	mc := &mockClient{searchErr: errors.New("search failed")}
	tool := NewSearchPointsTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{
		"query_vector": []interface{}{0.1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search points")
}

func TestScrollPointsHandle_Success(t *testing.T) {
	mc := &mockClient{
		scrollRes:  []client.ScrollResult{{ID: "s1", Payload: map[string]interface{}{}}},
		scrollNext: "next-id",
	}
	tool := NewScrollPointsTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"limit": float64(10),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "s1")
	assert.Contains(t, out, "next-id")
}

func TestScrollPointsHandle_ClientError(t *testing.T) {
	mc := &mockClient{scrollErr: errors.New("scroll failed")}
	tool := NewScrollPointsTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scroll points")
}

func TestGetPointHandle_Success(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{ID: "g1", Vector: []float32{0.1}, Payload: map[string]interface{}{"k": "v"}},
	}
	tool := NewGetPointTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"id": "g1"})
	require.NoError(t, err)
	assert.Contains(t, out, "g1")
}

func TestGetPointHandle_ClientError(t *testing.T) {
	mc := &mockClient{getErr: errors.New("not found")}
	tool := NewGetPointTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"id": "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get point")
}

func TestDeletePointsHandle_ByIDs_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewDeletePointsTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"ids": []interface{}{"id1", "id2"},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestDeletePointsHandle_ByFilter_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewDeletePointsTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"filter": map[string]interface{}{"type": "temp"},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestDeletePointsHandle_ClientError(t *testing.T) {
	mc := &mockClient{deleteErr: errors.New("delete failed")}
	tool := NewDeletePointsTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{
		"ids": []interface{}{"x"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete points")
}

func TestUpsertMemoryHandle_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewUpsertMemoryTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"content": "remember this",
		"tags":    []interface{}{"important"},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestUpsertMemoryHandle_WithTTL(t *testing.T) {
	mc := &mockClient{}
	tool := NewUpsertMemoryTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"content":     "ephemeral",
		"ttl_seconds": float64(60),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestUpsertMemoryHandle_ClientError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("upsert failed")}
	tool := NewUpsertMemoryTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"content": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert memory")
}

func TestSearchMemoryHandle_Success(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "m1", Score: 0.8, Payload: map[string]interface{}{
				"content": "hello",
				"tags":    []interface{}{"t1"},
			}},
		},
	}
	tool := NewSearchMemoryTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"query":           "hello",
		"query_embedding": []interface{}{0.1, 0.2},
		"limit":           float64(5),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, `"count":1`)
}

func TestSearchMemoryHandle_ClientError(t *testing.T) {
	mc := &mockClient{searchErr: errors.New("search failed")}
	tool := NewSearchMemoryTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"query": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search memory")
}

func TestListSessionsHandle_Success(t *testing.T) {
	mc := &mockClient{
		scrollRes: []client.ScrollResult{
			{ID: "sess1", Payload: map[string]interface{}{
				"name":  "my-session",
				"state": map[string]interface{}{"k": "v"},
			}},
		},
	}
	tool := NewListSessionsTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"limit": float64(10)})
	require.NoError(t, err)
	assert.Contains(t, out, "sess1")
	assert.Contains(t, out, "my-session")
}

func TestListSessionsHandle_ClientError(t *testing.T) {
	mc := &mockClient{scrollErr: errors.New("scroll failed")}
	tool := NewListSessionsTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list sessions")
}

func TestLoadSessionHandle_Success(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID:      "sess-abc",
			Payload: map[string]interface{}{"name": "loaded", "state": map[string]interface{}{"x": 1}},
		},
	}
	tool := NewLoadSessionTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"id": "sess-abc"})
	require.NoError(t, err)
	assert.Contains(t, out, "sess-abc")
	assert.Contains(t, out, "loaded")
}

func TestLoadSessionHandle_ClientError(t *testing.T) {
	mc := &mockClient{getErr: errors.New("not found")}
	tool := NewLoadSessionTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"id": "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load session")
}

func TestSaveSessionHandle_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewSaveSessionTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"name":  "mysession",
		"state": map[string]interface{}{"step": 1},
	})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestSaveSessionHandle_ClientError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("upsert failed")}
	tool := NewSaveSessionTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"name": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save session")
}

func TestInvalidateCacheHandle_WithPrefix(t *testing.T) {
	mc := &mockClient{}
	tool := NewInvalidateCacheTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"prefix": "user:"})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestInvalidateCacheHandle_AllCache(t *testing.T) {
	mc := &mockClient{}
	tool := NewInvalidateCacheTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, out, "true")
}

func TestInvalidateCacheHandle_ClientError(t *testing.T) {
	mc := &mockClient{deleteErr: errors.New("delete failed")}
	tool := NewInvalidateCacheTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalidate cache")
}

func TestUpsertCacheHandle_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewUpsertCacheTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{
		"key":         "mykey",
		"value":       map[string]interface{}{"result": 42},
		"ttl_seconds": float64(300),
	})
	require.NoError(t, err)
	assert.Contains(t, out, "mykey")
	assert.Contains(t, out, "true")
}

func TestUpsertCacheHandle_ClientError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("upsert failed")}
	tool := NewUpsertCacheTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{
		"key":   "k",
		"value": map[string]interface{}{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert cache")
}

func TestGetCacheHandle_Success(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID: "cache_abc",
			Payload: map[string]interface{}{
				"type":       "cache",
				"key":        "mykey",
				"created_at": "2026-01-01T00:00:00Z",
				"expires_at": "2099-01-01T00:00:00Z",
				"result":     "42",
			},
		},
	}
	tool := NewGetCacheTool(mc, rwCfg)
	out, err := tool.Handle(context.Background(), map[string]interface{}{"key": "mykey"})
	require.NoError(t, err)
	assert.Contains(t, out, "mykey")
}

func TestGetCacheHandle_Expired(t *testing.T) {
	mc := &mockClient{
		getRes: &client.GetResult{
			ID: "cache_abc",
			Payload: map[string]interface{}{
				"expires_at": "2000-01-01T00:00:00Z",
			},
		},
	}
	tool := NewGetCacheTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"key": "mykey"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestGetCacheHandle_ClientError(t *testing.T) {
	mc := &mockClient{getErr: errors.New("not found")}
	tool := NewGetCacheTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"key": "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get cache")
}
