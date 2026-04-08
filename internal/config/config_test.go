package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("QDRANT_ADMIN_URL", "http://qdrant.internal:6333")
	os.Setenv("QDRANT_ADMIN_KEY", "admin-key-123")
	os.Setenv("QDRANT_USER_SECRET", "user-secret-456")
	os.Setenv("QDRANT_HOST", "qdrant.internal")
	os.Setenv("QDRANT_USERNAME", "user@example.com")
	os.Setenv("QDRANT_COLLECTION", "user_example_com")
	os.Setenv("QDRANT_VECTOR_SIZE", "2048")
	os.Setenv("QDRANT_TIMEOUT_SECONDS", "60")
	defer func() {
		os.Unsetenv("QDRANT_ADMIN_URL")
		os.Unsetenv("QDRANT_ADMIN_KEY")
		os.Unsetenv("QDRANT_USER_SECRET")
		os.Setenv("QDRANT_HOST", "")
		os.Unsetenv("QDRANT_USERNAME")
		os.Unsetenv("QDRANT_COLLECTION")
		os.Unsetenv("QDRANT_VECTOR_SIZE")
		os.Unsetenv("QDRANT_TIMEOUT_SECONDS")
	}()

	ResetForTest()
	cfg := Load()

	assert.Equal(t, "http://qdrant.internal:6333", cfg.AdminURL)
	assert.Equal(t, "admin-key-123", cfg.AdminKey)
	assert.Equal(t, "user-secret-456", cfg.UserSecret)
	assert.Equal(t, "qdrant.internal", cfg.Host)
	assert.Equal(t, "user@example.com", cfg.Username)
	assert.Equal(t, "user_example_com", cfg.Collection)
	assert.Equal(t, 2048, cfg.VectorSize)
	assert.Equal(t, 60, cfg.TimeoutSeconds)
}

func TestLoadDefaults(t *testing.T) {
	ResetForTest()
	cfg := Load()

	assert.Equal(t, 1536, cfg.VectorSize)
	assert.Equal(t, 30, cfg.TimeoutSeconds)
	assert.False(t, cfg.isReadOnly)
	assert.False(t, cfg.LogJSON)
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
	// When no CLI flags were set (all registered with empty/zero defaults),
	// MergeCLIFlags should return the cfg unchanged.
	ResetForTest()
	cfg := Load()
	cfg.AdminURL = "http://original:6334"
	cfg.Collection = "original-collection"

	out := MergeCLIFlags(cfg)

	// Should be the same pointer and unchanged values.
	assert.Same(t, cfg, out)
	assert.Equal(t, "http://original:6334", out.AdminURL)
	assert.Equal(t, "original-collection", out.Collection)
}

func TestMergeCLIFlags_ReadOnlyFlag(t *testing.T) {
	// The "readonly" flag defaults to false; MergeCLIFlags should set
	// isReadOnly to false when the flag value is "false".
	ResetForTest()
	cfg := Load()

	out := MergeCLIFlags(cfg)
	assert.False(t, out.ReadOnly())
}
