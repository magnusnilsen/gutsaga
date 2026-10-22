package screenshot

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeUncompressedTGA(t *testing.T) {
	// 2x1, 32-bit BGRA, top-left origin, pixels red then green.
	header := make([]byte, 18)
	header[2] = 2
	header[12] = 2
	header[14] = 1
	header[16] = 32
	header[17] = 0x20
	pixels := []byte{
		0x00, 0x00, 0xff, 0xff,
		0x00, 0xff, 0x00, 0xff,
	}
	img, err := decodeTGA(append(header, pixels...))
	if err != nil {
		t.Fatal(err)
	}
	r, _, _, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 {
		t.Fatalf("pixel 0 red = %d", r>>8)
	}
	_, g, _, _ := img.At(1, 0).RGBA()
	if g>>8 != 255 {
		t.Fatalf("pixel 1 green = %d", g>>8)
	}
	out, ct, err := NormalizeUpload(append(header, pixels...))
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" {
		t.Fatalf("content type %s", ct)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal(err)
	}
}

func TestClosestScreenshot(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "WoWScrnShot_old.tga")
	neu := filepath.Join(dir, "WoWScrnShot_new.png")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(neu, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	around := time.Now().Add(-time.Hour)
	_ = os.Chtimes(old, around, around)
	_ = os.Chtimes(neu, time.Now(), time.Now())
	got, ok := Closest(dir, time.Now(), 2*time.Minute)
	if !ok || filepath.Base(got) != "WoWScrnShot_new.png" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got, ok = Closest(dir, around, 2*time.Minute)
	if !ok || filepath.Base(got) != "WoWScrnShot_old.tga" {
		t.Fatalf("old got %q ok=%v", got, ok)
	}
	if _, ok := Closest(dir, time.Now().Add(-48*time.Hour), time.Minute); ok {
		t.Fatal("expected no match outside slack")
	}
}
