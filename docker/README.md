# Local Qdrant + Ollama for MCP Testing

## Stack

| Service | Purpose | Port |
|---|---|---|
| Qdrant | Vector database with JWT RBAC | `localhost:6333` (REST), `localhost:6334` (gRPC) |
| Ollama | Local embedding model server | `localhost:11434` |
| ollama-model-pull | One-shot model download sidecar | — |

## Quick Start

```bash
cp .env.example .env
# Edit .env: paste two openssl rand -hex 32 values
docker compose up -d
docker compose logs -f ollama-model-pull   # watch the ~274MB model pull
```

## Generate API Keys

```bash
openssl rand -hex 32   # run twice — one per key
```

## What QDRANT__SERVICE__JWT_RBAC=true does

Enables collection-scoped JWT access control. The qdrant-mcp backend uses
QDRANT_API_KEY as both the admin key and the JWT signing secret. It generates
a short-lived (1h) JWT per process spawn that restricts the user to their own
collection only. Without this flag, collection-level JWT restrictions are not
enforced.

## Bridge Backend Config

```json
{
  "QDRANT_ADMIN_URL":   "http://localhost:6333",
  "QDRANT_ADMIN_KEY":   "<QDRANT_API_KEY value>",
  "QDRANT_OLLAMA_URL":  "http://localhost:11434",
  "QDRANT_VECTOR_SIZE": "768",
  "QDRANT_USERNAME":    "{{users.email}}",
  "QDRANT_COLLECTION":  "{{users.email|sanitised}}"
}
```

Note: QDRANT_USER_SECRET is no longer used.

## Test the Services

```bash
# Qdrant health (no auth needed)
curl http://localhost:6333/healthz

# List collections (admin key required)
curl http://localhost:6333/collections \
  -H "api-key: $(grep ^QDRANT_API_KEY .env | cut -d= -f2)"

# Ollama embedding (should return 768 floats)
curl http://localhost:11434/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model": "nomic-embed-text", "prompt": "hello world"}'
```

## Useful Commands

```bash
docker compose up -d          # start
docker compose down           # stop (data persists)
docker compose down -v        # wipe all data and models
docker compose logs -f        # all logs
```

## Notes

- QDRANT_VECTOR_SIZE must be 768 to match nomic-embed-text.
  Changing models requires wiping Qdrant storage: docker compose down -v
- Both ports are bound to 127.0.0.1 only.
- The Ollama image lacks curl so its healthcheck uses kill -0 1.
