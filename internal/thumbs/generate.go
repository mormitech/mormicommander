// This file turns one image file into a thumbnail. It first sniffs a few
// bytes to decide whether the file is a format we can decode at all, so a
// folder full of text files or videos costs one tiny read per file instead of
// a full load. For real images it checks the declared pixel count before
// decoding (to refuse decompression bombs), decodes, reads the EXIF
// orientation, scales the picture down with a good-quality filter so it fits
// the box, rotates or mirrors it upright, and encodes it as JPEG or PNG.

package thumbs

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"

	// Decoders registered with the image package.
	_ "image/gif"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// sniffLen is how many leading bytes sniffFormat needs.
const sniffLen = 16

// sniffFormat recognises the formats we can decode by their magic bytes and
// returns the decoder name, or "" when the file is something else.
func sniffFormat(h []byte) string {
	switch {
	case len(h) >= 3 && h[0] == 0xFF && h[1] == 0xD8 && h[2] == 0xFF:
		return "jpeg"
	case bytes.HasPrefix(h, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(h, []byte("GIF87a")), bytes.HasPrefix(h, []byte("GIF89a")):
		return "gif"
	case bytes.HasPrefix(h, []byte("BM")) && len(h) >= 14:
		return "bmp"
	case len(h) >= 12 && string(h[0:4]) == "RIFF" && string(h[8:12]) == "WEBP":
		return "webp"
	case bytes.HasPrefix(h, []byte("II*\x00")), bytes.HasPrefix(h, []byte("MM\x00*")):
		return "tiff"
	}
	return ""
}

// SniffFile reports the decoder name for the file at path ("jpeg", "png",
// ...) or "" when it is not a format this package decodes natively. Only the
// first few bytes are read.
func SniffFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := make([]byte, sniffLen)
	n, err := io.ReadFull(f, h)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	return sniffFormat(h[:n]), nil
}

// Generate makes a thumbnail of the image file at path without any caching
// or system fallback. Most callers want Thumbnailer.Thumbnail instead.
func Generate(ctx context.Context, path string, opt Options) (*Thumb, error) {
	opt = opt.normalized()
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, ErrIsDir
	}
	format, err := SniffFile(path)
	if err != nil {
		return nil, err
	}
	if format == "" {
		return nil, ErrUnsupported
	}
	if fi.Size() > opt.MaxSourceBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, fi.Size())
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return generateBytes(ctx, data, opt)
}

// GenerateBytes makes a thumbnail from an encoded image held in memory, for
// example a file inside an archive that the VFS layer has extracted.
func GenerateBytes(ctx context.Context, data []byte, opt Options) (*Thumb, error) {
	opt = opt.normalized()
	if int64(len(data)) > opt.MaxSourceBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(data))
	}
	return generateBytes(ctx, data, opt)
}

func generateBytes(ctx context.Context, data []byte, opt Options) (*Thumb, error) {
	format := sniffFormat(data[:min(len(data), sniffLen)])
	if format == "" {
		return nil, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: empty image", ErrDecode)
	}
	if int64(cfg.Width)*int64(cfg.Height) > opt.MaxPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels", ErrTooLarge, cfg.Width, cfg.Height)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	orient := exifOrientation(data, format)
	return makeThumb(img, Meta{Format: format, Orientation: orient}, opt)
}

// makeThumb scales, orients and encodes a decoded image. meta.Format and
// meta.Orientation must be set; the displayed size is filled in here.
func makeThumb(img image.Image, meta Meta, opt Options) (*Thumb, error) {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	swapped := meta.Orientation >= 5 && meta.Orientation <= 8
	ow, oh := sw, sh // displayed (upright) size
	if swapped {
		ow, oh = sh, sw
	}
	if meta.Format != "system" { // system previews do not reveal the original size
		meta.Width, meta.Height = ow, oh
	}
	tw, th := FitBox(ow, oh, opt.Width, opt.Height, opt.Upscale)
	if tw == 0 || th == 0 {
		return nil, fmt.Errorf("%w: empty image", ErrDecode)
	}
	rw, rh := tw, th // size before orientation
	if swapped {
		rw, rh = th, tw
	}
	scaled := scale(img, rw, rh)
	upright := applyOrientation(scaled, meta.Orientation)
	return encodeThumb(upright, meta, opt)
}

// FitBox returns the largest size with the aspect ratio of sw x sh that fits
// into bw x bh. Unless upscale is set, images already inside the box keep
// their size. Each edge is at least 1 pixel.
func FitBox(sw, sh, bw, bh int, upscale bool) (int, int) {
	if sw <= 0 || sh <= 0 || bw <= 0 || bh <= 0 {
		return 0, 0
	}
	if !upscale && sw <= bw && sh <= bh {
		return sw, sh
	}
	s := math.Min(float64(bw)/float64(sw), float64(bh)/float64(sh))
	w := int(math.Round(float64(sw) * s))
	h := int(math.Round(float64(sh) * s))
	return max(1, min(w, bw)), max(1, min(h, bh))
}

// scale resizes img to w x h into a fresh RGBA image. Large reductions use
// the bilinear kernel (its support grows with the scale factor, so it still
// averages every source pixel but costs half of Catmull-Rom); moderate ones
// and enlargements use Catmull-Rom for sharper results.
func scale(img image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst
	}
	var k xdraw.Interpolator = xdraw.CatmullRom
	if b.Dx() > 4*w || b.Dy() > 4*h {
		k = xdraw.BiLinear
	}
	k.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

// applyOrientation returns src turned upright for EXIF orientation o:
// 2 mirror, 3 rotate 180, 4 flip, 5 transpose, 6 rotate 90 clockwise,
// 7 transverse, 8 rotate 90 counter-clockwise. 1 and unknown values return
// src unchanged.
func applyOrientation(src *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			si := src.PixOffset(sx, sy)
			di := dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// encodeThumb picks JPEG for opaque thumbnails (several times smaller) and
// PNG when transparency must survive.
func encodeThumb(img *image.RGBA, meta Meta, opt Options) (*Thumb, error) {
	var buf bytes.Buffer
	t := &Thumb{Meta: meta, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	if img.Opaque() {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: opt.JPEGQuality}); err != nil {
			return nil, err
		}
		t.MIME = "image/jpeg"
	} else {
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, err
		}
		t.MIME = "image/png"
	}
	t.Data = buf.Bytes()
	return t, nil
}
