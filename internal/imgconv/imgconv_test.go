package imgconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSniff(t *testing.T) {
	for file, want := range map[string]string{"s.heic": HEIC, "s.avif": AVIF, "s.jxl": JXL, "s_c.jxl": JXL, "wide.jxl": JXL} {
		if got := SniffFile(filepath.Join("testdata", file)); got != want {
			t.Errorf("%s: %q, want %q", file, got, want)
		}
	}
	for _, b := range [][]byte{nil, []byte("\x89PNG\r\n\x1a\n...."), []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isommp42")} {
		if got := Sniff(b); got != "" {
			t.Errorf("%q: %q", b, got)
		}
	}
	if ExtFormat("a.HEIF") != HEIC || ExtFormat("b.avif") != AVIF || ExtFormat("c.jxl") != JXL || ExtFormat("d.png") != "" {
		t.Error("ExtFormat")
	}
}

func TestJXLSize(t *testing.T) {
	for file, want := range map[string][2]int{"s.jxl": {40, 24}, "s_c.jxl": {40, 24}, "wide.jxl": {1000, 563}} {
		f, err := os.Open(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		fi, _ := f.Stat()
		w, h, ok := JXLSize(f, fi.Size())
		f.Close()
		if !ok || w != want[0] || h != want[1] {
			t.Errorf("%s: %dx%d %v, want %v", file, w, h, ok, want)
		}
	}
	if _, _, ok := JXLSize(bytes.NewReader([]byte("\xff\x0a")), 2); ok {
		t.Error("truncated header accepted")
	}
	if _, _, ok := JXLSize(bytes.NewReader([]byte("hello world")), 11); ok {
		t.Error("text accepted")
	}
}

// withTools makes lookPath find only the named programs (as the given executables).
func withTools(t *testing.T, m map[string]string) {
	old := lookPath
	lookPath = func(name string) string { return m[name] }
	t.Cleanup(func() { lookPath = old })
}

func TestToolChoice(t *testing.T) {
	withTools(t, map[string]string{"djxl": "/x/djxl", "magick": "/x/magick"})
	if Available(JXL, Options{}) != "/x/djxl" {
		t.Error("djxl should win for JPEG XL")
	}
	if Available(AVIF, Options{}) != "/x/magick" {
		t.Error("magick should read AVIF")
	}
	if Available(AVIF, Options{Off: true}) != "" {
		t.Error("external tools off must start nothing")
	}
	withTools(t, map[string]string{})
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" && Available(HEIC, Options{}) != "" {
		t.Error("no tool expected")
	}
	if _, _, err := Decode(context.Background(), filepath.Join("testdata", "s.heic"), Options{Program: "no-such-program-xyz"}); !errors.Is(err, ErrNoTool) {
		t.Errorf("missing configured program: %v", err)
	}
	if _, _, err := Decode(context.Background(), "imgconv.go", Options{}); !errors.Is(err, ErrNotSupported) {
		t.Errorf("Go source: %v", err)
	}
}

// TestDecodeReal converts the samples with whatever this machine has (sips on macOS, or libheif/libavif/libjxl
// tools); formats without a program are skipped.
func TestDecodeReal(t *testing.T) {
	for _, file := range []string{"s.heic", "s.avif", "s.jxl", "s_c.jxl"} {
		p := filepath.Join("testdata", file)
		format := SniffFile(p)
		if findProgram("sips") == "" && Available(format, Options{}) == "" {
			t.Logf("%s: no converter installed", file)
			continue
		}
		img, tool, err := Decode(context.Background(), p, Options{})
		if err != nil {
			t.Logf("%s: %v (tool %s)", file, err, tool)
			continue // (an older sips may not read JPEG XL; another tool may lack a codec)
		}
		if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 24 {
			t.Errorf("%s via %s: %v", file, tool, b)
		}
	}
}

func TestPixelLimit(t *testing.T) {
	p := filepath.Join("testdata", "s.avif")
	if Available(AVIF, Options{}) == "" {
		t.Skip("no AVIF converter")
	}
	if _, _, err := Decode(context.Background(), p, Options{MaxPixels: 100}); err != nil && !errors.Is(err, ErrTooLarge) {
		t.Logf("conversion failed: %v", err)
	} else if err == nil {
		t.Error("pixel limit not applied")
	}
}

func TestConfiguredProgram(t *testing.T) {
	for prog, file := range map[string]string{"avifdec": "s.avif", "djxl": "s_c.jxl", "ffmpeg": "s.avif"} {
		exe := findProgram(prog)
		if exe == "" {
			t.Logf("%s not installed", prog)
			continue
		}
		withTools(t, map[string]string{})
		img, tool, err := Decode(context.Background(), filepath.Join("testdata", file), Options{Program: exe})
		if err != nil {
			if prog == "ffmpeg" {
				t.Logf("ffmpeg: %v (a build without an AV1 decoder)", err)
				continue
			}
			t.Fatalf("%s: %v", prog, err)
		}
		if tool != prog || img.Bounds().Dx() != 40 {
			t.Fatalf("%s: tool %s, %v", prog, tool, img.Bounds())
		}
		mime, data, _, err := DecodeForView(context.Background(), filepath.Join("testdata", file), Options{Program: exe})
		if err != nil || len(data) == 0 || (mime != "image/jpeg" && mime != "image/png") {
			t.Fatal(mime, err)
		}
	}
}
