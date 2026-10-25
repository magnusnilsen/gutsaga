package app

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/magnusnilsen/gutsaga/internal/wowpath"
)

func osName() string   { return runtime.GOOS }
func archName() string { return runtime.GOARCH }

func restart() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(0)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Start()
	os.Exit(0)
}

func installZip(wowDir string, body []byte) error {
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	dest := wowpath.AddonDir(wowDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == "" || name == "." || name == "SagaBoardData.lua" {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dest, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
