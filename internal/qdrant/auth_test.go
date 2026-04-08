package qdrant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeSegment base64url-decodes a JWT segment.
func decodeSegment(t *testing.T, seg string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(seg)
	require.NoError(t, err, "base64url decode segment")
	return b
}

func TestGenerateUserJWT_structure(t *testing.T) {
	tok, err := GenerateUserJWT("adminkey", "alice@example.com", "alice_at_example_com")
	require.NoError(t, err)

	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3, "JWT must have exactly three dot-separated parts")

	// Each part must be non-empty and valid base64url.
	for i, part := range parts {
		assert.NotEmpty(t, part, "JWT part %d must not be empty", i)
		_, err := base64.RawURLEncoding.DecodeString(part)
		assert.NoError(t, err, "JWT part %d must be valid base64url", i)
	}

	// Header must declare HS256 / JWT.
	var header map[string]string
	require.NoError(t, json.Unmarshal(decodeSegment(t, parts[0]), &header))
	assert.Equal(t, "HS256", header["alg"])
	assert.Equal(t, "JWT", header["typ"])
}

func TestGenerateUserJWT_claims(t *testing.T) {
	tok, err := GenerateUserJWT("adminkey", "alice@example.com", "alice_at_example_com")
	require.NoError(t, err)

	parts := strings.Split(tok, ".")
	var claims map[string]interface{}
	require.NoError(t, json.Unmarshal(decodeSegment(t, parts[1]), &claims))

	// sub must match the supplied username.
	assert.Equal(t, "alice@example.com", claims["sub"])

	// access.collections must be scoped to the supplied collection only.
	access, ok := claims["access"].(map[string]interface{})
	require.True(t, ok, "access claim must be a JSON object")

	collections, ok := access["collections"].(map[string]interface{})
	require.True(t, ok, "access.collections must be a JSON object")

	perms, ok := collections["alice_at_example_com"].([]interface{})
	require.True(t, ok, "access.collections[collection] must be an array")
	assert.Len(t, perms, 2)

	permStrs := make([]string, len(perms))
	for i, p := range perms {
		permStrs[i] = p.(string)
	}
	assert.ElementsMatch(t, []string{"read", "write"}, permStrs)
}

func TestGenerateUserJWT_signature(t *testing.T) {
	const adminKey = "supersecretkey"
	tok, err := GenerateUserJWT(adminKey, "bob@example.com", "bob_at_example_com")
	require.NoError(t, err)

	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)

	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(adminKey))
	mac.Write([]byte(unsigned))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expectedSig, parts[2], "HMAC-SHA256 signature must match")
}

func TestGenerateUserJWT_expiry(t *testing.T) {
	before := time.Now()
	tok, err := GenerateUserJWT("key", "user@example.com", "user_at_example_com")
	after := time.Now()
	require.NoError(t, err)

	parts := strings.Split(tok, ".")
	var claims map[string]interface{}
	require.NoError(t, json.Unmarshal(decodeSegment(t, parts[1]), &claims))

	expRaw, ok := claims["exp"].(float64)
	require.True(t, ok, "exp claim must be a number")

	expTime := time.Unix(int64(expRaw), 0)

	expectedLow := before.Add(time.Hour).Add(-5 * time.Second)
	expectedHigh := after.Add(time.Hour).Add(5 * time.Second)

	assert.True(t, expTime.After(expectedLow), "exp should be ~now+1h (got %v, want after %v)", expTime, expectedLow)
	assert.True(t, expTime.Before(expectedHigh), "exp should be ~now+1h (got %v, want before %v)", expTime, expectedHigh)
}
