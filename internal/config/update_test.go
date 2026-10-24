package config

import "testing"

func TestNormalizeUpdateRepo(t *testing.T) {
	cfg := Config{}
	cfg.Normalize()
	if cfg.Update.Repo != DefaultUpdateRepo || !cfg.Update.On() {
		t.Fatalf("%+v", cfg.Update)
	}
	cfg.Update.Repo = "magnusnilsen/gutsaga-releases"
	cfg.Normalize()
	if cfg.Update.Repo != DefaultUpdateRepo {
		t.Fatal(cfg.Update.Repo)
	}
	cfg.Update.SetEnabled(false)
	if cfg.Update.On() {
		t.Fatal("expected off")
	}
}
