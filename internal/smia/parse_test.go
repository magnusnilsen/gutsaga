package smia

import "testing"

func TestParseAccessPlainTextMembership(t *testing.T) {
	body := []byte("Authenticated as magnus@example.com\nHubs:\n- smia (Smia) id=11111111-1111-1111-1111-111111111111 — role: owner\n  · server: https://smia.tools/smia/mcp (Smia)\n- wow (WoW) id=22222222-2222-2222-2222-222222222222 — role: member\n  · server: https://smia.tools/wow/mcp (mcp)\n")
	id, err := ParseAccess(body, Hub, Server)
	if err != nil {
		t.Fatal(err)
	}
	if !id.Member || id.Email != "magnus@example.com" || id.Subject != "magnus@example.com" || id.Hub != "wow" || id.Server != "mcp" {
		t.Fatalf("%+v", id)
	}
}

func TestParseAccessRejectsOtherHub(t *testing.T) {
	body := []byte("Authenticated as a@b.c\nHubs:\n- smia (Smia) id=11111111-1111-1111-1111-111111111111 — role: owner\n  · server: https://smia.tools/smia/mcp (Smia)\n")
	id, err := ParseAccess(body, Hub, Server)
	if err != nil {
		t.Fatal(err)
	}
	if id.Member {
		t.Fatal("not a member of wow")
	}
}

func TestParseAccessToolResult(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Authenticated as a@b.c"}],"structuredContent":{"user_id":"user-9","email":"a@b.c","member":true,"hub":"wow","server":"mcp"}}}`)
	id, err := ParseAccess(body, Hub, Server)
	if err != nil {
		t.Fatal(err)
	}
	if !id.Member || id.Subject != "user-9" || id.Email != "a@b.c" {
		t.Fatalf("%+v", id)
	}
}

func TestParseAccessUnauthorized(t *testing.T) {
	_, err := ParseAccess([]byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32001,"message":"invalid_token"}}`), Hub, Server)
	if err != ErrUnauthorized {
		t.Fatal(err)
	}
}

func TestParseAccessForLine(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"Access for magnus@sparkel.io\nRoles: (none)\nTools: list_characters"}],"isError":false}}`)
	id, err := ParseAccess(body, Hub, Server)
	if err != nil {
		t.Fatal(err)
	}
	if !id.Member || id.Email != "magnus@sparkel.io" || id.Subject != "magnus@sparkel.io" {
		t.Fatalf("%+v", id)
	}
}
