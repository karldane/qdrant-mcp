package qdrant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURL_HTTPWithPort(t *testing.T) {
	host, port, tls, err := parseURL("http://localhost:6334")
	require.NoError(t, err)
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 6334, port)
	assert.False(t, tls)
}

func TestParseURL_HTTPSWithPort(t *testing.T) {
	host, port, tls, err := parseURL("https://qdrant.example.com:6334")
	require.NoError(t, err)
	assert.Equal(t, "qdrant.example.com", host)
	assert.Equal(t, 6334, port)
	assert.True(t, tls)
}

func TestParseURL_DefaultPort(t *testing.T) {
	host, port, tls, err := parseURL("http://localhost")
	require.NoError(t, err)
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 6334, port)
	assert.False(t, tls)
}

func TestParseURL_NonStandardPort(t *testing.T) {
	host, port, _, err := parseURL("http://localhost:9999")
	require.NoError(t, err)
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 9999, port)
}

func TestNewAdminClient_ValidURL(t *testing.T) {
	// SkipCompatibilityCheck=true means construction succeeds even with no live server.
	c, err := NewAdminClient("http://localhost:19999", "admin-key")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNewAdminClient_EmptyURL(t *testing.T) {
	// Empty URL still parses but gives an empty hostname.
	// Construction should not panic.
	_, err := NewAdminClient("http://localhost:19999", "")
	require.NoError(t, err)
}

func TestNewUserClient_ValidURL(t *testing.T) {
	c, err := NewUserClient("http://localhost:19999", "some.jwt.token")
	require.NoError(t, err)
	assert.NotNil(t, c)
}
