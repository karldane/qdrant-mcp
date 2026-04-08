// Package testutil provides test doubles for use across the tools package
// and integration tests.  It lives in its own package so that mock types can
// be imported without importing production dependencies that require a live
// server.
package testutil

import (
	"context"

	"github.com/karldane/qdrant-mcp/internal/client"
)

// ---------------------------------------------------------------------------
// MockQdrantClient — satisfies tools.QdrantClient
// ---------------------------------------------------------------------------

// MockQdrantClient is a controllable implementation of the QdrantClient
// interface.  Set the exported fields to pre-program responses.
type MockQdrantClient struct {
	UpsertErr  error
	SearchRes  []client.SearchResult
	SearchErr  error
	ScrollRes  []client.ScrollResult
	ScrollNext string
	ScrollErr  error
	GetRes     *client.GetResult
	GetErr     error
	DeleteErr  error

	// CollectionInfoResult is returned by CollectionInfo.
	CollectionInfoResult map[string]interface{}
	CollectionInfoErr    error
}

func (m *MockQdrantClient) UpsertPoint(_ context.Context, _ string, _ []float64, _ map[string]interface{}) error {
	return m.UpsertErr
}

func (m *MockQdrantClient) Search(_ context.Context, _ []float64, _ int, _ map[string]interface{}) ([]client.SearchResult, error) {
	return m.SearchRes, m.SearchErr
}

func (m *MockQdrantClient) Scroll(_ context.Context, _ int, _ map[string]interface{}, _ string) ([]client.ScrollResult, string, error) {
	return m.ScrollRes, m.ScrollNext, m.ScrollErr
}

func (m *MockQdrantClient) GetPoint(_ context.Context, _ string) (*client.GetResult, error) {
	return m.GetRes, m.GetErr
}

func (m *MockQdrantClient) DeletePoints(_ context.Context, _ []string, _ map[string]interface{}) error {
	return m.DeleteErr
}

func (m *MockQdrantClient) CollectionInfo(_ context.Context) (map[string]interface{}, error) {
	return m.CollectionInfoResult, m.CollectionInfoErr
}

// ---------------------------------------------------------------------------
// MockEmbedProvider — satisfies embed.Provider
// ---------------------------------------------------------------------------

// MockEmbedProvider is a controllable embed.Provider.
type MockEmbedProvider struct {
	// EmbedResult is returned by Embed when EmbedErr is nil.
	EmbedResult []float64
	// EmbedErr is returned by Embed when non-nil.
	EmbedErr error
	// Size is returned by VectorSize.
	Size int
	// EmbedCalled is incremented on each Embed call.
	EmbedCalled int
}

func (m *MockEmbedProvider) Embed(_ context.Context, _ string) ([]float64, error) {
	m.EmbedCalled++
	return m.EmbedResult, m.EmbedErr
}

func (m *MockEmbedProvider) VectorSize() int {
	if m.Size == 0 {
		return 768
	}
	return m.Size
}
