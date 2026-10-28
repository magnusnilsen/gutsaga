package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

type scriptedBnet struct {
	account string
	tag     string
}

func (s *scriptedBnet) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"id":"` + s.account + `","battletag":"` + s.tag + `"}`
	if strings.HasSuffix(r.URL.Path, "/token") {
		body = `{"access_token":"tok"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}, nil
}

func mcpRequest(t *testing.T, base, bearer, subject, email, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if subject != "" {
		req.Header.Set(headerCallerSubject, subject)
	}
	if email != "" {
		req.Header.Set(headerCallerEmail, email)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func finishBnetLink(t *testing.T, api *Server, base, attempt string) {
	t.Helper()
	api.bnet.mu.Lock()
	row := api.bnet.attempts[attempt]
	api.bnet.mu.Unlock()
	if row == nil || row.state == "" {
		t.Fatalf("attempt %q was not registered", attempt)
	}
	resp, err := http.Get(base + "/v1/auth/bnet/callback?code=ok&state=" + url.QueryEscape(row.state))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback %d %s", resp.StatusCode, raw)
	}
}

func TestMCPIdentityLinkAndAuthz(t *testing.T) {
	resetDB(t)
	owner, ownerToken := devSession(t, "bnet-owner", "Owner#1", "Owner")
	group := devGroup(t, owner.ID, "tutorial", "Tutorial")
	member, _ := devSession(t, "bnet-member", "Member#2", "Member")
	joinGroup(t, group.ID, member.ID)
	_, outsiderToken := devSession(t, "bnet-outsider", "Outsider#3", "Outsider")

	bnet := &scriptedBnet{account: "bnet-owner", tag: "Owner#1"}
	api := &Server{
		store:            NewStore(testPool),
		publicBaseURL:    "http://sagalore.example",
		bnetClientID:     "cid",
		bnetClientSecret: "csec",
		bnet:             newBnetFlow(),
		mcpServiceSecret: "svc-secret",
		callerVerifier:   SharedSecretVerifier{Secret: "svc-secret", Provider: smiaProvider},
	}
	api.bnet.client = &http.Client{Transport: bnet}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	resp, raw := mcpRequest(t, srv.URL, ownerToken, "smia-owner", "owner@example.com", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token plus caller header %d %s", resp.StatusCode, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "", "", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("secret without subject %d %s", resp.StatusCode, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "", "smia-owner", "owner@example.com", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("headers without secret %d %s", resp.StatusCode, raw)
	}

	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-owner", "owner@example.com", http.MethodGet, "/v1/groups", nil)
	var unlinked contract.ErrorBody
	if err := json.Unmarshal(raw, &unlinked); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden || unlinked.Error != "identity_not_linked" || unlinked.Attempt == "" || !strings.Contains(unlinked.LinkURL, "https://oauth.battle.net/authorize") {
		t.Fatalf("unlinked %d %+v", resp.StatusCode, unlinked)
	}
	finishBnetLink(t, api, srv.URL, unlinked.Attempt)

	var groups struct {
		Groups []contract.GroupSummary `json:"groups"`
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-owner", "owner@example.com", http.MethodGet, "/v1/groups", nil)
	if err := json.Unmarshal(raw, &groups); err != nil || resp.StatusCode != http.StatusOK || len(groups.Groups) != 1 || groups.Groups[0].Role != "owner" || groups.Groups[0].ID != group.ID {
		t.Fatalf("linked owner %d %v %s", resp.StatusCode, err, raw)
	}
	var email string
	if err := testPool.QueryRow(context.Background(), `SELECT email FROM identity_links WHERE provider='smia' AND subject='smia-owner'`).Scan(&email); err != nil || email != "owner@example.com" {
		t.Fatalf("email %q %v", email, err)
	}

	bnet.account, bnet.tag = "bnet-member", "Member#2"
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-member", "member@example.com", http.MethodPost, "/v1/auth/link", nil)
	var started contract.IdentityLinkStart
	if err := json.Unmarshal(raw, &started); err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(started.URL, "https://oauth.battle.net/authorize") || !strings.Contains(started.PollURL, "/v1/auth/bnet/poll?attempt=") {
		t.Fatalf("link start %d %v %s", resp.StatusCode, err, raw)
	}
	finishBnetLink(t, api, srv.URL, started.Attempt)

	var created contract.Invite
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-owner", "", http.MethodPost, "/v1/groups/"+group.ID+"/invites", map[string]string{
		"name": "Magni", "realm": "Everlook", "battletag": "Outsider#3",
	})
	if err := json.Unmarshal(raw, &created); err != nil || resp.StatusCode != http.StatusCreated || created.ID == "" {
		t.Fatalf("owner invite %d %v %s", resp.StatusCode, err, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-member", "", http.MethodGet, "/v1/groups/"+group.ID+"/invites?status=pending", nil)
	var listed struct {
		Invites []contract.Invite `json:"invites"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil || resp.StatusCode != http.StatusOK || len(listed.Invites) != 1 || listed.Invites[0].Name != "Magni" {
		t.Fatalf("member list %d %v %s", resp.StatusCode, err, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-member", "", http.MethodDelete, "/v1/groups/"+group.ID+"/invites/"+created.ID, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member revoke %d %s", resp.StatusCode, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-outsider", "out@example.com", http.MethodGet, "/v1/groups/"+group.ID+"/invites", nil)
	var outsider contract.ErrorBody
	_ = json.Unmarshal(raw, &outsider)
	if resp.StatusCode != http.StatusForbidden || outsider.Error != "identity_not_linked" {
		t.Fatalf("unlinked outsider %d %+v", resp.StatusCode, outsider)
	}
	outAPI := apiClient{base: srv.URL, token: outsiderToken, http: srv.Client()}
	resp, err := outAPI.do(http.MethodGet, "/v1/groups/"+group.ID+"/invites", nil, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("session outsider %d %v", statusOf(resp), err)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "smia-owner", "", http.MethodDelete, "/v1/groups/"+group.ID+"/invites/"+created.ID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("owner revoke %d %s", resp.StatusCode, raw)
	}
}

func TestCallerVerifierReplacement(t *testing.T) {
	resetDB(t)
	player, _ := devSession(t, "bnet-verifier", "Verifier#1", "Verifier")
	group := devGroup(t, player.ID, "verifier", "Verifier")
	if err := NewStore(testPool).BindIdentity(context.Background(), "smia", "from-verifier", "v@example.com", player.ID); err != nil {
		t.Fatal(err)
	}
	var calls int
	api := &Server{
		store:            NewStore(testPool),
		mcpServiceSecret: "svc-secret",
		publicBaseURL:    "http://sagalore.example",
		bnetClientID:     "cid",
		bnet:             newBnetFlow(),
		callerVerifier: VerifierFunc(func(r *http.Request) (CallerIdentity, error) {
			calls++
			if r.Header.Get(headerCallerSubject) == "spoof" {
				return CallerIdentity{}, errCallerAuth
			}
			return CallerIdentity{Provider: smiaProvider, Subject: "from-verifier", Email: "v@example.com"}, nil
		}),
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	resp, raw := mcpRequest(t, srv.URL, "nope", "from-verifier", "", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("wrong secret called verifier=%d %d %s", calls, resp.StatusCode, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "spoof", "", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized || calls != 1 {
		t.Fatalf("spoof %d calls %d %s", resp.StatusCode, calls, raw)
	}
	resp, raw = mcpRequest(t, srv.URL, "svc-secret", "someone-else", "", http.MethodGet, "/v1/groups", nil)
	var groups struct {
		Groups []contract.GroupSummary `json:"groups"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil || resp.StatusCode != http.StatusOK || len(groups.Groups) != 1 || groups.Groups[0].ID != group.ID || calls != 2 {
		t.Fatalf("verifier subject %d calls %d %v %s", resp.StatusCode, calls, err, raw)
	}

	api.mcpServiceSecret = ""
	blocked := httptest.NewServer(api.Handler())
	t.Cleanup(blocked.Close)
	before := calls
	resp, raw = mcpRequest(t, blocked.URL, "svc-secret", "from-verifier", "", http.MethodGet, "/v1/groups", nil)
	if resp.StatusCode != http.StatusUnauthorized || calls != before {
		t.Fatalf("empty secret still verified %d calls %d→%d %s", resp.StatusCode, before, calls, raw)
	}
}
