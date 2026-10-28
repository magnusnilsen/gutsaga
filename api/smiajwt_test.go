package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSmiaCallerJWT(t *testing.T) {
	key := p256Key(t)
	other := p256Key(t)
	var fetches atomic.Int32
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != smiaJWKSPath {
			http.NotFound(w, r)
			return
		}
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []any{ecJWK("test-key", &key.PublicKey)},
		})
	}))
	t.Cleanup(jwks.Close)

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	aud := "https://sagalore.example"
	v := NewSmiaCallerVerifier(jwks.URL, aud)
	v.Now = func() time.Time { return now }

	none := unsignedToken(t, "test-key", baseClaims(jwks.URL, aud, "none", now))
	if _, err := v.Verify(callerReq(none, "")); !errors.Is(err, errCallerAuth) || fetches.Load() != 0 {
		t.Fatalf("alg none err=%v fetches=%d", err, fetches.Load())
	}

	good := signES256(t, key, "test-key", baseClaims(jwks.URL, aud, "jti-valid", now))
	id, err := v.Verify(callerReq(good, "ignored@example.com"))
	if err != nil || id.Provider != smiaProvider || id.Subject != "user-1" || id.Email != "user@example.com" || fetches.Load() != 1 {
		t.Fatalf("valid id=%+v err=%v fetches=%d", id, err, fetches.Load())
	}
	if _, err := v.Verify(callerReq(good, "")); !errors.Is(err, errSmiaReplay) || fetches.Load() != 1 {
		t.Fatalf("replay err=%v fetches=%d", err, fetches.Load())
	}

	again := signES256(t, key, "test-key", baseClaims(jwks.URL, aud, "jti-again", now))
	if _, err := v.Verify(callerReq(again, "")); err != nil || fetches.Load() != 1 {
		t.Fatalf("cached key err=%v fetches=%d", err, fetches.Load())
	}

	unknown := signES256(t, key, "missing-key", baseClaims(jwks.URL, aud, "jti-unknown", now))
	if _, err := v.Verify(callerReq(unknown, "")); !errors.Is(err, errSmiaKid) || fetches.Load() != 2 {
		t.Fatalf("unknown kid err=%v fetches=%d", err, fetches.Load())
	}

	bad := signES256(t, other, "test-key", baseClaims(jwks.URL, aud, "jti-badsig", now))
	if _, err := v.Verify(callerReq(bad, "")); !errors.Is(err, errSmiaSig) || fetches.Load() != 2 {
		t.Fatalf("bad sig err=%v fetches=%d", err, fetches.Load())
	}

	wrongAud := signES256(t, key, "test-key", baseClaims(jwks.URL, "https://evil.example", "jti-aud", now))
	if _, err := v.Verify(callerReq(wrongAud, "")); !errors.Is(err, errSmiaAud) {
		t.Fatalf("wrong aud err=%v", err)
	}
	// A rejected token must not consume jti.
	recovered := signES256(t, key, "test-key", baseClaims(jwks.URL, aud, "jti-aud", now))
	if _, err := v.Verify(callerReq(recovered, "")); err != nil {
		t.Fatalf("jti after bad aud: %v", err)
	}
	if _, err := v.Verify(callerReq(recovered, "")); !errors.Is(err, errSmiaReplay) {
		t.Fatalf("replay after recover err=%v", err)
	}

	expiredClaims := baseClaims(jwks.URL, aud, "jti-expired", now)
	expiredClaims["iat"] = now.Add(-6 * time.Minute).Unix()
	expiredClaims["exp"] = now.Add(-time.Minute).Unix()
	expired := signES256(t, key, "test-key", expiredClaims)
	if _, err := v.Verify(callerReq(expired, "")); !errors.Is(err, errSmiaExpired) {
		t.Fatalf("expired err=%v", err)
	}

	skewClaims := baseClaims(jwks.URL, aud, "jti-skew", now)
	skewClaims["iat"] = now.Add(-4*time.Minute - 10*time.Second).Unix()
	skewClaims["exp"] = now.Add(-10 * time.Second).Unix()
	if _, err := v.Verify(callerReq(signES256(t, key, "test-key", skewClaims), "")); err != nil {
		t.Fatalf("within skew: %v", err)
	}
}

func TestMCPCallerJWTPrecedence(t *testing.T) {
	key := p256Key(t)
	var fetches atomic.Int32
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{ecJWK("test-key", &key.PublicKey)}})
	}))
	t.Cleanup(jwks.Close)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	v := NewSmiaCallerVerifier(jwks.URL, "https://sagalore.example")
	v.Now = func() time.Time { return now }
	mcp := &MCPCallerVerifier{
		Headers: SharedSecretVerifier{Secret: "svc-secret", Provider: smiaProvider},
		Smia:    v,
	}
	api := &Server{mcpServiceSecret: "svc-secret", callerVerifier: mcp}

	token := signES256(t, key, "test-key", baseClaims(jwks.URL, "https://sagalore.example", "jti-pref", now))
	req := authedReq(token, "svc-secret", "user-header", "header@example.com")
	id, err := api.verifiedCaller(req)
	if err != nil || id.Subject != "user-1" || id.Email != "user@example.com" {
		t.Fatalf("jwt wins %+v %v", id, err)
	}

	bad := signES256(t, p256Key(t), "test-key", baseClaims(jwks.URL, "https://sagalore.example", "jti-bad", now))
	if _, err := api.verifiedCaller(authedReq(bad, "svc-secret", "user-header", "header@example.com")); err == nil {
		t.Fatal("bad jwt fell through to headers")
	}

	headerOnly := authedReq("", "svc-secret", "user-header", "header@example.com")
	id, err = api.verifiedCaller(headerOnly)
	if err != nil || id.Subject != "user-header" || id.Email != "header@example.com" {
		t.Fatalf("headers %+v %v", id, err)
	}

	mcp.RequireSmiaJWT = true
	if _, err := api.verifiedCaller(authedReq("", "svc-secret", "user-header", "header@example.com")); err == nil {
		t.Fatal("require jwt accepted headers")
	}
	still := signES256(t, key, "test-key", baseClaims(jwks.URL, "https://sagalore.example", "jti-still", now))
	id, err = api.verifiedCaller(authedReq(still, "svc-secret", "user-header", ""))
	if err != nil || id.Subject != "user-1" {
		t.Fatalf("require jwt still accepts token %+v %v", id, err)
	}

	before := fetches.Load()
	if _, err := api.verifiedCaller(authedReq(still, "nope", "user-header", "")); err == nil || fetches.Load() != before {
		t.Fatalf("missing secret err=%v fetches %d→%d", err, before, fetches.Load())
	}
}

func callerReq(token, email string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://sagalore.example/v1/groups", nil)
	if token != "" {
		r.Header.Set(headerSmiaCaller, token)
	}
	if email != "" {
		r.Header.Set(headerCallerEmail, email)
	}
	r.Header.Set(headerCallerSubject, "header-subject")
	return r
}

func authedReq(token, secret, subject, email string) *http.Request {
	r := callerReq(token, email)
	r.Header.Set(headerCallerSubject, subject)
	if secret != "" {
		r.Header.Set("Authorization", "Bearer "+secret)
	}
	return r
}

func baseClaims(iss, aud, jti string, now time.Time) map[string]any {
	return map[string]any{
		"iss":        iss,
		"sub":        "user-1",
		"email":      "user@example.com",
		"hub":        "sagalore",
		"listing_id": "list-1",
		"tool":       "list_groups",
		"aud":        aud,
		"iat":        now.Unix(),
		"exp":        now.Add(4 * time.Minute).Unix(),
		"jti":        jti,
	}
}

func p256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func ecJWK(kid string, pub *ecdsa.PublicKey) map[string]string {
	return map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"kid": kid,
		"use": "sig",
		"alg": "ES256",
		"x":   base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, 32))),
		"y":   base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, 32))),
	}
}

func signES256(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(map[string]string{"alg": "ES256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	h := base64.RawURLEncoding.EncodeToString(hb)
	p := base64.RawURLEncoding.EncodeToString(pb)
	sum := sha256.Sum256([]byte(h + "." + p))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func unsignedToken(t *testing.T, kid string, claims map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(map[string]string{"alg": "none", "kid": kid})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	// Three segments so the split passes, then alg is rejected. A trailing dot
	// would be an empty signature and fail earlier.
	sig := base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	return base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb) + "." + sig
}
