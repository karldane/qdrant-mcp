package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("QDRANT_ADMIN_URL", "http://qdrant.internal:6333")
	os.Setenv("QDRANT_ADMIN_KEY", "admin-key-123")
	os.Setenv("QDRANT_USERNAME", "user@example.com")
	os.Setenv("QDRANT_COLLECTION", "user_example_com")
	os.Setenv("QDRANT_VECTOR_SIZE", "2048")
	os.Setenv("QDRANT_TIMEOUT_SECONDS", "60")
	os.Setenv("EMBEDDING_PROVIDER", "openai")
	os.Setenv("EMBEDDING_MODEL", "text-embedding-3-large")
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	os.Setenv("OPENAI_BASE_URL", "https://proxy.example.com/v1")
	defer func() {
		os.Unsetenv("QDRANT_ADMIN_URL")
		os.Unsetenv("QDRANT_ADMIN_KEY")
		os.Unsetenv("QDRANT_USERNAME")
		os.Unsetenv("QDRANT_COLLECTION")
		os.Unsetenv("QDRANT_VECTOR_SIZE")
		os.Unsetenv("QDRANT_TIMEOUT_SECONDS")
		os.Unsetenv("EMBEDDING_PROVIDER")
		os.Unsetenv("EMBEDDING_MODEL")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("OPENAI_BASE_URL")
	}()

	ResetForTest()
	cfg := Load()

	assert.Equal(t, "http://qdrant.internal:6333", cfg.AdminURL)
	assert.Equal(t, "admin-key-123", cfg.AdminKey)
	assert.Equal(t, "user@example.com", cfg.Username)
	assert.Equal(t, "user_example_com", cfg.Collection)
	assert.Equal(t, 2048, cfg.VectorSize)
	assert.Equal(t, 60, cfg.TimeoutSeconds)
	assert.Equal(t, "openai", cfg.EmbeddingProvider)
	assert.Equal(t, "text-embedding-3-large", cfg.EmbeddingModel)
	assert.Equal(t, "sk-test-key", cfg.OpenAIKey)
	assert.Equal(t, "https://proxy.example.com/v1", cfg.OpenAIBaseURL)
}

func TestLoadDefaults(t *testing.T) {
	ResetForTest()
	cfg := Load()

	assert.Equal(t, 768, cfg.VectorSize)
	assert.Equal(t, 30, cfg.TimeoutSeconds)
	assert.False(t, cfg.isReadOnly)
	assert.False(t, cfg.LogJSON)
	assert.Equal(t, "ollama", cfg.EmbeddingProvider)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaURL)
	assert.Equal(t, "https://api.openai.com/v1", cfg.OpenAIBaseURL)
}

func TestReadOnly(t *testing.T) {
	ResetForTest()
	cfg := Load()
	cfg.isReadOnly = true
	assert.True(t, cfg.ReadOnly())

	cfg.isReadOnly = false
	assert.False(t, cfg.ReadOnly())
}

func TestMergeCLIFlags_NoOpWhenFlagsEmpty(t *testing.T) {
	ResetForTest()
	cfg := Load()
	cfg.AdminURL = "http://original:6334"
	cfg.Collection = "original-collection"
	cfg.OllamaURL = "http://ollama.mcp-bridge-infra.svc.cluster.local:11434"
	cfg.EmbeddingProvider = "openai"
	cfg.EmbeddingModel = "text-embedding-3-large"
	cfg.OpenAIKey = "sk-override"
	cfg.OpenAIBaseURL = "https://proxy.example.com/v1"

	out := MergeCLIFlags(cfg)

	assert.Same(t, cfg, out)
	assert.Equal(t, "http://original:6334", out.AdminURL)
	assert.Equal(t, "original-collection", out.Collection)
	assert.Equal(t, "http://ollama.mcp-bridge-infra.svc.cluster.local:11434", out.OllamaURL, "should NOT be clobbered by flag default")
	assert.Equal(t, "openai", out.EmbeddingProvider, "should NOT be clobbered by flag default")
	assert.Equal(t, "text-embedding-3-large", out.EmbeddingModel)
	assert.Equal(t, "sk-override", out.OpenAIKey)
	assert.Equal(t, "https://proxy.example.com/v1", out.OpenAIBaseURL)
}

func TestMergeCLIFlags_ReadOnlyFlag(t *testing.T) {
	ResetForTest()
	cfg := Load()

	out := MergeCLIFlags(cfg)
	assert.False(t, out.ReadOnly())
}

func TestEmbedConfigInterface(t *testing.T) {
	ResetForTest()
	cfg := Load()

	// Config must satisfy the EmbedConfig interface used by embed.NewProvider.
	assert.Equal(t, cfg.EmbeddingProvider, cfg.GetEmbeddingProvider())
	assert.Equal(t, cfg.EmbeddingModel, cfg.GetEmbeddingModel())
	assert.Equal(t, cfg.OllamaURL, cfg.GetOllamaURL())
	assert.Equal(t, cfg.OpenAIKey, cfg.GetOpenAIKey())
	assert.Equal(t, cfg.OpenAIBaseURL, cfg.GetOpenAIBaseURL())
}
