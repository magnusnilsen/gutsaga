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
	data := filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagasData.lua")
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte("OurSagasData = { entries = { kept = true } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(wow, "forever"); err != nil {
		t.Fatal(err)
	}
	mainline, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagas_Mainline.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainline), "Interface: 16001") {
		t.Fatalf("forever install should force 16001:\n%s", mainline)
	}
	lua, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagas.lua"))
	if err != nil || !strings.Contains(string(lua), "CHARACTER") {
		t.Fatal("addon lua missing")
	}
	if _, err := os.Stat(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagasObjectives.lua")); err != nil {
		t.Fatal(err)
	}
	chains, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagasChains.lua"))
	if err != nil || !strings.Contains(string(chains), "chain.locked") {
		t.Fatal("chain helper missing from install")
	}
	forever, err := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagas_Forever.toc"))
	if err != nil || !strings.Contains(string(forever), "OurSagasChains.lua") {
		t.Fatal("forever toc should load chains")
	}
	kept, _ := os.ReadFile(data)
	if !strings.Contains(string(kept), "kept = true") {
		t.Fatal("data file was overwritten")
	}
	if err := Install(wow, "retail"); err != nil {
		t.Fatal(err)
	}
	toc, _ := os.ReadFile(filepath.Join(wow, "Interface", "AddOns", "OurSagas", "OurSagas.toc"))
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
		b, err := os.ReadFile(filepath.Join(root, "OurSagas", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "## Interface: "+want) {
			t.Fatalf("%s does not contain %s", name, want)
		}
	}
	check("OurSagas_Forever.toc", forever)
	check("OurSagas.toc", forever)
	check("OurSagas_Mainline.toc", mainline)
	check("OurSagas_Vanilla.toc", between(text, "VANILLA=", "\n"))
	check("OurSagas_Mists.toc", between(text, "MISTS=", "\n"))
	check("OurSagas_TBC.toc", between(text, "TBC=", "\n"))
}

func TestInstallClassicKeepsFlavorTOCs(t *testing.T) {
	wow := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wow, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(wow, "classic"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wow, "Interface", "AddOns", "OurSagas")
	toc, _ := os.ReadFile(filepath.Join(root, "OurSagas.toc"))
	vanilla, _ := os.ReadFile(filepath.Join(root, "OurSagas_Vanilla.toc"))
	mists, _ := os.ReadFile(filepath.Join(root, "OurSagas_Mists.toc"))
	if !strings.Contains(string(toc), "Interface: 50504") {
		t.Fatalf("classic fallback toc:\n%s", toc)
	}
	if !strings.Contains(string(vanilla), "Interface: 11509") || !strings.Contains(string(mists), "Interface: 50504") {
		t.Fatalf("flavor tocs were rewritten:\n%s\n%s", vanilla, mists)
	}
	if err := Install(wow, "classic_era"); err != nil {
		t.Fatal(err)
	}
	toc, _ = os.ReadFile(filepath.Join(root, "OurSagas.toc"))
	if !strings.Contains(string(toc), "Interface: 11509") {
		t.Fatalf("era fallback toc:\n%s", toc)
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
