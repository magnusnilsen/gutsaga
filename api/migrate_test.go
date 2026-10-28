package main

import (
	"context"
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

func TestIdentityLinksMigration(t *testing.T) {
	ctx := context.Background()
	var version int
	if err := testPool.QueryRow(ctx, `SELECT version FROM schema_migrations WHERE version=10`).Scan(&version); err != nil || version != 10 {
		t.Fatalf("version 10: %v %d", err, version)
	}
	var cols int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='identity_links'
		AND column_name IN ('provider','subject','email','player_id','created_at','updated_at')`).Scan(&cols); err != nil || cols != 6 {
		t.Fatalf("columns %d %v", cols, err)
	}
	var uniqueName, checkDef string
	if err := testPool.QueryRow(ctx, `
		SELECT conname FROM pg_constraint
		WHERE conrelid='identity_links'::regclass AND contype='u'`).Scan(&uniqueName); err != nil || uniqueName != "identity_links_provider_subject_key" {
		t.Fatalf("unique %q %v", uniqueName, err)
	}
	if err := testPool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid='identity_links'::regclass AND contype='c'`).Scan(&checkDef); err != nil || !strings.Contains(checkDef, "provider") || !strings.Contains(checkDef, "subject") {
		t.Fatalf("check %q %v", checkDef, err)
	}

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	var playerID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO players (battletag, display_name, bnet_account_id)
		VALUES ('Mig#1','Mig','bnet-mig-identity') RETURNING id::text`).Scan(&playerID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity_links (provider, subject, email, player_id)
		VALUES ('smia','user-1','a@example.com',$1)`, playerID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity_links (provider, subject, player_id)
		VALUES ('smia','user-1',$1)`, playerID); err == nil {
		t.Fatal("expected unique provider+subject")
	}
}
