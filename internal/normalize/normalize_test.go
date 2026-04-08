package normalize

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPoint(t *testing.T) {
	raw := map[string]interface{}{
		"id":      "point-123",
		"vector":  []float32{0.1, 0.2, 0.3},
		"payload": map[string]interface{}{"type": "test"},
		"score":   float32(0.95),
	}

	p := NewPoint(raw)
	assert.Equal(t, "point-123", p.ID)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, p.Vector)
	assert.Equal(t, map[string]interface{}{"type": "test"}, p.Payload)
	assert.Equal(t, float32(0.95), p.Score)
}

func TestMemory(t *testing.T) {
	raw := map[string]interface{}{
		"id":         "mem-123",
		"content":    "This is a memory",
		"metadata":   map[string]interface{}{"source": "user"},
		"tags":       []string{"important", "work"},
		"created_at": time.Now(),
	}

	m := NewMemory(raw)
	assert.Equal(t, "mem-123", m.ID)
	assert.Equal(t, "This is a memory", m.Content)
	assert.Equal(t, map[string]interface{}{"source": "user"}, m.Metadata)
	assert.Equal(t, []string{"important", "work"}, m.Tags)
}

func TestSession(t *testing.T) {
	raw := map[string]interface{}{
		"id":     "session-123",
		"name":   "My Session",
		"state":  map[string]interface{}{"counter": 42},
		"active": true,
	}

	s := NewSession(raw)
	assert.Equal(t, "session-123", s.ID)
	assert.Equal(t, "My Session", s.Name)
	assert.Equal(t, map[string]interface{}{"counter": 42}, s.State)
	assert.True(t, s.Active)
}

func TestCacheEntry(t *testing.T) {
	raw := map[string]interface{}{
		"key":        "cache-key",
		"value":      map[string]interface{}{"result": "cached"},
		"created_at": time.Now(),
	}

	c := NewCacheEntry(raw)
	assert.Equal(t, "cache-key", c.Key)
	assert.Equal(t, map[string]interface{}{"result": "cached"}, c.Value)
}
