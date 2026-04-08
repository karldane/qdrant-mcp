package qdrant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// GenerateUserJWT creates a short-lived HS256 JWT that restricts access to a
// single named Qdrant collection. The JWT is signed with adminKey, which is
// also the value Qdrant uses as its API key / JWT signing secret.
//
// No third-party library is required — this uses only the Go standard library.
func GenerateUserJWT(adminKey, username, collection string) (string, error) {
	header := base64url(mustJSON(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}))

	payload := base64url(mustJSON(map[string]interface{}{
		"sub": username,
		"exp": time.Now().Add(time.Hour).Unix(),
		"access": map[string]interface{}{
			"collections": map[string]interface{}{
				collection: []string{"read", "write"},
			},
		},
	}))

	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, []byte(adminKey))
	if _, err := mac.Write([]byte(unsigned)); err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	sig := base64url(mac.Sum(nil))

	return unsigned + "." + sig, nil
}

// base64url encodes b using base64 URL encoding with no padding.
func base64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// mustJSON marshals v to JSON and panics on error.
// Only used with known-good types (map literals); will never fail in practice.
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return b
}
