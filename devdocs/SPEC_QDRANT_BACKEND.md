# Qdrant MCP Backend — Specification (Revised)

> Revision history:
> - v1: Initial spec — per-collection isolation, QDRANT_USER_SECRET derived key
> - v2 (this document): Replace derived API key with JWT RBAC; remove QDRANT_USER_SECRET;
>   add server-side embedding via pluggable provider

---

## Overview

`qdrant-mcp` is a Go MCP backend using `mcp-framework`. It gives each agent user a
private, isolated Qdrant collection for agent memory, semantic search, session state,
and caching. It runs behind the MCP bridge, which injects per-user env vars via
`{{template}}` expressions at spawn time.

### Qdrant Concepts (MongoDB mapping)

| MongoDB | Qdrant | Notes |
|---|---|---|
| Database | Collection | One per user |
| Document | Point | Has a vector + JSON payload |
| Fields | Payload | Flat or nested JSON |
| Insert/update | Upsert | Point by UUID |
| Find | Scroll | Paginated payload filter |
| Search | Search | Vector similarity + payload filter |

---

## Authentication Model (v2 — JWT RBAC)

### Why JWT, not derived API keys

Qdrant does not support programmatic API key provisioning at runtime. Keys are
static — set at startup via config/env vars only. JWT RBAC (introduced in Qdrant
v1.9.0) is the correct mechanism for per-user scoped access.

### How it works

- `QDRANT_ADMIN_KEY` is both the admin API key and the JWT signing secret (HS256)
- On startup, `qdrant-mcp` generates a short-lived JWT signed with `QDRANT_ADMIN_KEY`
- The JWT payload restricts access to the user's collection only
- All tool calls use the JWT as their API key — not the raw admin key
- The raw admin key is only used during provisioning (`EnsureCollection`)

### JWT Payload

```json
{
  "sub": "alice@example.com",
  "exp": <now + 3600>,
  "access": {
    "collections": {
      "alice_at_example_com": ["read", "write"]
    }
  }
}
```

- `sub` — `QDRANT_USERNAME` (user's email address)
- `exp` — Unix timestamp, 1 hour from process spawn
- `access.collections` — restricted to the user's collection only; no other
  collection is accessible even if the Qdrant instance contains others

### Signing

```go
// internal/qdrant/auth.go

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "strings"
    "time"
)

func GenerateUserJWT(adminKey, username, collection string) (string, error) {
    header := base64url(mustJSON(map[string]string{"alg": "HS256", "typ": "JWT"}))
    payload := base64url(mustJSON(map[string]interface{}{
        "sub": username,
        "exp": time.Now().Add(time.Hour).Unix(),
        "access": map[string]interface{}{
            "collections": map[string]interface{}{
                collection: []string{"read", "write"},
            },
        },
    }))
    unsigned := header + "." + payload
    mac := hmac.New(sha256.New, []byte(adminKey))
    mac.Write([]byte(unsigned))
    sig := base64url(mac.Sum(nil))
    return unsigned + "." + sig, nil
}
```

No third-party JWT library required — standard library only.

### Startup Flow

```
1. Load config (env vars + flags)
2. Validate required config fields
3. Connect with QDRANT_ADMIN_KEY (admin client)
4. EnsureCollection: create collection if absent, validate vector size if present
5. Generate user JWT (1-hour expiry) signed with QDRANT_ADMIN_KEY
6. Construct user client using JWT
7. Ping user client — verify JWT is accepted
8. Start serving MCP tools
```

The admin client is discarded after step 4. All tool traffic uses the scoped JWT client.

---

## Configuration

`QDRANT_USER_SECRET` is removed in v2. All fields:

| Env Var | Flag | Default | Required | Description |
|---|---|---|---|---|
| `QDRANT_ADMIN_URL` | `--admin-url` | — | Yes | Qdrant base URL, e.g. `http://localhost:6333` |
| `QDRANT_ADMIN_KEY` | `--admin-key` | — | Yes | Admin API key; also JWT signing secret |
| `QDRANT_USERNAME` | `--username` | — | Yes | User identifier (email); injected via `{{users.email}}` |
| `QDRANT_COLLECTION` | `--collection` | — | Yes | Collection name; injected via `{{users.email\|sanitised}}` |
| `QDRANT_VECTOR_SIZE` | `--vector-size` | `768` | No | Must match embedding model |
| `EMBEDDING_PROVIDER` | `--embedding-provider` | `ollama` | No | `ollama`, `openai`, or `none` |
| `EMBEDDING_MODEL` | `--embedding-model` | *(provider default)* | No | Model name override |
| `QDRANT_OLLAMA_URL` | `--ollama-url` | `http://localhost:11434` | No | Required when provider=ollama |
| `OPENAI_API_KEY` | — | — | No | Required when provider=openai |
| `OPENAI_BASE_URL` | — | `https://api.openai.com/v1` | No | Override for OpenAI-compatible APIs |
| `--readonly` | `--readonly` | false | No | Disable all mutating tools at runtime |
| `--write-enabled` | `--write-enabled` | false | No | Explicitly enable destructive tools |

### Default Vector Sizes by Provider

| Provider | Default Model | Vector Size |
|---|---|---|
| `ollama` | `nomic-embed-text` | 768 |
| `openai` | `text-embedding-3-small` | 1536 |

`QDRANT_VECTOR_SIZE` must match the provider default or be overridden explicitly.

---

## Embedding Provider

See `SPEC_EMBEDDING_PROVIDER.md` for full detail. Summary:

- `internal/embed.Provider` interface with `Embed(ctx, text) ([]float64, error)`
- `OllamaProvider`: POST `/api/embeddings` to `QDRANT_OLLAMA_URL`
- `OpenAIProvider`: POST `/v1/embeddings` to OpenAI
- `NoOpProvider`: returns clear error; used when `EMBEDDING_PROVIDER=none`
- Auto-embed: if a tool receives text but no vector, the provider is called automatically
- Passthrough: if a pre-computed vector is supplied, the provider is not called

---

## MCP Tools

### Core CRUD

| Tool | Description | Risk | Impact | Read-only safe |
|---|---|---|---|---|
| `upsert_point` | Store vector + payload by ID | Med | Write | No |
| `get_point` | Retrieve point by ID | Low | Read | Yes |
| `scroll_points` | Paginated list with payload filters | Low | Read | Yes |
| `search_points` | Vector similarity search with filters | Low | Read | Yes |
| `delete_points` | Delete by ID or filter | High | Delete | No |

### Agent Memory

| Tool | Description | Risk | Impact | Read-only safe |
|---|---|---|---|---|
| `upsert_memory` | Store fact/note/summary with TTL and tags | Med | Write | No |
| `search_memory` | Semantic + keyword search over memories | Low | Read | Yes |
| `delete_memory` | Delete by ID or tag filter | High | Delete | No |

### Sessions

| Tool | Description | Risk | Impact | Read-only safe |
|---|---|---|---|---|
| `save_session` | Persist session state (checklist, plan, artifacts) | Med | Write | No |
| `load_session` | Resume session by ID or query | Low | Read | Yes |
| `list_sessions` | List active sessions with metadata | Low | Read | Yes |
| `delete_session` | Remove a session | Med | Delete | No |

### Cache

| Tool | Description | Risk | Impact | Read-only safe |
|---|---|---|---|---|
| `upsert_cache` | Cache result by key hash with TTL | Med | Write | No |
| `get_cache` | Retrieve cached result, returns miss if expired | Low | Read | Yes |
| `invalidate_cache` | Remove by key or tag pattern | Med | Write | No |

### Admin / Diagnostics

| Tool | Description | Risk | Impact | Read-only safe |
|---|---|---|---|---|
| `collection_info` | Vector count, size, index status | Low | Read | Yes |

---

## Point Payload Schema (Conventions)

All points share a common payload envelope. Tools enforce this structure.

```json
{
  "user_id":    "alice@example.com",
  "type":       "memory|session|cache|point",
  "content":    "User prefers pytest and black formatter",
  "tags":       ["dev-tools", "python"],
  "ttl":        "2026-05-08T00:00:00Z",
  "session_id": "uuid",
  "created":    "2026-04-08T14:00:00Z",
  "updated":    "2026-04-08T14:00:00Z",
  "metadata":   {}
}
```

- `type` is indexed for fast filtering by tool category
- `ttl` is a timestamp; `search_memory` and `get_cache` filter out expired points
- `user_id` is set by the backend from `QDRANT_USERNAME` — callers cannot override it

---

## Payload Indexes

Created by `EnsureCollection` alongside the collection:

| Field | Index type | Used by |
|---|---|---|
| `type` | keyword | all tools (filter by tool category) |
| `tags` | keyword | `search_memory`, `delete_memory` |
| `session_id` | keyword | `load_session`, `delete_session` |
| `ttl` | datetime | TTL expiry filtering |
| `created` | datetime | recency ordering |
| `content` | full-text | keyword fallback in `search_memory` |

---

## Docker Compose Change Required

Add `QDRANT__SERVICE__JWT_RBAC: "true"` to the Qdrant service env to enable
collection-scoped JWT access. Without this, JWTs are accepted as plain bearer
tokens but collection-level restriction is not enforced.

```yaml
services:
  qdrant:
    image: qdrant/qdrant:v1.14.0
    environment:
      QDRANT__SERVICE__API_KEY: "${QDRANT_API_KEY}"
      QDRANT__SERVICE__READ_ONLY_API_KEY: "${QDRANT_READ_ONLY_API_KEY}"
      QDRANT__SERVICE__JWT_RBAC: "true"        # ← required for v2
      QDRANT__LOG_LEVEL: "INFO"
```

---

## Bridge Backend Config

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

`QDRANT_USER_SECRET` is removed. `QDRANT_ADMIN_KEY` serves all authentication roles.

---

## Package Layout

```
qdrant-mcp/
├── main.go                     # Wiring only: config → embed → auth → client → tools → serve
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── embed/
│   │   ├── embed.go            # Provider interface + NoOpProvider
│   │   ├── ollama.go
│   │   ├── openai.go
│   │   ├── factory.go
│   │   └── embed_test.go
│   ├── qdrant/
│   │   ├── auth.go             # GenerateUserJWT
│   │   ├── auth_test.go
│   │   ├── client.go           # Admin + user client construction
│   │   ├── client_test.go
│   │   ├── provision.go        # EnsureCollection, EnsureIndexes
│   │   └── provision_test.go
│   ├── tools/
│   │   ├── crud.go             # upsert_point, get_point, scroll_points, search_points, delete_points
│   │   ├── memory.go           # upsert_memory, search_memory, delete_memory
│   │   ├── session.go          # save_session, load_session, list_sessions, delete_session
│   │   ├── cache.go            # upsert_cache, get_cache, invalidate_cache
│   │   ├── admin.go            # collection_info
│   │   └── *_test.go
│   └── testutil/
│       └── testutil.go         # Mock Qdrant server, mock embed provider
└── docs/
    └── TOOLS.md
```

---

## Security Properties

| Property | How achieved |
|---|---|
| User cannot access other users' data | JWT restricts to named collection only |
| User cannot escalate to admin | JWT has no `manage` claim |
| JWT leak is time-bounded | 1-hour expiry, regenerated on each spawn |
| Admin key never sent in tool calls | Used only in provisioning; discarded after |
| User cannot set their own `user_id` | Backend overwrites from `QDRANT_USERNAME` |
| Expired TTL points invisible | Filtered at query time in memory/cache tools |

---

## Testing Requirements

### `internal/qdrant/auth` (target: 100%)

| Test | Covers |
|---|---|
| `TestGenerateUserJWT_structure` | Valid three-part JWT |
| `TestGenerateUserJWT_claims` | sub, exp ~1h, access.collections scoped |
| `TestGenerateUserJWT_signature` | HS256 signature verifiable with admin key |
| `TestGenerateUserJWT_expiry` | exp is within ±5s of now+3600 |

### `internal/qdrant/provision` (target: 80%)

| Test | Covers |
|---|---|
| `TestEnsureCollection_creates` | Collection absent → created with correct vector size |
| `TestEnsureCollection_exists_match` | Collection present, sizes match → no error |
| `TestEnsureCollection_exists_mismatch` | Collection present, sizes differ → hard error |
| `TestEnsureIndexes` | All required payload indexes created |

### `internal/embed` (target: 90%) — see SPEC_EMBEDDING_PROVIDER.md

### `internal/tools` (target: 80%)

All tools tested with mock Qdrant client and mock embed provider.
Auto-embed path and passthrough path both verified for upsert/search tools.
Readonly enforcement verified for every mutating tool.

---

## Definition of Done

- [ ] TDD throughout — failing test before every production change
- [ ] `GenerateUserJWT` tested independently with signature verification
- [ ] `EnsureCollection` hard-fails on vector size mismatch
- [ ] All tools use scoped JWT client, never raw admin key
- [ ] Readonly blocks all mutating tools deterministically
- [ ] EnforcerProfile correct on every tool
- [ ] Full suite green: `go test ./... -race -count=1`
- [ ] README env var table updated (QDRANT_USER_SECRET removed)
- [ ] Docker Compose updated with `QDRANT__SERVICE__JWT_RBAC=true`
