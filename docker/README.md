# Local Qdrant for MCP Testing

## Quick Start

```bash
cp .env.example .env
# Edit .env and set your API keys (see below)
docker compose up -d
docker compose ps   # confirm healthy
```

## Generate API Keys

```bash
openssl rand -hex 32   # run twice — once for each key
```

Paste the results into `.env`.

## Connection Details

| | Value |
|---|---|
| REST API | `http://localhost:6333` |
| gRPC API | `localhost:6334` |
| Web UI | `http://localhost:6333/dashboard` |
| Admin API key | Value of `QDRANT_API_KEY` in `.env` |
| Read-only key | Value of `QDRANT_READ_ONLY_API_KEY` in `.env` |

## Test the API

```bash
# Health check (no auth needed)
curl http://localhost:6333/healthz

# List collections (admin key required)
curl http://localhost:6333/collections \
  -H "api-key: $(grep QDRANT_API_KEY .env | cut -d= -f2)"
```

## Bridge Backend Config

Set these as system env vars on your qdrant backend in the bridge admin:

```json
{
  "QDRANT_ADMIN_URL":    "http://qdrant.internal:6333",
  "QDRANT_ADMIN_KEY":    "<value of QDRANT_API_KEY>",
  "QDRANT_USER_SECRET":  "<openssl rand -hex 32>",
  "QDRANT_HOST":         "qdrant.internal",
  "QDRANT_USERNAME":     "{{users.email}}",
  "QDRANT_COLLECTION":   "{{users.email|sanitised}}"
}
```

For local testing, use `localhost` in place of `qdrant.internal`.

## Useful Commands

```bash
docker compose up -d        # start
docker compose down         # stop (data persists)
docker compose down -v      # stop AND wipe all data
docker compose logs -f      # tail logs
```

## Notes

- Both ports are bound to `127.0.0.1` only — not reachable from other hosts.
- Data lives in the `qdrant_storage` named volume; survives restarts.
- `docker compose down -v` wipes all collections and points — use to reset.
- The web UI at `/dashboard` is useful for inspecting collections and running
  test queries visually.
- API key auth uses the `api-key` HTTP header (not `Authorization: Bearer`).
