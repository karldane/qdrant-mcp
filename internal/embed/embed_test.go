package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func ollamaEmbeddingServer(t *testing.T, statusCode int, embedding []float64, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, body, statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"embedding": embedding})
	}))
}

func ollamaTagsServer(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, "error", statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
	}))
}

func openAIEmbeddingServer(t *testing.T, statusCode int, embedding []float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode == http.StatusUnauthorized {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if statusCode != http.StatusOK {
			http.Error(w, "error", statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"embedding": embedding},
			},
		})
	}))
}

// ---------------------------------------------------------------------------
// NoOpProvider
// ---------------------------------------------------------------------------

func TestNoOpProvider_Embed(t *testing.T) {
	p := &NoOpProvider{}
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embedding provider configured")
	assert.Equal(t, 0, p.VectorSize())
}

// ---------------------------------------------------------------------------
// OllamaProvider
// ---------------------------------------------------------------------------

func TestOllamaProvider_Embed_success(t *testing.T) {
	want := []float64{0.1, 0.2, 0.3}
	srv := ollamaEmbeddingServer(t, http.StatusOK, want, "")
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "nomic-embed-text")
	got, err := p.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOllamaProvider_Embed_modelNotFound(t *testing.T) {
	srv := ollamaEmbeddingServer(t, http.StatusNotFound, nil, "model not found")
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "unknown-model")
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama returned 404")
}

func TestOllamaProvider_Embed_serverDown(t *testing.T) {
	// Point at a port nothing is listening on.
	p := NewOllamaProvider("http://127.0.0.1:19998", "nomic-embed-text")
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama unreachable")
}

func TestOllamaProvider_Ping_success(t *testing.T) {
	srv := ollamaTagsServer(t, http.StatusOK)
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, "nomic-embed-text")
	err := p.Ping(context.Background())
	require.NoError(t, err)
}

func TestOllamaProvider_Ping_failure(t *testing.T) {
	p := NewOllamaProvider("http://127.0.0.1:19998", "nomic-embed-text")
	err := p.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama unreachable")
}

func TestOllamaProvider_VectorSize(t *testing.T) {
	p := NewOllamaProvider("", "")
	assert.Equal(t, OllamaDefaultVectorSize, p.VectorSize())
}

// ---------------------------------------------------------------------------
// OpenAIProvider
// ---------------------------------------------------------------------------

func TestOpenAIProvider_Embed_success(t *testing.T) {
	want := []float64{0.5, 0.6, 0.7}
	srv := openAIEmbeddingServer(t, http.StatusOK, want)
	defer srv.Close()

	p := NewOpenAIProvider("sk-test", srv.URL, "text-embedding-3-small")
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOpenAIProvider_Embed_authError(t *testing.T) {
	srv := openAIEmbeddingServer(t, http.StatusUnauthorized, nil)
	defer srv.Close()

	p := NewOpenAIProvider("bad-key", srv.URL, "text-embedding-3-small")
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai auth error")
}

func TestOpenAIProvider_VectorSize(t *testing.T) {
	p := NewOpenAIProvider("k", "", "")
	assert.Equal(t, OpenAIDefaultVectorSize, p.VectorSize())
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

type stubCfg struct {
	provider  string
	model     string
	ollamaURL string
	openaiKey string
	baseURL   string
}

func (s *stubCfg) GetEmbeddingProvider() string { return s.provider }
func (s *stubCfg) GetEmbeddingModel() string    { return s.model }
func (s *stubCfg) GetOllamaURL() string         { return s.ollamaURL }
func (s *stubCfg) GetOpenAIKey() string         { return s.openaiKey }
func (s *stubCfg) GetOpenAIBaseURL() string     { return s.baseURL }

func TestNewProvider_ollama(t *testing.T) {
	p, err := NewProvider(&stubCfg{provider: "ollama"})
	require.NoError(t, err)
	_, ok := p.(*OllamaProvider)
	assert.True(t, ok, "expected *OllamaProvider")
}

func TestNewProvider_openai(t *testing.T) {
	p, err := NewProvider(&stubCfg{provider: "openai", openaiKey: "sk-test"})
	require.NoError(t, err)
	_, ok := p.(*OpenAIProvider)
	assert.True(t, ok, "expected *OpenAIProvider")
}

func TestNewProvider_openai_missingKey(t *testing.T) {
	_, err := NewProvider(&stubCfg{provider: "openai", openaiKey: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestNewProvider_none(t *testing.T) {
	p, err := NewProvider(&stubCfg{provider: "none"})
	require.NoError(t, err)
	_, ok := p.(*NoOpProvider)
	assert.True(t, ok, "expected *NoOpProvider")
}

func TestNewProvider_empty(t *testing.T) {
	p, err := NewProvider(&stubCfg{provider: ""})
	require.NoError(t, err)
	_, ok := p.(*NoOpProvider)
	assert.True(t, ok, "expected *NoOpProvider for empty provider")
}

func TestNewProvider_unknown(t *testing.T) {
	_, err := NewProvider(&stubCfg{provider: "gpt4all"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown embedding provider")
}
