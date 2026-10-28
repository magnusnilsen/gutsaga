package savedvars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateOurSagasSavedVariables(t *testing.T) {
	wow := t.TempDir()
	dir := filepath.Join(wow, "WTF", "Account", "ACC", "Celebras", "Magni", "SavedVariables")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `
OurSagasDB = {
["completions"] = {
["4cc34091-0e9b-4c18-800a-3162c90acc56"] = {
["saga_id"] = "4cc34091-0e9b-4c18-800a-3162c90acc56",
["character"] = "Magni",
["version"] = 3,
["level"] = 2,
["completed_at"] = 1792698415,
["realm"] = "Celebras",
},
},
["objectives"] = {
},
}
`
	if err := os.WriteFile(filepath.Join(dir, "OurSagas.lua"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOurSagas(wow); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "Sagalore.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OurSagasDB") || !strings.Contains(string(got), "SagaloreDB") {
		t.Fatalf("rewritten file:\n%s", got)
	}
	files, err := ListSagaloreFiles(wow)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "Sagalore.lua" {
		t.Fatalf("files %v", files)
	}
	completions, err := ReadCompletions(wow)
	if err != nil || len(completions) != 1 || completions[0].Character != "Magni" {
		t.Fatalf("completions %+v err %v", completions, err)
	}

	// A later run must not clobber progress already stored under Sagalore.
	kept := "SagaloreDB = {\n[\"completions\"] = {\n[\"81006099-ddf8-4bc3-9ba0-b78a2614ba30\"] = {\n[\"saga_id\"] = \"81006099-ddf8-4bc3-9ba0-b78a2614ba30\",\n[\"character\"] = \"Mira\",\n[\"version\"] = 1,\n[\"level\"] = 4,\n[\"completed_at\"] = 1792698415,\n[\"realm\"] = \"Celebras\",\n},\n},\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "Sagalore.lua"), []byte(kept), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOurSagas(wow); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "Sagalore.lua"))
	if err != nil || string(after) != kept {
		t.Fatalf("overwrote current progress:\n%s", after)
	}
}

func TestMigrateFillsEmptySagaloreFile(t *testing.T) {
	wow := t.TempDir()
	dir := filepath.Join(wow, "WTF", "Account", "ACC", "SavedVariables")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	empty := "SagaloreDB = {\n[\"completions\"] = {\n},\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "Sagalore.lua"), []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}
	old := "OurSagasDB = {\n[\"seen_notice\"] = \"notice-1\",\n[\"completions\"] = {\n},\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "OurSagas.lua"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOurSagas(wow); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "Sagalore.lua"))
	if err != nil || !strings.Contains(string(got), `["seen_notice"]`) || strings.Contains(string(got), "OurSagasDB") {
		t.Fatalf("%s %v", got, err)
	}
}
