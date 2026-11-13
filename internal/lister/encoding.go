// Character encodings for the Lister. Total Commander's Lister can show a
// file as ANSI (the Windows code page of the system), ASCII (the old DOS
// "OEM" code page, usually 437 or 852), UTF-8, UTF-16 and a long list of
// other code pages picked from the Encoding menu. This file keeps the list of
// encodings we offer, and a tiny per-character decoder for each of them. We
// decode one character at a time, and always know how many bytes it used,
// because every other part of the Lister (line layout, wrapping, search
// highlighting, selection) has to map screen characters back to exact byte
// offsets in a file that may be many gigabytes long and is never loaded
// whole.

package lister

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// Encoding names one character encoding. The values are lowercase labels
// close to the IANA/WHATWG names, so they can be stored in settings files and
// shown in menus without translation tables.
type Encoding string

// The encodings the Lister offers. The list mirrors what Total Commander's
// Encoding menu has, plus the common East Asian multi-byte encodings.
const (
	UTF8        Encoding = "utf-8"
	UTF16LE     Encoding = "utf-16le"
	UTF16BE     Encoding = "utf-16be"
	Win874      Encoding = "windows-874"
	Win1250     Encoding = "windows-1250"
	Win1251     Encoding = "windows-1251"
	Win1252     Encoding = "windows-1252"
	Win1253     Encoding = "windows-1253"
	Win1254     Encoding = "windows-1254"
	Win1255     Encoding = "windows-1255"
	Win1256     Encoding = "windows-1256"
	Win1257     Encoding = "windows-1257"
	Win1258     Encoding = "windows-1258"
	ISO8859_1   Encoding = "iso-8859-1"
	ISO8859_2   Encoding = "iso-8859-2"
	ISO8859_3   Encoding = "iso-8859-3"
	ISO8859_4   Encoding = "iso-8859-4"
	ISO8859_5   Encoding = "iso-8859-5"
	ISO8859_6   Encoding = "iso-8859-6"
	ISO8859_7   Encoding = "iso-8859-7"
	ISO8859_8   Encoding = "iso-8859-8"
	ISO8859_9   Encoding = "iso-8859-9"
	ISO8859_10  Encoding = "iso-8859-10"
	ISO8859_13  Encoding = "iso-8859-13"
	ISO8859_14  Encoding = "iso-8859-14"
	ISO8859_15  Encoding = "iso-8859-15"
	ISO8859_16  Encoding = "iso-8859-16"
	DOS437      Encoding = "ibm437"
	DOS850      Encoding = "ibm850"
	DOS852      Encoding = "ibm852"
	DOS855      Encoding = "ibm855"
	DOS858      Encoding = "ibm858"
	DOS860      Encoding = "ibm860"
	DOS862      Encoding = "ibm862"
	DOS863      Encoding = "ibm863"
	DOS865      Encoding = "ibm865"
	DOS866      Encoding = "ibm866"
	KOI8R       Encoding = "koi8-r"
	KOI8U       Encoding = "koi8-u"
	MacRoman    Encoding = "macintosh"
	MacCyrillic Encoding = "x-mac-cyrillic"
	EBCDIC037   Encoding = "ibm037"
	ShiftJIS    Encoding = "shift_jis"
	EUCJP       Encoding = "euc-jp"
	GBK         Encoding = "gbk"
	GB18030     Encoding = "gb18030"
	Big5        Encoding = "big5"
	EUCKR       Encoding = "euc-kr"
)

// encKind says how characters of an encoding are split into bytes.
type encKind int

const (
	kindUTF8 encKind = iota
	kindUTF16
	kindSingle // one byte is always one character
	kindMulti  // East Asian double-byte encodings
)

// EncodingInfo describes one entry of the Encoding menu.
type EncodingInfo struct {
	ID         Encoding // stable identifier, e.g. "windows-1250"
	Name       string   // menu text, e.g. "Central European (Windows-1250)"
	Group      string   // menu group: "Unicode", "Windows (ANSI)", "DOS (OEM)", "ISO", "Other", "East Asian"
	Codepage   int      // Windows code page number, 0 if none
	SingleByte bool     // true when every byte is exactly one character
}

type encEntry struct {
	info EncodingInfo
	kind encKind
	cm   *charmap.Charmap  // for kindSingle
	mb   encoding.Encoding // for kindMulti
	be   bool              // for kindUTF16
}

var encTable = []encEntry{
	{info: EncodingInfo{UTF8, "Unicode (UTF-8)", "Unicode", 65001, false}, kind: kindUTF8},
	{info: EncodingInfo{UTF16LE, "Unicode (UTF-16 little endian)", "Unicode", 1200, false}, kind: kindUTF16},
	{info: EncodingInfo{UTF16BE, "Unicode (UTF-16 big endian)", "Unicode", 1201, false}, kind: kindUTF16, be: true},
	{info: EncodingInfo{Win1250, "Central European (Windows-1250)", "Windows (ANSI)", 1250, true}, kind: kindSingle, cm: charmap.Windows1250},
	{info: EncodingInfo{Win1251, "Cyrillic (Windows-1251)", "Windows (ANSI)", 1251, true}, kind: kindSingle, cm: charmap.Windows1251},
	{info: EncodingInfo{Win1252, "Western (Windows-1252)", "Windows (ANSI)", 1252, true}, kind: kindSingle, cm: charmap.Windows1252},
	{info: EncodingInfo{Win1253, "Greek (Windows-1253)", "Windows (ANSI)", 1253, true}, kind: kindSingle, cm: charmap.Windows1253},
	{info: EncodingInfo{Win1254, "Turkish (Windows-1254)", "Windows (ANSI)", 1254, true}, kind: kindSingle, cm: charmap.Windows1254},
	{info: EncodingInfo{Win1255, "Hebrew (Windows-1255)", "Windows (ANSI)", 1255, true}, kind: kindSingle, cm: charmap.Windows1255},
	{info: EncodingInfo{Win1256, "Arabic (Windows-1256)", "Windows (ANSI)", 1256, true}, kind: kindSingle, cm: charmap.Windows1256},
	{info: EncodingInfo{Win1257, "Baltic (Windows-1257)", "Windows (ANSI)", 1257, true}, kind: kindSingle, cm: charmap.Windows1257},
	{info: EncodingInfo{Win1258, "Vietnamese (Windows-1258)", "Windows (ANSI)", 1258, true}, kind: kindSingle, cm: charmap.Windows1258},
	{info: EncodingInfo{Win874, "Thai (Windows-874)", "Windows (ANSI)", 874, true}, kind: kindSingle, cm: charmap.Windows874},
	{info: EncodingInfo{DOS437, "DOS United States (437)", "DOS (OEM)", 437, true}, kind: kindSingle, cm: charmap.CodePage437},
	{info: EncodingInfo{DOS850, "DOS Western European (850)", "DOS (OEM)", 850, true}, kind: kindSingle, cm: charmap.CodePage850},
	{info: EncodingInfo{DOS852, "DOS Central European (852)", "DOS (OEM)", 852, true}, kind: kindSingle, cm: charmap.CodePage852},
	{info: EncodingInfo{DOS855, "DOS Cyrillic (855)", "DOS (OEM)", 855, true}, kind: kindSingle, cm: charmap.CodePage855},
	{info: EncodingInfo{DOS858, "DOS Western European with euro (858)", "DOS (OEM)", 858, true}, kind: kindSingle, cm: charmap.CodePage858},
	{info: EncodingInfo{DOS860, "DOS Portuguese (860)", "DOS (OEM)", 860, true}, kind: kindSingle, cm: charmap.CodePage860},
	{info: EncodingInfo{DOS862, "DOS Hebrew (862)", "DOS (OEM)", 862, true}, kind: kindSingle, cm: charmap.CodePage862},
	{info: EncodingInfo{DOS863, "DOS French Canadian (863)", "DOS (OEM)", 863, true}, kind: kindSingle, cm: charmap.CodePage863},
	{info: EncodingInfo{DOS865, "DOS Nordic (865)", "DOS (OEM)", 865, true}, kind: kindSingle, cm: charmap.CodePage865},
	{info: EncodingInfo{DOS866, "DOS Russian (866)", "DOS (OEM)", 866, true}, kind: kindSingle, cm: charmap.CodePage866},
	{info: EncodingInfo{ISO8859_1, "Western (ISO-8859-1)", "ISO", 28591, true}, kind: kindSingle, cm: charmap.ISO8859_1},
	{info: EncodingInfo{ISO8859_2, "Central European (ISO-8859-2)", "ISO", 28592, true}, kind: kindSingle, cm: charmap.ISO8859_2},
	{info: EncodingInfo{ISO8859_3, "South European (ISO-8859-3)", "ISO", 28593, true}, kind: kindSingle, cm: charmap.ISO8859_3},
	{info: EncodingInfo{ISO8859_4, "Baltic (ISO-8859-4)", "ISO", 28594, true}, kind: kindSingle, cm: charmap.ISO8859_4},
	{info: EncodingInfo{ISO8859_5, "Cyrillic (ISO-8859-5)", "ISO", 28595, true}, kind: kindSingle, cm: charmap.ISO8859_5},
	{info: EncodingInfo{ISO8859_6, "Arabic (ISO-8859-6)", "ISO", 28596, true}, kind: kindSingle, cm: charmap.ISO8859_6},
	{info: EncodingInfo{ISO8859_7, "Greek (ISO-8859-7)", "ISO", 28597, true}, kind: kindSingle, cm: charmap.ISO8859_7},
	{info: EncodingInfo{ISO8859_8, "Hebrew (ISO-8859-8)", "ISO", 28598, true}, kind: kindSingle, cm: charmap.ISO8859_8},
	{info: EncodingInfo{ISO8859_9, "Turkish (ISO-8859-9)", "ISO", 28599, true}, kind: kindSingle, cm: charmap.ISO8859_9},
	{info: EncodingInfo{ISO8859_10, "Nordic (ISO-8859-10)", "ISO", 28600, true}, kind: kindSingle, cm: charmap.ISO8859_10},
	{info: EncodingInfo{ISO8859_13, "Baltic (ISO-8859-13)", "ISO", 28603, true}, kind: kindSingle, cm: charmap.ISO8859_13},
	{info: EncodingInfo{ISO8859_14, "Celtic (ISO-8859-14)", "ISO", 28604, true}, kind: kindSingle, cm: charmap.ISO8859_14},
	{info: EncodingInfo{ISO8859_15, "Western with euro (ISO-8859-15)", "ISO", 28605, true}, kind: kindSingle, cm: charmap.ISO8859_15},
	{info: EncodingInfo{ISO8859_16, "South-Eastern European (ISO-8859-16)", "ISO", 28606, true}, kind: kindSingle, cm: charmap.ISO8859_16},
	{info: EncodingInfo{KOI8R, "Cyrillic (KOI8-R)", "Other", 20866, true}, kind: kindSingle, cm: charmap.KOI8R},
	{info: EncodingInfo{KOI8U, "Ukrainian (KOI8-U)", "Other", 21866, true}, kind: kindSingle, cm: charmap.KOI8U},
	{info: EncodingInfo{MacRoman, "Western (Mac Roman)", "Other", 10000, true}, kind: kindSingle, cm: charmap.Macintosh},
	{info: EncodingInfo{MacCyrillic, "Cyrillic (Mac)", "Other", 10007, true}, kind: kindSingle, cm: charmap.MacintoshCyrillic},
	{info: EncodingInfo{EBCDIC037, "EBCDIC US/Canada (037)", "Other", 37, true}, kind: kindSingle, cm: charmap.CodePage037},
	{info: EncodingInfo{ShiftJIS, "Japanese (Shift-JIS)", "East Asian", 932, false}, kind: kindMulti, mb: japanese.ShiftJIS},
	{info: EncodingInfo{EUCJP, "Japanese (EUC-JP)", "East Asian", 20932, false}, kind: kindMulti, mb: japanese.EUCJP},
	{info: EncodingInfo{GBK, "Chinese Simplified (GBK)", "East Asian", 936, false}, kind: kindMulti, mb: simplifiedchinese.GBK},
	{info: EncodingInfo{GB18030, "Chinese Simplified (GB18030)", "East Asian", 54936, false}, kind: kindMulti, mb: simplifiedchinese.GB18030},
	{info: EncodingInfo{Big5, "Chinese Traditional (Big5)", "East Asian", 950, false}, kind: kindMulti, mb: traditionalchinese.Big5},
	{info: EncodingInfo{EUCKR, "Korean (EUC-KR)", "East Asian", 949, false}, kind: kindMulti, mb: korean.EUCKR},
}

var encByID = func() map[Encoding]*encEntry {
	m := make(map[Encoding]*encEntry, len(encTable))
	for i := range encTable {
		m[encTable[i].info.ID] = &encTable[i]
	}
	return m
}()

// aliases maps other common spellings to our IDs so settings written by hand
// or names found in HTML meta tags still resolve.
var encAliases = map[string]Encoding{
	"utf8": UTF8, "utf-16": UTF16LE, "utf16": UTF16LE, "unicode": UTF16LE,
	"latin1": ISO8859_1, "latin-1": ISO8859_1, "latin2": ISO8859_2, "latin-2": ISO8859_2,
	"cp437": DOS437, "437": DOS437, "cp850": DOS850, "cp852": DOS852, "cp866": DOS866,
	"cp1250": Win1250, "cp1251": Win1251, "cp1252": Win1252, "cp1253": Win1253,
	"cp1254": Win1254, "cp1255": Win1255, "cp1256": Win1256, "cp1257": Win1257, "cp1258": Win1258,
	"sjis": ShiftJIS, "shift-jis": ShiftJIS, "cp932": ShiftJIS, "windows-31j": ShiftJIS,
	"cp936": GBK, "gb2312": GBK, "cp950": Big5, "cp949": EUCKR, "ks_c_5601-1987": EUCKR,
	"koi8r": KOI8R, "koi8u": KOI8U, "mac": MacRoman, "macroman": MacRoman,
	"ascii": UTF8, "us-ascii": UTF8, "tis-620": Win874, "cp874": Win874,
}

// Encodings returns every encoding the Lister supports, in menu order.
func Encodings() []EncodingInfo {
	out := make([]EncodingInfo, len(encTable))
	for i := range encTable {
		out[i] = encTable[i].info
	}
	return out
}

// LookupEncoding resolves an encoding name (ID, alias or Windows code page
// number written as text, case-insensitive). ok is false for unknown names.
func LookupEncoding(name string) (Encoding, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if e, ok := encByID[Encoding(n)]; ok {
		return e.info.ID, true
	}
	if e, ok := encAliases[n]; ok {
		return e, true
	}
	cp := 0
	for _, c := range n {
		if c < '0' || c > '9' {
			cp = -1
			break
		}
		cp = cp*10 + int(c-'0')
		if cp > 1<<20 {
			cp = -1
			break
		}
	}
	if cp > 0 {
		return EncodingForCodepage(cp)
	}
	return "", false
}

// EncodingForCodepage maps a Windows code page number (as returned by
// GetACP/GetOEMCP) to our encoding.
func EncodingForCodepage(cp int) (Encoding, bool) {
	for i := range encTable {
		if encTable[i].info.Codepage == cp {
			return encTable[i].info.ID, true
		}
	}
	return "", false
}

// Info returns the menu description of e, or a zero value for unknown names.
func (e Encoding) Info() EncodingInfo {
	if en, ok := encByID[e]; ok {
		return en.info
	}
	return EncodingInfo{}
}

// Valid reports whether e is one of the supported encodings.
func (e Encoding) Valid() bool { _, ok := encByID[e]; return ok }

// IsUTF16 reports whether e is one of the UTF-16 variants, whose characters
// are made of 2-byte units (newline is then 0x0A 0x00 or 0x00 0x0A).
func (e Encoding) IsUTF16() bool { return e == UTF16LE || e == UTF16BE }

// unitSize is the size of the code unit line breaks are made of.
func (e Encoding) unitSize() int {
	if e.IsUTF16() {
		return 2
	}
	return 1
}

func entryFor(e Encoding) *encEntry {
	if en, ok := encByID[e]; ok {
		return en
	}
	return encByID[UTF8]
}

// runeDecoder decodes exactly one character at the start of b and returns
// how many bytes it used. Invalid input yields utf8.RuneError with size 1 (or
// 2 for a broken UTF-16 unit), so callers always make progress. A decoder may
// keep scratch buffers, so one value must not be shared between goroutines.
type runeDecoder interface {
	decode(b []byte) (r rune, size int)
}

func newRuneDecoder(e Encoding) runeDecoder {
	en := entryFor(e)
	switch en.kind {
	case kindUTF16:
		return utf16Decoder{be: en.be}
	case kindSingle:
		return singleTable(e)
	case kindMulti:
		return &multiDecoder{id: en.info.ID, dec: en.mb.NewDecoder()}
	}
	return utf8Decoder{}
}

type utf8Decoder struct{}

func (utf8Decoder) decode(b []byte) (rune, int) {
	if len(b) == 0 {
		return utf8.RuneError, 0
	}
	if b[0] < utf8.RuneSelf {
		return rune(b[0]), 1
	}
	return utf8.DecodeRune(b)
}

type utf16Decoder struct{ be bool }

func (d utf16Decoder) unit(b []byte) rune {
	if d.be {
		return rune(b[0])<<8 | rune(b[1])
	}
	return rune(b[1])<<8 | rune(b[0])
}

func (d utf16Decoder) decode(b []byte) (rune, int) {
	if len(b) < 2 {
		if len(b) == 0 {
			return utf8.RuneError, 0
		}
		return utf8.RuneError, 1
	}
	u := d.unit(b)
	switch {
	case u >= 0xD800 && u < 0xDC00:
		if len(b) >= 4 {
			u2 := d.unit(b[2:])
			if u2 >= 0xDC00 && u2 < 0xE000 {
				return (u-0xD800)<<10 | (u2 - 0xDC00) + 0x10000, 4
			}
		}
		return utf8.RuneError, 2
	case u >= 0xDC00 && u < 0xE000:
		return utf8.RuneError, 2
	}
	return u, 2
}

// tableDecoder is a decoder for single-byte code pages: a plain 256-entry
// lookup table, which is also what binary and hex mode use.
type tableDecoder struct{ t *[256]rune }

func (d tableDecoder) decode(b []byte) (rune, int) {
	if len(b) == 0 {
		return utf8.RuneError, 0
	}
	return d.t[b[0]], 1
}

var tableCache = map[Encoding]*[256]rune{}

func init() {
	for i := range encTable {
		en := &encTable[i]
		if en.kind != kindSingle {
			continue
		}
		t := new([256]rune)
		for b := 0; b < 256; b++ {
			t[b] = en.cm.DecodeByte(byte(b))
		}
		tableCache[en.info.ID] = t
	}
}

func singleTable(e Encoding) runeDecoder {
	if t, ok := tableCache[e]; ok {
		return tableDecoder{t}
	}
	return tableDecoder{tableCache[ISO8859_1]}
}

// multiDecoder handles the East Asian double-byte encodings. It first works
// out the byte length of the character from its lead byte, then converts
// just those bytes with the x/text decoder.
type multiDecoder struct {
	id  Encoding
	dec *encoding.Decoder
	dst [16]byte
}

func (d *multiDecoder) charLen(b []byte) int {
	c := b[0]
	if c < 0x80 {
		return 1
	}
	switch d.id {
	case ShiftJIS:
		if (c >= 0x81 && c <= 0x9F) || (c >= 0xE0 && c <= 0xFC) {
			return 2
		}
		return 1
	case EUCJP:
		switch {
		case c == 0x8E:
			return 2
		case c == 0x8F:
			return 3
		case c >= 0xA1 && c <= 0xFE:
			return 2
		}
		return 1
	case GBK, GB18030:
		if c >= 0x81 && c <= 0xFE {
			if len(b) >= 2 && b[1] >= 0x30 && b[1] <= 0x39 {
				return 4
			}
			return 2
		}
		return 1
	default: // Big5, EUC-KR
		if c >= 0x81 && c <= 0xFE {
			return 2
		}
		return 1
	}
}

func (d *multiDecoder) decode(b []byte) (rune, int) {
	if len(b) == 0 {
		return utf8.RuneError, 0
	}
	n := d.charLen(b)
	if n == 1 {
		if b[0] < 0x80 {
			return rune(b[0]), 1
		}
	}
	if n > len(b) {
		return utf8.RuneError, 1
	}
	d.dec.Reset()
	nDst, _, err := d.dec.Transform(d.dst[:], b[:n], true)
	if err != nil || nDst == 0 {
		return utf8.RuneError, 1
	}
	r, _ := utf8.DecodeRune(d.dst[:nDst])
	if r == utf8.RuneError && n > 1 {
		return r, 1
	}
	return r, n
}

// DecodeString converts raw bytes in encoding e to a Go string. Invalid
// sequences become U+FFFD.
func DecodeString(e Encoding, b []byte) string {
	if e == UTF8 && utf8.Valid(b) {
		return string(b)
	}
	d := newRuneDecoder(e)
	var sb strings.Builder
	sb.Grow(len(b))
	for len(b) > 0 {
		r, n := d.decode(b)
		sb.WriteRune(r)
		b = b[n:]
	}
	return sb.String()
}

// EncodeString converts s into encoding e, for saving or for building byte
// patterns. Characters the encoding cannot represent become '?'.
func EncodeString(e Encoding, s string) []byte {
	en := entryFor(e)
	switch en.kind {
	case kindUTF8:
		return []byte(s)
	case kindUTF16:
		out := make([]byte, 0, len(s)*2)
		put := func(u uint16) {
			if en.be {
				out = append(out, byte(u>>8), byte(u))
			} else {
				out = append(out, byte(u), byte(u>>8))
			}
		}
		for _, r := range s {
			if r >= 0x10000 {
				r -= 0x10000
				put(uint16(0xD800 + (r >> 10)))
				put(uint16(0xDC00 + (r & 0x3FF)))
			} else {
				put(uint16(r))
			}
		}
		return out
	case kindSingle:
		out := make([]byte, 0, len(s))
		for _, r := range s {
			b, ok := en.cm.EncodeRune(r)
			if !ok {
				b = '?'
			}
			out = append(out, b)
		}
		return out
	}
	out, err := encoding.ReplaceUnsupported(en.mb.NewEncoder()).Bytes([]byte(s))
	if err != nil {
		return []byte(s)
	}
	return out
}
