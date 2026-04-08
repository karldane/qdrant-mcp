package config

import (
	"flag"
	"os"
	"strconv"
	"sync"
)

var initFlags sync.Once

func ResetForTest() {
	initFlags = sync.Once{}
}

type Config struct {
	AdminURL       string
	AdminKey       string
	Username       string
	Collection     string
	VectorSize     int
	TimeoutSeconds int
	isReadOnly     bool
	LogJSON        bool

	// Embedding provider
	EmbeddingProvider string
	EmbeddingModel    string
	OllamaURL         string
	OpenAIKey         string
	OpenAIBaseURL     string
}

func (c *Config) ReadOnly() bool { return c.isReadOnly }

// EmbedConfig interface (satisfies embed.EmbedConfig without importing embed).
func (c *Config) GetEmbeddingProvider() string { return c.EmbeddingProvider }
func (c *Config) GetEmbeddingModel() string    { return c.EmbeddingModel }
func (c *Config) GetOllamaURL() string         { return c.OllamaURL }
func (c *Config) GetOpenAIKey() string         { return c.OpenAIKey }
func (c *Config) GetOpenAIBaseURL() string     { return c.OpenAIBaseURL }

func Load() *Config {
	initFlags.Do(func() {
		flag.Parse()
	})

	cfg := &Config{
		VectorSize:        768,
		TimeoutSeconds:    30,
		isReadOnly:        false,
		LogJSON:           false,
		EmbeddingProvider: "ollama",
		OllamaURL:         "http://localhost:11434",
		OpenAIBaseURL:     "https://api.openai.com/v1",
	}

	if v := os.Getenv("QDRANT_ADMIN_URL"); v != "" {
		cfg.AdminURL = v
	}
	if v := os.Getenv("QDRANT_ADMIN_KEY"); v != "" {
		cfg.AdminKey = v
	}
	if v := os.Getenv("QDRANT_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("QDRANT_COLLECTION"); v != "" {
		cfg.Collection = v
	}
	if v := os.Getenv("QDRANT_VECTOR_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.VectorSize = size
		}
	}
	if v := os.Getenv("QDRANT_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.TimeoutSeconds = secs
		}
	}
	if v := os.Getenv("EMBEDDING_PROVIDER"); v != "" {
		cfg.EmbeddingProvider = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		cfg.EmbeddingModel = v
	}
	if v := os.Getenv("QDRANT_OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.OpenAIKey = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.OpenAIBaseURL = v
	}

	return cfg
}

func MergeCLIFlags(cfg *Config) *Config {
	lookup := func(name string) string {
		if f := flag.Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	}

	if v := lookup("admin-url"); v != "" {
		cfg.AdminURL = v
	}
	if v := lookup("admin-key"); v != "" {
		cfg.AdminKey = v
	}
	if v := lookup("username"); v != "" {
		cfg.Username = v
	}
	if v := lookup("collection"); v != "" {
		cfg.Collection = v
	}
	if v := lookup("vector-size"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.VectorSize = size
		}
	}
	if v := lookup("timeout"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.TimeoutSeconds = secs
		}
	}
	if v := lookup("embedding-provider"); v != "" {
		cfg.EmbeddingProvider = v
	}
	if v := lookup("embedding-model"); v != "" {
		cfg.EmbeddingModel = v
	}
	if v := lookup("ollama-url"); v != "" {
		cfg.OllamaURL = v
	}
	if f := flag.Lookup("readonly"); f != nil {
		cfg.isReadOnly = f.Value.String() == "true"
	}
	if f := flag.Lookup("log-json"); f != nil {
		cfg.LogJSON = f.Value.String() == "true"
	}
	return cfg
}

func init() {
	flag.String("admin-url", "", "Qdrant admin URL")
	flag.String("admin-key", "", "Qdrant admin API key (also JWT signing secret)")
	flag.String("username", "", "User identifier (email address)")
	flag.String("collection", "", "Collection name (sanitised email)")
	flag.Int("vector-size", 768, "Vector size for collection (default: 768 for nomic-embed-text)")
	flag.Int("timeout", 30, "HTTP timeout in seconds")
	flag.String("embedding-provider", "ollama", "Embedding provider: ollama, openai, or none")
	flag.String("embedding-model", "", "Embedding model name (uses provider default if empty)")
	flag.String("ollama-url", "http://localhost:11434", "Ollama base URL")
	flag.Bool("readonly", false, "Disable all mutating tools")
	flag.Bool("log-json", false, "Emit structured JSON logs")
}
