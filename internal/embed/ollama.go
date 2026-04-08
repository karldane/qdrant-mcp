package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	OllamaDefaultModel      = "nomic-embed-text"
	OllamaDefaultVectorSize = 768
)

// OllamaProvider calls a local Ollama instance to generate text embeddings.
type OllamaProvider struct {
	BaseURL string
	Model   string
	client  *http.Client
}

// NewOllamaProvider creates an OllamaProvider with sensible defaults.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = OllamaDefaultModel
	}
	return &OllamaProvider{
		BaseURL: baseURL,
		Model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed sends text to POST /api/embeddings and returns the embedding vector.
func (p *OllamaProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(map[string]string{
		"model":  p.Model,
		"prompt": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: ollama unreachable at %s — is Ollama running?", p.BaseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding failed: ollama returned %d: %s — run: ollama pull %s", resp.StatusCode, string(b), p.Model)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding for model %s", p.Model)
	}
	return result.Embedding, nil
}

// VectorSize returns the default vector size for the configured model.
// For nomic-embed-text this is 768. For other models this is an approximation;
// actual size is validated at startup after the first successful Embed call.
func (p *OllamaProvider) VectorSize() int {
	return OllamaDefaultVectorSize
}

// Ping verifies the Ollama server is reachable by calling GET /api/tags.
func (p *OllamaProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("FATAL: embedding provider ollama unreachable at %s — is Ollama running?", p.BaseURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping returned %d", resp.StatusCode)
	}
	return nil
}
