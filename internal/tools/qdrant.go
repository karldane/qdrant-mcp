package tools

import (
	"context"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/karldane/qdrant-mcp/internal/config"
)

// QdrantClient is the interface all tools use to interact with Qdrant.
// *client.Client satisfies this interface.
type QdrantClient interface {
	UpsertPoint(ctx context.Context, id string, vector []float64, payload map[string]interface{}) error
	// UpsertPayload stores a point with payload only (no embedding vector).
	// Qdrant still requires a vector in the collection, so the implementation
	// uses a zero-valued placeholder vector. Use this for sessions and other
	// payload-only records where semantic search is not required.
	UpsertPayload(ctx context.Context, id string, payload map[string]interface{}) error
	// SetPayload merges the given fields into an existing point's payload.
	// Used for in-place updates (e.g. dedup update, abandon_task status change).
	SetPayload(ctx context.Context, id string, payload map[string]interface{}) error
	Search(ctx context.Context, query []float64, limit int, filter map[string]interface{}) ([]client.SearchResult, error)
	Scroll(ctx context.Context, limit int, filter map[string]interface{}, offset string) ([]client.ScrollResult, string, error)
	GetPoint(ctx context.Context, id string) (*client.GetResult, error)
	DeletePoints(ctx context.Context, ids []string, filter map[string]interface{}) error
	// Count returns the number of points matching the given filter (nil = all).
	Count(ctx context.Context, filter map[string]interface{}) (int64, error)
	CollectionInfo(ctx context.Context) (map[string]interface{}, error)
}

// NewQdrantClient creates and returns a *client.Client, ensuring the
// configured collection exists before returning.
func NewQdrantClient(cfg *config.Config) (*client.Client, error) {
	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}

	if err := c.EnsureCollection(context.Background()); err != nil {
		return nil, err
	}

	return c, nil
}
