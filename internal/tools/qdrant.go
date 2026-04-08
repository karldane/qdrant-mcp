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
	Search(ctx context.Context, query []float64, limit int, filter map[string]interface{}) ([]client.SearchResult, error)
	Scroll(ctx context.Context, limit int, filter map[string]interface{}, offset string) ([]client.ScrollResult, string, error)
	GetPoint(ctx context.Context, id string) (*client.GetResult, error)
	DeletePoints(ctx context.Context, ids []string, filter map[string]interface{}) error
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
