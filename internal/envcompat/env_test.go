package envcompat

import "testing"

func TestGetPrefersSagaloreAndFallsBack(t *testing.T) {
	t.Setenv("SAGALORE_DEV_TOKEN", "new")
	t.Setenv("OURSAGAS_DEV_TOKEN", "old")
	if got := Get("SAGALORE_DEV_TOKEN"); got != "new" {
		t.Fatalf("prefer new, got %q", got)
	}
	t.Setenv("SAGALORE_DEV_TOKEN", "")
	if got := Get("SAGALORE_DEV_TOKEN"); got != "old" {
		t.Fatalf("fallback, got %q", got)
	}
	t.Setenv("OURSAGAS_DEV_TOKEN", "  ")
	if got := Get("SAGALORE_DEV_TOKEN"); got != "" {
		t.Fatalf("blank, got %q", got)
	}
	t.Setenv("SAGALORE_MCP_SERVICE_SECRET", "")
	t.Setenv("OURSAGAS_MCP_SERVICE_SECRET", "mcp-old")
	if got := Get("SAGALORE_MCP_SERVICE_SECRET"); got != "mcp-old" {
		t.Fatalf("mcp fallback, got %q", got)
	}
	t.Setenv("SAGALORE_SMIA_ISSUER", "")
	t.Setenv("OURSAGAS_SMIA_ISSUER", "https://smia.example")
	if got := Get("SAGALORE_SMIA_ISSUER"); got != "https://smia.example" {
		t.Fatalf("issuer fallback, got %q", got)
	}
	t.Setenv("SAGALORE_REQUIRE_SMIA_JWT", "")
	t.Setenv("OURSAGAS_REQUIRE_SMIA_JWT", "1")
	if got := Get("SAGALORE_REQUIRE_SMIA_JWT"); got != "1" {
		t.Fatalf("require jwt fallback, got %q", got)
	}
	t.Setenv("DATABASE_URL", "postgres://local")
	if got := Get("DATABASE_URL"); got != "postgres://local" {
		t.Fatalf("unrelated %q", got)
	}
}
