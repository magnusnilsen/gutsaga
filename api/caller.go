package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

const (
	headerCallerSubject = "X-Sagalore-Caller-Subject"
	headerCallerEmail   = "X-Sagalore-Caller-Email"
	smiaProvider        = "smia"
)

// ErrNotLinked means the external identity has not finished Battle.net linking.
var ErrNotLinked = errors.New("identity not linked")

var errCallerAuth = errors.New("caller auth")

// CallerIdentity is the external account an MCP call runs as.
// Provider is smia. Subject is that provider's stable user id.
type CallerIdentity struct {
	Provider string
	Subject  string
	Email    string
}

// CallerVerifier checks a service call and returns the caller's external
// identity. The HTTP middleware rejects the call when SAGALORE_MCP_SERVICE_SECRET
// does not match, before Verify runs. A verifier still must not treat caller
// headers or a JWT as proof by themselves.
type CallerVerifier interface {
	Verify(r *http.Request) (CallerIdentity, error)
}

// VerifierFunc adapts a function to CallerVerifier.
type VerifierFunc func(r *http.Request) (CallerIdentity, error)

func (f VerifierFunc) Verify(r *http.Request) (CallerIdentity, error) { return f(r) }

// SharedSecretVerifier trusts the service bearer together with
// X-Sagalore-Caller-Subject and optional X-Sagalore-Caller-Email.
// The provider is always smia. Callers cannot choose it.
type SharedSecretVerifier struct {
	Secret   string
	Provider string
}

func (v SharedSecretVerifier) Verify(r *http.Request) (CallerIdentity, error) {
	secret := strings.TrimSpace(v.Secret)
	if secret == "" || subtleConstantTimeString(bearer(r), secret) != 1 {
		return CallerIdentity{}, errCallerAuth
	}
	subject := strings.TrimSpace(r.Header.Get(headerCallerSubject))
	if subject == "" || len(subject) > 200 || strings.ContainsAny(subject, "\r\n") {
		return CallerIdentity{}, errCallerAuth
	}
	email := strings.TrimSpace(r.Header.Get(headerCallerEmail))
	if email != "" && (len(email) > 320 || !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n")) {
		return CallerIdentity{}, fmt.Errorf("%w: email", ErrInvalid)
	}
	provider := strings.TrimSpace(v.Provider)
	if provider == "" {
		provider = smiaProvider
	}
	return CallerIdentity{Provider: provider, Subject: subject, Email: email}, nil
}

// MCPCallerVerifier is the production caller check.
// X-Smia-Caller, when present, must verify and wins over X-Sagalore-Caller-*.
// A bad JWT does not fall back to those headers. RequireSmiaJWT rejects
// header-only identity. The service secret is required on every path.
type MCPCallerVerifier struct {
	RequireSmiaJWT bool
	Headers        SharedSecretVerifier
	Smia           *SmiaCallerVerifier
}

func (v *MCPCallerVerifier) Verify(r *http.Request) (CallerIdentity, error) {
	if v == nil {
		return CallerIdentity{}, errCallerAuth
	}
	secret := strings.TrimSpace(v.Headers.Secret)
	if secret == "" || subtleConstantTimeString(bearer(r), secret) != 1 {
		return CallerIdentity{}, errCallerAuth
	}
	if strings.TrimSpace(r.Header.Get(headerSmiaCaller)) != "" {
		if v.Smia == nil {
			return CallerIdentity{}, errCallerAuth
		}
		return v.Smia.Verify(r)
	}
	if v.RequireSmiaJWT {
		return CallerIdentity{}, errCallerAuth
	}
	return v.Headers.Verify(r)
}

// serviceCallerPresented is true when the request is trying to act as an
// external identity. Those requests never fall through to a gst_ or gss_ token.
func (s *Server) serviceCallerPresented(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get(headerSmiaCaller)) != "" || strings.TrimSpace(r.Header.Get(headerCallerSubject)) != "" || strings.TrimSpace(r.Header.Get(headerCallerEmail)) != "" {
		return true
	}
	secret := strings.TrimSpace(s.mcpServiceSecret)
	return secret != "" && subtleConstantTimeString(bearer(r), secret) == 1
}

// verifiedCaller requires the service secret, then asks the configured verifier
// who the caller is. Identity headers without the secret fail here.
func (s *Server) verifiedCaller(r *http.Request) (CallerIdentity, error) {
	secret := strings.TrimSpace(s.mcpServiceSecret)
	if secret == "" || subtleConstantTimeString(bearer(r), secret) != 1 {
		return CallerIdentity{}, errCallerAuth
	}
	v := s.callerVerifier
	if v == nil {
		v = SharedSecretVerifier{Secret: secret, Provider: smiaProvider}
	}
	id, err := v.Verify(r)
	if err != nil {
		return CallerIdentity{}, err
	}
	id.Provider = strings.TrimSpace(id.Provider)
	id.Subject = strings.TrimSpace(id.Subject)
	id.Email = strings.TrimSpace(id.Email)
	if id.Provider == "" {
		id.Provider = smiaProvider
	}
	if id.Subject == "" || len(id.Subject) > 200 || len(id.Provider) > 32 || strings.ContainsAny(id.Subject, "\r\n") {
		return CallerIdentity{}, errCallerAuth
	}
	if id.Email != "" && (len(id.Email) > 320 || !strings.Contains(id.Email, "@") || strings.ContainsAny(id.Email, "\r\n")) {
		return CallerIdentity{}, fmt.Errorf("%w: email", ErrInvalid)
	}
	return id, nil
}

func (s *Server) playerFromCaller(w http.ResponseWriter, r *http.Request) (contract.Player, bool) {
	id, err := s.verifiedCaller(r)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			writeErr(w, http.StatusBadRequest, "invalid", err.Error())
			return contract.Player{}, false
		}
		writeErr(w, http.StatusUnauthorized, "unauthorized", "service credential and caller subject are required")
		return contract.Player{}, false
	}
	p, err := s.store.PlayerByIdentity(r.Context(), id.Provider, id.Subject, id.Email)
	if errors.Is(err, ErrNotLinked) {
		s.writeNotLinked(w, id)
		return contract.Player{}, false
	}
	if err != nil {
		s.fail(w, err)
		return contract.Player{}, false
	}
	return p, true
}

func (s *Server) writeNotLinked(w http.ResponseWriter, id CallerIdentity) {
	body := contract.ErrorBody{
		Error:   "identity_not_linked",
		Message: "This Smia account is not linked to a Battle.net player. Open link_url and sign in with Battle.net.",
	}
	attempt, authURL, err := s.registerBnetAttempt("", &linkIntent{
		Provider: id.Provider, Subject: id.Subject, Email: id.Email,
	})
	if err != nil {
		if errors.Is(err, errBnetNotConfigured) {
			body.Message = "This Smia account is not linked to a Battle.net player. Battle.net login is not configured on the API."
			writeJSON(w, http.StatusForbidden, body)
			return
		}
		s.fail(w, err)
		return
	}
	body.LinkURL = authURL
	body.Attempt = attempt
	writeJSON(w, http.StatusForbidden, body)
}

func (s *Server) startIdentityLink(w http.ResponseWriter, r *http.Request) {
	id, err := s.verifiedCaller(r)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			writeErr(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
		writeErr(w, http.StatusUnauthorized, "unauthorized", "service credential and caller subject are required")
		return
	}
	attempt, authURL, err := s.registerBnetAttempt("", &linkIntent{
		Provider: id.Provider, Subject: id.Subject, Email: id.Email,
	})
	if errors.Is(err, errBnetNotConfigured) {
		writeErr(w, http.StatusServiceUnavailable, "bnet_not_configured", "BNET_CLIENT_ID and PUBLIC_BASE_URL are required")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.IdentityLinkStart{
		Attempt: attempt,
		URL:     authURL,
		PollURL: s.bnetPollURL(attempt),
	})
}
