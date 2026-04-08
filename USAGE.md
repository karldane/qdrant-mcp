# qdrant-mcp Usage Guide

Practical reference for running, configuring, and calling the qdrant-mcp backend.

## Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration Reference](#configuration-reference)
- [Authentication Model](#authentication-model)
- [Embedding Providers](#embedding-providers)
- [Running the Server](#running-the-server)
- [Tool Reference](#tool-reference)
  - [Core CRUD](#core-crud)
  - [Agent Memory](#agent-memory)
  - [Sessions](#sessions)
  - [Cache](#cache)
  - [Admin / Diagnostics](#admin--diagnostics)
- [MCP Bridge Integration](#mcp-bridge-integration)
- [Readonly Mode](#readonly-mode)
- [Testing](#testing)

---

## Prerequisites

| Requirement | Version |
|-------------|---------|
| Go          | 1.21+   |
| Qdrant      | 1.9+ (gRPC port 6334 must be reachable; JWT RBAC requires 1.9+) |

---

## Quick Start

### 1. Start Qdrant + Ollama

```bash
cd docker
cp .env.example .env
# Edit .env — set QDRANT_API_KEY and QDRANT_READ_ONLY_API_KEY
docker compose up -d
```

This starts:
- **Qdrant** at `localhost:6333` (REST/UI) and `localhost:6334` (gRPC) with JWT RBAC enabled
- **Ollama** at `localhost:11434` with `nomic-embed-text` pulled automatically

### 2. Build the server

```bash
go build -o qdrant-mcp .
```

### 3. Run the server

```bash
export QDRANT_ADMIN_URL="http://localhost:6333"
export QDRANT_ADMIN_KEY="your-api-key"
export QDRANT_USERNAME="alice@example.com"
export QDRANT_COLLECTION="alice_at_example_com"
./qdrant-mcp
```

The server communicates over **stdio** using the MCP protocol. It is designed to be spawned by the MCP Bridge, not run interactively.

On startup the server will:
1. Provision the collection if it does not exist
2. Create required payload indexes
3. Issue a scoped JWT for the user's collection
4. Ping Ollama (or your configured provider) to verify embeddings are available
5. Begin serving MCP tools

---

## Configuration Reference

All options can be set via environment variable or CLI flag. CLI flags override environment variables.

| Environment Variable     | CLI Flag               | Default                        | Required | Description |
|--------------------------|------------------------|--------------------------------|----------|-------------|
| `QDRANT_ADMIN_URL`       | `--admin-url`          | —                              | **Yes**  | Qdrant base URL, e.g. `http://localhost:6333` |
| `QDRANT_ADMIN_KEY`       | `--admin-key`          | —                              | **Yes**  | Admin API key; also the JWT signing secret |
| `QDRANT_USERNAME`        | `--username`           | —                              | **Yes**  | User identifier (email); injected via `{{users.email}}` |
| `QDRANT_COLLECTION`      | `--collection`         | —                              | **Yes**  | Collection name; injected via `{{users.email\|sanitised}}` |
| `QDRANT_VECTOR_SIZE`     | `--vector-size`        | `768`                          | No       | Must match your embedding model |
| `QDRANT_TIMEOUT_SECONDS` | `--timeout`            | `30`                           | No       | Request timeout in seconds |
| `EMBEDDING_PROVIDER`     | `--embedding-provider` | `ollama`                       | No       | `ollama`, `openai`, or `none` |
| `EMBEDDING_MODEL`        | `--embedding-model`    | *(provider default)*           | No       | Model name override |
| `QDRANT_OLLAMA_URL`      | `--ollama-url`         | `http://localhost:11434`       | No       | Required when provider=ollama |
| `OPENAI_API_KEY`         | —                      | —                              | No       | Required when provider=openai |
| `OPENAI_BASE_URL`        | —                      | `https://api.openai.com/v1`    | No       | Override for OpenAI-compatible APIs |
| —                        | `--readonly`           | off                            | No       | Disable all mutating tools |
| —                        | `--log-json`           | off                            | No       | Emit structured JSON logs to stderr |

> `QDRANT_USER_SECRET` has been removed in v2. Authentication is handled entirely
> via JWT RBAC — see [Authentication Model](#authentication-model) below.

### Default vector sizes by provider

| Provider | Default model          | Vector size |
|----------|------------------------|-------------|
| `ollama` | `nomic-embed-text`     | 768         |
| `openai` | `text-embedding-3-small` | 1536      |

`QDRANT_VECTOR_SIZE` must match the model in use. A mismatch between the configured
size and an existing collection is a hard startup error.

---

## Authentication Model

qdrant-mcp v2 uses Qdrant's JWT RBAC mechanism instead of derived API keys.

**How it works:**

1. On startup the server connects with `QDRANT_ADMIN_KEY` to provision the collection.
2. It then generates a short-lived HS256 JWT signed with `QDRANT_ADMIN_KEY`.
3. The JWT restricts access to the user's collection only — no other collection is reachable.
4. All tool calls use this JWT; the raw admin key is never used during tool execution.
5. The JWT expires after 1 hour (equal to the typical process lifetime of a spawned backend).

**JWT payload:**

```json
{
  "sub": "alice@example.com",
  "exp": 1744120800,
  "access": {
    "collections": {
      "alice_at_example_com": ["read", "write"]
    }
  }
}
```

**Qdrant must have JWT RBAC enabled** — the Docker Compose file includes this automatically:

```yaml
QDRANT__SERVICE__JWT_RBAC: "true"
```

Without this flag, JWTs are still accepted as bearer tokens but collection-level
restriction is not enforced.

---

## Embedding Providers

When a tool receives text without a pre-computed vector, the configured provider
is called automatically. If a vector is supplied, it is used directly (passthrough).

### Ollama (default)

```bash
EMBEDDING_PROVIDER=ollama
QDRANT_OLLAMA_URL=http://localhost:11434
EMBEDDING_MODEL=nomic-embed-text   # default
QDRANT_VECTOR_SIZE=768
```

The `nomic-embed-text` model is pulled automatically by the Docker Compose setup.

### OpenAI

```bash
EMBEDDING_PROVIDER=openai
OPENAI_API_KEY=sk-...
EMBEDDING_MODEL=text-embedding-3-small   # default
QDRANT_VECTOR_SIZE=1536
```

To use an OpenAI-compatible endpoint (e.g. Azure, local proxy):

```bash
OPENAI_BASE_URL=https://my-proxy.example.com/v1
```

### None (pre-computed vectors only)

```bash
EMBEDDING_PROVIDER=none
```

With `none`, `upsert_memory` and `search_memory` require callers to supply
`embedding` / `query_embedding` directly. Calling those tools without a vector
returns a descriptive error.

---

## Running the Server

### Standard (Ollama, auto-embed)

```bash
QDRANT_ADMIN_URL=http://localhost:6333 \
QDRANT_ADMIN_KEY=my-admin-key \
QDRANT_USERNAME=alice@example.com \
QDRANT_COLLECTION=alice_at_example_com \
./qdrant-mcp
```

### OpenAI embeddings

```bash
QDRANT_ADMIN_URL=http://localhost:6333 \
QDRANT_ADMIN_KEY=my-admin-key \
QDRANT_USERNAME=alice@example.com \
QDRANT_COLLECTION=alice_at_example_com \
EMBEDDING_PROVIDER=openai \
OPENAI_API_KEY=sk-... \
QDRANT_VECTOR_SIZE=1536 \
./qdrant-mcp
```

### With CLI flags

```bash
./qdrant-mcp \
  --admin-url http://localhost:6333 \
  --admin-key my-admin-key \
  --username alice@example.com \
  --collection alice_at_example_com \
  --vector-size 768 \
  --log-json
```

### Readonly mode

```bash
./qdrant-mcp \
  --admin-url http://localhost:6333 \
  --admin-key my-admin-key \
  --username alice@example.com \
  --collection alice_at_example_com \
  --readonly
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

Store a fact, observation, or note. Sets `type=memory` and `created` automatically.

If `EMBEDDING_PROVIDER` is configured and no `embedding` is supplied, the server
generates the vector from `content` automatically.

**Input:**

| Field         | Type          | Required | Description |
|---------------|---------------|----------|-------------|
| `content`     | string        | yes      | Text to store |
| `embedding`   | array[number] | no       | Pre-computed vector (skips auto-embed when supplied) |
| `metadata`    | object        | no       | Extra metadata |
| `tags`        | array[string] | no       | Categorisation tags |
| `ttl_seconds` | number        | no       | Expire after N seconds |

**Example:**

```json
{
  "content": "User prefers concise responses with code examples",
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

Search stored memories by semantic similarity. Filters to `type=memory` automatically.

If `EMBEDDING_PROVIDER` is configured and no `query_embedding` is supplied, the
server generates the query vector from `query` automatically.

**Input:**

| Field             | Type          | Required | Default | Description |
|-------------------|---------------|----------|---------|-------------|
| `query`           | string        | yes      | —       | Search query text |
| `query_embedding` | array[number] | no       | —       | Pre-computed query vector (skips auto-embed when supplied) |
| `limit`           | number        | no       | 5       | Maximum results |
| `filter`          | object        | no       | —       | Additional payload filters |

**Example:**

```json
{
  "query": "user response preferences",
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
      "metadata": {"created": "2026-04-08T10:00:00Z", "type": "memory"}
    }
  ],
  "count": 1
}
```

---

#### `delete_memory`

Delete memories by ID list or by tag. Provide at least one of `ids` or `tag`.

**Input:**

| Field | Type          | Required | Description |
|-------|---------------|----------|-------------|
| `ids` | array[string] | no*      | Memory point IDs to delete |
| `tag` | string        | no*      | Delete all memories with this tag |

\* At least one of `ids` or `tag` must be provided.

**Example — delete by IDs:**

```json
{"ids": ["mem_1712345678901234567"]}
```

**Example — delete by tag:**

```json
{"tag": "preference"}
```

**Response:**

```json
{"success": true}
```

---

### Sessions

#### `list_sessions`

List stored sessions. Filters to `type=session` automatically.

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
    {"id": "session_1712345678901234567", "name": "research-task", "active": true, "state": {}}
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
  "state": {"step": 3, "context": "analysing results"}
}
```

---

#### `save_session`

Persist session state. Sets `type=session` and `created` automatically. Generates an ID of the form `session_<unix-nano>`.

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

#### `delete_session`

Remove a session by ID.

**Input:**

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `id`  | string | yes      | Session ID to delete |

**Example:**

```json
{"id": "session_1712345678901234567"}
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
  "value": {"vector": [0.12, -0.34, 0.56], "model": "nomic-embed-text"},
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
  "value": {"vector": [0.12, -0.34, 0.56], "model": "nomic-embed-text"},
  "created": "2026-04-08T10:00:00Z"
}
```

**Response (expired or missing):** error — `"cache entry expired"` or `"get cache: …"`

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

### Admin / Diagnostics

#### `collection_info`

Return diagnostics for the user's collection: vector count, vector size, distance metric, and index status.

**Input:** none required

**Example:**

```json
{}
```

**Response:**

```json
{
  "collection": "alice_at_example_com",
  "status": "CollectionStatus_Green",
  "vector_size": 768,
  "distance": "Distance_Cosine",
  "points_count": 142,
  "indexed_vectors_count": 142
}
```

---

## MCP Bridge Integration

Add to your MCP Bridge backend configuration:

```json
{
  "QDRANT_ADMIN_URL":   "http://localhost:6333",
  "QDRANT_ADMIN_KEY":   "<openssl rand -hex 32>",
  "QDRANT_OLLAMA_URL":  "http://localhost:11434",
  "QDRANT_VECTOR_SIZE": "768",
  "QDRANT_USERNAME":    "{{users.email}}",
  "QDRANT_COLLECTION":  "{{users.email|sanitised}}"
}
```

The bridge injects `QDRANT_USERNAME` and `QDRANT_COLLECTION` per session via
template expressions. On each spawn the server provisions the collection if absent
and issues a fresh scoped JWT — no manual key management required.

> `QDRANT_USER_SECRET` is no longer used and should be removed from any existing
> bridge configurations.

---

## Readonly Mode

Start with `--readonly` to allow read operations only. All mutating tools remain
visible in `tools/list` but return a deterministic error when called:

```
write operations are disabled in readonly mode
```

Mutating tools:

| Tool               | Category |
|--------------------|----------|
| `upsert_point`     | CRUD     |
| `delete_points`    | CRUD     |
| `upsert_memory`    | Memory   |
| `delete_memory`    | Memory   |
| `save_session`     | Sessions |
| `delete_session`   | Sessions |
| `upsert_cache`     | Cache    |
| `invalidate_cache` | Cache    |

---

## Testing

### Unit tests (no Qdrant required)

```bash
go test ./... -race -count=1
```

All packages pass with no live Qdrant or Ollama instance. Coverage targets:
`tools` 80%+, `config` 60%+, `embed` 90%+, `qdrant` 80%+, `normalize` 100%, `readonly` 100%.

### Integration tests (live Qdrant required)

```bash
# Start the stack first (see Quick Start), then:
QDRANT_TEST_URL=http://localhost:6334 go test ./internal/client -v -run Integration
```

Integration tests exercise all I/O methods against a real Qdrant instance and are
automatically skipped when `QDRANT_TEST_URL` is unset.
