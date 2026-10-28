package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallForeverRewritesInterface(t *testing.T) {
	wow := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wow, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(wow, "Interface", "AddOns", "Sagalore", "SagaloreData.lua")
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte("SagaloreData = { entries = { kept = true } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(wow, "forever"); err != nil {
		t.Fatal(err)
	}
	mainline, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "Sagalore_Mainline.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainline), "Interface: 16001") {
		t.Fatalf("forever install should force 16001:\n%s", mainline)
	}
	lua, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "Sagalore.lua"))
	if err != nil || !strings.Contains(string(lua), "CHARACTER") {
		t.Fatal("addon lua missing")
	}
	if _, err := os.Stat(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "SagaloreObjectives.lua")); err != nil {
		t.Fatal(err)
	}
	chains, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "SagaloreChains.lua"))
	if err != nil || !strings.Contains(string(chains), "chain.locked") {
		t.Fatal("chain helper missing from install")
	}
	forever, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "Sagalore_Forever.toc"))
	if err != nil || !strings.Contains(string(forever), "SagaloreChains.lua") {
		t.Fatal("forever toc should load chains")
	}
	kept, _ := os.ReadFile(data)
	if !strings.Contains(string(kept), "kept = true") {
		t.Fatal("data file was overwritten")
	}
	if err := Install(wow, "retail"); err != nil {
		t.Fatal(err)
	}
	toc, _ := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "Sagalore.toc"))
	if !strings.Contains(string(toc), "Interface: 120000") {
		t.Fatalf("retail toc:\n%s", toc)
	}
}

func TestTOCFilesMatchInterfaceList(t *testing.T) {
	root := filepath.Join("..", "..", "addon")
	list, err := os.ReadFile(filepath.Join(root, "INTERFACE"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(list)
	forever := between(text, "FOREVER=", "\n")
	mainline := between(text, "MAINLINE=", "\n")
	check := func(name, want string) {
		b, err := os.ReadFile(filepath.Join(root, "Sagalore", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "## Interface: "+want) {
			t.Fatalf("%s does not contain %s", name, want)
		}
	}
	check("Sagalore_Forever.toc", forever)
	check("Sagalore.toc", forever)
	check("Sagalore_Mainline.toc", mainline)
	check("Sagalore_Vanilla.toc", between(text, "VANILLA=", "\n"))
	check("Sagalore_Mists.toc", between(text, "MISTS=", "\n"))
	check("Sagalore_TBC.toc", between(text, "TBC=", "\n"))
}

func TestInstallClassicKeepsFlavorTOCs(t *testing.T) {
	wow := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wow, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(wow, "classic"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wow, "Interface", "AddOns", "Sagalore")
	toc, _ := os.ReadFile(filepath.Join(root, "Sagalore.toc"))
	vanilla, _ := os.ReadFile(filepath.Join(root, "Sagalore_Vanilla.toc"))
	mists, _ := os.ReadFile(filepath.Join(root, "Sagalore_Mists.toc"))
	if !strings.Contains(string(toc), "Interface: 50504") {
		t.Fatalf("classic fallback toc:\n%s", toc)
	}
	if !strings.Contains(string(vanilla), "Interface: 11509") || !strings.Contains(string(mists), "Interface: 50504") {
		t.Fatalf("flavor tocs were rewritten:\n%s\n%s", vanilla, mists)
	}
	if err := Install(wow, "classic_era"); err != nil {
		t.Fatal(err)
	}
	toc, _ = os.ReadFile(filepath.Join(root, "Sagalore.toc"))
	if !strings.Contains(string(toc), "Interface: 11509") {
		t.Fatalf("era fallback toc:\n%s", toc)
	}
}

func TestInstallMigratesAndRemovesLegacyAddon(t *testing.T) {
	wow := t.TempDir()
	legacy := filepath.Join(wow, "Interface", "AddOns", "OurSagas")
	if err := os.MkdirAll(filepath.Join(legacy, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "OurSagasData.lua"), []byte("OurSagasData = { entries = { kept = true } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "media", "quest.tga"), []byte("TGAdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	sv := filepath.Join(wow, "WTF", "Account", "ACC", "Celebras", "Magni", "SavedVariables")
	if err := os.MkdirAll(sv, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "OurSagasDB = {\n[\"completions\"] = {\n[\"4cc34091-0e9b-4c18-800a-3162c90acc56\"] = {\n[\"saga_id\"] = \"4cc34091-0e9b-4c18-800a-3162c90acc56\",\n[\"character\"] = \"Magni\",\n[\"version\"] = 1,\n[\"level\"] = 2,\n[\"completed_at\"] = 1792698415,\n[\"realm\"] = \"Celebras\",\n},\n},\n}\n"
	if err := os.WriteFile(filepath.Join(sv, "OurSagas.lua"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(wow, "classic_era"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy addon still present: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "SagaloreData.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "kept = true") || strings.Contains(string(data), "OurSagas") || !strings.Contains(string(data), "SagaloreData") {
		t.Fatalf("data file:\n%s", data)
	}
	tga, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "media", "quest.tga"))
	if err != nil || string(tga) != "TGAdata" {
		t.Fatalf("tga %q err %v", tga, err)
	}
	migrated, err := os.ReadFile(filepath.Join(sv, "Sagalore.lua"))
	if err != nil || !strings.Contains(string(migrated), "SagaloreDB") || strings.Contains(string(migrated), "OurSagasDB") {
		t.Fatalf("saved vars:\n%s %v", migrated, err)
	}
	if err := Install(wow, "classic_era"); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "Sagalore", "SagaloreData.lua"))
	if err != nil || string(again) != string(data) {
		t.Fatalf("second install rewrote data:\n%s", again)
	}
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(s[:j])
}
