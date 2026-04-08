package main

import (
	"context"
	"fmt"
	"os"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/karldane/qdrant-mcp/internal/config"
	"github.com/karldane/qdrant-mcp/internal/embed"
	"github.com/karldane/qdrant-mcp/internal/qdrant"
	"github.com/karldane/qdrant-mcp/internal/tools"

	"github.com/karldane/mcp-framework/framework"
)

type Server struct {
	*framework.Server
}

func NewServer() (*Server, error) {
	// Step 1: Load config (env vars + CLI flags).
	cfg := config.Load()
	cfg = config.MergeCLIFlags(cfg)

	// Step 2: Validate required fields.
	if cfg.AdminURL == "" {
		return nil, fmt.Errorf("QDRANT_ADMIN_URL is required")
	}
	if cfg.AdminKey == "" {
		return nil, fmt.Errorf("QDRANT_ADMIN_KEY is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("QDRANT_USERNAME is required")
	}
	if cfg.Collection == "" {
		return nil, fmt.Errorf("QDRANT_COLLECTION is required")
	}

	ctx := context.Background()

	// Step 3: Connect with admin key.
	adminClient, err := qdrant.NewAdminClient(cfg.AdminURL, cfg.AdminKey)
	if err != nil {
		return nil, fmt.Errorf("create admin client: %w", err)
	}

	// Step 4: Provision collection + indexes (hard-fails on vector size mismatch).
	if err := qdrant.EnsureCollection(ctx, adminClient, cfg.Collection, cfg.VectorSize); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}
	if err := qdrant.EnsureIndexes(ctx, adminClient, cfg.Collection); err != nil {
		return nil, fmt.Errorf("ensure indexes: %w", err)
	}
	// Admin client is no longer needed — discard it.
	_ = adminClient

	// Step 5: Generate a scoped JWT (1-hour expiry) signed with the admin key.
	jwt, err := qdrant.GenerateUserJWT(cfg.AdminKey, cfg.Username, cfg.Collection)
	if err != nil {
		return nil, fmt.Errorf("generate user JWT: %w", err)
	}

	// Step 6: Build the user client authenticated with the JWT.
	userQdrantClient, err := qdrant.NewUserClient(cfg.AdminURL, jwt)
	if err != nil {
		return nil, fmt.Errorf("create user client: %w", err)
	}

	// Step 7: Ping the user client to verify the JWT is accepted.
	if err := qdrant.PingUserClient(ctx, userQdrantClient, cfg.Collection); err != nil {
		return nil, fmt.Errorf("user client ping: %w", err)
	}

	// Build the tool-facing client wrapper (wraps the JWT-authenticated qdrant client).
	c, err := client.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create tool client: %w", err)
	}

	// Step 8: Initialise embedding provider.
	embedProvider, err := embed.NewProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("init embed provider: %w", err)
	}
	if p, ok := embedProvider.(embed.Pinger); ok {
		if err := p.Ping(ctx); err != nil {
			// Non-fatal: warn but continue — tools can still be used with pre-computed vectors.
			fmt.Fprintf(os.Stderr, "WARNING: embedding provider ping failed: %v\n", err)
		}
	}

	// Step 9: Register tools and start serving.
	s := &Server{
		Server: framework.NewServerWithConfig(&framework.Config{
			Name:         "qdrant-mcp",
			Version:      "0.2.0",
			WriteEnabled: !cfg.ReadOnly(),
			Instructions: `Qdrant MCP Server

A vector database server providing semantic search and storage capabilities.

Available tools:
- Core CRUD:    upsert_point, search_points, scroll_points, get_point, delete_points
- Agent Memory: upsert_memory, search_memory, delete_memory
- Sessions:     save_session, load_session, list_sessions, delete_session
- Cache:        upsert_cache, get_cache, invalidate_cache
- Admin:        collection_info

All operations use the user's private collection for data isolation.
Embedding is automatic when EMBEDDING_PROVIDER is configured.`,
		}),
	}

	s.registerTools(c, cfg, embedProvider)
	return s, nil
}

func (s *Server) registerTools(c *client.Client, cfg *config.Config, ep embed.Provider) {
	// Core CRUD
	s.Server.RegisterTool(tools.NewUpsertPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewSearchPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewScrollPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewGetPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewDeletePointsTool(c, cfg))

	// Agent Memory (with embed provider)
	s.Server.RegisterTool(tools.NewUpsertMemoryTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewSearchMemoryTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewDeleteMemoryTool(c, cfg))

	// Sessions
	s.Server.RegisterTool(tools.NewListSessionsTool(c, cfg))
	s.Server.RegisterTool(tools.NewLoadSessionTool(c, cfg))
	s.Server.RegisterTool(tools.NewSaveSessionTool(c, cfg))
	s.Server.RegisterTool(tools.NewDeleteSessionTool(c, cfg))

	// Cache
	s.Server.RegisterTool(tools.NewUpsertCacheTool(c, cfg))
	s.Server.RegisterTool(tools.NewGetCacheTool(c, cfg))
	s.Server.RegisterTool(tools.NewInvalidateCacheTool(c, cfg))

	// Admin / diagnostics
	s.Server.RegisterTool(tools.NewCollectionInfoTool(c, cfg))
}

func (s *Server) Start() error {
	return s.Server.Start()
}

func main() {
	server, err := NewServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start qdrant-mcp: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "qdrant-mcp v0.2.0 initialized (JWT RBAC)")
	fmt.Fprintln(os.Stderr, "Ready to serve requests via stdio...")

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
