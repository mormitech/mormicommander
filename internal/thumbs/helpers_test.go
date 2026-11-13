// Shared test helpers: they build small synthetic images in every supported
// format, splice EXIF blocks and PNG chunks into encoded files, and write
// everything into t.TempDir() so the tests never touch real user files.

package thumbs

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

var (
	red  = color.RGBA{255, 0, 0, 255}
	blue = color.RGBA{0, 0, 255, 255}
)

// halves returns a w x h image whose left half is red and right half blue.
func halves(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.SetRGBA(x, y, red)
			} else {
				img.SetRGBA(x, y, blue)
			}
		}
	}
	return img
}

func encodeAs(t *testing.T, format string, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	case "png":
		err = png.Encode(&b, img)
	case "gif":
		err = gif.Encode(&b, img, nil)
	case "bmp":
		err = bmp.Encode(&b, img)
	case "tiff":
		err = tiff.Encode(&b, img, nil)
	default:
		t.Fatalf("unknown format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// tiffBlock builds a minimal TIFF structure with one orientation entry.
func tiffBlock(orient int, bigEndian bool) []byte {
	var bo binary.ByteOrder = binary.LittleEndian
	hdr := []byte("II")
	if bigEndian {
		bo = binary.BigEndian
		hdr = []byte("MM")
	}
	b := make([]byte, 8+2+12+4)
	copy(b, hdr)
	bo.PutUint16(b[2:], 42)
	bo.PutUint32(b[4:], 8)
	bo.PutUint16(b[8:], 1)
	bo.PutUint16(b[10:], tagOrientation)
	bo.PutUint16(b[12:], 3)
	bo.PutUint32(b[14:], 1)
	bo.PutUint16(b[18:], uint16(orient))
	return b
}

// withJPEGExif inserts an APP1 Exif segment right after SOI.
func withJPEGExif(jpg []byte, orient int, bigEndian bool) []byte {
	payload := append([]byte("Exif\x00\x00"), tiffBlock(orient, bigEndian)...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

// pngChunk encodes one PNG chunk with its CRC.
func pngChunk(typ string, body []byte) []byte {
	c := make([]byte, 8, 12+len(body))
	binary.BigEndian.PutUint32(c, uint32(len(body)))
	copy(c[4:], typ)
	c = append(c, body...)
	crc := crc32.ChecksumIEEE(c[4:])
	return binary.BigEndian.AppendUint32(c, crc)
}

// withPNGChunk inserts a chunk right after IHDR (8 byte signature + 25 byte
// IHDR chunk).
func withPNGChunk(p []byte, typ string, body []byte) []byte {
	const at = 8 + 25
	out := append([]byte{}, p[:at]...)
	out = append(out, pngChunk(typ, body)...)
	return append(out, p[at:]...)
}

// colorAt returns the colour of the decoded thumbnail at (x, y).
func colorAt(t *testing.T, th *Thumb, x, y int) color.RGBA {
	t.Helper()
	img, err := th.Image()
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// isRedish / isBlueish tolerate JPEG ringing.
func isRedish(c color.RGBA) bool  { return c.R > 180 && c.B < 90 }
func isBlueish(c color.RGBA) bool { return c.B > 180 && c.R < 90 }
