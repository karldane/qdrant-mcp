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
	upsertErr        error
	upsertPayloadErr error
	setPayloadErr    error
	searchRes        []client.SearchResult
	searchErr        error
	scrollRes        []client.ScrollResult
	scrollNext       string
	scrollErr        error
	getRes           *client.GetResult
	getErr           error
	deleteErr        error
	countResult      int64
	countErr         error

	collectionInfoRes map[string]interface{}
	collectionInfoErr error
}

func (m *mockClient) UpsertPoint(_ context.Context, _ string, _ []float64, _ map[string]interface{}) error {
	return m.upsertErr
}
func (m *mockClient) UpsertPayload(_ context.Context, _ string, _ map[string]interface{}) error {
	return m.upsertPayloadErr
}
func (m *mockClient) SetPayload(_ context.Context, _ string, _ map[string]interface{}) error {
	return m.setPayloadErr
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
func (m *mockClient) Count(_ context.Context, _ map[string]interface{}) (int64, error) {
	return m.countResult, m.countErr
}
func (m *mockClient) CollectionInfo(_ context.Context) (map[string]interface{}, error) {
	return m.collectionInfoRes, m.collectionInfoErr
}

// mockEmbedProvider is a controllable embed.Provider for auto-embed tests.
type mockEmbedProvider struct {
	result     []float64
	err        error
	called     int
	vectorSize int
}

func (m *mockEmbedProvider) Embed(_ context.Context, _ string) ([]float64, error) {
	m.called++
	return m.result, m.err
}
func (m *mockEmbedProvider) VectorSize() int {
	if m.vectorSize == 0 {
		return 768
	}
	return m.vectorSize
}

// ---------------------------------------------------------------------------
// Name / Description smoke tests — CRUD + new agent tools
// ---------------------------------------------------------------------------

func TestToolNames(t *testing.T) {
	ep := &mockEmbedProvider{}
	tools := []struct {
		name string
		got  string
	}{
		// CRUD primitives
		{"upsert_point", NewUpsertPointTool(nil, rwCfg).Name()},
		{"search_points", NewSearchPointsTool(nil, rwCfg).Name()},
		{"scroll_points", NewScrollPointsTool(nil, rwCfg).Name()},
		{"get_point", NewGetPointTool(nil, rwCfg).Name()},
		{"delete_points", NewDeletePointsTool(nil, rwCfg).Name()},
		// Semantic
		{"remember", NewRememberTool(nil, rwCfg, ep, 0.95).Name()},
		{"recall", NewRecallTool(nil, rwCfg, ep).Name()},
		{"forget", NewForgetTool(nil, rwCfg, ep).Name()},
		{"reflect", NewReflectTool(nil, rwCfg, ep).Name()},
		// Episodic
		{"log_event", NewLogEventTool(nil, rwCfg, ep).Name()},
		{"recall_events", NewRecallEventsTool(nil, rwCfg, ep).Name()},
		{"summarise_period", NewSummarisePeriodTool(nil, rwCfg, ep).Name()},
		// Procedural
		{"learn_procedure", NewLearnProcedureTool(nil, rwCfg, ep).Name()},
		{"recall_procedure", NewRecallProcedureTool(nil, rwCfg, ep).Name()},
		{"update_procedure", NewUpdateProcedureTool(nil, rwCfg, ep).Name()},
		// Tasks
		{"save_progress", NewSaveProgressTool(nil, rwCfg, ep).Name()},
		{"resume_task", NewResumeTaskTool(nil, rwCfg, ep).Name()},
		{"list_tasks", NewListTasksTool(nil, rwCfg).Name()},
		{"abandon_task", NewAbandonTaskTool(nil, rwCfg).Name()},
		// Cache
		{"store_result", NewStoreResultTool(nil, rwCfg, ep).Name()},
		{"lookup_result", NewLookupResultTool(nil, rwCfg, ep).Name()},
		{"invalidate_result", NewInvalidateResultTool(nil, rwCfg).Name()},
		// Introspection
		{"what_do_i_know", NewWhatDoIKnowTool(nil, rwCfg, ep).Name()},
		{"memory_stats", NewMemoryStatsTool(nil, rwCfg).Name()},
		// Admin
		{"collection_info", NewCollectionInfoTool(nil, rwCfg).Name()},
	}
	for _, tt := range tools {
		assert.Equal(t, tt.name, tt.got, "tool name mismatch")
	}
}

func TestToolDescriptionsNonEmpty(t *testing.T) {
	ep := &mockEmbedProvider{}
	descs := []string{
		NewUpsertPointTool(nil, rwCfg).Description(),
		NewSearchPointsTool(nil, rwCfg).Description(),
		NewRememberTool(nil, rwCfg, ep, 0.95).Description(),
		NewRecallTool(nil, rwCfg, ep).Description(),
		NewForgetTool(nil, rwCfg, ep).Description(),
		NewReflectTool(nil, rwCfg, ep).Description(),
		NewLogEventTool(nil, rwCfg, ep).Description(),
		NewRecallEventsTool(nil, rwCfg, ep).Description(),
		NewSummarisePeriodTool(nil, rwCfg, ep).Description(),
		NewLearnProcedureTool(nil, rwCfg, ep).Description(),
		NewRecallProcedureTool(nil, rwCfg, ep).Description(),
		NewUpdateProcedureTool(nil, rwCfg, ep).Description(),
		NewSaveProgressTool(nil, rwCfg, ep).Description(),
		NewResumeTaskTool(nil, rwCfg, ep).Description(),
		NewListTasksTool(nil, rwCfg).Description(),
		NewAbandonTaskTool(nil, rwCfg).Description(),
		NewStoreResultTool(nil, rwCfg, ep).Description(),
		NewLookupResultTool(nil, rwCfg, ep).Description(),
		NewInvalidateResultTool(nil, rwCfg).Description(),
		NewWhatDoIKnowTool(nil, rwCfg, ep).Description(),
		NewMemoryStatsTool(nil, rwCfg).Description(),
		NewCollectionInfoTool(nil, rwCfg).Description(),
	}
	for _, d := range descs {
		assert.NotEmpty(t, d)
	}
}

// ---------------------------------------------------------------------------
// EnforcerProfile tests — CRUD tools
// ---------------------------------------------------------------------------

func TestEnforcerProfiles(t *testing.T) {
	tests := []struct {
		toolName    string
		profile     *framework.EnforcerProfile
		wantRisk    framework.RiskLevel
		wantImpact  framework.ImpactScope
		wantPII     bool
		wantIdemp   bool
		wantResCost int
	}{
		{
			toolName: "search_points", profile: NewSearchPointsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: true, wantIdemp: true, wantResCost: 2,
		},
		{
			toolName: "upsert_point", profile: NewUpsertPointTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskMed, wantImpact: framework.ImpactWrite, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "delete_points", profile: NewDeletePointsTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskHigh, wantImpact: framework.ImpactDelete, wantPII: true, wantIdemp: true,
		},
		{
			toolName: "collection_info", profile: NewCollectionInfoTool(nil, rwCfg).GetEnforcerProfile(),
			wantRisk: framework.RiskLow, wantImpact: framework.ImpactRead, wantPII: false, wantIdemp: true,
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
	mc := &mockClient{}
	ep := &mockEmbedProvider{result: []float64{0.1}}

	mutatingTools := []struct {
		name   string
		handle func(args map[string]interface{}) (framework.ToolResult, error)
	}{
		{"upsert_point", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewUpsertPointTool(mc, roCfg).Handle(ctx, args)
		}},
		{"delete_points", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewDeletePointsTool(mc, roCfg).Handle(ctx, args)
		}},
		{"remember", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewRememberTool(mc, roCfg, ep, 0.95).Handle(ctx, args)
		}},
		{"forget", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewForgetTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"log_event", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewLogEventTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"learn_procedure", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewLearnProcedureTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"update_procedure", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewUpdateProcedureTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"save_progress", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewSaveProgressTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"abandon_task", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewAbandonTaskTool(mc, roCfg).Handle(ctx, args)
		}},
		{"store_result", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewStoreResultTool(mc, roCfg, ep).Handle(ctx, args)
		}},
		{"invalidate_result", func(args map[string]interface{}) (framework.ToolResult, error) {
			return NewInvalidateResultTool(mc, roCfg).Handle(ctx, args)
		}},
	}

	for _, tt := range mutatingTools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.handle(map[string]interface{}{
				"id":          "test-id",
				"key":         "test-key",
				"result":      "v",
				"content":     "hello",
				"event":       "happened",
				"name":        "proc",
				"description": "desc",
				"steps":       []interface{}{"s1"},
				"title":       "task-title",
				"task_id":     "some-uuid",
			})
			require.Error(t, err)
			assert.True(t,
				strings.Contains(err.Error(), "readonly") || strings.Contains(err.Error(), "not permitted"),
				"expected readonly error, got: %v", err)
		})
	}
}

// ---------------------------------------------------------------------------
// CRUD tool handle tests
// ---------------------------------------------------------------------------

func TestDeletePointsRequiresIdsOrFilter(t *testing.T) {
	tool := NewDeletePointsTool(&mockClient{}, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filter")
}

func TestUpsertPointHandle_Success(t *testing.T) {
	mc := &mockClient{}
	tool := NewUpsertPointTool(mc, rwCfg)
	result, err := tool.Handle(context.Background(), map[string]interface{}{
		"id":      "abc-123",
		"vector":  []interface{}{0.1, 0.2, 0.3},
		"payload": map[string]interface{}{"text": "hello"},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "abc-123")
}

func TestUpsertPointHandle_ClientError(t *testing.T) {
	mc := &mockClient{upsertErr: errors.New("upsert failed")}
	tool := NewUpsertPointTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{"id": "x"})
	require.Error(t, err)
}

func TestSearchPointsHandle_Success(t *testing.T) {
	mc := &mockClient{
		searchRes: []client.SearchResult{
			{ID: "p1", Score: 0.9, Payload: map[string]interface{}{"text": "hi"}},
		},
	}
	tool := NewSearchPointsTool(mc, rwCfg)
	result, err := tool.Handle(context.Background(), map[string]interface{}{
		"query_vector": []interface{}{0.1, 0.2},
		"limit":        float64(3),
	})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "p1")
}

func TestCollectionInfoHandle_Success(t *testing.T) {
	mc := &mockClient{
		collectionInfoRes: map[string]interface{}{
			"collection":   "test_collection",
			"points_count": int64(42),
		},
	}
	tool := NewCollectionInfoTool(mc, rwCfg)
	result, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "test_collection")
}

func TestCollectionInfoHandle_ClientError(t *testing.T) {
	mc := &mockClient{collectionInfoErr: errors.New("collection not found")}
	tool := NewCollectionInfoTool(mc, rwCfg)
	_, err := tool.Handle(context.Background(), map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection_info")
}
