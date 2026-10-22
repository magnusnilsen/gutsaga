// Package smia verifies a Smia access token. The token is opaque: there is no
// userinfo endpoint and no JWKS. MCPVerifier calls the hub MCP and trusts only
// that response. A future OIDC "Sign in with Smia" verifier can implement
// Verifier and replace MCPVerifier without changing session issuance.
package smia

import (
	"context"
	"crypto/subtle"
	"errors"
)

const (
	// Hub and Server are the GutSaga access rule.
	Hub    = "wow"
	Server = "mcp"
)

var (
	// ErrUnauthorized means Smia rejected the token.
	ErrUnauthorized = errors.New("smia token rejected")
	// ErrUnavailable means Smia could not be asked, so no session is issued.
	ErrUnavailable = errors.New("smia mcp unavailable")
	// ErrNotConfigured means this process has no verifier credentials.
	ErrNotConfigured = errors.New("smia auth is not configured")
)

// Identity is what Smia itself reported. Callers must not accept these fields
// from the companion.
type Identity struct {
	Subject     string
	Email       string
	DisplayName string
	Member      bool
	Hub         string
	Server      string
}

// Verifier checks one opaque Smia access token.
type Verifier interface {
	Verify(ctx context.Context, accessToken string) (Identity, error)
}

// Cache stores a verification for a few minutes. The key is a hash, never the token.
type Cache interface {
	Get(ctx context.Context, tokenHash string) (Identity, bool, error)
	Put(ctx context.Context, tokenHash string, id Identity, until int64) error
}

// DevVerifier accepts one static token and reports a member of hub wow.
// Every caller of that token is the same player. Production uses MCPVerifier.
type DevVerifier struct {
	Token string
}

func (d DevVerifier) Verify(_ context.Context, accessToken string) (Identity, error) {
	if d.Token == "" {
		return Identity{}, ErrNotConfigured
	}
	if accessToken == "" || subtle.ConstantTimeCompare([]byte(accessToken), []byte(d.Token)) != 1 {
		return Identity{}, ErrUnauthorized
	}
	return Identity{
		Subject:     "dev",
		Email:       "dev@localhost",
		DisplayName: "Utvikler",
		Member:      true,
		Hub:         Hub,
		Server:      Server,
	}, nil
}
