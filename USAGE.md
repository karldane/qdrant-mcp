# qdrant-mcp Usage Guide

Practical reference for running, configuring, and calling the qdrant-mcp backend.

## Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration Reference](#configuration-reference)
- [Running the Server](#running-the-server)
- [Tool Reference](#tool-reference)
  - [Core CRUD](#core-crud)
  - [Agent Memory](#agent-memory)
  - [Cache](#cache)
- [MCP Bridge Integration](#mcp-bridge-integration)
- [Readonly Mode](#readonly-mode)
- [Testing](#testing)

---

## Prerequisites

| Requirement | Version |
|-------------|---------|
| Go          | 1.25+   |
| Qdrant      | 1.x (gRPC port 6334 must be reachable) |

---

## Quick Start

### 1. Start Qdrant

```bash
cd docker
cp .env.example .env
# Edit .env — set QDRANT_API_KEY and QDRANT_READ_ONLY_API_KEY
docker compose up -d
```

Qdrant will be available at:
- gRPC: `localhost:6334` (used by this server)
- REST + Web UI: `http://localhost:6333`

### 2. Build the server

```bash
# From repo root
make          # downloads deps + builds ./qdrant-mcp binary
```

Or install directly:

```bash
go install github.com/karldane/qdrant-mcp@latest
```

### 3. Run the server

```bash
export QDRANT_ADMIN_URL="http://localhost:6334"
export QDRANT_ADMIN_KEY="your-api-key"
export QDRANT_COLLECTION="my_collection"
./qdrant-mcp
```

The server communicates over **stdio** using the MCP protocol. It is designed to be spawned by the MCP Bridge, not run interactively.

---

## Configuration Reference

All options can be set via environment variable or CLI flag. CLI flags override environment variables.

| Environment Variable   | CLI Flag          | Default | Description |
|------------------------|-------------------|---------|-------------|
| `QDRANT_ADMIN_URL`     | `--admin-url`     | —       | Qdrant server URL, e.g. `http://localhost:6334` (**required**) |
| `QDRANT_ADMIN_KEY`     | `--admin-key`     | —       | Admin API key for collection management |
| `QDRANT_USER_SECRET`   | `--user-secret`   | —       | Secret for deriving per-user API keys |
| `QDRANT_USERNAME`      | `--username`      | —       | Username for the current session |
| `QDRANT_COLLECTION`    | `--collection`    | —       | Collection to use for all operations |
| `QDRANT_VECTOR_SIZE`   | `--vector-size`   | `1536`  | Vector dimensions — must match your embedding model |
| `QDRANT_TIMEOUT_SECONDS` | `--timeout`     | `30`    | Request timeout in seconds |
| —                      | `--readonly`      | off     | Disable all mutating tools |
| —                      | `--log-json`      | off     | Emit structured JSON logs to stderr |

### Per-user API key derivation

When both `QDRANT_USER_SECRET` and `QDRANT_USERNAME` are set, the server derives a per-user API key:

```
api_key = hex(SHA-256(username + user_secret))
```

This is stateless — no key storage required. The MCP Bridge injects `QDRANT_USERNAME` per session automatically.

### Vector size

The default is `1536`, matching OpenAI `text-embedding-3-small` and `text-embedding-ada-002`. Change this to match your model:

| Model | Dimensions |
|-------|-----------|
| OpenAI text-embedding-3-small | 1536 |
| OpenAI text-embedding-3-large | 3072 |
| OpenAI text-embedding-ada-002 | 1536 |
| Cohere embed-english-v3.0 | 1024 |
| Nomic embed-text-v1 | 768 |

---

## Running the Server

### Minimal (no auth)

```bash
QDRANT_ADMIN_URL=http://localhost:6334 \
QDRANT_COLLECTION=myapp \
./qdrant-mcp
```

### With API key and user isolation

```bash
QDRANT_ADMIN_URL=http://localhost:6334 \
QDRANT_ADMIN_KEY=my-admin-key \
QDRANT_USER_SECRET=my-secret \
QDRANT_USERNAME=alice@example.com \
QDRANT_COLLECTION=alice_collection \
./qdrant-mcp
```

### With CLI flags

```bash
./qdrant-mcp \
  --admin-url http://localhost:6334 \
  --admin-key my-admin-key \
  --collection myapp \
  --vector-size 768 \
  --log-json
```

### Readonly mode

```bash
./qdrant-mcp --admin-url http://localhost:6334 --collection myapp --readonly
```

---

## Tool Reference

All tools accept and return JSON. Required fields are marked **bold**.

---

### Core CRUD

#### `upsert_point`

Store a vector with payload data.

**Input:**

| Field     | Type            | Required | Description |
|-----------|-----------------|----------|-------------|
| `id`      | string          | yes      | Unique identifier for the point |
| `vector`  | array[number]   | no       | Vector embedding |
| `payload` | object          | no       | Metadata to store alongside the vector |

**Example:**

```json
{
  "id": "doc-001",
  "vector": [0.12, -0.34, 0.56],
  "payload": {
    "title": "Getting started with Go",
    "source": "blog"
  }
}
```

**Response:**

```json
{"success": true, "id": "doc-001"}
```

---

#### `search_points`

Search vectors by semantic similarity. Returns points ordered by relevance score descending.

**Input:**

| Field          | Type          | Required | Default | Description |
|----------------|---------------|----------|---------|-------------|
| `query_vector` | array[number] | yes      | —       | Query embedding |
| `limit`        | number        | no       | 5       | Maximum results |
| `filter`       | object        | no       | —       | Payload field filter, e.g. `{"source": "blog"}` |

**Example:**

```json
{
  "query_vector": [0.12, -0.34, 0.56],
  "limit": 3,
  "filter": {"source": "blog"}
}
```

**Response:**

```json
{
  "results": [
    {"id": "doc-001", "score": 0.97, "payload": {"title": "Getting started with Go"}},
    {"id": "doc-042", "score": 0.84, "payload": {"title": "Go concurrency patterns"}}
  ],
  "count": 2
}
```

---

#### `scroll_points`

List points with optional filtering and cursor-based pagination.

**Input:**

| Field    | Type   | Required | Default | Description |
|----------|--------|----------|---------|-------------|
| `limit`  | number | no       | 20      | Points per page |
| `filter` | object | no       | —       | Payload field filter |
| `offset` | string | no       | —       | Cursor from a previous response's `next_offset` |

**Example:**

```json
{"limit": 10, "filter": {"source": "blog"}}
```

**Response:**

```json
{
  "points": [
    {"id": "doc-001", "payload": {"title": "Getting started with Go"}},
    {"id": "doc-042", "payload": {"title": "Go concurrency patterns"}}
  ],
  "count": 2,
  "next_offset": "doc-042"
}
```

Pass `next_offset` as `offset` in the next call to page through results. A `null` or empty `next_offset` means no more pages.

---

#### `get_point`

Retrieve a single point by ID, including its vector.

**Input:**

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `id`  | string | yes      | Point ID |

**Example:**

```json
{"id": "doc-001"}
```

**Response:**

```json
{
  "id": "doc-001",
  "vector": [0.12, -0.34, 0.56],
  "payload": {"title": "Getting started with Go", "source": "blog"}
}
```

---

#### `delete_points`

Delete points by explicit IDs or by filter. Provide at least one of `ids` or `filter`.

**Input:**

| Field    | Type          | Required | Description |
|----------|---------------|----------|-------------|
| `ids`    | array[string] | no*      | Point IDs to delete |
| `filter` | object        | no*      | Delete all points matching this filter |

\* At least one of `ids` or `filter` must be provided.

**Example — delete by IDs:**

```json
{"ids": ["doc-001", "doc-042"]}
```

**Example — delete by filter:**

```json
{"filter": {"source": "blog"}}
```

**Response:**

```json
{"success": true}
```

---

### Agent Memory

#### `upsert_memory`

Store a fact or observation. Automatically sets `type=memory` and `created_at` in the payload.

**Input:**

| Field         | Type          | Required | Description |
|---------------|---------------|----------|-------------|
| `content`     | string        | yes      | Text to store |
| `embedding`   | array[number] | no       | Pre-computed vector for semantic search |
| `metadata`    | object        | no       | Extra metadata |
| `tags`        | array[string] | no       | Categorisation tags |
| `ttl_seconds` | number        | no       | Expire after N seconds |

**Example:**

```json
{
  "content": "User prefers concise responses with code examples",
  "embedding": [0.12, -0.34, 0.56],
  "tags": ["preference", "style"],
  "ttl_seconds": 86400
}
```

**Response:**

```json
{"success": true, "id": "mem_1712345678901234567"}
```

The generated ID is `mem_<unix-nano>`.

---

#### `search_memory`

Search stored memories by semantic similarity. Automatically filters to `type=memory`.

**Input:**

| Field             | Type          | Required | Default | Description |
|-------------------|---------------|----------|---------|-------------|
| `query`           | string        | yes      | —       | Search text (for context; vector search requires `query_embedding`) |
| `query_embedding` | array[number] | no       | —       | Pre-computed query vector |
| `limit`           | number        | no       | 5       | Maximum results |
| `filter`          | object        | no       | —       | Additional payload filters |

**Example:**

```json
{
  "query": "user response preferences",
  "query_embedding": [0.12, -0.34, 0.56],
  "limit": 3
}
```

**Response:**

```json
{
  "memories": [
    {
      "id": "mem_1712345678901234567",
      "content": "User prefers concise responses with code examples",
      "tags": ["preference", "style"],
      "score": 0.95,
      "metadata": {"created_at": "2026-04-08T10:00:00Z", "type": "memory"}
    }
  ],
  "count": 1
}
```

---

#### `list_sessions`

List all stored sessions. Filters to `type=session` automatically.

**Input:**

| Field   | Type   | Required | Default | Description |
|---------|--------|----------|---------|-------------|
| `limit` | number | no       | 20      | Maximum sessions to return |

**Example:**

```json
{"limit": 10}
```

**Response:**

```json
{
  "sessions": [
    {"id": "session_1712345678901234567", "name": "research-task", "active": true, "state": {...}}
  ],
  "count": 1
}
```

---

#### `load_session`

Load a session's full state by its ID.

**Input:**

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `id`  | string | yes      | Session ID |

**Example:**

```json
{"id": "session_1712345678901234567"}
```

**Response:**

```json
{
  "id": "session_1712345678901234567",
  "name": "research-task",
  "state": {"step": 3, "context": "analysing results"},
  "active": false
}
```

---

#### `save_session`

Persist session state. Automatically sets `type=session` and `created_at`. Generates a unique ID of the form `session_<unix-nano>`.

**Input:**

| Field   | Type   | Required | Description |
|---------|--------|----------|-------------|
| `name`  | string | yes      | Human-readable session name |
| `state` | object | no       | Arbitrary state to persist |

**Example:**

```json
{
  "name": "research-task",
  "state": {"step": 3, "context": "analysing results", "urls": ["https://example.com"]}
}
```

**Response:**

```json
{"success": true, "id": "session_1712345678901234567"}
```

---

#### `invalidate_cache`

Delete cache entries by key prefix, or clear all cache entries.

**Input:**

| Field    | Type   | Required | Description |
|----------|--------|----------|-------------|
| `prefix` | string | no       | Key prefix to match; omit to clear all cache entries |

**Example — clear a prefix:**

```json
{"prefix": "embeddings/v2/"}
```

**Example — clear all cache:**

```json
{}
```

**Response:**

```json
{"success": true}
```

---

### Cache

#### `upsert_cache`

Store a result under a cache key with TTL. The point ID is `cache_<sha256(key)>`, making writes idempotent.

**Input:**

| Field         | Type   | Required | Default | Description |
|---------------|--------|----------|---------|-------------|
| `key`         | string | yes      | —       | Cache key (typically a hash of the input) |
| `value`       | object | yes      | —       | Value to cache |
| `ttl_seconds` | number | no       | 3600    | Expiry in seconds |

**Example:**

```json
{
  "key": "embed/sha256:abc123",
  "value": {"vector": [0.12, -0.34, 0.56], "model": "text-embedding-3-small"},
  "ttl_seconds": 7200
}
```

**Response:**

```json
{"success": true, "key": "embed/sha256:abc123"}
```

---

#### `get_cache`

Retrieve a cached value by key. Returns an error if the entry does not exist or has expired.

**Input:**

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `key` | string | yes      | Cache key |

**Example:**

```json
{"key": "embed/sha256:abc123"}
```

**Response (hit):**

```json
{
  "key": "embed/sha256:abc123",
  "value": {"vector": [0.12, -0.34, 0.56], "model": "text-embedding-3-small"},
  "created_at": "2026-04-08T10:00:00Z"
}
```

**Response (expired or missing):** error — `"cache entry expired"` or `"get cache: …"`

---

## MCP Bridge Integration

Add to your MCP Bridge backend configuration:

```json
{
  "backend": "qdrant-mcp",
  "command": "/usr/local/bin/qdrant-mcp",
  "env": {
    "QDRANT_ADMIN_URL": "http://qdrant:6334",
    "QDRANT_ADMIN_KEY": "${QDRANT_ADMIN_KEY}",
    "QDRANT_USER_SECRET": "${QDRANT_USER_SECRET}",
    "QDRANT_VECTOR_SIZE": "1536"
  }
}
```

The bridge injects `QDRANT_USERNAME` and `QDRANT_COLLECTION` automatically per session. The server will create the collection on first use if it does not exist (`EnsureCollection` is idempotent).

---

## Readonly Mode

Start with `--readonly` to allow read operations only. All six mutating tools remain visible in `tools/list` but return a deterministic error when called:

```
write operations are disabled in readonly mode
```

Mutating tools:

| Tool | Category |
|------|----------|
| `upsert_point`    | CRUD |
| `delete_points`   | CRUD |
| `upsert_memory`   | Memory |
| `save_session`    | Memory |
| `invalidate_cache`| Memory |
| `upsert_cache`    | Cache |

---

## Testing

### Unit tests (no Qdrant required)

```bash
make test
```

Coverage targets: `tools` 80%+ (96%), `config` 60%+ (90%), `normalize` 100%, `readonly` 100%.

### Integration tests (live Qdrant required)

```bash
# Start Qdrant first (see Quick Start above), then:
QDRANT_TEST_URL=http://localhost:6334 go test ./internal/client -v -run Integration
```

Integration tests exercise all six I/O methods against a real Qdrant instance and are automatically skipped when `QDRANT_TEST_URL` is unset.
