package syncdata

import "testing"

func TestFormatOsloWhen(t *testing.T) {
	// 2026-10-22 17:00 UTC = 19:00 CEST (UTC+2 still in effect until last Sunday of October)
	got := FormatOsloWhen("2026-10-22T17:00:00Z")
	want := "22 oktober 2026 19:00"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Unix seconds for the same instant.
	got = FormatOsloWhen("1792688400")
	if got != want {
		t.Fatalf("unix got %q want %q", got, want)
	}
	if FormatOsloWhen("") != "" {
		t.Fatal("empty")
	}
}
