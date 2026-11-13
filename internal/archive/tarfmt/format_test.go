// Tests for format recognition by name and content, and for the path rules
// that keep hostile archive names inside the target folder.
package tarfmt

import (
	"runtime"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name string
		want Format
	}{
		{"a.tar", FormatTar},
		{"A.TAR", FormatTar},
		{"a.tar.gz", FormatTarGz},
		{"a.tgz", FormatTarGz},
		{"a.taz", FormatTarGz},
		{"a.tar.bz2", FormatTarBz2},
		{"a.tbz", FormatTarBz2},
		{"a.tbz2", FormatTarBz2},
		{"a.tb2", FormatTarBz2},
		{"a.tar.xz", FormatTarXz},
		{"a.txz", FormatTarXz},
		{"a.tar.zst", FormatTarZst},
		{"a.tzst", FormatTarZst},
		{"a.tar.lzma", FormatTarLzma},
		{"a.tlz", FormatTarLzma},
		{"a.gz", FormatGz},
		{"a.txt.bz2", FormatBz2},
		{"a.xz", FormatXz},
		{"a.zst", FormatZst},
		{"a.lzma", FormatLzma},
		{"a.zip", FormatUnknown},
		{".gz", FormatUnknown},
		{"tar", FormatUnknown},
		{"", FormatUnknown},
	}
	for _, c := range cases {
		if got := DetectFormat(c.name); got != c.want {
			t.Errorf("DetectFormat(%q) = %v, want %v", c.name, got, c.want)
		}
		if IsArchiveName(c.name) != (c.want != FormatUnknown) {
			t.Errorf("IsArchiveName(%q) wrong", c.name)
		}
	}
}

func TestBaseName(t *testing.T) {
	cases := map[string]string{
		"a.tar.gz": "a", "src-1.2.tgz": "src-1.2", "notes.txt.gz": "notes.txt",
		"b.tar": "b", "c.zip": "c.zip", "D.TAR.XZ": "D",
	}
	for in, want := range cases {
		if got := BaseName(in); got != want {
			t.Errorf("BaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatProperties(t *testing.T) {
	for _, f := range tarFormats {
		if !f.IsTar() || f.withTar() != f {
			t.Errorf("%v: IsTar/withTar wrong", f)
		}
	}
	for _, f := range singleFormats {
		if f.IsTar() || f.withTar().Compression() != f.Compression() || f.withTar().withoutTar() != f {
			t.Errorf("%v: single properties wrong", f)
		}
	}
	if FormatTarGz.Ext() != ".tar.gz" || FormatUnknown.Ext() != "" || Format(99).String() != "Format(99)" {
		t.Error("Ext/String wrong")
	}
	if n := len(Formats()); n != 11 {
		t.Errorf("Formats() has %d entries", n)
	}
}

func TestSniffCompression(t *testing.T) {
	data := []byte("hello hello hello")
	for _, c := range []Compression{CompGzip, CompBzip2, CompXz, CompZstd} {
		got, ok := sniffCompression(compressBytes(t, c, data))
		if !ok || got != c {
			t.Errorf("sniff %v = %v %v", c, got, ok)
		}
	}
	if _, ok := sniffCompression([]byte("plain")); ok {
		t.Error("plain text sniffed as compressed")
	}
	if _, ok := sniffCompression(compressBytes(t, CompLzma, data)); ok {
		t.Error("lzma has no magic and must not be sniffed")
	}
}

func TestLooksLikeTar(t *testing.T) {
	ustar := makeTar(t, []member{{name: "a", body: "x"}})
	if !looksLikeTar(ustar[:512]) {
		t.Error("ustar header not recognised")
	}
	v7 := append([]byte(nil), ustar[:512]...)
	copy(v7[257:265], make([]byte, 8)) // wipe magic+version: checksum now wrong
	if looksLikeTar(v7) {
		t.Error("block with bad checksum accepted")
	}
	if looksLikeTar(make([]byte, 512)) || looksLikeTar([]byte("short")) {
		t.Error("zero/short block accepted")
	}
}

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"a/b":       "a/b",
		"./a/b/":    "a/b",
		"/abs/x":    "abs/x",
		"//x":       "x",
		"a/./b":     "a/b",
		"a/../b":    "b",
		"../x":      "../x",
		"a/../../x": "../x",
		`a\b`:       "a/b",
		".":         "",
		"./":        "",
		"":          "",
	}
	for in, want := range cases {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLocalName(t *testing.T) {
	cases := map[string]bool{
		"a": true, "a/b": true, "../a": false, "..": false, "": false, "a\x00b": false,
	}
	if runtime.GOOS == "windows" {
		cases["NUL"] = false
		cases["c:/x"] = false
	}
	for in, want := range cases {
		if got := isLocalName(in); got != want {
			t.Errorf("isLocalName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLinkSafety(t *testing.T) {
	cases := []struct {
		link, target string
		want         bool
	}{
		{"a", "b", true},
		{"d/a", "../b", true},
		{"d/a", "../../b", false},
		{"a", "../b", false},
		{"a", "/etc/passwd", false},
		{"a", "", false},
		{"d/a", ".", true},
		{"a", ".", true},
		{"a", "b/..", false}, // ".." after a name: refused by the strict rule
		{"a", "b/../../x", false},
		{"d/e/a", "../../x/y", true},
		{"a", `..\x`, false},
	}
	for _, c := range cases {
		if got := strictLinkSafe(c.link, c.target); got != c.want {
			t.Errorf("strictLinkSafe(%q, %q) = %v, want %v", c.link, c.target, got, c.want)
		}
	}
}

func TestSelected(t *testing.T) {
	if !selected("a/b", nil) || !selected("a/b", []string{"a"}) || selected("ab", []string{"a"}) ||
		!selected("x", []string{""}) || selected("b", []string{"a", "c"}) {
		t.Error("selected wrong")
	}
}

func TestCompressLevelRange(t *testing.T) {
	if _, err := compress(CompGzip, nil, 10); err == nil {
		t.Error("level 10 accepted")
	}
}
