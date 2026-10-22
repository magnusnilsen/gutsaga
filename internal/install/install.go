package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/magnusnilsen/gutsaga/addon"
	"github.com/magnusnilsen/gutsaga/internal/wowpath"
)

// Install copies the embedded addon into Interface/AddOns/GutSaga.
// mode "forever" makes every TOC use the Forever interface number, because a
// Midnight-based client may load GutSaga_Mainline.toc. mode "retail" keeps the
// retail TOC and uses it as GutSaga.toc. An existing GutSagaData.lua is kept.
func Install(wowDir, mode string) error {
	dest := wowpath.AddonDir(wowDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	forever, err := addon.Files.ReadFile("GutSaga/GutSaga_Forever.toc")
	if err != nil {
		return err
	}
	mainline, err := addon.Files.ReadFile("GutSaga/GutSaga_Mainline.toc")
	if err != nil {
		return err
	}
	entries, err := fs.ReadDir(addon.Files, "GutSaga")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		target := filepath.Join(dest, name)
		if name == "GutSagaData.lua" {
			if _, err := os.Stat(target); err == nil {
				continue
			}
		}
		body, err := addon.Files.ReadFile("GutSaga/" + name)
		if err != nil {
			return err
		}
		switch name {
		case "GutSaga.toc", "GutSaga_Forever.toc", "GutSaga_Mainline.toc":
			if mode == "retail" && name == "GutSaga.toc" {
				body = mainline
			}
			if mode != "retail" {
				body = forever
			}
		}
		if err := writeFile(target, body); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, body []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tmp, path)
}
