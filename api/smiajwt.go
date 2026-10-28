package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	headerSmiaCaller = "X-Smia-Caller"
	smiaJWKSPath     = "/.well-known/smia-caller-jwks.json"
	smiaDefaultISS   = "https://app.smia.io"
	smiaMaxLife      = 5 * time.Minute
	smiaClockSkew    = 30 * time.Second
	smiaTokenMax     = 8 << 10
)

var (
	errSmiaAud     = errors.New("smia audience")
	errSmiaExpired = errors.New("smia token expired")
	errSmiaSig     = errors.New("smia signature")
	errSmiaKid     = errors.New("smia unknown kid")
	errSmiaReplay  = errors.New("smia replayed jti")
)

// SmiaCallerVerifier checks an X-Smia-Caller JWT (ES256, P-256).
// sub is the Smia user id. email is stored on the identity link.
// JWKS is {issuer}/.well-known/smia-caller-jwks.json, cached until an unknown kid.
type SmiaCallerVerifier struct {
	Issuer   string
	Audience string
	JWKSURL  string
	Client   *http.Client
	Now      func() time.Time
	Skew     time.Duration
	MaxLife  time.Duration

	mu   sync.Mutex
	keys map[string]*ecdsa.PublicKey
	seen map[string]time.Time
}

func NewSmiaCallerVerifier(issuer, audience string) *SmiaCallerVerifier {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	return &SmiaCallerVerifier{
		Issuer:   issuer,
		Audience: publicOrigin(audience),
		JWKSURL:  issuer + smiaJWKSPath,
		Client:   &http.Client{Timeout: 10 * time.Second},
		Skew:     smiaClockSkew,
		MaxLife:  smiaMaxLife,
		keys:     map[string]*ecdsa.PublicKey{},
		seen:     map[string]time.Time{},
	}
}

func (v *SmiaCallerVerifier) now() time.Time {
	if v != nil && v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *SmiaCallerVerifier) Verify(r *http.Request) (CallerIdentity, error) {
	if v == nil || strings.TrimSpace(v.Issuer) == "" || strings.TrimSpace(v.Audience) == "" || strings.TrimSpace(v.JWKSURL) == "" {
		return CallerIdentity{}, errCallerAuth
	}
	raw := strings.TrimSpace(r.Header.Get(headerSmiaCaller))
	if raw == "" || len(raw) > smiaTokenMax || strings.ContainsAny(raw, " \t\r\n") {
		return CallerIdentity{}, errCallerAuth
	}
	header, payload, signing, sig, err := splitJWT(raw)
	if err != nil {
		return CallerIdentity{}, errCallerAuth
	}
	var hdr struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	if err := json.Unmarshal(header, &hdr); err != nil || hdr.Alg != "ES256" || strings.TrimSpace(hdr.Kid) == "" || len(hdr.Crit) > 0 {
		return CallerIdentity{}, errCallerAuth
	}
	if len(hdr.Kid) > 128 {
		return CallerIdentity{}, errCallerAuth
	}
	key, err := v.publicKey(r.Context(), hdr.Kid)
	if err != nil {
		return CallerIdentity{}, err
	}
	sum := sha256.Sum256(signing)
	if !verifyES256(key, sum[:], sig) {
		return CallerIdentity{}, fmt.Errorf("%w: %w", errCallerAuth, errSmiaSig)
	}
	claims, err := parseSmiaClaims(payload)
	if err != nil {
		return CallerIdentity{}, err
	}
	if strings.TrimRight(claims.Iss, "/") != v.Issuer {
		return CallerIdentity{}, errCallerAuth
	}
	if !audienceMatches(claims.Aud, v.Audience) {
		return CallerIdentity{}, fmt.Errorf("%w: %w", errCallerAuth, errSmiaAud)
	}
	now := v.now()
	skew := v.Skew
	if skew < 0 {
		skew = 0
	}
	maxLife := v.MaxLife
	if maxLife <= 0 {
		maxLife = smiaMaxLife
	}
	iat := time.Unix(claims.Iat, 0)
	exp := time.Unix(claims.Exp, 0)
	if !exp.After(iat) || exp.Sub(iat) > maxLife+skew {
		return CallerIdentity{}, errCallerAuth
	}
	if iat.After(now.Add(skew)) || now.After(exp.Add(skew)) {
		return CallerIdentity{}, fmt.Errorf("%w: %w", errCallerAuth, errSmiaExpired)
	}
	if err := v.consumeJTI(claims.JTI, exp); err != nil {
		return CallerIdentity{}, err
	}
	return CallerIdentity{Provider: smiaProvider, Subject: claims.Sub, Email: claims.Email}, nil
}

func (v *SmiaCallerVerifier) publicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if key, ok := v.lookup(kid); ok {
		return key, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if key, ok := v.lookup(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("%w: %w", errCallerAuth, errSmiaKid)
}

func (v *SmiaCallerVerifier) lookup(kid string) (*ecdsa.PublicKey, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key, ok := v.keys[kid]
	return key, ok
}

func (v *SmiaCallerVerifier) refresh(ctx context.Context) error {
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.JWKSURL, nil)
	if err != nil {
		return errCallerAuth
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return errCallerAuth
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errCallerAuth
	}
	if resp.StatusCode != http.StatusOK {
		return errCallerAuth
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return errCallerAuth
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func (v *SmiaCallerVerifier) consumeJTI(jti string, exp time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.seen == nil {
		v.seen = map[string]time.Time{}
	}
	now := v.now()
	skew := v.Skew
	if skew < 0 {
		skew = 0
	}
	for id, until := range v.seen {
		if now.After(until.Add(skew)) {
			delete(v.seen, id)
		}
	}
	if until, ok := v.seen[jti]; ok && !now.After(until.Add(skew)) {
		return fmt.Errorf("%w: %w", errCallerAuth, errSmiaReplay)
	}
	v.seen[jti] = exp
	return nil
}

type smiaClaims struct {
	Iss       string
	Sub       string
	Email     string
	Hub       string
	ListingID string
	Tool      string
	Aud       []string
	Iat       int64
	Exp       int64
	JTI       string
}

func parseSmiaClaims(payload []byte) (smiaClaims, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return smiaClaims{}, errCallerAuth
	}
	for _, key := range []string{"iss", "sub", "email", "hub", "listing_id", "tool", "aud", "iat", "exp", "jti"} {
		if _, ok := raw[key]; !ok {
			return smiaClaims{}, errCallerAuth
		}
	}
	var c smiaClaims
	if err := jsonString(raw["iss"], &c.Iss); err != nil || c.Iss == "" || len(c.Iss) > 200 {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["sub"], &c.Sub); err != nil || c.Sub == "" || len(c.Sub) > 200 || strings.ContainsAny(c.Sub, "\r\n") {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["email"], &c.Email); err != nil || len(c.Email) > 320 || strings.ContainsAny(c.Email, "\r\n") {
		return smiaClaims{}, errCallerAuth
	}
	if c.Email != "" && !strings.Contains(c.Email, "@") {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["hub"], &c.Hub); err != nil || strings.TrimSpace(c.Hub) == "" || len(c.Hub) > 200 {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["listing_id"], &c.ListingID); err != nil || strings.TrimSpace(c.ListingID) == "" || len(c.ListingID) > 200 {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["tool"], &c.Tool); err != nil || strings.TrimSpace(c.Tool) == "" || len(c.Tool) > 200 {
		return smiaClaims{}, errCallerAuth
	}
	aud, err := jsonAudience(raw["aud"])
	if err != nil || len(aud) == 0 {
		return smiaClaims{}, errCallerAuth
	}
	c.Aud = aud
	if err := json.Unmarshal(raw["iat"], &c.Iat); err != nil {
		return smiaClaims{}, errCallerAuth
	}
	if err := json.Unmarshal(raw["exp"], &c.Exp); err != nil {
		return smiaClaims{}, errCallerAuth
	}
	if err := jsonString(raw["jti"], &c.JTI); err != nil || c.JTI == "" || len(c.JTI) > 128 || strings.ContainsAny(c.JTI, "\r\n") {
		return smiaClaims{}, errCallerAuth
	}
	return c, nil
}

func jsonString(raw json.RawMessage, dst *string) error {
	if len(raw) == 0 || raw[0] != '"' {
		return errCallerAuth
	}
	return json.Unmarshal(raw, dst)
}

func jsonAudience(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errCallerAuth
	}
	if raw[0] == '"' {
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	return many, nil
}

func audienceMatches(aud []string, want string) bool {
	for _, item := range aud {
		if publicOrigin(item) == want && want != "" {
			return true
		}
	}
	return false
}

func publicOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := parseOrigin(raw)
	if err != nil || u == "" {
		return ""
	}
	return u
}

func parseOrigin(raw string) (string, error) {
	// url.Parse accepts scheme-relative input. Require an absolute URL.
	if !strings.Contains(raw, "://") {
		return "", errCallerAuth
	}
	i := strings.Index(raw, "://")
	scheme := strings.ToLower(raw[:i])
	if scheme != "https" && scheme != "http" {
		return "", errCallerAuth
	}
	rest := raw[i+3:]
	// Drop userinfo, path, query, and fragment. Host is up to the first / ? #.
	if at := strings.IndexAny(rest, "/?#"); at >= 0 {
		rest = rest[:at]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[at+1:]
	}
	host := strings.ToLower(strings.TrimSpace(rest))
	if host == "" || strings.Contains(host, " ") {
		return "", errCallerAuth
	}
	return scheme + "://" + host, nil
}

func splitJWT(raw string) (header, payload, signing, sig []byte, err error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, nil, nil, nil, errCallerAuth
	}
	header, err = base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	payload, err = base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	sig, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(sig) != 64 {
		return nil, nil, nil, nil, errCallerAuth
	}
	return header, payload, []byte(parts[0] + "." + parts[1]), sig, nil
}

func verifyES256(key *ecdsa.PublicKey, digest, sig []byte) bool {
	if key == nil || key.Curve != elliptic.P256() || len(sig) != 64 {
		return false
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(key, digest, r, s)
}

func parseJWKS(body []byte) (map[string]*ecdsa.PublicKey, error) {
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Kid string `json:"kid"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := map[string]*ecdsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || strings.TrimSpace(k.Kid) == "" {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Alg != "" && k.Alg != "ES256" {
			continue
		}
		if _, dup := out[k.Kid]; dup {
			return nil, errCallerAuth
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		if len(x) != 32 || len(y) != 32 {
			return nil, errCallerAuth
		}
		uncompressed := make([]byte, 65)
		uncompressed[0] = 4
		copy(uncompressed[1:33], x)
		copy(uncompressed[33:], y)
		px, py := elliptic.Unmarshal(elliptic.P256(), uncompressed)
		if px == nil || py == nil {
			return nil, errCallerAuth
		}
		out[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: px, Y: py}
	}
	return out, nil
}
