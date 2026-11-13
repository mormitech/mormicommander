// Tests for formatting and parsing checksum lines in the GNU, BSD and SFV
// layouts, including the awkward cases found in files from the wild.

package fileutil

import (
	"strings"
	"testing"
)

const (
	md5abc    = "900150983cd24fb0d6963f7d28e17f72"
	sha1ab    = "a9993e364706816aba3e25717850c26c9cd0d89d"
	sha256abc = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
)

func TestFormatLine(t *testing.T) {
	tests := []struct {
		style  LineStyle
		alg    Algorithm
		name   string
		digest string
		binary bool
		want   string
	}{
		{StyleGNU, MD5, "a.txt", md5abc, true, md5abc + " *a.txt"},
		{StyleGNU, MD5, "a.txt", md5abc, false, md5abc + "  a.txt"},
		{StyleGNU, MD5, `sub\a.txt`, md5abc, true, md5abc + ` *sub\a.txt`},
		{StyleBSD, SHA256, "a b.txt", sha256abc, true, "SHA256 (a b.txt) = " + sha256abc},
		{StyleSFV, CRC32, "a.txt", "352441c2", true, "a.txt 352441C2"},
		{StyleGNU, MD5, "new\nline", md5abc, true, `\` + md5abc + ` *new\nline`},
		{StyleGNU, MD5, "back\\slash\nx", md5abc, true, `\` + md5abc + ` *back\\slash\nx`},
	}
	for _, tt := range tests {
		if got := FormatLine(tt.style, tt.alg, tt.name, tt.digest, tt.binary); got != tt.want {
			t.Errorf("FormatLine(%v,%q) = %q, want %q", tt.style, tt.name, got, tt.want)
		}
	}
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		name    string
		hint    Algorithm
		implied string
		line    string
		ok      bool
		want    ChecksumEntry
	}{
		{"gnu binary", MD5, "", md5abc + " *a.txt", true, ChecksumEntry{Name: "a.txt", Digest: md5abc, Binary: true, Algorithm: MD5}},
		{"gnu text", MD5, "", md5abc + "  a.txt", true, ChecksumEntry{Name: "a.txt", Digest: md5abc, Algorithm: MD5}},
		{"gnu single space", MD5, "", md5abc + " a.txt", true, ChecksumEntry{Name: "a.txt", Digest: md5abc, Algorithm: MD5}},
		{"gnu tab", MD5, "", md5abc + "\ta.txt", true, ChecksumEntry{Name: "a.txt", Digest: md5abc, Algorithm: MD5}},
		{"gnu uppercase digest", MD5, "", strings.ToUpper(md5abc) + " *A.TXT", true, ChecksumEntry{Name: "A.TXT", Digest: md5abc, Binary: true, Algorithm: MD5}},
		{"gnu name with spaces", MD5, "", md5abc + "  my file .txt", true, ChecksumEntry{Name: "my file .txt", Digest: md5abc, Algorithm: MD5}},
		{"gnu name leading space", MD5, "", md5abc + "   x", true, ChecksumEntry{Name: " x", Digest: md5abc, Algorithm: MD5}},
		{"gnu windows path", SHA1, "", sha1ab + ` *sub\dir\a.txt`, true, ChecksumEntry{Name: `sub\dir\a.txt`, Digest: sha1ab, Binary: true, Algorithm: SHA1}},
		{"gnu escaped", MD5, "", `\` + md5abc + ` *new\nline\\x`, true, ChecksumEntry{Name: "new\nline\\x", Digest: md5abc, Binary: true, Algorithm: MD5}},
		{"bsd", InvalidAlgorithm, "", "SHA256 (a b.txt) = " + sha256abc, true, ChecksumEntry{Name: "a b.txt", Digest: sha256abc, Algorithm: SHA256}},
		{"bsd md5 macOS", InvalidAlgorithm, "", "MD5 (a.txt) = " + md5abc, true, ChecksumEntry{Name: "a.txt", Digest: md5abc, Algorithm: MD5}},
		{"bsd tag overrides hint", MD5, "", "SHA1 (a.txt) = " + sha1ab, true, ChecksumEntry{Name: "a.txt", Digest: sha1ab, Algorithm: SHA1}},
		{"bsd parens in name", InvalidAlgorithm, "", "MD5 (a (1).txt) = " + md5abc, true, ChecksumEntry{Name: "a (1).txt", Digest: md5abc, Algorithm: MD5}},
		{"bsd wrong length", InvalidAlgorithm, "", "SHA256 (a.txt) = " + md5abc, false, ChecksumEntry{}},
		{"sfv", CRC32, "", "a.txt 352441C2", true, ChecksumEntry{Name: "a.txt", Digest: "352441c2", Algorithm: CRC32}},
		{"sfv spaces in name", CRC32, "", "my file.txt   352441c2", true, ChecksumEntry{Name: "my file.txt", Digest: "352441c2", Algorithm: CRC32}},
		{"sfv without hint", InvalidAlgorithm, "", "a.txt 352441C2", true, ChecksumEntry{Name: "a.txt", Digest: "352441c2", Algorithm: CRC32}},
		{"sfv style in md5 file is junk", MD5, "", "a.txt 352441C2", false, ChecksumEntry{}},
		{"gnu crc in sfv file", CRC32, "", "352441c2 *a.txt", true, ChecksumEntry{Name: "a.txt", Digest: "352441c2", Binary: true, Algorithm: CRC32}},
		{"ambiguous 64 digits", InvalidAlgorithm, "", sha256abc + "  a", true, ChecksumEntry{Name: "a", Digest: sha256abc, Algorithm: SHA256, Candidates: []Algorithm{SHA256, SHA3_256, BLAKE3}}},
		{"hint resolves 64 digits", BLAKE3, "", sha256abc + "  a", true, ChecksumEntry{Name: "a", Digest: sha256abc, Algorithm: BLAKE3}},
		{"hint mismatched length falls back", SHA1, "", md5abc + "  a", true, ChecksumEntry{Name: "a", Digest: md5abc, Algorithm: MD5}},
		{"digest only", SHA256, "image.iso", sha256abc, true, ChecksumEntry{Name: "image.iso", Digest: sha256abc, Algorithm: SHA256}},
		{"digest only no implied name", SHA256, "", sha256abc, false, ChecksumEntry{}},
		{"bad length", InvalidAlgorithm, "", "abcdef  a.txt", false, ChecksumEntry{}},
		{"garbage", MD5, "", "hello world", false, ChecksumEntry{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseLine(tt.line, tt.hint, tt.implied)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tt.ok, got)
			}
			if !ok {
				return
			}
			if got.Name != tt.want.Name || got.Digest != tt.want.Digest || got.Binary != tt.want.Binary || got.Algorithm != tt.want.Algorithm || !equalAlgs(got.Candidates, tt.want.Candidates) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func equalAlgs(a, b []Algorithm) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseChecksumsFile(t *testing.T) {
	text := "\xef\xbb\xbf; Generated by mormiCommander\r\n" +
		"\r\n" +
		"# comment\r\n" +
		"a.txt 352441C2\r\n" +
		"this line is broken\r\n" +
		"b.txt DEADBEEF\r\n"
	l, err := ParseChecksums(strings.NewReader(text), CRC32, "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 2 || l.Entries[0].Name != "a.txt" || l.Entries[0].Line != 4 || l.Entries[1].Line != 6 {
		t.Errorf("entries = %+v", l.Entries)
	}
	if len(l.BadLines) != 1 || l.BadLines[0].Line != 5 {
		t.Errorf("bad lines = %+v", l.BadLines)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	names := []string{"a.txt", "with space.bin", `win\path\f`, "unicode-árvíztűrő.txt", "new\nline", "tab\there", "a (1).txt"}
	for _, style := range []LineStyle{StyleGNU, StyleBSD} {
		for _, n := range names {
			line := FormatLine(style, SHA1, n, sha1ab, true)
			e, ok := parseLine(line, SHA1, "")
			if !ok || e.Name != n || e.Digest != sha1ab || e.Algorithm != SHA1 {
				t.Errorf("style %v name %q: line %q parsed to %+v ok=%v", style, n, line, e, ok)
			}
		}
	}
	for _, n := range names[:5] {
		line := FormatLine(StyleSFV, CRC32, n, "352441c2", true)
		e, ok := parseLine(line, CRC32, "")
		if !ok || e.Name != n || e.Digest != "352441c2" {
			t.Errorf("sfv name %q: line %q parsed to %+v ok=%v", n, line, e, ok)
		}
	}
}
