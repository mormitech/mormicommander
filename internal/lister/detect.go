// Automatic encoding and file-kind detection. When Total Commander's Lister
// opens a file it guesses how to show it: a byte order mark settles the
// question at once, otherwise it checks whether the text is valid UTF-8,
// whether it looks like UTF-16 without a mark (every other byte zero), and
// falls back to the system's ANSI code page. We do the same on a sample from
// the start of the file, and for Cyrillic systems we also pick between the
// three common Russian code pages (Windows-1251, KOI8-R and DOS 866) by
// counting which one turns the bytes into the most frequent Russian letters.
// SuggestView additionally decides whether a file should open as text,
// binary, image/media, HTML or RTF, which is what the Lister does when
// "auto-detect" view is on.

package lister

import (
	"bytes"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SampleSize is how much of a file detection looks at. It is large enough
// to see past long headers but small enough to read instantly from any disk.
const SampleSize = 64 * 1024

// Detection is the result of looking at a file's first bytes.
type Detection struct {
	Encoding Encoding // the best guess
	BOMLen   int      // length of a byte order mark at offset 0 (0 if none)
	Binary   bool     // the data does not look like text at all
}

// BOM returns the encoding announced by a byte order mark at the start of
// b, and the mark's length. It returns ("", 0) when there is no mark.
func BOM(b []byte) (Encoding, int) {
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return UTF8, 3
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return UTF16LE, 2
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return UTF16BE, 2
	}
	return "", 0
}

// Detect guesses the encoding of sample (the first bytes of a file). ansi is
// the fallback 8-bit code page, normally DefaultANSI().
func Detect(sample []byte, ansi Encoding) Detection {
	if !ansi.Valid() {
		ansi = Win1252
	}
	if e, n := BOM(sample); n > 0 {
		return Detection{Encoding: e, BOMLen: n}
	}
	if e, ok := guessUTF16(sample); ok {
		return Detection{Encoding: e}
	}
	if LooksBinary(sample) {
		return Detection{Encoding: ansi, Binary: true}
	}
	ascii := true
	for _, c := range sample {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii || validUTF8Prefix(sample) {
		return Detection{Encoding: UTF8}
	}
	if isCyrillicFamily(ansi) {
		return Detection{Encoding: bestCyrillic(sample, ansi)}
	}
	return Detection{Encoding: ansi}
}

// validUTF8Prefix accepts b as UTF-8 even if the sample cut the last
// multi-byte character in half.
func validUTF8Prefix(b []byte) bool {
	if utf8.Valid(b) {
		return true
	}
	// Drop up to 3 trailing bytes that could be a cut-off sequence.
	for cut := 1; cut <= 3 && cut < len(b); cut++ {
		c := b[len(b)-cut]
		if c >= 0xC0 { // a lead byte: everything after it may be incomplete
			return utf8.Valid(b[:len(b)-cut])
		}
		if c < 0x80 {
			break
		}
	}
	return false
}

// guessUTF16 recognises UTF-16 text without a byte order mark: in Latin text
// almost every second byte is zero.
func guessUTF16(b []byte) (Encoding, bool) {
	n := len(b) &^ 1
	if n < 4 {
		return "", false
	}
	textish := func(c byte) bool { return c >= 0x20 && c < 0x7F || c == '\t' || c == '\n' || c == '\r' }
	var zeroEven, zeroOdd, textEven, textOdd int
	for i := 0; i < n; i += 2 {
		if b[i] == 0 {
			zeroEven++
			if textish(b[i+1]) {
				textEven++
			}
		}
		if b[i+1] == 0 {
			zeroOdd++
			if textish(b[i]) {
				textOdd++
			}
		}
	}
	// The zero bytes must sit next to printable ASCII, otherwise this is
	// just binary data with many zeros.
	pairs := n / 2
	switch {
	case zeroOdd*10 >= pairs*4 && zeroEven*20 <= pairs && textOdd*10 >= zeroOdd*9:
		return UTF16LE, true
	case zeroEven*10 >= pairs*4 && zeroOdd*20 <= pairs && textEven*10 >= zeroEven*9:
		return UTF16BE, true
	}
	return "", false
}

// LooksBinary reports whether b is unlikely to be text: it contains NUL
// bytes, or more than one in ten bytes are control characters other than
// tab, newline, carriage return, form feed and escape.
func LooksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	ctrl := 0
	for _, c := range b {
		switch {
		case c == 0:
			return true
		case c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' && c != 0x1B && c != 0x1A:
			ctrl++
		}
	}
	return ctrl*10 > len(b)
}

func isCyrillicFamily(e Encoding) bool {
	switch e {
	case Win1251, KOI8R, KOI8U, DOS866, DOS855, ISO8859_5, MacCyrillic:
		return true
	}
	return false
}

// bestCyrillic picks the Russian code page that makes the sample read most
// like real text: lowercase letters dominate natural text, and "оеаинтср"
// are the most frequent of them.
func bestCyrillic(b []byte, fallback Encoding) Encoding {
	const frequent = "оеаинтсрвл"
	best, bestScore := fallback, -1<<31
	for _, e := range []Encoding{fallback, Win1251, KOI8R, DOS866} {
		t := tableCache[e]
		score := 0
		for _, c := range b {
			if c < 0x80 {
				continue
			}
			r := t[c]
			switch {
			case strings.ContainsRune(frequent, r):
				score += 3
			case unicode.Is(unicode.Cyrillic, r) && unicode.IsLower(r):
				score++
			case unicode.Is(unicode.Cyrillic, r) && unicode.IsUpper(r):
				score--
			default:
				score -= 2
			}
		}
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	return best
}

// ViewKind is the Lister mode a file should open in.
type ViewKind int

const (
	ViewText     ViewKind = iota // plain text (key 1)
	ViewBinary                   // fixed-width binary (key 2)
	ViewHex                      // hex dump (key 3)
	ViewMedia                    // image/audio/video/PDF shown by the web view (key 4)
	ViewHTML                     // HTML: rendered, or as text via HTMLToText
	ViewRTF                      // RTF: shown as text via RTFToText
	ViewMarkdown                 // Markdown: rendered by the web view
)

func (k ViewKind) String() string {
	return [...]string{"text", "binary", "hex", "media", "html", "rtf", "markdown"}[k]
}

var mediaExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".jpe": true, ".gif": true, ".bmp": true, ".webp": true,
	".tif": true, ".tiff": true, ".ico": true, ".cur": true, ".heic": true, ".heif": true, ".avif": true,
	".svg": true, ".psd": true, ".pdf": true, ".mp3": true, ".wav": true, ".flac": true, ".ogg": true,
	".oga": true, ".opus": true, ".m4a": true, ".aac": true, ".mp4": true, ".m4v": true, ".mov": true,
	".avi": true, ".mkv": true, ".webm": true,
}

// SuggestView decides how a file should first be shown, from its name and
// the first bytes (as Total Commander does with "auto-detect" view). For
// binary files that are not a known media type it returns ViewBinary; Total
// Commander still opens those in text mode by default, so the UI decides
// whether to honour the suggestion (an option in its settings).
func SuggestView(name string, sample []byte) ViewKind {
	if _, ok := sniffMedia(sample); ok {
		return ViewMedia
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".htm", ".html", ".xhtml":
		return ViewHTML
	case ".rtf":
		return ViewRTF
	case ".md", ".markdown":
		return ViewMarkdown
	}
	if bytes.HasPrefix(sample, []byte(`{\rtf`)) {
		return ViewRTF
	}
	if mediaExt[ext] && LooksBinary(sample) {
		return ViewMedia
	}
	head := bytes.ToLower(sample[:min(len(sample), 512)])
	head = bytes.TrimLeft(head, "\xef\xbb\xbf \t\r\n")
	if bytes.HasPrefix(head, []byte("<!doctype html")) || bytes.HasPrefix(head, []byte("<html")) {
		return ViewHTML
	}
	if _, n := BOM(sample); n == 0 {
		if _, ok := guessUTF16(sample); !ok && LooksBinary(sample) {
			return ViewBinary
		}
	}
	return ViewText
}
