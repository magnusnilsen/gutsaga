package smia

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseAccess reads an MCP tools/call response (JSON-RPC, or the tool text)
// and decides whether the token belongs to hub/server. Membership is taken
// from the payload. When the payload has an identity but no hub list, the
// call itself is the proof: it was made to that hub's MCP and Smia accepted it.
func ParseAccess(payload []byte, hub, server string) (Identity, error) {
	payload = bytesTrim(payload)
	if len(payload) == 0 {
		return Identity{}, fmt.Errorf("%w: empty response", ErrUnavailable)
	}
	var env map[string]any
	if err := json.Unmarshal(payload, &env); err != nil {
		id := fromText(string(payload), hub, server)
		if id.Subject == "" {
			return Identity{}, fmt.Errorf("%w: unreadable response", ErrUnavailable)
		}
		return id, nil
	}
	if errObj, ok := env["error"].(map[string]any); ok {
		msg := strings.ToLower(fmt.Sprint(errObj["message"]))
		if isAuthn(msg) {
			return Identity{}, ErrUnauthorized
		}
		if isDenied(msg) {
			return Identity{Member: false, Hub: hub, Server: server}, nil
		}
		return Identity{}, fmt.Errorf("%w: %s", ErrUnavailable, errObj["message"])
	}
	var texts []string
	var objects []map[string]any
	walk(env, &texts, &objects)
	p := acc{hub: hub, server: server}
	for _, obj := range objects {
		p.absorb(obj)
	}
	for _, text := range texts {
		var nested map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(text)), &nested) == nil {
			p.absorb(nested)
		}
		p.absorbText(text)
	}
	return p.identity()
}

type acc struct {
	hub, server string
	subject     string
	email       string
	name        string
	sawHubs     bool
	hubOK       bool
	serverOK    bool
	explicit    *bool
	saidHub     string
	saidServer  string
}

func (p *acc) absorb(obj map[string]any) {
	if s := firstString(obj, "email", "user_email"); s != "" && p.email == "" {
		p.email = s
	}
	if u, ok := obj["user"].(map[string]any); ok {
		p.absorb(u)
	}
	if s := firstString(obj, "sub", "user_id", "userId"); s != "" {
		p.subject = s
	}
	if s := firstString(obj, "display_name", "displayName"); s != "" {
		p.name = s
	}
	if _, ok := obj["email"]; ok || obj["user_email"] != nil {
		if s := firstString(obj, "name"); s != "" && p.name == "" {
			p.name = s
		}
	}
	if m, ok := boolField(obj, "member", "is_member", "isMember"); ok {
		p.explicit = &m
	}
	if s := firstString(obj, "hub", "hub_slug"); s != "" && p.saidHub == "" {
		p.saidHub = s
	}
	if s := firstString(obj, "server", "server_slug"); s != "" && p.saidServer == "" && !strings.Contains(s, " ") {
		p.saidServer = s
	}
	if hubs, ok := obj["hubs"].([]any); ok {
		p.sawHubs = true
		for _, raw := range hubs {
			h, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			slug := firstString(h, "slug", "hub", "hub_slug")
			if !strings.EqualFold(slug, p.hub) {
				continue
			}
			p.hubOK = true
			if servers, ok := h["servers"].([]any); ok {
				for _, sr := range servers {
					sm, ok := sr.(map[string]any)
					if !ok {
						continue
					}
					ss := firstString(sm, "slug", "server", "server_slug", "url")
					if serverMatch(ss, p.hub, p.server) {
						p.serverOK = true
					}
				}
			} else {
				p.serverOK = true
			}
		}
	}
}

func (p *acc) absorbText(text string) {
	lines := strings.Split(text, "\n")
	var currentHub string
	sawHubHeader := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)
		if strings.HasPrefix(lower, "authenticated as ") {
			email := strings.TrimSpace(trim[len("authenticated as "):])
			if i := strings.IndexAny(email, " \t"); i >= 0 {
				email = email[:i]
			}
			if strings.Contains(email, "@") && p.email == "" {
				p.email = email
			}
		}
		if strings.HasPrefix(lower, "hubs:") {
			sawHubHeader = true
			p.sawHubs = true
			continue
		}
		if strings.HasPrefix(trim, "- ") && (sawHubHeader || strings.Contains(lower, "id=")) {
			slug := strings.TrimPrefix(trim, "- ")
			if i := strings.IndexAny(slug, " ("); i >= 0 {
				slug = slug[:i]
			}
			currentHub = slug
			if strings.EqualFold(slug, p.hub) {
				p.hubOK = true
			}
			continue
		}
		if currentHub != "" && strings.EqualFold(currentHub, p.hub) && (strings.HasPrefix(trim, "·") || strings.HasPrefix(trim, "*") || strings.Contains(lower, "/"+p.hub+"/")) {
			if serverMatch(trim, p.hub, p.server) {
				p.serverOK = true
			}
		}
	}
}

func (p *acc) identity() (Identity, error) {
	subject := p.subject
	if subject == "" {
		subject = p.email
	}
	name := p.name
	if name == "" {
		name = p.email
	}
	if subject == "" {
		return Identity{}, fmt.Errorf("%w: mcp response had no user", ErrUnavailable)
	}
	member := false
	switch {
	case p.explicit != nil && !*p.explicit:
		member = false
	case p.saidHub != "" && !strings.EqualFold(p.saidHub, p.hub):
		member = false
	case p.saidServer != "" && !strings.EqualFold(p.saidServer, p.server) && !serverMatch(p.saidServer, p.hub, p.server):
		member = false
	case p.sawHubs:
		member = p.hubOK && p.serverOK
	case p.explicit != nil && *p.explicit:
		member = true
	case p.saidHub != "" || p.saidServer != "":
		member = strings.EqualFold(p.saidHub, p.hub) && (p.saidServer == "" || strings.EqualFold(p.saidServer, p.server) || serverMatch(p.saidServer, p.hub, p.server))
	default:
		// Identity without a hub catalogue: Smia served this resource.
		member = true
	}
	return Identity{
		Subject: subject, Email: p.email, DisplayName: name,
		Member: member, Hub: p.hub, Server: p.server,
	}, nil
}

func fromText(text, hub, server string) Identity {
	p := acc{hub: hub, server: server}
	p.absorbText(text)
	id, err := p.identity()
	if err != nil {
		return Identity{}
	}
	return id
}

func walk(v any, texts *[]string, objects *[]map[string]any) {
	switch t := v.(type) {
	case map[string]any:
		*objects = append(*objects, t)
		if s, ok := t["text"].(string); ok && strings.TrimSpace(s) != "" {
			*texts = append(*texts, s)
		}
		for _, child := range t {
			walk(child, texts, objects)
		}
	case []any:
		for _, child := range t {
			walk(child, texts, objects)
		}
	}
}

func firstString(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := obj[k].(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func boolField(obj map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		if b, ok := obj[k].(bool); ok {
			return b, true
		}
	}
	return false, false
}

func serverMatch(value, hub, server string) bool {
	v := strings.ToLower(value)
	if strings.EqualFold(strings.TrimSpace(value), server) {
		return true
	}
	if strings.Contains(v, "/"+hub+"/"+server) {
		return true
	}
	if strings.Contains(v, "("+server+")") {
		return true
	}
	return false
}

func isAuthn(msg string) bool {
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid_token") || strings.Contains(msg, "invalid token")
}

func isDenied(msg string) bool {
	return strings.Contains(msg, "not_a_member") || strings.Contains(msg, "not a member") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "access denied")
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
