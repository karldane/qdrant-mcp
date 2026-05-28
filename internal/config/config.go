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

	// Deduplication
	DedupThreshold float64
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
		DedupThreshold:    0.95,
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
	if v := os.Getenv("QDRANT_DEDUP_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			cfg.DedupThreshold = f
		}
	}

	return cfg
}

func MergeCLIFlags(cfg *Config) *Config {
	// flag.Visit only fires for flags explicitly set by the user,
	// unlike flag.Lookup which also returns defaults and would
	// clobber env-var-sourced values.
	flagValueIfSet := func(name string) (string, bool) {
		var val string
		var set bool
		flag.Visit(func(f *flag.Flag) {
			if f.Name == name {
				val = f.Value.String()
				set = true
			}
		})
		return val, set
	}

	if v, ok := flagValueIfSet("admin-url"); ok && v != "" {
		cfg.AdminURL = v
	}
	if v, ok := flagValueIfSet("admin-key"); ok && v != "" {
		cfg.AdminKey = v
	}
	if v, ok := flagValueIfSet("username"); ok && v != "" {
		cfg.Username = v
	}
	if v, ok := flagValueIfSet("collection"); ok && v != "" {
		cfg.Collection = v
	}
	if v, ok := flagValueIfSet("vector-size"); ok {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.VectorSize = size
		}
	}
	if v, ok := flagValueIfSet("timeout"); ok {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.TimeoutSeconds = secs
		}
	}
	if v, ok := flagValueIfSet("embedding-provider"); ok {
		cfg.EmbeddingProvider = v
	}
	if v, ok := flagValueIfSet("embedding-model"); ok {
		cfg.EmbeddingModel = v
	}
	if v, ok := flagValueIfSet("ollama-url"); ok {
		cfg.OllamaURL = v
	}
	if v, ok := flagValueIfSet("readonly"); ok {
		cfg.isReadOnly = v == "true"
	}
	if v, ok := flagValueIfSet("log-json"); ok {
		cfg.LogJSON = v == "true"
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
