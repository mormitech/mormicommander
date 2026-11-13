// This file converts between PNG and the device-independent bitmap (DIB) that Windows keeps images in on the
// clipboard. Screenshot tools and most Windows programs only put CF_DIB or CF_DIBV5 on the clipboard, while
// mormiCommander's "paste image as a new file" (TC 10+) wants PNG bytes, so the Windows backend runs every bitmap
// through DecodeDIB. A packed DIB is a BITMAPINFOHEADER (or its larger V4/V5 forms) followed by optional colour masks,
// an optional palette and the pixel rows, which are bottom-up unless the height is negative and padded to 4 bytes.
// The decoder handles 1, 4 and 8 bits with a palette and 16, 24 and 32 bits with or without bit-field masks, which
// covers what real programs write. It is plain byte work, so it lives in a platform-neutral file and is tested on
// every OS. The standard library has no BMP codec, which is why this small one exists.

package clipfiles

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
)

const (
	biRGB            = 0
	biBitfields      = 3
	biAlphaBitfields = 6
	maxDIBPixels     = 1 << 28 // refuse absurd sizes from a corrupt header instead of allocating gigabytes
)

// DecodeDIB reads a packed DIB (the contents of CF_DIB or CF_DIBV5) into an image.
func DecodeDIB(b []byte) (image.Image, error) {
	if len(b) < 40 {
		return nil, errors.New("bitmap header is too short")
	}
	le := binary.LittleEndian
	hsize := int(le.Uint32(b[0:]))
	if hsize < 40 || hsize > len(b) {
		return nil, fmt.Errorf("unsupported bitmap header size %d", hsize)
	}
	w := int(int32(le.Uint32(b[4:])))
	h := int(int32(le.Uint32(b[8:])))
	bpp := int(le.Uint16(b[14:]))
	comp := le.Uint32(b[16:])
	clrUsed := int(le.Uint32(b[32:]))
	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w*h > maxDIBPixels || w > maxDIBPixels || h > maxDIBPixels {
		return nil, fmt.Errorf("bad bitmap size %dx%d", w, h)
	}
	if comp != biRGB && comp != biBitfields && comp != biAlphaBitfields {
		return nil, fmt.Errorf("compressed bitmaps (type %d) are not supported", comp)
	}

	off := hsize
	var rMask, gMask, bMask, aMask uint32
	switch {
	case comp == biRGB && bpp == 16:
		rMask, gMask, bMask = 0x7C00, 0x03E0, 0x001F
	case comp == biRGB && bpp == 32:
		rMask, gMask, bMask = 0x00FF0000, 0x0000FF00, 0x000000FF
	case comp != biRGB:
		if bpp != 16 && bpp != 32 {
			return nil, fmt.Errorf("bit-field masks with %d bits per pixel", bpp)
		}
		n := 3
		if comp == biAlphaBitfields {
			n = 4
		}
		maskAt := 40
		if hsize == 40 { // masks follow a plain BITMAPINFOHEADER; larger headers carry them inside
			off += 4 * n
		}
		if len(b) < maskAt+4*n {
			return nil, errors.New("bitmap colour masks are missing")
		}
		rMask, gMask, bMask = le.Uint32(b[maskAt:]), le.Uint32(b[maskAt+4:]), le.Uint32(b[maskAt+8:])
		if n == 4 || hsize >= 56 {
			aMask = le.Uint32(b[maskAt+12:])
		}
	}

	var palette []color.NRGBA
	switch bpp {
	case 1, 4, 8:
		n := clrUsed
		if n == 0 || n > 1<<bpp {
			n = 1 << bpp
		}
		if len(b) < off+4*n {
			return nil, errors.New("bitmap palette is missing")
		}
		palette = make([]color.NRGBA, n)
		for i := range palette {
			p := b[off+4*i:]
			palette[i] = color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255}
		}
		off += 4 * n
	case 16, 24, 32:
		if clrUsed > 0 && clrUsed <= 1<<16 { // an optional palette that true-colour bitmaps may still carry
			off += 4 * clrUsed
		}
	default:
		return nil, fmt.Errorf("unsupported bitmap depth %d", bpp)
	}

	stride := ((w*bpp + 31) / 32) * 4
	if off > len(b) || len(b)-off < stride*h {
		return nil, errors.New("bitmap pixel data is truncated")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := 0; y < h; y++ {
		row := b[off+y*stride : off+(y+1)*stride]
		dy := h - 1 - y
		if topDown {
			dy = y
		}
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch bpp {
			case 1, 4, 8:
				bit := x * bpp
				idx := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				} else {
					c = color.NRGBA{A: 255}
				}
			case 24:
				p := row[3*x:]
				c = color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255}
			case 16, 32:
				var v uint32
				if bpp == 16 {
					v = uint32(le.Uint16(row[2*x:]))
				} else {
					v = le.Uint32(row[4*x:])
				}
				c = color.NRGBA{R: channel(v, rMask), G: channel(v, gMask), B: channel(v, bMask), A: 255}
				if aMask != 0 {
					c.A = channel(v, aMask)
				} else if bpp == 32 && comp == biRGB {
					c.A = uint8(v >> 24) // kept for now; replaced by opaque below if no pixel uses it
					if c.A != 0 {
						anyAlpha = true
					}
				}
			}
			img.SetNRGBA(x, dy, c)
		}
	}
	if bpp == 32 && comp == biRGB && aMask == 0 && !anyAlpha {
		// Plain 32-bit bitmaps usually leave the fourth byte at zero; reading it as alpha would give an invisible
		// picture, so such images are made opaque.
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img, nil
}

// channel extracts the masked bits of v and scales them to 0..255.
func channel(v, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := bits.TrailingZeros32(mask)
	width := bits.OnesCount32(mask)
	x := (v & mask) >> shift
	max := uint32(1)<<width - 1
	if width >= 32 {
		max = ^uint32(0)
	}
	return uint8(uint64(x) * 255 / uint64(max))
}

// EncodeDIB writes img as a packed 32-bit bottom-up DIB with a plain BITMAPINFOHEADER, which every Windows program
// reads. The fourth byte carries straight (not premultiplied) alpha.
func EncodeDIB(img image.Image) []byte {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	b := make([]byte, 40+4*w*h)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 40)
	le.PutUint32(b[4:], uint32(w))
	le.PutUint32(b[8:], uint32(h))
	le.PutUint16(b[12:], 1)  // planes
	le.PutUint16(b[14:], 32) // bits per pixel
	le.PutUint32(b[16:], biRGB)
	le.PutUint32(b[20:], uint32(4*w*h))
	le.PutUint32(b[24:], 2835) // 72 dpi in pixels per metre
	le.PutUint32(b[28:], 2835)
	for y := 0; y < h; y++ {
		row := b[40+4*w*(h-1-y):]
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
			row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = c.B, c.G, c.R, c.A
		}
	}
	return b
}

// DIBToPNG converts a packed DIB to PNG bytes.
func DIBToPNG(dib []byte) ([]byte, error) {
	img, err := DecodeDIB(dib)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PNGToDIB converts PNG bytes to a packed DIB.
func PNGToDIB(data []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a PNG image: %w", err)
	}
	return EncodeDIB(img), nil
}
