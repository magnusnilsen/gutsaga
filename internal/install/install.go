package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magnusnilsen/gutsaga/addon"
	"github.com/magnusnilsen/gutsaga/internal/wowpath"
)

// Install copies the addon into Interface/AddOns/OurSagas.
// When OURSAGAS_ADDON_SRC is set to an OurSagas folder (or its parent that
// contains OurSagas/), that tree is used instead of the embedded copy — so
// local go run picks up lua edits without rebuilding the binary.
// Each flavor TOC keeps its own interface number. OurSagas.toc is the fallback
// and matches mode. mode "forever" rewrites every TOC, because that client may
// load the Mainline file. An existing OurSagasData.lua is kept.
func Install(wowDir, mode string) error {
	dest := wowpath.AddonDir(wowDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	mode = wowpath.NormalizeClient(mode)
	if src := addonSrc(); src != "" {
		return installFromDir(src, dest, mode)
	}
	return installEmbedded(dest, mode)
}

func addonSrc() string {
	src := strings.TrimSpace(os.Getenv("OURSAGAS_ADDON_SRC"))
	if src == "" {
		return ""
	}
	if st, err := os.Stat(filepath.Join(src, "OurSagas.lua")); err == nil && !st.IsDir() {
		return src
	}
	nested := filepath.Join(src, "OurSagas")
	if st, err := os.Stat(filepath.Join(nested, "OurSagas.lua")); err == nil && !st.IsDir() {
		return nested
	}
	return ""
}

func installEmbedded(dest, mode string) error {
	bodies, err := readTOCBodies(func(name string) ([]byte, error) {
		return addon.Files.ReadFile("OurSagas/" + name)
	})
	if err != nil {
		return err
	}
	return fs.WalkDir(addon.Files, "OurSagas", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "OurSagas")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if filepath.Base(rel) == "OurSagasData.lua" {
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
		if d.Name() == "OurSagasData.lua" {
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
		"OurSagas_Forever.toc",
		"OurSagas_Mainline.toc",
		"OurSagas_Vanilla.toc",
		"OurSagas_Mists.toc",
		"OurSagas_TBC.toc",
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
	forever := bodies["OurSagas_Forever.toc"]
	if mode == "forever" {
		return forever
	}
	if name != "OurSagas.toc" {
		if body, ok := bodies[name]; ok {
			return body
		}
		return original
	}
	switch mode {
	case "retail":
		return bodies["OurSagas_Mainline.toc"]
	case "classic_era":
		return bodies["OurSagas_Vanilla.toc"]
	case "classic":
		return bodies["OurSagas_Mists.toc"]
	case "anniversary":
		return bodies["OurSagas_TBC.toc"]
	default:
		return forever
	}
}

// WriteLanguage records the companion language for the addon. Empty means the
// addon uses GetLocale() when that catalog exists.
func WriteLanguage(wowDir, lang string) error {
	path := filepath.Join(wowpath.AddonDir(wowDir), "OurSagasLanguage.lua")
	body := []byte("OurSagasLanguage = nil\n")
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "en", "nb":
		body = []byte("OurSagasLanguage = \"" + strings.ToLower(strings.TrimSpace(lang)) + "\"\n")
	}
	return writeFile(path, body)
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
