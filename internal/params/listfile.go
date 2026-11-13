// List files for the %L %F %D (and %WL %UL ...) placeholders. Total
// Commander can pass thousands of selected names to a program by writing
// them, one per line, to a temporary file and passing only that file's name.
// The caller deletes the file once the started program has exited (TC does
// the same), which Result.Cleanup and Launch.Cleanup do. Plain and U lists
// are UTF-8 without a byte-order mark (TC's ANSI lists are replaced by
// UTF-8, the native encoding on every system we run on); W lists are
// UTF-16 little-endian with a BOM, as TC writes them. Lines end in CRLF in
// the Windows style and LF otherwise.
package params

import (
	"os"
	"strings"
	"unicode/utf16"
)

// ListEncoding is the text encoding of a list file.
type ListEncoding int

const (
	// ListUTF8 is UTF-8 without a byte-order mark.
	ListUTF8 ListEncoding = iota
	// ListUTF16 is UTF-16 little-endian with a byte-order mark.
	ListUTF16
)

// EncodeList renders lines in the given encoding and line ending.
func EncodeList(lines []string, enc ListEncoding, crlf bool) []byte {
	eol := "\n"
	if crlf {
		eol = "\r\n"
	}
	var s strings.Builder
	for _, l := range lines {
		s.WriteString(l)
		s.WriteString(eol)
	}
	if enc != ListUTF16 {
		return []byte(s.String())
	}
	u := utf16.Encode([]rune(s.String()))
	out := make([]byte, 2, 2+2*len(u))
	out[0], out[1] = 0xFF, 0xFE
	for _, r := range u {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

// WriteListFile writes lines to a new temporary file in dir (os.TempDir
// when empty) and returns its path.
func WriteListFile(dir string, lines []string, enc ListEncoding, crlf bool) (string, error) {
	f, err := os.CreateTemp(dir, "mcmd*.lst")
	if err != nil {
		return "", err
	}
	_, err = f.Write(EncodeList(lines, enc, crlf))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
