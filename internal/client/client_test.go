package client

import (
	"context"
	"testing"

	"github.com/karldane/qdrant-mcp/internal/config"
	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		wantHost string
		wantPort int
		wantTLS  bool
		wantErr  bool
	}{
		{
			name:     "http with explicit port",
			rawURL:   "http://localhost:6334",
			wantHost: "localhost",
			wantPort: 6334,
			wantTLS:  false,
		},
		{
			name:     "https with explicit port",
			rawURL:   "https://qdrant.example.com:6334",
			wantHost: "qdrant.example.com",
			wantPort: 6334,
			wantTLS:  true,
		},
		{
			name:     "https without port defaults to 6334",
			rawURL:   "https://qdrant.example.com",
			wantHost: "qdrant.example.com",
			wantPort: 6334,
			wantTLS:  true,
		},
		{
			name:     "http without port defaults to 6334",
			rawURL:   "http://localhost",
			wantHost: "localhost",
			wantPort: 6334,
			wantTLS:  false,
		},
		{
			name:     "non-standard port",
			rawURL:   "http://localhost:9999",
			wantHost: "localhost",
			wantPort: 9999,
			wantTLS:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, tls, err := parseURL(tt.rawURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
			assert.Equal(t, tt.wantTLS, tls)
		})
	}
}

// ---------------------------------------------------------------------------
// New()
// ---------------------------------------------------------------------------

func TestNew_MissingAdminURL(t *testing.T) {
	cfg := &config.Config{}
	_, err := New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "QDRANT_ADMIN_URL")
}

func TestNew_InvalidAdminURL(t *testing.T) {
	cfg := &config.Config{
		AdminURL:   "://bad-url",
		Collection: "test",
		VectorSize: 768,
	}
	_, err := New(cfg)
	require.Error(t, err)
}

func TestNew_ValidURL_NoLiveServer(t *testing.T) {
	// With SkipCompatibilityCheck=true (set inside New), this should succeed
	// at construction time even with no Qdrant server running.
	cfg := &config.Config{
		AdminURL:   "http://localhost:19999",
		Collection: "test",
		VectorSize: 768,
	}
	c, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNew_ValidURL_UsesAdminKey(t *testing.T) {
	// v2: user client always uses the admin key (JWT issuance happens in
	// internal/qdrant, not here). Construction must succeed without a live server.
	cfg := &config.Config{
		AdminURL:   "http://localhost:19999",
		AdminKey:   "my-admin-key",
		Username:   "user@example.com",
		Collection: "test",
		VectorSize: 768,
	}
	c, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.NotNil(t, c.userClient)
}

// ---------------------------------------------------------------------------
// filterToQdrant
// ---------------------------------------------------------------------------

func TestFilterToQdrant_Nil(t *testing.T) {
	result := filterToQdrant(nil)
	assert.Nil(t, result)
}

func TestFilterToQdrant_SingleField(t *testing.T) {
	filter := map[string]interface{}{"type": "memory"}
	conds := filterToQdrant(filter)
	require.Len(t, conds, 1)

	field := conds[0].GetField()
	require.NotNil(t, field)
	assert.Equal(t, "type", field.Key)
	assert.Equal(t, "memory", field.Match.GetKeyword())
}

func TestFilterToQdrant_MultipleFields(t *testing.T) {
	filter := map[string]interface{}{
		"type":   "session",
		"active": true,
	}
	conds := filterToQdrant(filter)
	assert.Len(t, conds, 2)
}

// ---------------------------------------------------------------------------
// pointIDToString
// ---------------------------------------------------------------------------

func TestPointIDToString_Nil(t *testing.T) {
	assert.Equal(t, "", pointIDToString(nil))
}

func TestPointIDToString_UUID(t *testing.T) {
	id := &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "abc-123"}}
	assert.Equal(t, "abc-123", pointIDToString(id))
}

func TestPointIDToString_Num(t *testing.T) {
	id := &qdrant.PointId{PointIdOptions: &qdrant.PointId_Num{Num: 42}}
	assert.Equal(t, "42", pointIDToString(id))
}

// ---------------------------------------------------------------------------
// valueToInterface / valueMapToInterface
// ---------------------------------------------------------------------------

func TestValueToInterface_Nil(t *testing.T) {
	assert.Nil(t, valueToInterface(nil))
}

func TestValueToInterface_NullValue(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_NullValue{}}
	assert.Nil(t, valueToInterface(v))
}

func TestValueToInterface_Bool(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: true}}
	assert.Equal(t, true, valueToInterface(v))
}

func TestValueToInterface_Integer(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 99}}
	assert.Equal(t, int64(99), valueToInterface(v))
}

func TestValueToInterface_Double(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: 3.14}}
	assert.InDelta(t, 3.14, valueToInterface(v), 1e-6)
}

func TestValueToInterface_String(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: "hello"}}
	assert.Equal(t, "hello", valueToInterface(v))
}

func TestValueToInterface_StructNil(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_StructValue{StructValue: nil}}
	assert.Nil(t, valueToInterface(v))
}

func TestValueToInterface_Struct(t *testing.T) {
	v := &qdrant.Value{
		Kind: &qdrant.Value_StructValue{
			StructValue: &qdrant.Struct{
				Fields: map[string]*qdrant.Value{
					"key": {Kind: &qdrant.Value_StringValue{StringValue: "val"}},
				},
			},
		},
	}
	result := valueToInterface(v)
	m, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "val", m["key"])
}

func TestValueToInterface_ListNil(t *testing.T) {
	v := &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: nil}}
	assert.Nil(t, valueToInterface(v))
}

func TestValueToInterface_List(t *testing.T) {
	v := &qdrant.Value{
		Kind: &qdrant.Value_ListValue{
			ListValue: &qdrant.ListValue{
				Values: []*qdrant.Value{
					{Kind: &qdrant.Value_StringValue{StringValue: "a"}},
					{Kind: &qdrant.Value_IntegerValue{IntegerValue: 1}},
				},
			},
		},
	}
	result := valueToInterface(v)
	list, ok := result.([]interface{})
	require.True(t, ok)
	assert.Len(t, list, 2)
	assert.Equal(t, "a", list[0])
	assert.Equal(t, int64(1), list[1])
}

func TestValueMapToInterface_Nil(t *testing.T) {
	assert.Nil(t, valueMapToInterface(nil))
}

func TestValueMapToInterface_Values(t *testing.T) {
	m := map[string]*qdrant.Value{
		"name": {Kind: &qdrant.Value_StringValue{StringValue: "test"}},
		"num":  {Kind: &qdrant.Value_IntegerValue{IntegerValue: 7}},
	}
	result := valueMapToInterface(m)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, int64(7), result["num"])
}

// ---------------------------------------------------------------------------
// UpsertPoint — early vector validation
// ---------------------------------------------------------------------------

func TestUpsertPoint_RequiresVector(t *testing.T) {
	// UpsertPoint must return a clear error before calling Qdrant when no vector is provided.
	// We use a Client constructed against a non-existent server; the early validation
	// fires before any network call is made, so no live server is needed.
	cfg := &config.Config{
		AdminURL:   "http://localhost:19999",
		Collection: "test",
		VectorSize: 768,
	}
	c, err := New(cfg)
	require.NoError(t, err)

	err = c.UpsertPoint(context.Background(), "550e8400-e29b-41d4-a716-446655440000", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vector is required")

	err = c.UpsertPoint(context.Background(), "550e8400-e29b-41d4-a716-446655440000", []float64{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vector is required")
}
