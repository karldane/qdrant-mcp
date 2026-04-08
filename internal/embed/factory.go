package embed

import "fmt"

// EmbedConfig holds the subset of configuration that the embed factory needs.
// This is satisfied by *config.Config once the embedding fields are added, but
// we use a plain interface here to keep the embed package free of a config
// import cycle.
type EmbedConfig interface {
	GetEmbeddingProvider() string
	GetEmbeddingModel() string
	GetOllamaURL() string
	GetOpenAIKey() string
	GetOpenAIBaseURL() string
}

// NewProvider constructs the appropriate Provider based on the config.
// Returns a NoOpProvider when provider is "none" or empty.
func NewProvider(cfg EmbedConfig) (Provider, error) {
	switch cfg.GetEmbeddingProvider() {
	case "ollama":
		return NewOllamaProvider(cfg.GetOllamaURL(), cfg.GetEmbeddingModel()), nil
	case "openai":
		if cfg.GetOpenAIKey() == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required when EMBEDDING_PROVIDER=openai")
		}
		return NewOpenAIProvider(cfg.GetOpenAIKey(), cfg.GetOpenAIBaseURL(), cfg.GetEmbeddingModel()), nil
	case "none", "":
		return &NoOpProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown embedding provider %q: must be ollama, openai, or none", cfg.GetEmbeddingProvider())
	}
}
