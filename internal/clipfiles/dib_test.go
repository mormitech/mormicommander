// These tests check the DIB codec against hand-built bitmaps in each layout Windows programs put on the clipboard:
// 24-bit bottom-up, 32-bit with an unused fourth byte, 32-bit V5 with an alpha mask, 16-bit 5-5-5, 8-bit with a
// palette, 1-bit and top-down rows. They also check the PNG round trip and that corrupt headers give errors instead
// of panics or huge allocations.

package clipfiles

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// header builds a BITMAPINFOHEADER of the given size.
func header(size, w, h, bpp int, comp uint32, clrUsed int) []byte {
	b := make([]byte, size)
	le := binary.LittleEndian
	le.PutUint32(b[0:], uint32(size))
	le.PutUint32(b[4:], uint32(int32(w)))
	le.PutUint32(b[8:], uint32(int32(h)))
	le.PutUint16(b[12:], 1)
	le.PutUint16(b[14:], uint16(bpp))
	le.PutUint32(b[16:], comp)
	le.PutUint32(b[32:], uint32(clrUsed))
	return b
}

func px(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func TestDecodeDIB(t *testing.T) {
	red, green, blue := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 255, 0, 255}, color.NRGBA{0, 0, 255, 255}
	white := color.NRGBA{255, 255, 255, 255}

	tests := []struct {
		name string
		dib  []byte
		// expected pixels: top-left, top-right, bottom-left
		tl, tr, bl color.NRGBA
	}{
		{
			name: "24-bit bottom-up 2x2",
			dib: append(header(40, 2, 2, 24, biRGB, 0),
				// bottom row: blue, white (padded to 8 bytes), top row: red, green
				255, 0, 0, 255, 255, 255, 0, 0,
				0, 0, 255, 0, 255, 0, 0, 0),
			tl: red, tr: green, bl: blue,
		},
		{
			name: "24-bit top-down",
			dib: append(header(40, 2, -2, 24, biRGB, 0),
				0, 0, 255, 0, 255, 0, 0, 0,
				255, 0, 0, 255, 255, 255, 0, 0),
			tl: red, tr: green, bl: blue,
		},
		{
			name: "32-bit with zero fourth byte is opaque",
			dib: append(header(40, 2, 2, 32, biRGB, 0),
				255, 0, 0, 0, 255, 255, 255, 0,
				0, 0, 255, 0, 0, 255, 0, 0),
			tl: red, tr: green, bl: blue,
		},
		{
			name: "32-bit V5 bitfields with alpha",
			dib: func() []byte {
				h := header(124, 2, 2, 32, biBitfields, 0)
				le := binary.LittleEndian
				le.PutUint32(h[40:], 0x00FF0000)
				le.PutUint32(h[44:], 0x0000FF00)
				le.PutUint32(h[48:], 0x000000FF)
				le.PutUint32(h[52:], 0xFF000000)
				return append(h,
					255, 0, 0, 255, 255, 255, 255, 255,
					0, 0, 255, 128, 0, 255, 0, 255)
			}(),
			tl: color.NRGBA{255, 0, 0, 128}, tr: green, bl: blue,
		},
		{
			name: "32-bit bitfields after a plain header",
			dib: func() []byte {
				h := header(40, 2, 2, 32, biBitfields, 0)
				m := make([]byte, 12)
				binary.LittleEndian.PutUint32(m[0:], 0x000000FF) // red in the low byte (RGBA order)
				binary.LittleEndian.PutUint32(m[4:], 0x0000FF00)
				binary.LittleEndian.PutUint32(m[8:], 0x00FF0000)
				return append(append(h, m...),
					0, 0, 255, 0, 255, 255, 255, 0,
					255, 0, 0, 0, 0, 255, 0, 0)
			}(),
			tl: red, tr: green, bl: blue,
		},
		{
			name: "16-bit 5-5-5",
			dib: append(header(40, 2, 2, 16, biRGB, 0),
				0x1F, 0x00, 0xFF, 0x7F, // blue, white
				0x00, 0x7C, 0xE0, 0x03), // red, green
			tl: red, tr: green, bl: blue,
		},
		{
			name: "8-bit palette",
			dib: append(append(header(40, 2, 2, 8, biRGB, 4),
				0, 0, 255, 0, 0, 255, 0, 0, 255, 0, 0, 0, 255, 255, 255, 0), // red, green, blue, white
				2, 3, 0, 0,
				0, 1, 0, 0),
			tl: red, tr: green, bl: blue,
		},
		{
			name: "1-bit palette",
			dib: append(append(header(40, 2, 2, 1, biRGB, 0),
				0, 0, 0, 0, 255, 255, 255, 0), // black, white
				0x40, 0, 0, 0, // bottom row: black, white
				0x80, 0, 0, 0), // top row: white, black
			tl: white, tr: color.NRGBA{0, 0, 0, 255}, bl: color.NRGBA{0, 0, 0, 255},
		},
	}
	for _, tt := range tests {
		img, err := DecodeDIB(tt.dib)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got := px(img, 0, 0); got != tt.tl {
			t.Errorf("%s: top-left = %v, want %v", tt.name, got, tt.tl)
		}
		if got := px(img, 1, 0); got != tt.tr {
			t.Errorf("%s: top-right = %v, want %v", tt.name, got, tt.tr)
		}
		if got := px(img, 0, 1); got != tt.bl {
			t.Errorf("%s: bottom-left = %v, want %v", tt.name, got, tt.bl)
		}
	}
}

func TestDecodeDIBErrors(t *testing.T) {
	tests := map[string][]byte{
		"short":       {1, 2, 3},
		"tiny header": header(40, 1, 1, 24, biRGB, 0)[:20],
		"bad size":    func() []byte { b := header(40, 1, 1, 24, biRGB, 0); b[0] = 12; return b }(),
		"zero width":  append(header(40, 0, 1, 24, biRGB, 0), 0, 0, 0, 0),
		"huge":        header(40, 1<<20, 1<<20, 24, biRGB, 0),
		"rle":         append(header(40, 1, 1, 8, 1, 0), make([]byte, 1028)...),
		"depth 2":     append(header(40, 1, 1, 2, biRGB, 0), make([]byte, 64)...),
		"truncated":   append(header(40, 4, 4, 24, biRGB, 0), 1, 2, 3),
		"no palette":  header(40, 1, 1, 8, biRGB, 0),
	}
	for name, b := range tests {
		if _, err := DecodeDIB(b); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestPNGDIBRoundTrip(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	src.SetNRGBA(0, 0, color.NRGBA{10, 20, 30, 255})
	src.SetNRGBA(2, 0, color.NRGBA{200, 100, 50, 128})
	src.SetNRGBA(1, 1, color.NRGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	dib, err := PNGToDIB(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	out, err := DIBToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v, want %v", img.Bounds(), src.Bounds())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, want := px(img, x, y), src.NRGBAAt(x, y); got != want && want.A != 0 {
				t.Errorf("pixel %d,%d = %v, want %v", x, y, got, want)
			}
		}
	}
	if _, err := PNGToDIB([]byte("not a png")); err == nil {
		t.Fatal("PNGToDIB must reject non-PNG data")
	}
}
