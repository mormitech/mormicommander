// This file loads and writes plain lists of names, one per line. Total
// Commander can take the new names from a text file ("Load names from
// file") or from the clipboard, and our F3 "edit names in editor" feature
// writes the current new names to a temporary file, lets the user edit them
// in any editor and reads them back. Such files come from many sources, so
// ParseNames accepts UTF-8 with or without a byte-order mark, UTF-16 in
// either byte order when it starts with a BOM (Windows Notepad's "Unicode"),
// and falls back to Latin-1 for bytes that are not valid UTF-8 rather than
// producing replacement characters. Both LF and CRLF line ends work, and a
// final line break does not add an empty name.

package multirename

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ParseNames splits a name list into lines.
func ParseNames(data []byte) []string {
	text := decodeText(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

// FormatNames joins names into a list file. crlf selects Windows line ends,
// which old Windows editors need.
func FormatNames(names []string, crlf bool) []byte {
	sep := "\n"
	if crlf {
		sep = "\r\n"
	}
	if len(names) == 0 {
		return nil
	}
	return []byte(strings.Join(names, sep) + sep)
}

// NewNamesOf returns the new names of a preview, ready for FormatNames.
func NewNamesOf(res *Result) []string {
	out := make([]string, len(res.Entries))
	for i, e := range res.Entries {
		out[i] = e.NewName
	}
	return out
}

// decodeText turns bytes of an unknown text encoding into a string.
func decodeText(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		b = b[3:]
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return decodeUTF16(b[2:], false)
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return decodeUTF16(b[2:], true)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}

func decodeUTF16(b []byte, bigEndian bool) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		if bigEndian {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(u))
}
