package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
)

type iconDir struct {
	Reserved uint16
	Type     uint16
	Count    uint16
}

type iconDirEntry struct {
	Width       byte
	Height      byte
	ColorCount  byte
	Reserved    byte
	Planes      uint16
	BitCount    uint16
	BytesInRes  uint32
	ImageOffset uint32
}

func resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	// Bilinear interpolation / nearest neighbor
	bounds := src.Bounds()
	dx := bounds.Dx()
	dy := bounds.Dy()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + (x*dx)/width
			srcY := bounds.Min.Y + (y*dy)/height
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func main() {
	f, err := os.Open("web/logo.png")
	if err != nil {
		log.Fatalf("Failed to open web/logo.png: %v", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		log.Fatalf("Failed to decode logo.png: %v", err)
	}

	sizes := []int{16, 32, 48, 64, 128, 256}
	var pngBuffers [][]byte

	for _, sz := range sizes {
		resized := resizeImage(img, sz, sz)
		var buf bytes.Buffer
		if err := png.Encode(&buf, resized); err != nil {
			log.Fatalf("Failed to encode png size %d: %v", sz, err)
		}
		pngBuffers = append(pngBuffers, buf.Bytes())
	}

	// Prepare ICO binary
	outBuf := new(bytes.Buffer)
	header := iconDir{
		Reserved: 0,
		Type:     1,
		Count:    uint16(len(sizes)),
	}
	_ = binary.Write(outBuf, binary.LittleEndian, header)

	offset := uint32(6 + len(sizes)*16)
	for i, sz := range sizes {
		var wByte, hByte byte
		if sz >= 256 {
			wByte = 0
			hByte = 0
		} else {
			wByte = byte(sz)
			hByte = byte(sz)
		}
		entry := iconDirEntry{
			Width:       wByte,
			Height:      hByte,
			ColorCount:  0,
			Reserved:    0,
			Planes:      1,
			BitCount:    32,
			BytesInRes:  uint32(len(pngBuffers[i])),
			ImageOffset: offset,
		}
		_ = binary.Write(outBuf, binary.LittleEndian, entry)
		offset += uint32(len(pngBuffers[i]))
	}

	for _, pBytes := range pngBuffers {
		outBuf.Write(pBytes)
	}

	if err := os.WriteFile("web/logo.ico", outBuf.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write logo.ico: %v", err)
	}
	if err := os.WriteFile("cmd/slimbox/logo.ico", outBuf.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write cmd/slimbox/logo.ico: %v", err)
	}

	fmt.Printf("Successfully generated web/logo.ico (%d bytes) with %d resolutions: %v\n", outBuf.Len(), len(sizes), sizes)
}
