package screenshot

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// decodeTGA reads an uncompressed (type 2) or RLE (type 10) true-color TGA.
// WoW screenshots are usually uncompressed 24- or 32-bit BGR(A).
func decodeTGA(data []byte) (image.Image, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("tga too short")
	}
	idLen := int(data[0])
	colorMapType := data[1]
	imageType := data[2]
	if colorMapType != 0 {
		return nil, fmt.Errorf("tga color maps are not supported")
	}
	if imageType != 2 && imageType != 10 {
		return nil, fmt.Errorf("unsupported tga type %d", imageType)
	}
	width := int(binary.LittleEndian.Uint16(data[12:14]))
	height := int(binary.LittleEndian.Uint16(data[14:16]))
	depth := data[16]
	desc := data[17]
	if width <= 0 || height <= 0 || width > 10000 || height > 10000 {
		return nil, fmt.Errorf("tga size %dx%d", width, height)
	}
	if depth != 24 && depth != 32 {
		return nil, fmt.Errorf("unsupported tga depth %d", depth)
	}
	offset := 18 + idLen
	if offset > len(data) {
		return nil, fmt.Errorf("tga header truncated")
	}
	bpp := int(depth / 8)
	topOrigin := desc&0x20 != 0
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	pix := data[offset:]

	put := func(x, y int, b []byte) {
		if x < 0 || y < 0 || x >= width || y >= height || len(b) < bpp {
			return
		}
		yy := y
		if !topOrigin {
			yy = height - 1 - y
		}
		c := color.NRGBA{R: b[2], G: b[1], B: b[0], A: 255}
		if bpp == 4 {
			c.A = b[3]
		}
		img.SetNRGBA(x, yy, c)
	}

	if imageType == 2 {
		need := width * height * bpp
		if len(pix) < need {
			return nil, fmt.Errorf("tga pixels truncated")
		}
		i := 0
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				put(x, y, pix[i:i+bpp])
				i += bpp
			}
		}
		return img, nil
	}

	// RLE type 10.
	x, y := 0, 0
	i := 0
	for y < height {
		if i >= len(pix) {
			return nil, fmt.Errorf("tga rle truncated")
		}
		header := pix[i]
		i++
		count := int(header&0x7f) + 1
		raw := header&0x80 == 0
		var packet []byte
		if !raw {
			if i+bpp > len(pix) {
				return nil, fmt.Errorf("tga rle packet truncated")
			}
			packet = pix[i : i+bpp]
			i += bpp
		}
		for n := 0; n < count; n++ {
			if raw {
				if i+bpp > len(pix) {
					return nil, fmt.Errorf("tga rle raw truncated")
				}
				packet = pix[i : i+bpp]
				i += bpp
			}
			put(x, y, packet)
			x++
			if x == width {
				x = 0
				y++
			}
		}
	}
	return img, nil
}

// NormalizeUpload returns image bytes Discord can attach.
// PNG, JPEG, GIF and WebP pass through. TGA is converted to PNG.
func NormalizeUpload(data []byte) (out []byte, contentType string, err error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty image")
	}
	if len(data) > 8<<20 {
		return nil, "", fmt.Errorf("image larger than 8 MiB")
	}
	switch {
	case isPNG(data):
		return data, "image/png", nil
	case isJPEG(data):
		return data, "image/jpeg", nil
	case isGIF(data):
		return data, "image/gif", nil
	case isWEBP(data):
		return data, "image/webp", nil
	}
	img, err := decodeTGA(data)
	if err != nil {
		return nil, "", fmt.Errorf("unsupported screenshot: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/png", nil
}

func Extension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func isPNG(b []byte) bool {
	return len(b) > 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}
func isJPEG(b []byte) bool { return len(b) > 2 && b[0] == 0xff && b[1] == 0xd8 }
func isGIF(b []byte) bool {
	return len(b) > 6 && (bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")))
}
func isWEBP(b []byte) bool {
	return len(b) > 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
}
