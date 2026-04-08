package qdrant

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	qdrantclient "github.com/qdrant/go-client/qdrant"
)

// NewAdminClient creates a Qdrant client authenticated with the admin API key.
// It is used only during provisioning (EnsureCollection, EnsureIndexes) and
// then discarded — it must never be passed to tool handlers.
func NewAdminClient(adminURL, adminKey string) (*qdrantclient.Client, error) {
	host, port, useTLS, err := parseURL(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse admin URL: %w", err)
	}
	return qdrantclient.NewClient(&qdrantclient.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 adminKey,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	})
}

// NewUserClient creates a Qdrant client authenticated with a scoped JWT.
// All tool traffic uses this client — it has access only to the user's
// own collection.
func NewUserClient(adminURL, jwt string) (*qdrantclient.Client, error) {
	host, port, useTLS, err := parseURL(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse admin URL: %w", err)
	}
	return qdrantclient.NewClient(&qdrantclient.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 jwt,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
	})
}

// parseURL extracts host, port, and TLS flag from a URL string.
// Accepts e.g. "http://localhost:6334" or "https://example.com:6334".
// Falls back to port 6334 if not specified.
func parseURL(rawURL string) (host string, port int, useTLS bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, false, err
	}
	host = u.Hostname()
	if host == "" {
		host = rawURL // treat as bare host
	}
	useTLS = u.Scheme == "https"
	portStr := u.Port()
	if portStr == "" {
		port = 6334
	} else {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return "", 0, false, fmt.Errorf("invalid port %q: %w", portStr, err)
		}
	}
	return host, port, useTLS, nil
}

// PingUserClient attempts a lightweight operation to verify the JWT is accepted.
// Uses GetCollectionInfo on the user's collection; the JWT must allow read access.
func PingUserClient(ctx context.Context, c *qdrantclient.Client, collection string) error {
	_, err := c.GetCollectionInfo(ctx, collection)
	if err != nil {
		return fmt.Errorf("user client ping failed: %w", err)
	}
	return nil
}
