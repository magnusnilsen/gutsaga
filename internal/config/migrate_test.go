package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateOurSagasConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	olds, err := legacyDirs()
	if err != nil {
		t.Fatal(err)
	}
	if len(olds) == 0 || !strings.HasSuffix(olds[0], "OurSagas") {
		t.Fatalf("legacy order %v", olds)
	}
	if err := os.MkdirAll(olds[0], 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("{\"api_base_url\":\"http://127.0.0.1:8080\",\"client\":\"classic_era\"}\n")
	if err := os.WriteFile(filepath.Join(olds[0], "config.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacy(); err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, "Sagalore") {
		t.Fatalf("dir %s", dir)
	}
	got, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("got %s", got)
	}
	if _, err := os.Stat(filepath.Join(olds[0], "config.json")); err != nil {
		t.Fatal("old config should stay in place")
	}
	if err := MigrateLegacy(); err != nil {
		t.Fatal(err)
	}
}
