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
	OpenAIDefaultModel      = "text-embedding-3-small"
	OpenAIDefaultBaseURL    = "https://api.openai.com/v1"
	OpenAIDefaultVectorSize = 1536
)

// OpenAIProvider calls the OpenAI (or compatible) embeddings API.
type OpenAIProvider struct {
	APIKey  string
	BaseURL string
	Model   string
	client  *http.Client
}

// NewOpenAIProvider creates an OpenAIProvider with sensible defaults.
func NewOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = OpenAIDefaultBaseURL
	}
	if model == "" {
		model = OpenAIDefaultModel
	}
	return &OpenAIProvider{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed sends text to POST /v1/embeddings and returns the embedding vector.
func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(map[string]string{
		"model": p.Model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("openai auth error: check OPENAI_API_KEY")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}
	return result.Data[0].Embedding, nil
}

// VectorSize returns the default vector size for text-embedding-3-small.
func (p *OpenAIProvider) VectorSize() int {
	return OpenAIDefaultVectorSize
}

// Ping makes a minimal test embedding call to verify the API key and model work.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	_, err := p.Embed(ctx, "ping")
	if err != nil {
		return fmt.Errorf("FATAL: openai provider unreachable or misconfigured: %w", err)
	}
	return nil
}
