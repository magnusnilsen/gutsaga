package main

import (
	"strings"
	"testing"
)

func TestSplitSQLKeepsFunctionBody(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/007_realtime_notify.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := splitSQL(string(body))
	fn, triggers := 0, 0
	for _, p := range parts {
		if strings.Contains(p, "CREATE OR REPLACE FUNCTION") {
			fn++
			if !strings.Contains(p, "pg_notify") || !strings.Contains(p, "saga.retired") || strings.Contains(p, "CREATE TRIGGER") {
				t.Fatalf("function was split: %s", p)
			}
		}
		if strings.Contains(p, "CREATE TRIGGER") {
			triggers++
		}
	}
	if fn != 1 || triggers != 3 {
		t.Fatalf("fn=%d triggers=%d parts=%d", fn, triggers, len(parts))
	}

	init, err := migrationFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitSQL(string(init))
	if len(statements) < 5 {
		t.Fatalf("001 split into %d", len(statements))
	}
}
