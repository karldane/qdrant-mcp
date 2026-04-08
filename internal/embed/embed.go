package embed

import (
	"context"
	"fmt"
)

// Provider is the interface all embedding backends must implement.
// Tools call Embed to get a vector for a piece of text; the vector is then
// stored in or searched against Qdrant.
type Provider interface {
	// Embed converts text to a float vector.
	// Returns an error if the provider is unavailable or the model rejects input.
	Embed(ctx context.Context, text string) ([]float64, error)

	// VectorSize returns the dimension of vectors this provider produces.
	// Used at startup to validate against QDRANT_VECTOR_SIZE.
	VectorSize() int
}

// Pinger is an optional interface that providers may implement for startup
// health checks. factory.go calls Ping if the provider satisfies this interface.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NoOpProvider is used when EMBEDDING_PROVIDER=none or is unset.
// All calls to Embed fail with a descriptive error that tells the agent
// what to do.
type NoOpProvider struct{}

func (n *NoOpProvider) Embed(_ context.Context, _ string) ([]float64, error) {
	return nil, fmt.Errorf("no embedding provider configured: supply a pre-computed vector or set EMBEDDING_PROVIDER=ollama")
}

func (n *NoOpProvider) VectorSize() int { return 0 }
