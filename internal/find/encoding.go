// This file turns raw file bytes into text for the content search (F-529). Total
// Commander's "Find text" can look for the same words in several encodings at once:
// the ANSI code page (Windows-1252 and friends), the OEM/DOS code page (CP437, CP852),
// UTF-8 and UTF-16 in both byte orders. Rather than encoding the search text into each
// byte form, we decode the file into Go strings once per selected encoding and search
// there; that keeps case-insensitive and whole-word matching identical for every
// encoding. Decoders work on a stream of chunks, so each one carries over the bytes of
// a character that was cut in half at a chunk boundary.

package find

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Encoding selects which byte forms of the search text are looked for. Several can be
// combined; a file matches if the text is found in any of them.
type Encoding uint8

const (
	EncANSI    Encoding = 1 << iota // the ANSI code page (Options.ANSICodepage, default windows-1252)
	EncOEM                          // the OEM/DOS code page (Options.OEMCodepage, default cp437)
	EncUTF8                         // UTF-8
	EncUTF16LE                      // UTF-16 little endian (Windows "Unicode")
	EncUTF16BE                      // UTF-16 big endian
)

// DefaultEncodings is what Total Commander ticks by default, plus UTF-8 because almost
// every text file on macOS and Linux is UTF-8.
const DefaultEncodings = EncANSI | EncUTF8

// codepages lists the single-byte code pages the ANSI and OEM choices can use.
var codepages = map[string]*charmap.Charmap{
	"windows-1250": charmap.Windows1250, "windows-1251": charmap.Windows1251,
	"windows-1252": charmap.Windows1252, "windows-1253": charmap.Windows1253,
	"windows-1254": charmap.Windows1254, "windows-1255": charmap.Windows1255,
	"windows-1256": charmap.Windows1256, "windows-1257": charmap.Windows1257,
	"windows-1258": charmap.Windows1258, "windows-874": charmap.Windows874,
	"iso-8859-1": charmap.ISO8859_1, "iso-8859-2": charmap.ISO8859_2,
	"iso-8859-3": charmap.ISO8859_3, "iso-8859-4": charmap.ISO8859_4,
	"iso-8859-5": charmap.ISO8859_5, "iso-8859-6": charmap.ISO8859_6,
	"iso-8859-7": charmap.ISO8859_7, "iso-8859-8": charmap.ISO8859_8,
	"iso-8859-9": charmap.ISO8859_9, "iso-8859-10": charmap.ISO8859_10,
	"iso-8859-13": charmap.ISO8859_13, "iso-8859-14": charmap.ISO8859_14,
	"iso-8859-15": charmap.ISO8859_15, "iso-8859-16": charmap.ISO8859_16,
	"cp437": charmap.CodePage437, "cp850": charmap.CodePage850,
	"cp852": charmap.CodePage852, "cp855": charmap.CodePage855,
	"cp858": charmap.CodePage858, "cp860": charmap.CodePage860,
	"cp862": charmap.CodePage862, "cp863": charmap.CodePage863,
	"cp865": charmap.CodePage865, "cp866": charmap.CodePage866,
	"koi8-r": charmap.KOI8R, "koi8-u": charmap.KOI8U,
	"macintosh": charmap.Macintosh, "mac-cyrillic": charmap.MacintoshCyrillic,
}

// Codepages returns the names accepted for Options.ANSICodepage and OEMCodepage,
// sorted, so the UI can offer them in a list.
func Codepages() []string {
	out := make([]string, 0, len(codepages))
	for k := range codepages {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normCodepage accepts a few common spellings ("1252", "CP1252", "ibm437").
func normCodepage(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "-")
	switch {
	case strings.HasPrefix(n, "ibm"):
		n = "cp" + strings.TrimPrefix(n, "ibm")
	case strings.HasPrefix(n, "cp125"), strings.HasPrefix(n, "cp874"):
		n = "windows-" + strings.TrimPrefix(n, "cp")
	case strings.HasPrefix(n, "iso8859-"):
		n = "iso-8859-" + strings.TrimPrefix(n, "iso8859-")
	}
	if len(n) == 4 && strings.HasPrefix(n, "125") {
		n = "windows-" + n
	}
	if len(n) == 3 && (n[0] == '4' || n[0] == '8') {
		n = "cp" + n
	}
	return n
}

// codepageTable builds a byte-to-rune table for a code page name.
func codepageTable(name, def string) (*[256]rune, error) {
	if strings.TrimSpace(name) == "" {
		name = def
	}
	cm, ok := codepages[normCodepage(name)]
	if !ok {
		return nil, fmt.Errorf("find: unknown code page %q", name)
	}
	var t [256]rune
	for i := range t {
		t[i] = cm.DecodeByte(byte(i))
	}
	return &t, nil
}

// decoder turns a stream of raw chunks into text. eof tells it no more bytes follow,
// so any carried-over partial character is flushed.
type decoder interface {
	decode(raw []byte, eof bool) string
}

// tableDecoder decodes a single-byte code page; no state is needed.
type tableDecoder struct{ t *[256]rune }

func (d tableDecoder) decode(raw []byte, eof bool) string {
	var b strings.Builder
	b.Grow(len(raw) + len(raw)/4)
	for _, c := range raw {
		if c < 0x80 && d.t[c] == rune(c) {
			b.WriteByte(c)
		} else {
			b.WriteRune(d.t[c])
		}
	}
	return b.String()
}

// utf8Decoder passes UTF-8 through, holding back a sequence cut at the chunk end.
// Invalid bytes become U+FFFD, which can never be part of a match.
type utf8Decoder struct{ carry []byte }

func (d *utf8Decoder) decode(raw []byte, eof bool) string {
	buf := append(d.carry, raw...)
	d.carry = nil
	if !eof {
		// Find the start of the last rune; if it is incomplete, keep it for later.
		i := len(buf) - 1
		for i >= 0 && len(buf)-i < utf8.UTFMax && !utf8.RuneStart(buf[i]) {
			i--
		}
		if i >= 0 && !utf8.FullRune(buf[i:]) {
			d.carry = append([]byte(nil), buf[i:]...)
			buf = buf[:i]
		}
	}
	return strings.ToValidUTF8(string(buf), "�")
}

// utf16Decoder decodes UTF-16 in one byte order. skip drops leading bytes once; the
// search runs a second decoder with skip=1 so text at odd offsets inside binary files
// is found too, as Total Commander does.
type utf16Decoder struct {
	bigEndian bool
	skip      int
	carry     []byte
}

func (d *utf16Decoder) decode(raw []byte, eof bool) string {
	if d.skip > 0 {
		n := min(d.skip, len(raw))
		raw = raw[n:]
		d.skip -= n
	}
	buf := append(d.carry, raw...)
	d.carry = nil
	n := len(buf) / 2
	units := make([]uint16, 0, n)
	for i := 0; i < n; i++ {
		if d.bigEndian {
			units = append(units, uint16(buf[2*i])<<8|uint16(buf[2*i+1]))
		} else {
			units = append(units, uint16(buf[2*i+1])<<8|uint16(buf[2*i]))
		}
	}
	rest := buf[2*n:]
	// A high surrogate at the very end waits for its partner in the next chunk.
	if !eof && len(units) > 0 && utf16.IsSurrogate(rune(units[len(units)-1])) && units[len(units)-1] < 0xDC00 {
		last := units[len(units)-1]
		units = units[:len(units)-1]
		if d.bigEndian {
			rest = append([]byte{byte(last >> 8), byte(last)}, rest...)
		} else {
			rest = append([]byte{byte(last), byte(last >> 8)}, rest...)
		}
	}
	if !eof && len(rest) > 0 {
		d.carry = append([]byte(nil), rest...)
	}
	// U+0000 is common between UTF-16 strings in binaries; it is harmless here.
	return string(utf16.Decode(units))
}

// rawDecoder hands bytes through unchanged; used for hex search, where the "text" is
// a byte string compared exactly.
type rawDecoder struct{}

func (rawDecoder) decode(raw []byte, eof bool) string { return string(raw) }
