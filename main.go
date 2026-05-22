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
			Version:      "0.3.0",
			WriteEnabled: !cfg.ReadOnly(),
			Instructions: `Qdrant MCP Server v0.3.0

A vector database server providing agent memory capabilities organised by cognitive memory type.

Available tools:
- Semantic Memory:  remember, recall, forget, reflect
- Episodic Memory:  log_event, recall_events, summarise_period
- Procedural Memory: learn_procedure, recall_procedure, update_procedure
- Working Memory:   save_progress, resume_task, list_tasks, abandon_task
- Cache:            store_result, lookup_result, invalidate_result
- Introspection:    what_do_i_know, memory_stats
- Core CRUD:        upsert_point, search_points, scroll_points, get_point, delete_points
- Admin:            collection_info

All operations use the user's private collection for data isolation.
Embedding is automatic when EMBEDDING_PROVIDER is configured.
Deduplication is applied on remember (threshold configurable via QDRANT_DEDUP_THRESHOLD).`,
		}),
	}

	s.registerTools(c, cfg, embedProvider)
	return s, nil
}

func (s *Server) registerTools(c *client.Client, cfg *config.Config, ep embed.Provider) {
	// Semantic Memory
	s.Server.RegisterTool(tools.NewRememberTool(c, cfg, ep, cfg.DedupThreshold))
	s.Server.RegisterTool(tools.NewRecallTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewForgetTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewReflectTool(c, cfg, ep))

	// Episodic Memory
	s.Server.RegisterTool(tools.NewLogEventTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewRecallEventsTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewSummarisePeriodTool(c, cfg, ep))

	// Procedural Memory
	s.Server.RegisterTool(tools.NewLearnProcedureTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewRecallProcedureTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewUpdateProcedureTool(c, cfg, ep))

	// Working Memory (Tasks)
	s.Server.RegisterTool(tools.NewSaveProgressTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewResumeTaskTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewListTasksTool(c, cfg))
	s.Server.RegisterTool(tools.NewAbandonTaskTool(c, cfg))

	// Cache
	s.Server.RegisterTool(tools.NewStoreResultTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewLookupResultTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewInvalidateResultTool(c, cfg))

	// Introspection
	s.Server.RegisterTool(tools.NewWhatDoIKnowTool(c, cfg, ep))
	s.Server.RegisterTool(tools.NewMemoryStatsTool(c, cfg))

	// Core CRUD (raw primitives)
	s.Server.RegisterTool(tools.NewUpsertPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewSearchPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewScrollPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewGetPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewDeletePointsTool(c, cfg))

	// Admin / diagnostics
	s.Server.RegisterTool(tools.NewCollectionInfoTool(c, cfg))
}

func (s *Server) Start() error {
	return s.Server.Start()
}

func main() {
	// Check for --scan flag BEFORE config validation to enable scan-mode
	scanMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--scan" || arg == "--scan-mode" {
			scanMode = true
			break
		}
	}

	// If scan mode, create minimal server without Qdrant connection
	if scanMode {
		runScanMode()
		return
	}

	server, err := NewServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start qdrant-mcp: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "qdrant-mcp v0.3.0 initialized (JWT RBAC)")
	fmt.Fprintln(os.Stderr, "Ready to serve requests via stdio...")

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// runScanMode creates a minimal server for scan-mode (no Qdrant connection needed)
func runScanMode() {
	dummyCfg := &config.Config{
		VectorSize:       768,
		DedupThreshold: 0.95,
	}

	dummyEmbed := &minimalEmbedProvider{}

	s := &Server{
		Server: framework.NewServerWithConfig(&framework.Config{
			Name:    "qdrant-mcp",
			Version: "0.3.0",
		}),
	}

	s.registerTools(nil, dummyCfg, dummyEmbed)

	s.Server.SetScanMode(true)
	if err := s.Server.RunScanMode(); err != nil {
		os.Exit(1)
	}
}

type minimalEmbedProvider struct{}

func (e *minimalEmbedProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	return nil, nil
}

func (e *minimalEmbedProvider) VectorSize() int {
	return 768
}
