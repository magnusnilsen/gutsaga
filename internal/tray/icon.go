package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

func Icon() []byte {
	pngBytes := iconPNG()
	if runtime.GOOS == "windows" {
		return icoFromPNG(pngBytes)
	}
	return pngBytes
}

func iconPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	gold := color.NRGBA{R: 212, G: 168, B: 74, A: 255}
	dark := color.NRGBA{R: 48, G: 32, B: 12, A: 255}
	ink := color.NRGBA{R: 48, G: 28, B: 8, A: 255}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, dark)
			if x < 2 || y < 2 || x > 29 || y > 29 {
				img.SetNRGBA(x, y, gold)
			}
		}
	}
	// A simple G.
	for _, p := range [][2]int{
		{10, 8}, {11, 8}, {12, 8}, {13, 8}, {14, 8}, {15, 8}, {16, 8}, {17, 8}, {18, 8}, {19, 8}, {20, 8},
		{10, 9}, {20, 9},
		{9, 10}, {10, 10}, {21, 10},
		{9, 11}, {21, 11},
		{9, 12},
		{9, 13},
		{9, 14}, {16, 14}, {17, 14}, {18, 14}, {19, 14}, {20, 14}, {21, 14},
		{9, 15}, {21, 15},
		{9, 16}, {21, 16},
		{9, 17}, {21, 17},
		{9, 18}, {21, 18},
		{10, 19}, {20, 19},
		{11, 20}, {12, 20}, {13, 20}, {14, 20}, {15, 20}, {16, 20}, {17, 20}, {18, 20}, {19, 20},
	} {
		img.SetNRGBA(p[0], p[1], ink)
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func icoFromPNG(pngBytes []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	buf.WriteByte(32)
	buf.WriteByte(32)
	buf.WriteByte(0)
	buf.WriteByte(0)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(22))
	buf.Write(pngBytes)
	return buf.Bytes()
}
