package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// CollectionInfoTool
// ---------------------------------------------------------------------------

type CollectionInfoTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewCollectionInfoTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *CollectionInfoTool {
	return &CollectionInfoTool{client: c, cfg: cfg}
}

func (t *CollectionInfoTool) Name() string { return "collection_info" }

func (t *CollectionInfoTool) Description() string {
	return "Return collection diagnostics: vector count, vector size, index status."
}

func (t *CollectionInfoTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type:       "object",
		Properties: map[string]interface{}{},
	}
}

func (t *CollectionInfoTool) Handle(ctx context.Context, _ map[string]interface{}) (string, error) {
	info, err := t.client.CollectionInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("collection_info: %w", err)
	}

	b, _ := json.Marshal(info)
	return string(b), nil
}

func (t *CollectionInfoTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}
