# Local Qdrant + Ollama for MCP Testing

## Stack

| Service | Purpose | Port |
|---|---|---|
| Qdrant | Vector database | `localhost:6333` (REST), `localhost:6334` (gRPC) |
| Ollama | Local embedding model server | `localhost:11434` |
| ollama-model-pull | One-shot sidecar to pull the model | — |

## Quick Start

```bash
cp .env.example .env
# Edit .env and set your API keys (see below)
docker compose up -d
docker compose logs -f ollama-model-pull  # watch the model pull
```

The first start downloads `nomic-embed-text` (~274MB). Subsequent starts skip
the pull — the model is cached in the `ollama_models` volume.

## Generate API Keys

```bash
openssl rand -hex 32   # run twice — once for each key
```

Paste the results into `.env`.

## Test the Services

```bash
# Qdrant health
curl http://localhost:6333/healthz

# Qdrant collections (requires API key)
curl http://localhost:6333/collections \
  -H "api-key: $(grep ^QDRANT_API_KEY .env | cut -d= -f2)"

# Ollama: generate an embedding
curl http://localhost:11434/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model": "nomic-embed-text", "prompt": "hello world"}'
```

The embedding response should contain a 768-element float array.

## Bridge Backend Config

```json
{
  "QDRANT_ADMIN_URL":    "http://qdrant.internal:6333",
  "QDRANT_ADMIN_KEY":    "<value of QDRANT_API_KEY>",
  "QDRANT_USER_SECRET":  "<openssl rand -hex 32>",
  "QDRANT_HOST":         "qdrant.internal",
  "QDRANT_OLLAMA_URL":   "http://ollama.internal:11434",
  "QDRANT_VECTOR_SIZE":  "768",
  "QDRANT_USERNAME":     "{{users.email}}",
  "QDRANT_COLLECTION":   "{{users.email|sanitised}}"
}
```

For local testing use `localhost` in place of `*.internal`.

## Important: QDRANT_VECTOR_SIZE

`nomic-embed-text` produces **768-dimension** vectors.
Set `QDRANT_VECTOR_SIZE=768` in the backend config to match.
If you change embedding models, wipe Qdrant storage first:

```bash
docker compose down -v && docker compose up -d
```

## Useful Commands

```bash
docker compose up -d            # start everything
docker compose down             # stop (data persists)
docker compose down -v          # stop AND wipe all data
docker compose logs -f          # tail all logs
docker compose logs -f ollama   # ollama only
```

## GPU Acceleration (optional)

If you have an NVIDIA GPU and nvidia-container-toolkit installed,
add the following to the `ollama` service to accelerate embedding generation:

```yaml
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```
