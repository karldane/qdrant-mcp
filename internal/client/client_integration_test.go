package client

import (
	"context"
	"os"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a live Qdrant server.
// Set QDRANT_TEST_URL to enable, e.g.:
//
//	QDRANT_TEST_URL=http://localhost:6334 go test ./internal/client -v -run Integration
func integrationClient(t *testing.T) *Client {
	t.Helper()
	url := os.Getenv("QDRANT_TEST_URL")
	if url == "" {
		t.Skip("QDRANT_TEST_URL not set; skipping integration tests")
	}
	cfg := &config.Config{
		AdminURL:   url,
		Collection: "qdrant_mcp_test",
		VectorSize: 4,
	}
	c, err := New(cfg)
	require.NoError(t, err)
	return c
}

func TestIntegration_EnsureCollection(t *testing.T) {
	c := integrationClient(t)
	err := c.EnsureCollection(context.Background())
	require.NoError(t, err)
	// Idempotent: second call should also succeed.
	err = c.EnsureCollection(context.Background())
	require.NoError(t, err)
}

func TestIntegration_UpsertAndGetPoint(t *testing.T) {
	c := integrationClient(t)
	require.NoError(t, c.EnsureCollection(context.Background()))

	ctx := context.Background()
	id := "integ-test-point-1"
	vec := []float64{0.1, 0.2, 0.3, 0.4}
	payload := map[string]interface{}{"text": "integration test", "type": "test"}

	err := c.UpsertPoint(ctx, id, vec, payload)
	require.NoError(t, err)

	result, err := c.GetPoint(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, result.ID)
	assert.Equal(t, "integration test", result.Payload["text"])
}

func TestIntegration_Search(t *testing.T) {
	c := integrationClient(t)
	require.NoError(t, c.EnsureCollection(context.Background()))

	ctx := context.Background()
	// Upsert a point first.
	_ = c.UpsertPoint(ctx, "integ-search-1", []float64{1.0, 0.0, 0.0, 0.0},
		map[string]interface{}{"type": "test"})

	results, err := c.Search(ctx, []float64{1.0, 0.0, 0.0, 0.0}, 5,
		map[string]interface{}{"type": "test"})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestIntegration_Scroll(t *testing.T) {
	c := integrationClient(t)
	require.NoError(t, c.EnsureCollection(context.Background()))

	ctx := context.Background()
	results, _, err := c.Scroll(ctx, 10, map[string]interface{}{"type": "test"}, "")
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestIntegration_DeletePoints_ByIDs(t *testing.T) {
	c := integrationClient(t)
	require.NoError(t, c.EnsureCollection(context.Background()))

	ctx := context.Background()
	id := "integ-delete-1"
	_ = c.UpsertPoint(ctx, id, []float64{0.5, 0.5, 0.5, 0.5}, nil)

	err := c.DeletePoints(ctx, []string{id}, nil)
	require.NoError(t, err)
}

func TestIntegration_DeletePoints_ByFilter(t *testing.T) {
	c := integrationClient(t)
	require.NoError(t, c.EnsureCollection(context.Background()))

	ctx := context.Background()
	err := c.DeletePoints(ctx, nil, map[string]interface{}{"type": "test"})
	require.NoError(t, err)
}
