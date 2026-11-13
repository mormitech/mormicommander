// Tests for decoding, scaling, EXIF orientation and encoding of single
// thumbnails, covering every supported format and the safety limits.

package thumbs

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClampSize(t *testing.T) {
	for _, c := range []struct{ in, want int }{
		{0, DefaultSize}, {-5, DefaultSize}, {1, MinSize}, {31, MinSize},
		{32, 32}, {200, 200}, {512, 512}, {513, MaxSize}, {99999, MaxSize},
	} {
		if got := ClampSize(c.in); got != c.want {
			t.Errorf("ClampSize(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFitBox(t *testing.T) {
	for _, c := range []struct {
		sw, sh, bw, bh int
		up             bool
		w, h           int
	}{
		{1000, 500, 100, 100, false, 100, 50},
		{500, 1000, 100, 100, false, 50, 100},
		{50, 20, 100, 100, false, 50, 20},
		{50, 20, 100, 100, true, 100, 40},
		{100, 100, 100, 100, false, 100, 100},
		{10000, 1, 100, 100, false, 100, 1},
		{1, 10000, 100, 100, false, 1, 100},
		{300, 200, 120, 60, false, 90, 60},
		{0, 10, 100, 100, false, 0, 0},
		{10, 10, 0, 100, false, 0, 0},
	} {
		w, h := FitBox(c.sw, c.sh, c.bw, c.bh, c.up)
		if w != c.w || h != c.h {
			t.Errorf("FitBox(%d,%d,%d,%d,%v) = %dx%d, want %dx%d", c.sw, c.sh, c.bw, c.bh, c.up, w, h, c.w, c.h)
		}
	}
}

func TestSniffFormat(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
	}{
		{"\xFF\xD8\xFF\xE0rest", "jpeg"},
		{"\x89PNG\r\n\x1a\nxxxx", "png"},
		{"GIF89a....", "gif"},
		{"GIF87a....", "gif"},
		{"BM" + strings.Repeat("\x00", 14), "bmp"},
		{"BM", ""},
		{"RIFF\x00\x00\x00\x00WEBPVP8 ", "webp"},
		{"RIFF\x00\x00\x00\x00WAVEfmt ", ""},
		{"II*\x00\x08\x00\x00\x00", "tiff"},
		{"MM\x00*\x00\x00\x00\x08", "tiff"},
		{"hello world, plain text", ""},
		{"", ""},
	} {
		if got := sniffFormat([]byte(c.in)); got != c.want {
			t.Errorf("sniffFormat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGenerateFormats(t *testing.T) {
	dir := t.TempDir()
	src := halves(200, 100)
	opt := Options{Width: 64, Height: 64}
	cases := []struct {
		name, format, mime string
		data               []byte
		w, h               int
	}{
		{"a.jpg", "jpeg", "image/jpeg", encodeAs(t, "jpeg", src), 64, 32},
		{"a.png", "png", "image/jpeg", encodeAs(t, "png", src), 64, 32},
		{"a.gif", "gif", "image/jpeg", encodeAs(t, "gif", src), 64, 32},
		{"a.bmp", "bmp", "image/jpeg", encodeAs(t, "bmp", src), 64, 32},
		{"a.tif", "tiff", "image/jpeg", encodeAs(t, "tiff", src), 64, 32},
	}
	lossy, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := os.ReadFile("testdata/gopher-doc.with-alpha.lossless.webp")
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases,
		struct {
			name, format, mime string
			data               []byte
			w, h               int
		}{"lossy.webp", "webp", "image/jpeg", lossy, 0, 0},
		struct {
			name, format, mime string
			data               []byte
			w, h               int
		}{"alpha.webp", "webp", "image/png", alpha, 0, 0},
	)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeFile(t, dir, c.name, c.data)
			th, err := Generate(context.Background(), p, opt)
			if err != nil {
				t.Fatal(err)
			}
			if th.Meta.Format != c.format {
				t.Errorf("format %q, want %q", th.Meta.Format, c.format)
			}
			if th.MIME != c.mime {
				t.Errorf("mime %q, want %q", th.MIME, c.mime)
			}
			if c.w != 0 && (th.Width != c.w || th.Height != c.h) {
				t.Errorf("thumb %dx%d, want %dx%d", th.Width, th.Height, c.w, c.h)
			}
			if th.Width > 64 || th.Height > 64 || th.Width < 1 || th.Height < 1 {
				t.Errorf("thumb %dx%d does not fit 64x64", th.Width, th.Height)
			}
			if th.Width != 64 && th.Height != 64 && (th.Meta.Width > 64 || th.Meta.Height > 64) {
				t.Errorf("large source not scaled to the box: %dx%d", th.Width, th.Height)
			}
			img, err := th.Image()
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != th.Width || img.Bounds().Dy() != th.Height {
				t.Errorf("decoded %v, header %dx%d", img.Bounds(), th.Width, th.Height)
			}
			if c.w != 0 {
				if c := colorAt(t, th, 2, th.Height/2); !isRedish(c) {
					t.Errorf("left pixel %v, want red", c)
				}
				if c := colorAt(t, th, th.Width-3, th.Height/2); !isBlueish(c) {
					t.Errorf("right pixel %v, want blue", c)
				}
				if th.Meta.Width != 200 || th.Meta.Height != 100 {
					t.Errorf("meta size %dx%d", th.Meta.Width, th.Meta.Height)
				}
			}
		})
	}
}

func TestGenerateUpscale(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "small.png", encodeAs(t, "png", halves(40, 20)))
	th, err := Generate(context.Background(), p, Options{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}
	if th.Width != 40 || th.Height != 20 {
		t.Errorf("no upscale: got %dx%d", th.Width, th.Height)
	}
	th, err = Generate(context.Background(), p, Options{Width: 100, Height: 100, Upscale: true})
	if err != nil {
		t.Fatal(err)
	}
	if th.Width != 100 || th.Height != 50 {
		t.Errorf("upscale: got %dx%d", th.Width, th.Height)
	}
}

func TestTransparentPNGStaysPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	img.Set(50, 50, color.NRGBA{255, 0, 0, 255})
	p := writeFile(t, t.TempDir(), "t.png", encodeAs(t, "png", img))
	th, err := Generate(context.Background(), p, Options{Width: 50, Height: 50})
	if err != nil {
		t.Fatal(err)
	}
	if th.MIME != "image/png" {
		t.Fatalf("mime %s", th.MIME)
	}
	if c := colorAt(t, th, 0, 0); c.A != 0 {
		t.Errorf("corner alpha %d, want 0", c.A)
	}
	if !strings.HasPrefix(th.DataURL(), "data:image/png;base64,") {
		t.Errorf("data url %q", th.DataURL()[:30])
	}
}

func TestApplyOrientation(t *testing.T) {
	// 3x2 source with distinct pixels; we check where the top-left (A) and
	// top-right (B) source pixels end up.
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < 6; i++ {
		src.SetRGBA(i%3, i/3, color.RGBA{uint8(i * 40), uint8(i), 0, 255})
	}
	A, B := src.RGBAAt(0, 0), src.RGBAAt(2, 0)
	for _, c := range []struct {
		o      int
		w, h   int
		ax, ay int
		bx, by int
	}{
		{1, 3, 2, 0, 0, 2, 0},
		{2, 3, 2, 2, 0, 0, 0},
		{3, 3, 2, 2, 1, 0, 1},
		{4, 3, 2, 0, 1, 2, 1},
		{5, 2, 3, 0, 0, 0, 2},
		{6, 2, 3, 1, 0, 1, 2},
		{7, 2, 3, 1, 2, 1, 0},
		{8, 2, 3, 0, 2, 0, 0},
		{9, 3, 2, 0, 0, 2, 0},
	} {
		d := applyOrientation(src, c.o)
		if d.Bounds().Dx() != c.w || d.Bounds().Dy() != c.h {
			t.Errorf("o=%d size %v", c.o, d.Bounds())
			continue
		}
		if got := d.RGBAAt(c.ax, c.ay); got != A {
			t.Errorf("o=%d A at (%d,%d) = %v, want %v", c.o, c.ax, c.ay, got, A)
		}
		if got := d.RGBAAt(c.bx, c.by); got != B {
			t.Errorf("o=%d B at (%d,%d) = %v, want %v", c.o, c.bx, c.by, got, B)
		}
	}
}

func TestEXIFOrientationEndToEnd(t *testing.T) {
	dir := t.TempDir()
	jpg := encodeAs(t, "jpeg", halves(160, 80))
	// After orientation the red (left) half must be at the given side.
	for _, c := range []struct {
		o       int
		big     bool
		w, h    int
		redSide string
	}{
		{1, false, 80, 40, "left"},
		{3, true, 80, 40, "right"},
		{6, false, 40, 80, "top"},
		{8, true, 40, 80, "bottom"},
		{2, false, 80, 40, "right"},
	} {
		p := writeFile(t, dir, "o.jpg", withJPEGExif(jpg, c.o, c.big))
		th, err := Generate(context.Background(), p, Options{Width: 80, Height: 80})
		if err != nil {
			t.Fatal(err)
		}
		if th.Meta.Orientation != c.o {
			t.Errorf("o=%d: meta orientation %d", c.o, th.Meta.Orientation)
		}
		if th.Width != c.w || th.Height != c.h {
			t.Errorf("o=%d: size %dx%d, want %dx%d", c.o, th.Width, th.Height, c.w, c.h)
			continue
		}
		if c.o == 6 || c.o == 8 {
			if th.Meta.Width != 80 || th.Meta.Height != 160 {
				t.Errorf("o=%d: meta %dx%d, want 80x160", c.o, th.Meta.Width, th.Meta.Height)
			}
		}
		var px, py int
		switch c.redSide {
		case "left":
			px, py = 3, th.Height/2
		case "right":
			px, py = th.Width-4, th.Height/2
		case "top":
			px, py = th.Width/2, 3
		case "bottom":
			px, py = th.Width/2, th.Height-4
		}
		if col := colorAt(t, th, px, py); !isRedish(col) {
			t.Errorf("o=%d: %s pixel %v, want red", c.o, c.redSide, col)
		}
	}
}

func TestExifParsers(t *testing.T) {
	jpg := encodeAs(t, "jpeg", halves(8, 8))
	pngData := encodeAs(t, "png", halves(8, 8))
	webp, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	webpWith := func(chunk []byte) []byte {
		out := append([]byte{}, webp...)
		out = append(out, "EXIF"...)
		out = append(out, byte(len(chunk)), byte(len(chunk)>>8), byte(len(chunk)>>16), byte(len(chunk)>>24))
		out = append(out, chunk...)
		if len(chunk)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	truncated := withJPEGExif(jpg, 6, false)[:12]
	for _, c := range []struct {
		name   string
		format string
		data   []byte
		want   int
	}{
		{"jpeg none", "jpeg", jpg, 1},
		{"jpeg LE 6", "jpeg", withJPEGExif(jpg, 6, false), 6},
		{"jpeg BE 8", "jpeg", withJPEGExif(jpg, 8, true), 8},
		{"jpeg invalid value", "jpeg", withJPEGExif(jpg, 42, false), 1},
		{"jpeg truncated", "jpeg", truncated, 1},
		{"png none", "png", pngData, 1},
		{"png eXIf", "png", withPNGChunk(pngData, "eXIf", tiffBlock(3, true)), 3},
		{"png eXIf with header", "png", withPNGChunk(pngData, "eXIf", append([]byte("Exif\x00\x00"), tiffBlock(5, false)...)), 5},
		{"webp none", "webp", webp, 1},
		{"webp EXIF", "webp", webpWith(tiffBlock(7, false)), 7},
		{"webp EXIF header", "webp", webpWith(append([]byte("Exif\x00\x00"), tiffBlock(2, true)...)), 2},
		{"tiff", "tiff", tiffBlock(4, false), 4},
		{"tiff garbage", "tiff", []byte("XX*\x00garbage"), 1},
		{"tiff bad offset", "tiff", []byte("II*\x00\xff\xff\x00\x00"), 1},
		{"gif never", "gif", encodeAs(t, "gif", halves(8, 8)), 1},
	} {
		if got := exifOrientation(c.data, c.format); got != c.want {
			t.Errorf("%s: orientation %d, want %d", c.name, got, c.want)
		}
	}
}

func TestGenerateErrors(t *testing.T) {
	dir := t.TempDir()
	good := encodeAs(t, "png", halves(100, 100))
	txt := writeFile(t, dir, "a.txt", []byte("just some text in a file"))
	empty := writeFile(t, dir, "empty.jpg", nil)
	broken := writeFile(t, dir, "broken.png", good[:40])
	big := writeFile(t, dir, "big.png", good)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		path string
		opt  Options
		want error
	}{
		{"text", txt, Options{}, ErrUnsupported},
		{"empty", empty, Options{}, ErrUnsupported},
		{"broken", broken, Options{}, ErrDecode},
		{"bytes limit", big, Options{MaxSourceBytes: 10}, ErrTooLarge},
		{"pixel limit", big, Options{MaxPixels: 100}, ErrTooLarge},
		{"dir", sub, Options{}, ErrIsDir},
		{"missing", filepath.Join(dir, "nope.png"), Options{}, os.ErrNotExist},
	} {
		_, err := Generate(context.Background(), c.path, c.opt)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err %v, want %v", c.name, err, c.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Generate(ctx, big, Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: err %v", err)
	}
}

func TestGenerateBytes(t *testing.T) {
	th, err := GenerateBytes(context.Background(), encodeAs(t, "gif", halves(300, 150)), Options{Width: 100, Height: 100})
	if err != nil {
		t.Fatal(err)
	}
	if th.Width != 100 || th.Height != 50 || th.Meta.Format != "gif" {
		t.Errorf("got %dx%d %s", th.Width, th.Height, th.Meta.Format)
	}
	if _, err := GenerateBytes(context.Background(), []byte("nope"), Options{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("err %v", err)
	}
	if _, err := GenerateBytes(context.Background(), make([]byte, 100), Options{MaxSourceBytes: 10}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err %v", err)
	}
}

func TestSniffFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "x.bin", encodeAs(t, "bmp", halves(4, 4)))
	if f, err := SniffFile(p); err != nil || f != "bmp" {
		t.Errorf("SniffFile = %q, %v", f, err)
	}
	p = writeFile(t, dir, "short", []byte("BM"))
	if f, err := SniffFile(p); err != nil || f != "" {
		t.Errorf("short SniffFile = %q, %v", f, err)
	}
	if _, err := SniffFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestLargeDownscaleUsesWholeImage(t *testing.T) {
	// A 2000x1000 image scaled to 50x25 must still show both halves.
	th, err := GenerateBytes(context.Background(), encodeAs(t, "png", halves(2000, 1000)), Options{Width: 50, Height: 50})
	if err != nil {
		t.Fatal(err)
	}
	if th.Width != 50 || th.Height != 25 {
		t.Fatalf("size %dx%d", th.Width, th.Height)
	}
	if c := colorAt(t, th, 5, 12); !isRedish(c) {
		t.Errorf("left %v", c)
	}
	if c := colorAt(t, th, 45, 12); !isBlueish(c) {
		t.Errorf("right %v", c)
	}
}
