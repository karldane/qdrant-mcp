package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/karldane/qdrant-mcp/internal/normalize"
	"github.com/karldane/qdrant-mcp/internal/readonly"

	"github.com/karldane/mcp-framework/framework"
	"github.com/mark3labs/mcp-go/mcp"
)

type UpsertCacheTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewUpsertCacheTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *UpsertCacheTool {
	return &UpsertCacheTool{client: c, cfg: cfg}
}

func (t *UpsertCacheTool) Name() string { return "upsert_cache" }

func (t *UpsertCacheTool) Description() string {
	return "Cache an expensive result by input hash. Use for avoiding repeated expensive operations."
}

func (t *UpsertCacheTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Cache key (typically hash of input)",
			},
			"value": map[string]interface{}{
				"type":        "object",
				"description": "Value to cache",
			},
			"ttl_seconds": map[string]interface{}{
				"type":        "number",
				"description": "Time-to-live in seconds",
				"default":     3600,
			},
		},
		Required: []string{"key", "value"},
	}
}

func (t *UpsertCacheTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	key, _ := args["key"].(string)
	value, _ := args["value"].(map[string]interface{})
	if value == nil {
		value = make(map[string]interface{})
	}

	ttl := 3600
	if t, ok := args["ttl_seconds"].(float64); ok && t > 0 {
		ttl = int(t)
	}

	value["type"] = "cache"
	value["key"] = key
	value["created_at"] = time.Now().Format(time.RFC3339)

	expires := time.Now().Add(time.Duration(ttl) * time.Second)
	value["expires_at"] = expires.Format(time.RFC3339)

	hash := sha256.Sum256([]byte(key))
	id := fmt.Sprintf("cache_%s", hex.EncodeToString(hash[:]))

	if err := t.client.UpsertPoint(ctx, id, nil, value); err != nil {
		return "", fmt.Errorf("upsert cache: %w", err)
	}

	return fmt.Sprintf(`{"success": true, "key": "%s"}`, key), nil
}

func (t *UpsertCacheTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

type GetCacheTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewGetCacheTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *GetCacheTool {
	return &GetCacheTool{client: c, cfg: cfg}
}

func (t *GetCacheTool) Name() string { return "get_cache" }

func (t *GetCacheTool) Description() string {
	return "Retrieve cached result by key, with TTL check."
}

func (t *GetCacheTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Cache key to retrieve",
			},
		},
		Required: []string{"key"},
	}
}

func (t *GetCacheTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	key, _ := args["key"].(string)

	hash := sha256.Sum256([]byte(key))
	id := fmt.Sprintf("cache_%s", hex.EncodeToString(hash[:]))

	result, err := t.client.GetPoint(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get cache: %w", err)
	}

	if expires, ok := result.Payload["expires_at"].(string); ok {
		if expTime, err := time.Parse(time.RFC3339, expires); err == nil {
			if time.Now().After(expTime) {
				return "", fmt.Errorf("cache entry expired")
			}
		}
	}

	value := make(map[string]interface{})
	for k, v := range result.Payload {
		if k != "type" && k != "key" && k != "created_at" && k != "expires_at" {
			value[k] = v
		}
	}

	c := &normalize.CacheEntry{
		Key:       key,
		Value:     value,
		CreatedAt: time.Now(),
	}

	b, _ := json.Marshal(c)
	return string(b), nil
}

func (t *GetCacheTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskLow),
		framework.WithImpact(framework.ImpactRead),
		framework.WithPII(false),
		framework.WithIdempotent(true),
	)
}

// ---------------------------------------------------------------------------
// InvalidateCacheTool
// ---------------------------------------------------------------------------

type InvalidateCacheTool struct {
	client QdrantClient
	cfg    readonly.ReadOnlyChecker
}

func NewInvalidateCacheTool(c QdrantClient, cfg readonly.ReadOnlyChecker) *InvalidateCacheTool {
	return &InvalidateCacheTool{client: c, cfg: cfg}
}

func (t *InvalidateCacheTool) Name() string { return "invalidate_cache" }

func (t *InvalidateCacheTool) Description() string {
	return "Clear stale cache entries by key prefix or all."
}

func (t *InvalidateCacheTool) Schema() mcp.ToolInputSchema {
	return mcp.ToolInputSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"prefix": map[string]interface{}{
				"type":        "string",
				"description": "Key prefix to invalidate (optional, clears all if empty)",
			},
		},
	}
}

func (t *InvalidateCacheTool) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
	if err := readonly.EnforceWrite(t.cfg); err != nil {
		return "", err
	}

	prefix, _ := args["prefix"].(string)

	var filter map[string]interface{}
	if prefix != "" {
		filter = map[string]interface{}{"key": prefix}
	} else {
		filter = map[string]interface{}{"type": "cache"}
	}

	if err := t.client.DeletePoints(ctx, nil, filter); err != nil {
		return "", fmt.Errorf("invalidate cache: %w", err)
	}

	return `{"success": true}`, nil
}

func (t *InvalidateCacheTool) GetEnforcerProfile() *framework.EnforcerProfile {
	return framework.NewEnforcerProfile(
		framework.WithRisk(framework.RiskMed),
		framework.WithImpact(framework.ImpactWrite),
		framework.WithPII(true),
		framework.WithIdempotent(false),
	)
}
