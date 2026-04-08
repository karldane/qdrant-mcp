package main

import (
	"fmt"
	"os"

	"github.com/karldane/qdrant-mcp/internal/client"
	"github.com/karldane/qdrant-mcp/internal/config"
	"github.com/karldane/qdrant-mcp/internal/tools"

	"github.com/karldane/mcp-framework/framework"
)

type Server struct {
	*framework.Server
}

func NewServer() (*Server, error) {
	cfg := config.Load()
	cfg = config.MergeCLIFlags(cfg)

	c, err := tools.NewQdrantClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	s := &Server{
		Server: framework.NewServerWithConfig(&framework.Config{
			Name:    "qdrant-mcp",
			Version: "0.1.0",
			Instructions: `Qdrant MCP Server

A vector database server providing semantic search and storage capabilities.

Available tools:
- Core CRUD: upsert_point, search_points, scroll_points, get_point, delete_points
- Agent Memory: upsert_memory, search_memory, list_sessions, load_session, save_session, invalidate_cache
- Cache: upsert_cache, get_cache

All operations use the user's private collection for data isolation.`,
		}),
	}

	s.registerTools(c, cfg)
	return s, nil
}

func (s *Server) registerTools(c *client.Client, cfg *config.Config) {
	s.Server.RegisterTool(tools.NewUpsertPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewSearchPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewScrollPointsTool(c, cfg))
	s.Server.RegisterTool(tools.NewGetPointTool(c, cfg))
	s.Server.RegisterTool(tools.NewDeletePointsTool(c, cfg))

	s.Server.RegisterTool(tools.NewUpsertMemoryTool(c, cfg))
	s.Server.RegisterTool(tools.NewSearchMemoryTool(c, cfg))
	s.Server.RegisterTool(tools.NewListSessionsTool(c, cfg))
	s.Server.RegisterTool(tools.NewLoadSessionTool(c, cfg))
	s.Server.RegisterTool(tools.NewSaveSessionTool(c, cfg))
	s.Server.RegisterTool(tools.NewInvalidateCacheTool(c, cfg))

	s.Server.RegisterTool(tools.NewUpsertCacheTool(c, cfg))
	s.Server.RegisterTool(tools.NewGetCacheTool(c, cfg))
}

func (s *Server) Start() error {
	return s.Server.Start()
}

func main() {
	server, err := NewServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Qdrant MCP Server initialized")
	fmt.Fprintln(os.Stderr, "Ready to serve requests via stdio...")

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
