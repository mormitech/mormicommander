// Package tarfmt is mormiCommander's engine for the Unix family of archives:
// plain tar, tar wrapped in gzip, bzip2, xz, zstd or lzma, and the matching
// single-file compressed formats (.gz, .bz2, .xz, .zst, .lzma) that hold just
// one file. Total Commander treats all of these as "archives you can enter
// like a folder", so this package lists them, extracts them safely, tests
// them, creates them and rewrites them to add, delete, rename or make
// folders. This file knows the formats themselves: which file names and
// which magic bytes belong to which format, and how to wrap a byte stream in
// the right decompressor or compressor.
package tarfmt

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"

	dbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
)

// Format identifies one archive flavour handled by this package.
type Format int

const (
	FormatUnknown Format = iota
	FormatTar
	FormatTarGz
	FormatTarBz2
	FormatTarXz
	FormatTarZst
	FormatTarLzma
	FormatGz
	FormatBz2
	FormatXz
	FormatZst
	FormatLzma
)

// Compression is the outer compression layer of a format.
type Compression int

const (
	CompNone Compression = iota
	CompGzip
	CompBzip2
	CompXz
	CompZstd
	CompLzma
)

var formatNames = map[Format]string{
	FormatUnknown: "unknown",
	FormatTar:     "tar",
	FormatTarGz:   "tar.gz",
	FormatTarBz2:  "tar.bz2",
	FormatTarXz:   "tar.xz",
	FormatTarZst:  "tar.zst",
	FormatTarLzma: "tar.lzma",
	FormatGz:      "gz",
	FormatBz2:     "bz2",
	FormatXz:      "xz",
	FormatZst:     "zst",
	FormatLzma:    "lzma",
}

// String returns the short name used in the pack dialog's packer list.
func (f Format) String() string {
	if s, ok := formatNames[f]; ok {
		return s
	}
	return fmt.Sprintf("Format(%d)", int(f))
}

// Ext returns the usual file extension (with the dot) for new archives.
func (f Format) Ext() string {
	if f == FormatUnknown {
		return ""
	}
	return "." + f.String()
}

// IsTar reports whether the format holds a tar stream (many files) rather
// than one single compressed file.
func (f Format) IsTar() bool { return f >= FormatTar && f <= FormatTarLzma }

// Compression returns the outer compression layer.
func (f Format) Compression() Compression {
	switch f {
	case FormatTarGz, FormatGz:
		return CompGzip
	case FormatTarBz2, FormatBz2:
		return CompBzip2
	case FormatTarXz, FormatXz:
		return CompXz
	case FormatTarZst, FormatZst:
		return CompZstd
	case FormatTarLzma, FormatLzma:
		return CompLzma
	}
	return CompNone
}

// withTar turns a single-file format into the tar flavour with the same
// compression, and back with withoutTar. Detection uses these when the
// content turns out to differ from what the name promised.
func (f Format) withTar() Format {
	switch f.Compression() {
	case CompGzip:
		return FormatTarGz
	case CompBzip2:
		return FormatTarBz2
	case CompXz:
		return FormatTarXz
	case CompZstd:
		return FormatTarZst
	case CompLzma:
		return FormatTarLzma
	}
	return FormatTar
}

func (f Format) withoutTar() Format {
	switch f.Compression() {
	case CompGzip:
		return FormatGz
	case CompBzip2:
		return FormatBz2
	case CompXz:
		return FormatXz
	case CompZstd:
		return FormatZst
	case CompLzma:
		return FormatLzma
	}
	return FormatUnknown
}

// FormatInfo describes a format for the pack dialog.
type FormatInfo struct {
	Format   Format
	Name     string
	Ext      string
	CanWrite bool
	Single   bool // holds exactly one file
}

// Formats lists every format in pack-dialog order. All of them can be read
// and written; the list exists so the UI does not hard-code it.
func Formats() []FormatInfo {
	var out []FormatInfo
	for f := FormatTar; f <= FormatLzma; f++ {
		out = append(out, FormatInfo{Format: f, Name: f.String(), Ext: f.Ext(), CanWrite: true, Single: !f.IsTar()})
	}
	return out
}

// extTable maps lower-case name suffixes to formats. Longer suffixes come
// first so ".tar.gz" wins over ".gz".
var extTable = []struct {
	ext string
	f   Format
}{
	{".tar.gz", FormatTarGz}, {".tar.bz2", FormatTarBz2}, {".tar.xz", FormatTarXz},
	{".tar.zst", FormatTarZst}, {".tar.zstd", FormatTarZst}, {".tar.lzma", FormatTarLzma},
	{".tgz", FormatTarGz}, {".taz", FormatTarGz}, {".tbz", FormatTarBz2}, {".tbz2", FormatTarBz2},
	{".tb2", FormatTarBz2}, {".tz2", FormatTarBz2}, {".txz", FormatTarXz}, {".tzst", FormatTarZst},
	{".tlz", FormatTarLzma}, {".tar", FormatTar},
	{".gz", FormatGz}, {".bz2", FormatBz2}, {".xz", FormatXz}, {".zst", FormatZst},
	{".zstd", FormatZst}, {".lzma", FormatLzma},
}

// DetectFormat guesses the format from a file name alone. It is what the
// panel uses to decide whether Enter on a file should open it as a folder.
func DetectFormat(name string) Format {
	lower := strings.ToLower(name)
	for _, e := range extTable {
		if strings.HasSuffix(lower, e.ext) && len(lower) > len(e.ext) {
			return e.f
		}
	}
	return FormatUnknown
}

// IsArchiveName reports whether DetectFormat recognises the name.
func IsArchiveName(name string) bool { return DetectFormat(name) != FormatUnknown }

// trimExt removes the format extension from a base name ("a.tar.gz" ->
// "a"), which is the default inner name of single-file archives and the
// default folder name for "unpack each archive to its own folder".
func trimExt(name string) string {
	lower := strings.ToLower(name)
	for _, e := range extTable {
		if strings.HasSuffix(lower, e.ext) && len(lower) > len(e.ext) {
			return name[:len(name)-len(e.ext)]
		}
	}
	return name
}

// BaseName returns the archive name without its format extension.
func BaseName(name string) string { return trimExt(name) }

// sniffCompression looks at the first bytes of a file. lzma has no magic
// number, so it is never sniffed and only recognised by name.
func sniffCompression(head []byte) (Compression, bool) {
	switch {
	case len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b:
		return CompGzip, true
	case len(head) >= 3 && string(head[:3]) == "BZh":
		return CompBzip2, true
	case len(head) >= 6 && bytes.Equal(head[:6], []byte{0xfd, '7', 'z', 'X', 'Z', 0}):
		return CompXz, true
	case len(head) >= 4 && bytes.Equal(head[:4], []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return CompZstd, true
	}
	return CompNone, false
}

// looksLikeTar checks a 512-byte block for the "ustar" magic or, for old v7
// archives, a valid header checksum.
func looksLikeTar(block []byte) bool {
	if len(block) < 512 {
		return false
	}
	if string(block[257:262]) == "ustar" {
		return true
	}
	// v7: checksum field at 148..156 is octal; sum all bytes with that
	// field counted as spaces.
	field := strings.TrimRight(strings.TrimLeft(string(block[148:156]), " \x00"), " \x00")
	if field == "" {
		return false
	}
	var want int64
	for _, c := range field {
		if c < '0' || c > '7' {
			return false
		}
		want = want*8 + int64(c-'0')
	}
	var sum int64
	for i, b := range block {
		if i >= 148 && i < 156 {
			sum += ' '
		} else {
			sum += int64(b)
		}
	}
	return sum == want && block[0] != 0
}

// decompress wraps r in the decompressor for c. The returned closer must be
// closed to release decoder resources (zstd keeps goroutines).
func decompress(c Compression, r io.Reader) (io.ReadCloser, error) {
	switch c {
	case CompNone:
		return io.NopCloser(r), nil
	case CompGzip:
		return gzip.NewReader(r)
	case CompBzip2:
		return io.NopCloser(bzip2.NewReader(r)), nil
	case CompXz:
		x, err := xz.NewReader(r)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(x), nil
	case CompZstd:
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		return d.IOReadCloser(), nil
	case CompLzma:
		l, err := lzma.NewReader(bufio.NewReader(r))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(l), nil
	}
	return nil, errors.New("tarfmt: unknown compression")
}

// compress wraps w in the compressor for c. level 0 means the codec's
// default; otherwise 1 (fastest) .. 9 (smallest), as in TC's packer page.
// xz and lzma ignore the level apart from choosing a larger dictionary for
// high levels, because the Go codec has no presets.
func compress(c Compression, w io.Writer, level int) (io.WriteCloser, error) {
	if level < 0 || level > 9 {
		return nil, fmt.Errorf("tarfmt: compression level %d out of range 0..9", level)
	}
	switch c {
	case CompNone:
		return nopWriteCloser{w}, nil
	case CompGzip:
		if level == 0 {
			level = gzip.DefaultCompression
		}
		return gzip.NewWriterLevel(w, level)
	case CompBzip2:
		cfg := &dbzip2.WriterConfig{Level: dbzip2.DefaultCompression}
		if level != 0 {
			cfg.Level = level
		}
		return dbzip2.NewWriter(w, cfg)
	case CompXz:
		cfg := xz.WriterConfig{}
		if level >= 7 {
			cfg.DictCap = 32 << 20
		}
		return cfg.NewWriter(w)
	case CompZstd:
		opts := []zstd.EOption{zstd.WithEncoderConcurrency(1)}
		if level != 0 {
			opts = append(opts, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(zstdLevel(level))))
		}
		return zstd.NewWriter(w, opts...)
	case CompLzma:
		cfg := lzma.WriterConfig{}
		if level >= 7 {
			cfg.DictCap = 32 << 20
		}
		return cfg.NewWriter(w)
	}
	return nil, errors.New("tarfmt: unknown compression")
}

// zstdLevel maps TC's 1..9 scale onto zstd's 1..19 scale.
func zstdLevel(level int) int { return level*2 + 1 }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
