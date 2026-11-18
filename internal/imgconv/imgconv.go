// Package imgconv turns HEIC/HEIF, AVIF and JPEG XL pictures into PNG with what the computer already has, for the
// thumbnails view, the Lister and Quick View on systems whose web view or Go decoders can't read them (issue #25).
// There are no pure-Go decoders for HEVC, AV1 or JPEG XL that are small enough to bundle, and cgo libraries would
// break the cross-compiled build, so this package starts a program instead:
//
//   - the program named in Options > Plugins > "Program for HEIC, AVIF and JPEG XL", when it is set;
//   - else the first installed one of: sips (macOS, part of the system), heif-dec / heif-convert (libheif), avifdec
//     (libavif), djxl (libjxl), magick (ImageMagick 7), vips (libvips), ffmpeg, and on Windows the system's own WIC
//     codecs through PowerShell (the HEIF, AV1 and JPEG XL extensions from the Microsoft Store).
//
// Each run gets its own temporary folder and a time limit; the PNG it writes is decoded and checked against a pixel
// limit before it is used. Nothing is downloaded and nothing goes online: these are local programs on local files.
package imgconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Formats this package converts.
const (
	HEIC = "heic" // HEIF with HEVC pictures (iPhone photos); also other HEIF brands
	AVIF = "avif"
	JXL  = "jxl"
)

// Errors.
var (
	// ErrNotSupported: the data is not HEIC, AVIF or JPEG XL.
	ErrNotSupported = errors.New("imgconv: not a HEIC, AVIF or JPEG XL picture")
	// ErrNoTool: no installed program can convert this format.
	ErrNoTool = errors.New("imgconv: no program to convert this picture is installed")
	// ErrTooLarge: the converted picture is larger than the pixel limit.
	ErrTooLarge = errors.New("imgconv: picture too large")
)

// Timeout bounds one conversion.
var Timeout = 30 * time.Second

// DefaultMaxPixels is the pixel limit when Options.MaxPixels is zero (about 100 megapixels, as for thumbnails).
const DefaultMaxPixels = 100 << 20

// Sniff returns HEIC, AVIF or JXL for the first bytes of a file (at least 32 are needed for HEIF brands), or "".
func Sniff(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0x0A {
		return JXL // bare JPEG XL codestream
	}
	if bytes.HasPrefix(b, []byte("\x00\x00\x00\x0cJXL \r\n\x87\n")) {
		return JXL // JPEG XL container
	}
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return ""
	}
	size := int(binary.BigEndian.Uint32(b))
	if size < 16 || size > len(b) {
		size = min(len(b), 64)
	}
	heif := ""
	for i := 8; i+4 <= size; i += 4 {
		if i == 12 {
			continue // (the minor version, not a brand)
		}
		switch string(b[i : i+4]) {
		case "avif", "avis":
			return AVIF
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1":
			heif = HEIC
		}
	}
	return heif
}

// SniffFile is Sniff on the first bytes of a file.
func SniffFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 64)
	n, _ := f.Read(b)
	return Sniff(b[:n])
}

// ExtFormat is the format a file name suggests (".heic", ".heif", ".hif", ".avif", ".jxl"), or "".
func ExtFormat(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".heic", ".heif", ".hif":
		return HEIC
	case ".avif":
		return AVIF
	case ".jxl":
		return JXL
	}
	return ""
}

// Options says which programs may be used.
type Options struct {
	// Program is the configured converter (Options > Plugins); "" searches for an installed one.
	Program string
	// Off turns every external program off (Options > Plugins > Use configured external tools).
	Off bool
	// MaxPixels limits the decoded picture; zero means DefaultMaxPixels.
	MaxPixels int64
}

// tool is one way of converting: its program name, the formats it reads, and its arguments for in -> out (PNG).
type tool struct {
	name    string
	formats string // the formats it reads, space separated
	args    func(in, out string) []string
	osOnly  string // "" or the GOOS it is for
}

var tools = []tool{
	{"sips", "heic avif jxl", func(in, out string) []string { return []string{"-s", "format", "png", in, "--out", out} }, "darwin"},
	{"heif-dec", "heic avif", func(in, out string) []string { return []string{in, out} }, ""},
	{"heif-convert", "heic avif", func(in, out string) []string { return []string{in, out} }, ""},
	{"avifdec", "avif", func(in, out string) []string { return []string{in, out} }, ""},
	{"djxl", "jxl", func(in, out string) []string { return []string{in, out} }, ""},
	{"magick", "heic avif jxl", func(in, out string) []string { return []string{in + "[0]", "png:" + out} }, ""},
	{"vips", "heic avif jxl", func(in, out string) []string { return []string{"copy", in, out} }, ""},
	{"ffmpeg", "heic avif jxl", func(in, out string) []string {
		return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-i", in, "-frames:v", "1", "-y", out}
	}, ""},
	{"powershell", "heic avif jxl", func(in, out string) []string {
		return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", wicScript}
	}, "windows"},
}

// wicScript decodes the first frame with Windows Imaging Component codecs and writes it as PNG. The file names come
// from the environment (MC_IMG_IN, MC_IMG_OUT), so no quoting of user paths is involved.
const wicScript = `$ErrorActionPreference='Stop'; Add-Type -AssemblyName PresentationCore; ` +
	`$s=[IO.File]::OpenRead($env:MC_IMG_IN); try { ` +
	`$d=[Windows.Media.Imaging.BitmapDecoder]::Create($s,[Windows.Media.Imaging.BitmapCreateOptions]::None,[Windows.Media.Imaging.BitmapCacheOption]::OnLoad); ` +
	`$e=New-Object Windows.Media.Imaging.PngBitmapEncoder; $e.Frames.Add([Windows.Media.Imaging.BitmapFrame]::Create($d.Frames[0])); ` +
	`$o=[IO.File]::Create($env:MC_IMG_OUT); try { $e.Save($o) } finally { $o.Close() } } finally { $s.Close() }`

// lookPath finds a program; tests replace it.
var lookPath = findProgram

var (
	foundMu sync.Mutex
	found   = map[string]string{}
)

// findProgram looks on the PATH and where Homebrew, MacPorts and the usual installers put programs (an app started
// from the Dock has a short PATH on macOS). "" when it is not installed. Results are remembered.
func findProgram(name string) string {
	foundMu.Lock()
	defer foundMu.Unlock()
	if p, ok := found[name]; ok {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		p = ""
		if runtime.GOOS != "windows" {
			for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/usr/bin"} {
				c := filepath.Join(dir, name)
				if fi, err := os.Stat(c); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
					p = c
					break
				}
			}
		}
	}
	found[name] = p
	return p
}

// toolFor picks the program for a format: the configured one (its arguments chosen by its name, a program of
// unknown name is called as "program in out.png"), else the first installed one that reads the format.
func toolFor(format string, o Options) (exe string, t tool, ok bool) {
	if o.Off {
		return "", tool{}, false
	}
	if p := strings.TrimSpace(o.Program); p != "" {
		exe = p
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			if exe = lookPath(p); exe == "" {
				return "", tool{}, false
			}
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe)))
		for _, t := range tools {
			if t.name == base {
				return exe, t, true
			}
		}
		return exe, tool{name: base, formats: "heic avif jxl", args: func(in, out string) []string { return []string{in, out} }}, true
	}
	for _, t := range tools {
		if t.osOnly != "" && t.osOnly != runtime.GOOS {
			continue
		}
		if !strings.Contains(t.formats, format) {
			continue
		}
		if exe := lookPath(t.name); exe != "" {
			return exe, t, true
		}
	}
	return "", tool{}, false
}

// Available reports which program would convert a format ("" when none would).
func Available(format string, o Options) string {
	if exe, _, ok := toolFor(format, o); ok {
		return exe
	}
	return ""
}

// Decode converts the HEIC, AVIF or JPEG XL file at path and returns the picture and the program that made it. The
// format comes from the file's first bytes, or from its name when they say nothing.
func Decode(ctx context.Context, path string, o Options) (image.Image, string, error) {
	format := SniffFile(path)
	if format == "" {
		format = ExtFormat(path)
	}
	if format == "" {
		return nil, "", ErrNotSupported
	}
	exe, t, ok := toolFor(format, o)
	if !ok {
		return nil, "", ErrNoTool
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "mc-imgconv-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(dir)
	in := abs // (absolute, so it never starts with "-" and looks like an option)
	out := filepath.Join(dir, "out.png")
	cmd := exec.CommandContext(ctx, exe, t.args(in, out)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MC_IMG_IN="+in, "MC_IMG_OUT="+out)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, t.name, fmt.Errorf("imgconv: %s: %w", t.name, ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, t.name, fmt.Errorf("imgconv: %s: %v %s", t.name, err, msg)
	}
	img, err := readPNG(firstPNG(dir, out), o.MaxPixels)
	if err != nil {
		return nil, t.name, err
	}
	return img, t.name, nil
}

// firstPNG is the PNG a program wrote: out itself, or (heif-convert with several pictures in the file writes
// out-1.png, out-2.png...) the first PNG in dir.
func firstPNG(dir, out string) string {
	if _, err := os.Stat(out); err == nil {
		return out
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(m) > 0 {
		return m[0]
	}
	return out
}

func readPNG(path string, maxPixels int64) (image.Image, error) {
	if maxPixels <= 0 {
		maxPixels = DefaultMaxPixels
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("imgconv: the program wrote no picture")
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("imgconv: %v", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels", ErrTooLarge, cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("imgconv: %v", err)
	}
	return img, nil
}

// DecodeForView is Decode with the result encoded for a web view: JPEG (quality 92) when the picture is opaque, as a
// photo usually is and much smaller than PNG, else PNG.
func DecodeForView(ctx context.Context, path string, o Options) (mime string, data []byte, tool string, err error) {
	img, tool, err := Decode(ctx, path, o)
	if err != nil {
		return "", nil, tool, err
	}
	var b bytes.Buffer
	if opaque(img) {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 92})
		mime = "image/jpeg"
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img)
		mime = "image/png"
	}
	if err != nil {
		return "", nil, tool, err
	}
	return mime, b.Bytes(), tool, nil
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}
