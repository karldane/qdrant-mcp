package normalize

import "time"

type Point struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector,omitempty"`
	Payload map[string]interface{} `json:"payload"`
	Score   float32                `json:"score,omitempty"`
}

type Memory struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Score     float32                `json:"score,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
	Tags      []string               `json:"tags,omitempty"`
	TTL       *time.Time             `json:"ttl,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type Session struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	State     map[string]interface{} `json:"state"`
	Active    bool                   `json:"active"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type CacheEntry struct {
	Key       string                 `json:"key"`
	Value     map[string]interface{} `json:"value"`
	TTL       *time.Time             `json:"ttl,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

func NewPoint(raw map[string]interface{}) *Point {
	p := &Point{
		Payload: make(map[string]interface{}),
	}
	if v, ok := raw["id"].(string); ok {
		p.ID = v
	}
	if v, ok := raw["vector"].([]float32); ok {
		p.Vector = v
	}
	if v, ok := raw["payload"].(map[string]interface{}); ok {
		p.Payload = v
	}
	if v, ok := raw["score"].(float32); ok {
		p.Score = v
	}
	return p
}

func NewMemory(raw map[string]interface{}) *Memory {
	m := &Memory{
		Metadata: make(map[string]interface{}),
	}
	if v, ok := raw["id"].(string); ok {
		m.ID = v
	}
	if v, ok := raw["content"].(string); ok {
		m.Content = v
	}
	if v, ok := raw["metadata"].(map[string]interface{}); ok {
		m.Metadata = v
	}
	if v, ok := raw["tags"].([]string); ok {
		m.Tags = v
	}
	if v, ok := raw["created_at"].(time.Time); ok {
		m.CreatedAt = v
	}
	return m
}

func NewSession(raw map[string]interface{}) *Session {
	s := &Session{
		State: make(map[string]interface{}),
	}
	if v, ok := raw["id"].(string); ok {
		s.ID = v
	}
	if v, ok := raw["name"].(string); ok {
		s.Name = v
	}
	if v, ok := raw["state"].(map[string]interface{}); ok {
		s.State = v
	}
	if v, ok := raw["active"].(bool); ok {
		s.Active = v
	}
	return s
}

func NewCacheEntry(raw map[string]interface{}) *CacheEntry {
	c := &CacheEntry{
		Value: make(map[string]interface{}),
	}
	if v, ok := raw["key"].(string); ok {
		c.Key = v
	}
	if v, ok := raw["value"].(map[string]interface{}); ok {
		c.Value = v
	}
	if v, ok := raw["created_at"].(time.Time); ok {
		c.CreatedAt = v
	}
	return c
}
