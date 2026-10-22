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
// Each flavor TOC keeps its own interface number so the client can pick
// GutSaga_Vanilla.toc, GutSaga_Mists.toc, GutSaga_TBC.toc, or GutSaga_Mainline.toc.
// GutSaga.toc is the fallback and matches mode. mode "forever" rewrites every
// TOC, because that client may load the Mainline file. An existing
// GutSagaData.lua is kept.
func Install(wowDir, mode string) error {
	dest := wowpath.AddonDir(wowDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	mode = wowpath.NormalizeClient(mode)
	bodies := map[string][]byte{}
	for _, name := range []string{
		"GutSaga_Forever.toc",
		"GutSaga_Mainline.toc",
		"GutSaga_Vanilla.toc",
		"GutSaga_Mists.toc",
		"GutSaga_TBC.toc",
	} {
		body, err := addon.Files.ReadFile("GutSaga/" + name)
		if err != nil {
			return err
		}
		bodies[name] = body
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
		if isTOC(name) {
			body = tocBody(mode, name, bodies, body)
		}
		if err := writeFile(target, body); err != nil {
			return err
		}
	}
	return nil
}

func isTOC(name string) bool {
	return len(name) > 4 && name[len(name)-4:] == ".toc"
}

func tocBody(mode, name string, bodies map[string][]byte, original []byte) []byte {
	forever := bodies["GutSaga_Forever.toc"]
	if mode == "forever" {
		return forever
	}
	if name != "GutSaga.toc" {
		if body, ok := bodies[name]; ok {
			return body
		}
		return original
	}
	switch mode {
	case "retail":
		return bodies["GutSaga_Mainline.toc"]
	case "classic_era":
		return bodies["GutSaga_Vanilla.toc"]
	case "classic":
		return bodies["GutSaga_Mists.toc"]
	case "anniversary":
		return bodies["GutSaga_TBC.toc"]
	default:
		return forever
	}
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
