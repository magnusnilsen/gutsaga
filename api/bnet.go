package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	bnetAuthorizeURL = "https://oauth.battle.net/authorize"
	bnetTokenURL     = "https://oauth.battle.net/token"
	bnetUserInfoURL  = "https://oauth.battle.net/userinfo"
)

type bnetAttempt struct {
	verifier string
	state    string
	session  *sessionWire
	errCode  string
	errMsg   string
	created  time.Time
}

type sessionWire struct {
	TokenType    string `json:"token_type"`
	SessionToken string `json:"session_token"`
	ExpiresAt    string `json:"expires_at"`
	Player       any    `json:"player"`
}

type bnetFlow struct {
	mu       sync.Mutex
	attempts map[string]*bnetAttempt
	client   *http.Client
	now      func() time.Time
}

func newBnetFlow() *bnetFlow {
	return &bnetFlow{attempts: map[string]*bnetAttempt{}, client: &http.Client{Timeout: 20 * time.Second}, now: time.Now}
}

func (f *bnetFlow) prune() {
	cut := f.now().Add(-30 * time.Minute)
	for id, a := range f.attempts {
		if a.created.Before(cut) {
			delete(f.attempts, id)
		}
	}
}

func (s *Server) bnetStart(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.bnetClientID) == "" || strings.TrimSpace(s.publicBaseURL) == "" {
		writeErr(w, http.StatusServiceUnavailable, "bnet_not_configured", "BNET_CLIENT_ID and PUBLIC_BASE_URL are required")
		return
	}
	attempt := strings.TrimSpace(r.URL.Query().Get("attempt"))
	if attempt == "" || len(attempt) > 80 {
		writeErr(w, http.StatusBadRequest, "invalid", "attempt is required")
		return
	}
	verifier, challenge, err := pkcePair()
	if err != nil {
		s.fail(w, err)
		return
	}
	state, err := randomToken(16)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.bnet.mu.Lock()
	s.bnet.prune()
	s.bnet.attempts[attempt] = &bnetAttempt{verifier: verifier, state: state, created: s.bnet.now()}
	s.bnet.mu.Unlock()
	q := url.Values{}
	q.Set("client_id", s.bnetClientID)
	q.Set("redirect_uri", s.bnetRedirect())
	q.Set("response_type", "code")
	q.Set("scope", "openid wow.profile")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	http.Redirect(w, r, bnetAuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

func (s *Server) bnetRedirect() string {
	return strings.TrimRight(s.publicBaseURL, "/") + "/v1/auth/bnet/callback"
}

func (s *Server) bnetCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	s.bnet.mu.Lock()
	var attempt string
	var row *bnetAttempt
	for id, a := range s.bnet.attempts {
		if a.state == state {
			attempt = id
			row = a
			break
		}
	}
	s.bnet.mu.Unlock()
	if row == nil || code == "" {
		writeErr(w, http.StatusBadRequest, "invalid", "unknown Battle.net login attempt")
		return
	}
	info, err := s.exchangeBnet(r.Context(), code, row.verifier)
	if err != nil {
		s.bnet.mu.Lock()
		row.errCode = "bnet_unavailable"
		row.errMsg = err.Error()
		s.bnet.mu.Unlock()
		writeErr(w, http.StatusBadGateway, "bnet_unavailable", "Battle.net login failed")
		return
	}
	player, err := s.store.MergeBattleNet(r.Context(), info.AccountID, info.BattleTag, info.DisplayName)
	if err != nil {
		s.fail(w, err)
		return
	}
	sess, err := s.store.IssueSessionFor(r.Context(), player.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	wire := &sessionWire{TokenType: sess.TokenType, SessionToken: sess.SessionToken, ExpiresAt: sess.ExpiresAt, Player: sess.Player}
	s.bnet.mu.Lock()
	if cur := s.bnet.attempts[attempt]; cur != nil {
		cur.session = wire
	}
	s.bnet.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><title>OurSagas</title><p>You are logged in. You can close this window.</p>")
}

func (s *Server) bnetPoll(w http.ResponseWriter, r *http.Request) {
	attempt := strings.TrimSpace(r.URL.Query().Get("attempt"))
	s.bnet.mu.Lock()
	row := s.bnet.attempts[attempt]
	var sess *sessionWire
	var code, msg string
	if row != nil {
		sess = row.session
		code, msg = row.errCode, row.errMsg
	}
	s.bnet.mu.Unlock()
	if row == nil {
		writeErr(w, http.StatusNotFound, "not_found", "unknown attempt")
		return
	}
	if code != "" {
		writeErr(w, http.StatusBadGateway, code, msg)
		return
	}
	if sess == nil {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.devToken) == "" || subtleConstantTimeString(bearer(r), s.devToken) != 1 {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid dev token")
		return
	}
	var body struct {
		AccountID   string `json:"bnet_account_id"`
		BattleTag   string `json:"battletag"`
		DisplayName string `json:"display_name"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &body, 1<<16); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid", "bad json")
			return
		}
	}
	if strings.TrimSpace(body.AccountID) == "" {
		body.AccountID = "dev"
	}
	if strings.TrimSpace(body.BattleTag) == "" {
		body.BattleTag = "Player#1234"
	}
	if strings.TrimSpace(body.DisplayName) == "" {
		body.DisplayName = "Player"
	}
	player, err := s.store.MergeBattleNet(r.Context(), body.AccountID, body.BattleTag, body.DisplayName)
	if err != nil {
		s.fail(w, err)
		return
	}
	sess, err := s.store.IssueSessionFor(r.Context(), player.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

type bnetUser struct {
	AccountID   string
	BattleTag   string
	DisplayName string
}

func (s *Server) exchangeBnet(ctx context.Context, code, verifier string) (bnetUser, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.bnetRedirect())
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bnetTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return bnetUser{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.bnetClientID, s.bnetClientSecret)
	resp, err := s.bnet.client.Do(req)
	if err != nil {
		return bnetUser{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bnetUser{}, fmt.Errorf("token status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.AccessToken == "" {
		return bnetUser{}, fmt.Errorf("token response")
	}
	ureq, err := http.NewRequestWithContext(ctx, http.MethodGet, bnetUserInfoURL, nil)
	if err != nil {
		return bnetUser{}, err
	}
	ureq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := s.bnet.client.Do(ureq)
	if err != nil {
		return bnetUser{}, err
	}
	defer uresp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(uresp.Body, 1<<20))
	if uresp.StatusCode < 200 || uresp.StatusCode >= 300 {
		return bnetUser{}, fmt.Errorf("userinfo status %d", uresp.StatusCode)
	}
	var info struct {
		ID        json.RawMessage `json:"id"`
		BattleTag string          `json:"battletag"`
		Sub       string          `json:"sub"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return bnetUser{}, err
	}
	id := strings.Trim(string(info.ID), `"`)
	if id == "" {
		id = strings.TrimSpace(info.Sub)
	}
	name := info.BattleTag
	if i := strings.Index(name, "#"); i > 0 {
		name = name[:i]
	}
	return bnetUser{AccountID: id, BattleTag: info.BattleTag, DisplayName: name}, nil
}

func pkcePair() (verifier, challenge string, err error) {
	verifier, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
