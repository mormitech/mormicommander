// Text encodings for settings files. Total Commander has written wincmd.ini in
// several encodings over the years: old versions used the Windows "ANSI" code
// page of the machine (Windows-1252 in Western Europe, Windows-1250 in Hungary
// and the rest of Central Europe), and newer versions switch the file to
// UTF-16 little endian with a byte order mark as soon as a value contains a
// character the code page cannot hold. Our own files are plain UTF-8. To import
// a real wincmd.ini and to write it back without damage we detect the encoding
// when reading, remember it, and use the same one when saving. The two code
// page tables are small and fixed, so they live here instead of pulling in
// golang.org/x/text.

package config

import (
	"bytes"
	"encoding/binary"
	"errors"
	"unicode/utf16"
	"unicode/utf8"
)

// Encoding says how the bytes of a settings file map to text.
type Encoding int

const (
	// EncUTF8 is UTF-8 without a byte order mark; the default for new files.
	EncUTF8 Encoding = iota
	// EncUTF8BOM is UTF-8 that starts with EF BB BF.
	EncUTF8BOM
	// EncUTF16LE is UTF-16 little endian with a FF FE mark (TC's Unicode ini).
	EncUTF16LE
	// EncUTF16BE is UTF-16 big endian with a FE FF mark.
	EncUTF16BE
	// EncCP1252 is the Western European Windows code page.
	EncCP1252
	// EncCP1250 is the Central European Windows code page (Hungarian TC installs).
	EncCP1250
)

// String returns a short human-readable name of the encoding.
func (e Encoding) String() string {
	switch e {
	case EncUTF8:
		return "UTF-8"
	case EncUTF8BOM:
		return "UTF-8 BOM"
	case EncUTF16LE:
		return "UTF-16LE"
	case EncUTF16BE:
		return "UTF-16BE"
	case EncCP1252:
		return "Windows-1252"
	case EncCP1250:
		return "Windows-1250"
	}
	return "unknown"
}

// ErrUnencodable is returned when text can't be written in a code page.
var ErrUnencodable = errors.New("config: text not representable in code page")

// cp1250 and cp1252 map the bytes 0x80..0xFF to Unicode (0xFFFD = unused).
var cp1250 = [128]rune{
	0x20AC, 0xFFFD, 0x201A, 0xFFFD, 0x201E, 0x2026, 0x2020, 0x2021,
	0xFFFD, 0x2030, 0x0160, 0x2039, 0x015A, 0x0164, 0x017D, 0x0179,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0xFFFD, 0x2122, 0x0161, 0x203A, 0x015B, 0x0165, 0x017E, 0x017A,
	0x00A0, 0x02C7, 0x02D8, 0x0141, 0x00A4, 0x0104, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x015E, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x017B,
	0x00B0, 0x00B1, 0x02DB, 0x0142, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x0105, 0x015F, 0x00BB, 0x013D, 0x02DD, 0x013E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7,
	0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7,
	0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7,
	0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7,
	0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}

var cp1252 = [128]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178,
	0x00A0, 0x00A1, 0x00A2, 0x00A3, 0x00A4, 0x00A5, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x00AA, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x00AF,
	0x00B0, 0x00B1, 0x00B2, 0x00B3, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x00B9, 0x00BA, 0x00BB, 0x00BC, 0x00BD, 0x00BE, 0x00BF,
	0x00C0, 0x00C1, 0x00C2, 0x00C3, 0x00C4, 0x00C5, 0x00C6, 0x00C7,
	0x00C8, 0x00C9, 0x00CA, 0x00CB, 0x00CC, 0x00CD, 0x00CE, 0x00CF,
	0x00D0, 0x00D1, 0x00D2, 0x00D3, 0x00D4, 0x00D5, 0x00D6, 0x00D7,
	0x00D8, 0x00D9, 0x00DA, 0x00DB, 0x00DC, 0x00DD, 0x00DE, 0x00DF,
	0x00E0, 0x00E1, 0x00E2, 0x00E3, 0x00E4, 0x00E5, 0x00E6, 0x00E7,
	0x00E8, 0x00E9, 0x00EA, 0x00EB, 0x00EC, 0x00ED, 0x00EE, 0x00EF,
	0x00F0, 0x00F1, 0x00F2, 0x00F3, 0x00F4, 0x00F5, 0x00F6, 0x00F7,
	0x00F8, 0x00F9, 0x00FA, 0x00FB, 0x00FC, 0x00FD, 0x00FE, 0x00FF,
}

func codepageTable(e Encoding) *[128]rune {
	if e == EncCP1250 {
		return &cp1250
	}
	return &cp1252
}

// Decode turns raw file bytes into text. A byte order mark wins; otherwise
// valid UTF-8 is taken as UTF-8 and anything else as the fallback code page
// (EncCP1252 or EncCP1250). It returns the text and the detected encoding.
func Decode(data []byte, fallback Encoding) (string, Encoding) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:]), EncUTF8BOM
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeUTF16(data[2:], binary.LittleEndian), EncUTF16LE
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data[2:], binary.BigEndian), EncUTF16BE
	}
	if utf8.Valid(data) {
		return string(data), EncUTF8
	}
	if fallback != EncCP1250 {
		fallback = EncCP1252
	}
	tbl := codepageTable(fallback)
	var b []rune
	for _, c := range data {
		if c < 0x80 {
			b = append(b, rune(c))
		} else {
			b = append(b, tbl[c-0x80])
		}
	}
	return string(b), fallback
}

func decodeUTF16(data []byte, order binary.ByteOrder) string {
	u := make([]uint16, len(data)/2)
	for i := range u {
		u[i] = order.Uint16(data[2*i:])
	}
	return string(utf16.Decode(u))
}

// Encode turns text into bytes in the given encoding (with a byte order mark
// where the encoding has one). For the code pages it returns ErrUnencodable
// if a character has no byte; callers usually fall back to UTF-16LE then.
func Encode(text string, enc Encoding) ([]byte, error) {
	switch enc {
	case EncUTF8BOM:
		return append([]byte{0xEF, 0xBB, 0xBF}, text...), nil
	case EncUTF16LE, EncUTF16BE:
		u := utf16.Encode([]rune(text))
		out := make([]byte, 2+2*len(u))
		var order binary.ByteOrder = binary.LittleEndian
		copy(out, []byte{0xFF, 0xFE})
		if enc == EncUTF16BE {
			order = binary.BigEndian
			copy(out, []byte{0xFE, 0xFF})
		}
		for i, c := range u {
			order.PutUint16(out[2+2*i:], c)
		}
		return out, nil
	case EncCP1250, EncCP1252:
		tbl := codepageTable(enc)
		out := make([]byte, 0, len(text))
		for _, r := range text {
			if r < 0x80 {
				out = append(out, byte(r))
				continue
			}
			found := false
			for i, t := range tbl {
				if t == r && t != 0xFFFD {
					out = append(out, byte(0x80+i))
					found = true
					break
				}
			}
			if !found {
				return nil, ErrUnencodable
			}
		}
		return out, nil
	}
	return []byte(text), nil
}
