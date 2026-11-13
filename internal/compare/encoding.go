// This file detects and converts the character encoding of the files being
// compared. Total Commander's compare tool opens UTF-8, UTF-16 and old
// single-byte "ANSI" or "OEM" (DOS) files alike, shows them correctly and
// writes an edited file back in the encoding it came in, and it has an
// encoding menu for the cases where the guess is wrong. Detection looks for a
// byte order mark first, then for the zero-byte pattern of BOM-less UTF-16,
// then decides between binary data, plain ASCII, valid UTF-8 and a single
// byte code page (Windows-1252 by default, configurable, with Windows-1250
// chosen when bytes undefined in 1252 appear, which is typical of Central
// European text such as Hungarian). The same check tells the compare window
// whether to start in text or in binary mode.
//
// Single-byte code pages leave a few byte values undefined (0x81, 0x8D, 0x8F,
// 0x90 and 0x9D in Windows-1252, for example). The generic decoder would turn
// them into U+FFFD, which cannot be written back, so a file holding one such
// byte could never be saved after an edit. Instead each undefined byte b is
// decoded to the private-use character U+F700+b and encoded back to b, so
// every line the user did not touch is saved byte for byte.
package compare

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

// Encoding identifies a text encoding.
type Encoding int

const (
	// EncAuto asks for detection (only meaningful as an input).
	EncAuto Encoding = iota
	EncUTF8
	EncUTF16LE
	EncUTF16BE
	EncUTF32LE
	EncUTF32BE
	EncWindows1250
	EncWindows1251
	EncWindows1252
	EncISO8859_1
	EncISO8859_2
	EncCP437
	EncCP850
	EncCP852
	EncCP866
	EncMacRoman
	EncKOI8R
	encCount
)

var encNames = [...]string{
	EncAuto:        "Auto",
	EncUTF8:        "UTF-8",
	EncUTF16LE:     "UTF-16LE",
	EncUTF16BE:     "UTF-16BE",
	EncUTF32LE:     "UTF-32LE",
	EncUTF32BE:     "UTF-32BE",
	EncWindows1250: "Windows-1250",
	EncWindows1251: "Windows-1251",
	EncWindows1252: "Windows-1252",
	EncISO8859_1:   "ISO-8859-1",
	EncISO8859_2:   "ISO-8859-2",
	EncCP437:       "CP437",
	EncCP850:       "CP850",
	EncCP852:       "CP852",
	EncCP866:       "CP866",
	EncMacRoman:    "Macintosh",
	EncKOI8R:       "KOI8-R",
}

// String returns the canonical name, e.g. "UTF-8" or "Windows-1250".
func (e Encoding) String() string {
	if e >= 0 && e < encCount {
		return encNames[e]
	}
	return fmt.Sprintf("Encoding(%d)", int(e))
}

// Encodings lists every concrete encoding, in menu order.
func Encodings() []Encoding {
	out := make([]Encoding, 0, encCount-1)
	for e := EncUTF8; e < encCount; e++ {
		out = append(out, e)
	}
	return out
}

// ParseEncoding finds an encoding by name, case-insensitively, also
// accepting common aliases ("utf8", "cp1250", "latin2", "oem").
func ParseEncoding(name string) (Encoding, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.NewReplacer("_", "-", " ", "-").Replace(n)
	aliases := map[string]Encoding{
		"utf8": EncUTF8, "utf16": EncUTF16LE, "utf-16": EncUTF16LE, "unicode": EncUTF16LE,
		"utf16le": EncUTF16LE, "utf16be": EncUTF16BE, "utf32": EncUTF32LE, "utf-32": EncUTF32LE,
		"cp1250": EncWindows1250, "cp1251": EncWindows1251, "cp1252": EncWindows1252,
		"ansi": EncWindows1252, "latin1": EncISO8859_1, "latin-1": EncISO8859_1,
		"latin2": EncISO8859_2, "latin-2": EncISO8859_2, "oem": EncCP437, "ascii": EncUTF8,
		"dos": EncCP437, "ibm437": EncCP437, "ibm852": EncCP852, "mac": EncMacRoman, "macroman": EncMacRoman,
		"koi8r": EncKOI8R, "auto": EncAuto,
	}
	if e, ok := aliases[n]; ok {
		return e, nil
	}
	for e := EncAuto; e < encCount; e++ {
		if strings.EqualFold(encNames[e], n) {
			return e, nil
		}
	}
	return EncAuto, fmt.Errorf("compare: unknown encoding %q", name)
}

// IsUnicode reports whether the encoding is a Unicode transformation format.
func (e Encoding) IsUnicode() bool {
	return e >= EncUTF8 && e <= EncUTF32BE
}

func (e Encoding) xenc() encoding.Encoding {
	switch e {
	case EncUTF16LE:
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case EncUTF16BE:
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	case EncUTF32LE:
		return utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM)
	case EncUTF32BE:
		return utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM)
	case EncWindows1250:
		return charmap.Windows1250
	case EncWindows1251:
		return charmap.Windows1251
	case EncWindows1252:
		return charmap.Windows1252
	case EncISO8859_1:
		return charmap.ISO8859_1
	case EncISO8859_2:
		return charmap.ISO8859_2
	case EncCP437:
		return charmap.CodePage437
	case EncCP850:
		return charmap.CodePage850
	case EncCP852:
		return charmap.CodePage852
	case EncCP866:
		return charmap.CodePage866
	case EncMacRoman:
		return charmap.Macintosh
	case EncKOI8R:
		return charmap.KOI8R
	}
	return nil
}

var boms = []struct {
	bom []byte
	enc Encoding
}{
	// UTF-32LE must be tested before UTF-16LE (its BOM starts the same).
	{[]byte{0xFF, 0xFE, 0x00, 0x00}, EncUTF32LE},
	{[]byte{0x00, 0x00, 0xFE, 0xFF}, EncUTF32BE},
	{[]byte{0xEF, 0xBB, 0xBF}, EncUTF8},
	{[]byte{0xFF, 0xFE}, EncUTF16LE},
	{[]byte{0xFE, 0xFF}, EncUTF16BE},
}

// BOM returns the byte order mark of a Unicode encoding (nil otherwise).
func (e Encoding) BOM() []byte {
	for _, b := range boms {
		if b.enc == e {
			return b.bom
		}
	}
	return nil
}

// Detection is the result of Detect.
type Detection struct {
	Encoding Encoding
	// BOM is true when the data starts with the encoding's byte order mark.
	BOM bool
	// Binary is true when the data does not look like text in any supported
	// encoding; Encoding is then the fallback and should not be trusted.
	Binary bool
	// ASCII is true when every byte is 7-bit (Encoding is then EncUTF8).
	ASCII bool
}

// DetectOptions tunes Detect.
type DetectOptions struct {
	// ANSI is the single-byte code page assumed for text that is neither
	// ASCII nor valid UTF-8. Zero means Windows-1252.
	ANSI Encoding
	// SniffLen is how many leading bytes are inspected for the binary and
	// UTF-16 heuristics. Zero means 64 KiB.
	SniffLen int
}

// Detect guesses the encoding of data.
func Detect(data []byte, o DetectOptions) Detection {
	if o.ANSI == EncAuto {
		o.ANSI = EncWindows1252
	}
	if o.SniffLen <= 0 {
		o.SniffLen = 64 * 1024
	}
	for _, b := range boms {
		if bytes.HasPrefix(data, b.bom) {
			return Detection{Encoding: b.enc, BOM: true}
		}
	}
	sniff := data
	if len(sniff) > o.SniffLen {
		sniff = sniff[:o.SniffLen]
	}
	if e, ok := detectUTF16(sniff); ok {
		return Detection{Encoding: e}
	}
	if looksBinary(sniff) {
		return Detection{Encoding: o.ANSI, Binary: true}
	}
	ascii := true
	for _, c := range data {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return Detection{Encoding: EncUTF8, ASCII: true}
	}
	if utf8.Valid(data) || validUTF8Prefix(data) {
		return Detection{Encoding: EncUTF8}
	}
	if o.ANSI == EncWindows1252 {
		// Bytes that are unassigned in Windows-1252 but are letters in
		// Windows-1250 (Ť, Ź, ť) point to Central European text.
		for _, c := range data {
			if c == 0x8D || c == 0x8F || c == 0x9D {
				return Detection{Encoding: EncWindows1250}
			}
		}
	}
	return Detection{Encoding: o.ANSI}
}

// validUTF8Prefix accepts data that is valid UTF-8 except for a rune cut off
// at the very end (a truncated read). The part before the cut must contain at
// least one multi-byte sequence, otherwise a single-byte file ending in an
// accented letter ("café" in Windows-1252) would be mistaken for UTF-8.
func validUTF8Prefix(data []byte) bool {
	for cut := 1; cut <= 3 && cut <= len(data); cut++ {
		head := data[:len(data)-cut]
		if utf8.Valid(head) && !utf8.FullRune(data[len(data)-cut:]) {
			return bytes.IndexFunc(head, func(r rune) bool { return r >= utf8.RuneSelf }) >= 0
		}
	}
	return false
}

// detectUTF16 recognises BOM-less UTF-16 by the zero high bytes of ASCII
// characters.
func detectUTF16(s []byte) (Encoding, bool) {
	pairs := len(s) / 2
	if pairs < 2 {
		return 0, false
	}
	var le, be int
	for i := 0; i+1 < len(s); i += 2 {
		if s[i] != 0 && s[i+1] == 0 {
			le++
		} else if s[i] == 0 && s[i+1] != 0 {
			be++
		}
	}
	switch {
	case le*10 >= pairs*4 && be*10 < pairs:
		return EncUTF16LE, true
	case be*10 >= pairs*4 && le*10 < pairs:
		return EncUTF16BE, true
	}
	return 0, false
}

// looksBinary reports NUL bytes or a high share of control characters.
func looksBinary(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	ctrl := 0
	for _, c := range s {
		if c == 0 {
			return true
		}
		if c < 0x20 {
			switch c {
			case '\t', '\n', '\r', '\f', '\v', '\b', 0x1a, 0x1b:
			default:
				ctrl++
			}
		}
	}
	return ctrl*10 > len(s)
}

// IsBinary reports whether data should be compared in binary mode.
func IsBinary(data []byte) bool {
	return Detect(data, DetectOptions{}).Binary
}

// Decode converts data in the given encoding to a Go (UTF-8) string. A
// leading BOM of a Unicode encoding is removed. UTF-8 data is returned
// unchanged byte for byte (invalid sequences survive a round trip).
func Decode(data []byte, e Encoding) (string, error) {
	if e == EncAuto {
		e = Detect(data, DetectOptions{}).Encoding
	}
	if bom := e.BOM(); bom != nil && bytes.HasPrefix(data, bom) {
		data = data[len(bom):]
	}
	if e == EncUTF8 {
		return string(data), nil
	}
	x := e.xenc()
	if x == nil {
		return "", fmt.Errorf("compare: cannot decode %v", e)
	}
	if cm, ok := x.(*charmap.Charmap); ok {
		return decodeSingleByte(cm, data), nil
	}
	out, err := x.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ErrUnencodable is returned (wrapped) by Encode when the text contains a
// character the target code page cannot represent.
var ErrUnencodable = errors.New("compare: text contains characters not representable in the encoding")

// Encode converts s to the given encoding, optionally prefixed by its BOM.
func Encode(s string, e Encoding, withBOM bool) ([]byte, error) {
	var out []byte
	if withBOM {
		out = append(out, e.BOM()...)
	}
	if e == EncUTF8 || e == EncAuto {
		return append(out, s...), nil
	}
	x := e.xenc()
	if x == nil {
		return nil, fmt.Errorf("compare: cannot encode %v", e)
	}
	if cm, ok := x.(*charmap.Charmap); ok {
		b, err := encodeSingleByte(cm, s, e)
		if err != nil {
			return nil, err
		}
		return append(out, b...), nil
	}
	b, err := x.NewEncoder().Bytes([]byte(s))
	if err != nil {
		return nil, fmt.Errorf("%w (%v): %v", ErrUnencodable, e, err)
	}
	return append(out, b...), nil
}

// undefinedBase is where bytes that a single-byte code page leaves undefined
// are parked in the Unicode private-use area: byte b decodes to
// undefinedBase+b and encodes back to b.
const undefinedBase = 0xF700

// decodeSingleByte decodes data byte by byte, keeping undefined bytes as
// private-use characters so they survive a save.
func decodeSingleByte(cm *charmap.Charmap, data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	for _, c := range data {
		r := cm.DecodeByte(c)
		if r == utf8.RuneError {
			r = undefinedBase + rune(c)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// encodeSingleByte is the inverse of decodeSingleByte. A character the code
// page cannot represent gives ErrUnencodable.
func encodeSingleByte(cm *charmap.Charmap, s string, e Encoding) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r >= undefinedBase && r <= undefinedBase+0xFF {
			out = append(out, byte(r-undefinedBase))
			continue
		}
		if c, ok := cm.EncodeRune(r); ok {
			out = append(out, c)
			continue
		}
		return nil, fmt.Errorf("%w (%v): %q", ErrUnencodable, e, r)
	}
	return out, nil
}
