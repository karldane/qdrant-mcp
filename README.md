# Qdrant MCP Server

A Go-native MCP (Model Context Protocol) server for Qdrant vector database, providing AI agents with semantic search, agent memory, and caching capabilities through a safety-annotated tool interface.

## Overview

This server connects to a Qdrant instance and exposes tools for working with vector data:

- **Core CRUD**: Store, search, scroll, retrieve, and delete vector points
- **Agent Memory**: Persist facts and observations across sessions with optional TTL and tags
- **Session Management**: Save and reload named agent sessions with arbitrary state
- **Cache**: Store and retrieve expensive computation results by key with TTL
- **Safety First**: All tools include `EnforcerProfile` metadata for automated policy enforcement by the MCP Bridge
- **Readonly Mode**: `--readonly` flag disables all mutating tools at runtime

## Tools

### Core CRUD

| Tool | Description | Risk | Impact |
|------|-------------|------|--------|
| `upsert_point` | Store a vector with payload data | Med | Write |
| `search_points` | Semantic similarity search | Low | Read |
| `scroll_points` | List all points with optional filtering and pagination | Low | Read |
| `get_point` | Retrieve a single point by ID | Low | Read |
| `delete_points` | Delete points by ID or filter | High | Delete |

### Agent Memory

| Tool | Description | Risk | Impact |
|------|-------------|------|--------|
| `upsert_memory` | Store a fact or observation with optional TTL and tags | Med | Write |
| `search_memory` | Search recent or related facts semantically | Low | Read |
| `list_sessions` | List active sessions stored in the collection | Low | Read |
| `load_session` | Load session state by ID | Low | Read |
| `save_session` | Persist current session state | Med | Write |
| `invalidate_cache` | Clear stale cache entries by key prefix or all | Med | Write |

### Cache

| Tool | Description | Risk | Impact |
|------|-------------|------|--------|
| `upsert_cache` | Cache an expensive result by input hash | Med | Write |
| `get_cache` | Retrieve cached result by key, with TTL check | Low | Read |

## Installation

### Prerequisites

- Go 1.25 or later
- Qdrant 1.x (accessible via gRPC on port 6334)

### Building from Source

```bash
git clone https://github.com/karldane/qdrant-mcp.git
cd qdrant-mcp
make
```

This downloads dependencies and builds a stripped binary.

#### Build Options

```bash
make              # Download deps and build (default)
make deps         # Download dependencies only
make build        # Build binary only (assumes deps exist)
make build-all    # Build for Linux, macOS, and Windows
make test         # Run unit tests
make clean        # Remove build artifacts
make install      # Install to GOPATH/bin
make help         # Show all options
```

### Install via go install

```bash
go install github.com/karldane/qdrant-mcp@latest
```

## Configuration

### Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `QDRANT_ADMIN_URL` | Qdrant server URL (e.g. `http://localhost:6334`) | Yes | - |
| `QDRANT_ADMIN_KEY` | Admin API key for collection management | No | - |
| `QDRANT_USER_SECRET` | Secret for deriving per-user API keys | No | - |
| `QDRANT_USERNAME` | Username for the current session | No | - |
| `QDRANT_COLLECTION` | Collection name to use | No | - |
| `QDRANT_VECTOR_SIZE` | Vector dimensions (must match your embedding model) | No | `1536` |
| `QDRANT_TIMEOUT_SECONDS` | Request timeout | No | `30` |

### CLI Flags

All environment variables have corresponding CLI flags:

```
--admin-url        Qdrant admin URL
--admin-key        Qdrant admin API key
--user-secret      Secret for deriving user API key
--username         User identifier
--collection       Collection name
--vector-size      Vector size for collection (default: 1536)
--timeout          HTTP timeout in seconds (default: 30)
--readonly         Disable all mutating tools
--log-json         Emit structured JSON logs
```

CLI flags override environment variables.

### Per-User API Key Derivation

When `QDRANT_USER_SECRET` and `QDRANT_USERNAME` are both set, the server derives a per-user API key as:

```
SHA-256(QDRANT_USERNAME + QDRANT_USER_SECRET)
```

This is deterministic and stateless — no key storage is required. The MCP Bridge injects the username per session.

## Usage

### Basic Usage

```bash
export QDRANT_ADMIN_URL="http://localhost:6334"
export QDRANT_ADMIN_KEY="your-admin-api-key"
export QDRANT_USERNAME="alice@example.com"
export QDRANT_USER_SECRET="your-secret"
export QDRANT_COLLECTION="alice_collection"
./qdrant-mcp
```

The server communicates over stdio using the MCP protocol and is intended to be launched by the MCP Bridge.

### MCP Bridge Configuration

```json
{
  "backend": "qdrant-mcp",
  "command": "/path/to/qdrant-mcp",
  "env": {
    "QDRANT_ADMIN_URL": "http://qdrant:6334",
    "QDRANT_ADMIN_KEY": "${QDRANT_ADMIN_KEY}",
    "QDRANT_USER_SECRET": "${QDRANT_USER_SECRET}"
  }
}
```

The bridge injects `QDRANT_USERNAME` and `QDRANT_COLLECTION` automatically per session.

### Readonly Mode

```bash
./qdrant-mcp --readonly
```

In readonly mode all mutating tools (`upsert_point`, `delete_points`, `upsert_memory`, `save_session`, `invalidate_cache`, `upsert_cache`) return a deterministic error. The tools still appear in `tools/list`.

## Safety Features

### EnforcerProfile Metadata

Every tool self-reports its safety characteristics:

```go
type EnforcerProfile struct {
    RiskLevel    RiskLevel   // low, med, high, critical
    ImpactScope  ImpactScope // read, write, delete, admin
    ResourceCost int         // 1-10 (operation weight)
    PIIExposure  bool        // Returns sensitive user data?
    Idempotent   bool        // Safe to retry on timeout?
    ApprovalReq  bool        // Requires human-in-the-loop?
}
```

### Risk Classification

- **Low Risk** (`search_points`, `scroll_points`, `get_point`, `search_memory`, `list_sessions`, `load_session`, `get_cache`): Read-only, idempotent
- **Medium Risk** (`upsert_point`, `upsert_memory`, `save_session`, `invalidate_cache`, `upsert_cache`): Write operations — reversible
- **High Risk** (`delete_points`): Irreversible deletion

## Architecture

```
MCP Bridge
    │
    ├─ spawns: qdrant-mcp (one process per user pool slot)
    │          env: QDRANT_ADMIN_URL, QDRANT_USER_SECRET, QDRANT_USERNAME, ...
    │
    └─ stdio MCP protocol
           │
           ▼
       main.go
         │
         ├─ tools.NewQdrantClient(cfg)
         │    ├─ client.New(cfg)     — constructs admin + user gRPC clients
         │    └─ EnsureCollection()  — creates collection if absent (idempotent)
         │
         └─ framework.Server (13 tools registered)
              ├─ internal/tools/crud.go    (5 tools)
              ├─ internal/tools/memory.go  (6 tools)
              └─ internal/tools/cache.go   (2 tools)
```

## Testing

Run the unit test suite (no live Qdrant required):

```bash
make test
```

Coverage targets:
- `internal/tools`: 80%+ (96% unit)
- `internal/config`: 60%+ (90% unit)
- `internal/normalize`: 100%
- `internal/readonly`: 100%

Integration tests against a live Qdrant instance:

```bash
QDRANT_TEST_URL=http://localhost:6334 go test ./internal/client -v -run Integration
```

## License

This project is licensed under the Functional Source License, Version 1.1, ALv2 Future License.

Copyright 2026 Karl Dane

See LICENSE file for full terms.

## References

- [MCP Framework](https://github.com/karldane/mcp-framework) — Base framework with EnforcerProfile support
- [Qdrant Go Client](https://github.com/qdrant/go-client) — Official Go client for Qdrant
- [Qdrant Documentation](https://qdrant.tech/documentation/) — Qdrant vector database docs
