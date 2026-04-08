
# Qdrant MCP Backend — Specification

> Replaces the MongoDB backend with Qdrant, keeping the same provisioning model and MCP Framework conventions. [SPEC_MCP_BACKEND.md][SPEC_MONGODB_BACKEND.md]
> Maps MongoDB concepts to Qdrant: collection = database, point = document, payload = document fields.

## Overview

Qdrant is a self-hosted vector database with excellent support for agent memory use cases:
- **Vector search** — semantic similarity for lean retrieval
- **Payload filtering** — exact metadata matching (user_id, TTL, tags)
- **Full-text search** — keyword precision
- **Docker compose** — single container, tiny footprint

This backend provides the same isolation model as MongoDB — each agent gets their own **collection** (Qdrant equivalent of database) provisioned on first connection.

## Why Qdrant over MongoDB

| | MongoDB | Qdrant |
|---|---|---|
| Vector search | Manual or Atlas-only | Native ANN with hybrid (vector+keyword) |
| Filtering | Aggregation pipeline | Indexed payload filters, sub-ms latency |
| Docker size | 2GB+ | ~200MB |
| Agent fit | General-purpose | Memory/cache optimised |

## Security Model

Same as MongoDB backend:
- Qdrant instance on private network, no external access
- Bridge injects `QDRANT_ADMIN_URL` (system env) for provisioning
- `QDRANT_USER_SECRET` derives per-user API key
- `{{users.email|sanitised}}` becomes the **collection name**

## Environment Variables

| Variable | Source | Purpose |
|---|---|---|
| `QDRANT_ADMIN_URL` | System env | `http://qdrant.internal:6333` with admin API key |
| `QDRANT_USER_SECRET` | System env | Derives per-user API key |
| `QDRANT_HOST` | System env | Qdrant host |
| `QDRANT_USERNAME` | Bridge template `{{users.email}}` | User identifier (for payloads) |
| `QDRANT_COLLECTION` | Bridge template `{{users.email|sanitised}}` | Per-user collection name |

## Provisioning Flow (First Connection)

1. Connect using `QDRANT_ADMIN_URL`
2. Check if collection `{{users.email|sanitised}}` exists
3. If not, create collection with:
   - Vector size: 1536 (OpenAI embeddings)
   - Distance: Cosine
   - Payload schema: optional indexed fields
4. Close admin connection
5. Reconnect using derived user API key

**Derived API key:** `hash(QDRANT_USERNAME + QDRANT_USER_SECRET)`

## MCP Tools

### Core CRUD (MongoDB-like)

| Tool | Description | Risk | Impact |
|---|---|---|---|
| `upsert_point` | Store vector + payload | Med | Write |
| `search_points` | Vector/hybrid search | Low | Read |
| `scroll_points` | Paginated list/filter | Low | Read |
| `get_point` | Retrieve by ID | Low | Read |
| `delete_points` | Delete by ID or filter | High | Delete |

### Agent Memory

| Tool | Description | Risk | Impact |
|---|---|---|---|
| `upsert_memory` | Store fact/session with TTL/tags | Med | Write |
| `search_memory` | Semantic search recent/related facts | Low | Read |
| `list_sessions` | List active sessions | Low | Read |
| `load_session` | Resume session state | Low | Read |
| `save_session` | Persist current session | Med | Write |
| `invalidate_cache` | Clear stale cache entries | Med | Write |

### Cache

| Tool | Description | Risk | Impact |
|---|---|---|---|
| `upsert_cache` | Cache expensive result by input hash | Med | Write |
| `get_cache` | Retrieve with TTL check | Low | Read |

## Tool Schemas (Examples)

### `upsert_memory`
```json
{
  "type": "object",
  "properties": {
    "content": {"type": "string", "description": "Text content to store"},
    "embedding": {"type": "array", "items": {"type": "number"}},
    "metadata": {"type": "object"}
  },
  "required": ["content"]
}
```

### `search_memory`
```json
{
  "type": "object",
  "properties": {
    "query": {"type": "string"},
    "query_embedding": {"type": "array", "items": {"type": "number"}},
    "limit": {"type": "integer", "default": 5},
    "filter": {"type": "object"}
  },
  "required": ["query"]
}
```

## EnforcerProfile Examples

```go
// upsert_memory — agent state, medium risk
framework.WithRisk(framework.RiskMed),
framework.WithImpact(framework.ImpactWrite),
framework.WithPII(true),  // user data
framework.WithIdempotent(false),

// search_memory — read-only, low risk
framework.WithRisk(framework.RiskLow),
framework.WithImpact(framework.ImpactRead),
framework.WithResourceCost(2),  // fast vector search
```

## Qdrant Client

Uses official Go client: `github.com/qdrant/go-client/qdrant` [web:249]

```go
client, err := qdrant.NewClient("http://qdrant.internal:6333")
collectionName := cfg.Collection  // "{{users.email|sanitised}}"

// Create collection (provisioning)
createCollection := &qdrant.CreateCollection{
    Vectors: &qdrant.VectorsConfig{
        Size:     1536,  // OpenAI embedding size
        Distance: qdrant.DistanceCosine,
    },
}
client.CreateCollection(ctx, collectionName, createCollection)
```

## Provisioning Differences from MongoDB

| | MongoDB | Qdrant |
|---|---|---|
| Admin auth | Username/password | API key in header |
| "Database" | Create database | Create collection |
| User auth | Create user | Derived API key (no users in Qdrant) |
| Init data | Insert init doc | Optional — Qdrant collections are empty by default |

## Docker Compose (Local Testing)

```yaml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
    volumes:
      - qdrant_storage:/qdrant/storage
volumes:
  qdrant_storage:
```

**Test connection:**
```bash
curl http://localhost:6333/collections
```

## Backend Config (Bridge Admin)

```json
{
  "QDRANT_ADMIN_URL": "http://qdrant.internal:6333",
  "QDRANT_ADMIN_KEY": "your-admin-api-key",
  "QDRANT_USER_SECRET": "openssl-rand-hex-32",
  "QDRANT_HOST": "qdrant.internal",
  "QDRANT_USERNAME": "{{users.email}}",
  "QDRANT_COLLECTION": "{{users.email|sanitised}}"
}
```

## Implementation Notes

- **No embeddings required** — tools accept either `query_embedding` or auto-generate from `query` text
- **Vector size configurable** — default 1536, override via env var
- **Payload indexing** — auto-index common fields (`user_id`, `type`, `timestamp`) for fast filtering
- **TTL support** — store expiry in payload, filter out expired in `search_memory`
- **Collection per user** — same isolation as MongoDB, provisioned on first connection

## Testing Strategy

- **Unit tests** — mock Qdrant client, test EnforcerProfile, schema validation
- **Integration** — Docker compose with real Qdrant, test provisioning + full tool flow
- **Edge cases** — collection already exists, admin API failure, derived key derivation

This backend feels identical to MongoDB from the agent's perspective while unlocking semantic search.
