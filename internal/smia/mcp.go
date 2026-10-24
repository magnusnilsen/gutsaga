package smia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultResource = "https://smia.tools/wow/mcp"
	protocol        = "2025-03-26"
	positiveTTL     = 8 * time.Minute
	negativeTTL     = time.Minute
)

// MCPVerifier checks an opaque Smia access token by calling the hub MCP
// (initialize, then tools/call check_access, then a whoami-style tool if the
// first result has no user). Only that response is trusted.
type MCPVerifier struct {
	ResourceURL string
	Hub         string
	Server      string
	HTTP        *http.Client
	Cache       Cache
	Now         func() time.Time
}

func (v MCPVerifier) Verify(ctx context.Context, accessToken string) (Identity, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return Identity{}, ErrUnauthorized
	}
	if v.ResourceURL == "" {
		v.ResourceURL = defaultResource
	}
	if v.Hub == "" {
		v.Hub = Hub
	}
	if v.Server == "" {
		v.Server = Server
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	sum := sha256.Sum256([]byte(accessToken))
	hash := hex.EncodeToString(sum[:])
	if v.Cache != nil {
		if id, ok, err := v.Cache.Get(ctx, hash); err != nil {
			return Identity{}, err
		} else if ok {
			return id, nil
		}
	}
	id, err := v.fetch(ctx, accessToken)
	if err != nil {
		return Identity{}, err
	}
	if v.Cache != nil {
		ttl := positiveTTL
		if !id.Member {
			ttl = negativeTTL
		}
		_ = v.Cache.Put(ctx, hash, id, now.Add(ttl).Unix())
	}
	return id, nil
}

func (v MCPVerifier) fetch(ctx context.Context, token string) (Identity, error) {
	session, initBody, err := v.post(ctx, token, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "gutsaga", "version": "0.1.0"},
		},
	})
	if err != nil {
		return Identity{}, err
	}
	if _, err := ParseAccess(initBody, v.Hub, v.Server); err != nil && isUnauthorized(err) {
		return Identity{}, ErrUnauthorized
	}
	_, _, _ = v.post(ctx, token, session, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	_, body, err := v.post(ctx, token, session, toolCall(2, "check_access", map[string]any{}))
	if err != nil {
		return Identity{}, err
	}
	id, perr := ParseAccess(body, v.Hub, v.Server)
	if perr != nil && isUnauthorized(perr) {
		return Identity{}, ErrUnauthorized
	}
	if perr == nil && id.Subject != "" {
		return id, nil
	}
	_, listed, lerr := v.post(ctx, token, session, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/list",
	})
	if lerr != nil {
		if perr != nil {
			return Identity{}, perr
		}
		return Identity{}, lerr
	}
	for _, name := range []string{"whoami", "get_self", "me"} {
		if !toolListed(listed, name) {
			continue
		}
		_, who, werr := v.post(ctx, token, session, toolCall(4, name, map[string]any{}))
		if werr != nil {
			return Identity{}, werr
		}
		id, perr = ParseAccess(who, v.Hub, v.Server)
		if perr != nil {
			return Identity{}, perr
		}
		if id.Subject != "" {
			return id, nil
		}
	}
	if perr != nil {
		return Identity{}, perr
	}
	return Identity{}, fmt.Errorf("%w: check_access returned no user", ErrUnavailable)
}

func toolCall(id int, name string, args map[string]any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
}

func (v MCPVerifier) post(ctx context.Context, token, session string, payload any) (string, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.ResourceURL, bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocol)
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	resp, err := v.http().Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", ErrUnavailable, err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", nil, ErrUnauthorized
	}
	if resp.StatusCode == http.StatusForbidden {
		return "", body, nil
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusAccepted {
		return "", nil, fmt.Errorf("%w: mcp status %d", ErrUnavailable, resp.StatusCode)
	}
	parsed := decodeMCPBody(resp.Header.Get("Content-Type"), body)
	return resp.Header.Get("Mcp-Session-Id"), parsed, nil
}

func (v MCPVerifier) http() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func decodeMCPBody(contentType string, body []byte) []byte {
	ct := strings.ToLower(contentType)
	looksSSE := strings.Contains(ct, "text/event-stream") ||
		strings.Contains(string(body), "\ndata:") ||
		strings.HasPrefix(strings.TrimSpace(string(body)), "event:")
	if !looksSSE {
		return body
	}
	var last []byte
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			last = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(last) == 0 {
		return body
	}
	return last
}

func toolListed(body []byte, name string) bool {
	var env struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	for _, t := range env.Result.Tools {
		if strings.EqualFold(t.Name, name) {
			return true
		}
	}
	return false
}

func isUnauthorized(err error) bool {
	return err == ErrUnauthorized || (err != nil && strings.Contains(err.Error(), ErrUnauthorized.Error()))
}
