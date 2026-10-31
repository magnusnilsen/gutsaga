package install

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magnusnilsen/gutsaga/addon"
	"github.com/magnusnilsen/gutsaga/internal/envcompat"
	"github.com/magnusnilsen/gutsaga/internal/savedvars"
	"github.com/magnusnilsen/gutsaga/internal/wowpath"
)

// Install copies the addon into Interface/AddOns/Sagalore.
// When SAGALORE_ADDON_SRC is set to a Sagalore folder (or its parent that
// contains Sagalore/), that tree is used instead of the embedded copy — so
// local go run picks up lua edits without rebuilding the binary.
// OURSAGAS_ADDON_SRC is still accepted.
// Each flavor TOC keeps its own interface number. Sagalore.toc is the fallback
// and matches mode. mode "forever" rewrites every TOC, because that client may
// load the Mainline file. An existing SagaloreData.lua is kept.
// A previous Interface/AddOns/OurSagas folder is migrated (data file and cover
// TGAs) and then removed so the client does not load two addons. SavedVariables
// are copied from OurSagas.lua to Sagalore.lua.
func Install(wowDir, mode string) error {
	dest := wowpath.AddonDir(wowDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := importLegacyAddon(wowDir, dest); err != nil {
		return err
	}
	mode = wowpath.NormalizeClient(mode)
	var err error
	if src := addonSrc(); src != "" {
		err = installFromDir(src, dest, mode)
	} else {
		err = installEmbedded(dest, mode)
	}
	if err != nil {
		return err
	}
	if err := savedvars.MigrateOurSagas(wowDir); err != nil {
		return err
	}
	return retireLegacyAddon(wowDir)
}

func addonSrc() string {
	src := envcompat.Get("SAGALORE_ADDON_SRC")
	if src == "" {
		return ""
	}
	if st, err := os.Stat(filepath.Join(src, "Sagalore.lua")); err == nil && !st.IsDir() {
		return src
	}
	nested := filepath.Join(src, "Sagalore")
	if st, err := os.Stat(filepath.Join(nested, "Sagalore.lua")); err == nil && !st.IsDir() {
		return nested
	}
	return ""
}

func installEmbedded(dest, mode string) error {
	bodies, err := readTOCBodies(func(name string) ([]byte, error) {
		return addon.Files.ReadFile("Sagalore/" + name)
	})
	if err != nil {
		return err
	}
	return fs.WalkDir(addon.Files, "Sagalore", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "Sagalore")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if filepath.Base(rel) == "SagaloreData.lua" {
			if _, err := os.Stat(target); err == nil {
				return nil
			}
		}
		body, err := addon.Files.ReadFile(path)
		if err != nil {
			return err
		}
		if isTOC(rel) {
			body = tocBody(mode, filepath.Base(rel), bodies, body)
		}
		return writeFile(target, body)
	})
}

func installFromDir(src, dest, mode string) error {
	bodies, err := readTOCBodies(func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(src, name))
	})
	if err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Name() == "SagaloreData.lua" {
			if _, err := os.Stat(target); err == nil {
				return nil
			}
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isTOC(d.Name()) {
			body = tocBody(mode, d.Name(), bodies, body)
		}
		return writeFile(target, body)
	})
}

func readTOCBodies(read func(string) ([]byte, error)) (map[string][]byte, error) {
	bodies := map[string][]byte{}
	for _, name := range []string{
		"Sagalore_Forever.toc",
		"Sagalore_Mainline.toc",
		"Sagalore_Vanilla.toc",
		"Sagalore_Mists.toc",
		"Sagalore_TBC.toc",
	} {
		body, err := read(name)
		if err != nil {
			return nil, err
		}
		bodies[name] = body
	}
	return bodies, nil
}

func isTOC(name string) bool {
	return len(name) > 4 && name[len(name)-4:] == ".toc"
}

func tocBody(mode, name string, bodies map[string][]byte, original []byte) []byte {
	forever := bodies["Sagalore_Forever.toc"]
	if mode == "forever" {
		return forever
	}
	if name != "Sagalore.toc" {
		if body, ok := bodies[name]; ok {
			return body
		}
		return original
	}
	switch mode {
	case "retail":
		return bodies["Sagalore_Mainline.toc"]
	case "classic_era":
		return bodies["Sagalore_Vanilla.toc"]
	case "classic":
		return bodies["Sagalore_Mists.toc"]
	case "anniversary":
		return bodies["Sagalore_TBC.toc"]
	default:
		return forever
	}
}

// WriteLanguage records the companion language for the addon.
// English only for now; lang is ignored so a stale config cannot flip the UI.
func WriteLanguage(wowDir, lang string) error {
	_ = lang
	path := filepath.Join(wowpath.AddonDir(wowDir), "SagaloreLanguage.lua")
	body := []byte("-- English only for now; other catalogs remain in Locales/ for later.\nSagaloreLanguage = \"en\"\n")
	return writeFile(path, body)
}

func legacyAddonDir(wowDir string) string {
	return filepath.Join(wowDir, "Interface", "AddOns", "OurSagas")
}

func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// importLegacyAddon copies the generated data file and cover TGAs out of
// Interface/AddOns/OurSagas before that folder is removed. Code and textures
// come from the new embed. An existing Sagalore file is kept.
func importLegacyAddon(wowDir, dest string) error {
	legacy := legacyAddonDir(wowDir)
	if sameDir(legacy, dest) {
		return nil
	}
	st, err := os.Stat(legacy)
	if err != nil || !st.IsDir() {
		return nil
	}
	if err := importLegacyData(legacy, dest); err != nil {
		return err
	}
	return importLegacyMedia(legacy, dest)
}

func importLegacyData(legacy, dest string) error {
	target := filepath.Join(dest, "SagaloreData.lua")
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	body, err := os.ReadFile(filepath.Join(legacy, "OurSagasData.lua"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Rename the global and the generated header only. Saga text in the file
	// can contain the old product name and must stay as the player wrote it.
	body = bytes.ReplaceAll(body, []byte("OurSagasData"), []byte("SagaloreData"))
	body = bytes.ReplaceAll(body, []byte("OurSagas companion"), []byte("Sagalore companion"))
	return writeFile(target, body)
}

func importLegacyMedia(legacy, dest string) error {
	src := filepath.Join(legacy, "media")
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	media := filepath.Join(dest, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".tga") {
			continue
		}
		target := filepath.Join(media, name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		body, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			return err
		}
		if err := writeFile(target, body); err != nil {
			return err
		}
	}
	return nil
}

// retireLegacyAddon removes Interface/AddOns/OurSagas so the client does not
// load the old addon beside Sagalore. If the folder is locked, it is renamed
// to OurSagas.disabled, which WoW will not load.
func retireLegacyAddon(wowDir string) error {
	legacy := legacyAddonDir(wowDir)
	if sameDir(legacy, wowpath.AddonDir(wowDir)) {
		return nil
	}
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	if err := os.RemoveAll(legacy); err != nil {
		disabled := legacy + ".disabled"
		_ = os.RemoveAll(disabled)
		if renErr := os.Rename(legacy, disabled); renErr != nil {
			return fmt.Errorf("remove Interface/AddOns/OurSagas: %w", err)
		}
	}
	return nil
}

func writeFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tmp, path)
}
