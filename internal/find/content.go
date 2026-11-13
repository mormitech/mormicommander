// This file is the "Find text" part of Find Files (F-529): does a file contain a piece
// of text? It supports Total Commander's options: case-sensitive, whole words only,
// regular expressions, hex byte strings, "NOT containing" (files that lack the text),
// and the encoding choices from encoding.go. Files are read in 1 MiB chunks so a huge
// file never has to fit in memory, and the search stops at the first hit. Between
// chunks we keep a tail of the previous text, long enough that a match cut by the chunk
// boundary is still seen whole, and we remember the character before that tail so the
// whole-word test is exact there too. A regular expression match can be at most the
// tail length (64 KiB) long to be found across a boundary; matches inside one chunk can
// be any length.

package find

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// chunkSize is how much of a file is read at a time. It is a variable only so tests
// can shrink it and exercise the chunk-boundary code with small files.
var chunkSize = 1 << 20

// regexOverlap is the tail kept between chunks for regular expressions.
var regexOverlap = 64 << 10

// ContentOptions describes the "Find text" settings.
type ContentOptions struct {
	Text          string   `json:"text,omitempty"`          // what to look for; empty means no content filter
	CaseSensitive bool     `json:"caseSensitive,omitempty"` // TC "Case sensitive"
	WholeWords    bool     `json:"wholeWords,omitempty"`    // TC "Only whole words"
	Regex         bool     `json:"regex,omitempty"`         // Text is a Go (RE2) regular expression
	Hex           bool     `json:"hex,omitempty"`           // Text is hex bytes, e.g. "4D 5A 90 00"
	NotContaining bool     `json:"notContaining,omitempty"` // TC "Find files NOT containing the text"
	Encodings     Encoding `json:"encodings,omitempty"`     // zero means DefaultEncodings
	ANSICodepage  string   `json:"ansiCodepage,omitempty"`  // default "windows-1252"
	OEMCodepage   string   `json:"oemCodepage,omitempty"`   // default "cp437"
}

// Active reports whether a content filter is set at all.
func (c ContentOptions) Active() bool { return c.Text != "" }

// contentMatcher is a compiled ContentOptions.
type contentMatcher struct {
	opts    ContentOptions
	newDecs func() []decoder
	lit     string         // literal needle (lower-cased unless CaseSensitive)
	re      *regexp.Regexp // regex needle
	overlap int            // bytes of decoded text kept between chunks
	fold    bool           // lower-case the haystack before searching
	wordL   bool           // check the boundary before a match
	wordR   bool           // check the boundary after a match
}

// compileContent validates the options and prepares the matcher.
func compileContent(c ContentOptions) (*contentMatcher, error) {
	m := &contentMatcher{opts: c}
	if c.Hex {
		if c.Regex {
			return nil, errors.New("find: hex and regex search cannot be combined")
		}
		b, err := ParseHex(c.Text)
		if err != nil {
			return nil, err
		}
		m.lit = string(b)
		m.overlap = len(b)
		m.newDecs = func() []decoder { return []decoder{rawDecoder{}} }
		return m, nil
	}
	enc := c.Encodings
	if enc == 0 {
		enc = DefaultEncodings
	}
	var ansi, oem *[256]rune
	var err error
	if enc&EncANSI != 0 {
		if ansi, err = codepageTable(c.ANSICodepage, "windows-1252"); err != nil {
			return nil, err
		}
	}
	if enc&EncOEM != 0 {
		if oem, err = codepageTable(c.OEMCodepage, "cp437"); err != nil {
			return nil, err
		}
	}
	m.newDecs = func() []decoder {
		var d []decoder
		if enc&EncANSI != 0 {
			d = append(d, tableDecoder{ansi})
		}
		if enc&EncOEM != 0 {
			d = append(d, tableDecoder{oem})
		}
		if enc&EncUTF8 != 0 {
			d = append(d, &utf8Decoder{})
		}
		if enc&EncUTF16LE != 0 {
			d = append(d, &utf16Decoder{}, &utf16Decoder{skip: 1})
		}
		if enc&EncUTF16BE != 0 {
			d = append(d, &utf16Decoder{bigEndian: true}, &utf16Decoder{bigEndian: true, skip: 1})
		}
		return d
	}
	if c.Regex {
		expr := "(?m)" + c.Text
		if !c.CaseSensitive {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("find: bad regular expression: %w", err)
		}
		m.re = re
		m.overlap = regexOverlap
		m.wordL, m.wordR = c.WholeWords, c.WholeWords
		return m, nil
	}
	m.fold = !c.CaseSensitive
	m.lit = c.Text
	if m.fold {
		m.lit = strings.ToLower(c.Text)
	}
	// Lower-casing can change byte lengths a little (e.g. "İ"), hence the slack.
	m.overlap = 2*len(c.Text) + 16
	if c.WholeWords {
		first, _ := utf8.DecodeRuneInString(c.Text)
		last, _ := utf8.DecodeLastRuneInString(c.Text)
		m.wordL, m.wordR = isWordRune(first), isWordRune(last)
	}
	return m, nil
}

// ParseHex reads a hex byte string such as "4D 5A", "4d5a" or "0x4D,0x5A".
func ParseHex(s string) ([]byte, error) {
	var b strings.Builder
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '-' || r == ':' }) {
		f = strings.TrimPrefix(strings.TrimPrefix(f, "0x"), "0X")
		if len(f)%2 == 1 {
			f = "0" + f
		}
		b.WriteString(f)
	}
	out, err := hex.DecodeString(b.String())
	if err != nil {
		return nil, fmt.Errorf("find: bad hex search string %q: %w", s, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("find: empty hex search string")
	}
	return out, nil
}

// isWordRune is the "word character" test for whole-word matching: letters (in any
// script), digits and the underscore, as in Total Commander.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

// stream is the per-decoder search state for one file.
type stream struct {
	dec  decoder
	tail string
	prev rune // rune before tail; -1 at the start of the file
}

// matchReader reports whether r contains the text in any selected encoding. It does
// not apply NotContaining; the caller inverts.
func (m *contentMatcher) matchReader(ctx context.Context, r io.Reader) (bool, error) {
	decs := m.newDecs()
	streams := make([]*stream, len(decs))
	for i, d := range decs {
		streams[i] = &stream{dec: d, prev: -1}
	}
	buf := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, err := io.ReadFull(r, buf)
		eof := false
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			eof = true
		} else if err != nil {
			return false, err
		}
		for _, s := range streams {
			if m.feed(s, buf[:n], eof) {
				return true, nil
			}
		}
		if eof {
			return false, nil
		}
	}
}

// matchBytes is matchReader for data already in memory.
func (m *contentMatcher) matchBytes(data []byte) bool {
	ok, _ := m.matchReader(context.Background(), bytes.NewReader(data))
	return ok
}

// feed decodes one chunk into a stream and searches it.
func (m *contentMatcher) feed(s *stream, raw []byte, eof bool) bool {
	text := s.tail + s.dec.decode(raw, eof)
	if m.find(text, s.prev, eof) {
		return true
	}
	if eof {
		return false
	}
	// Keep a tail for the next round, cut at a rune start (and for regex, if possible,
	// just after a line break so "^" keeps meaning "start of line").
	cut := len(text) - m.overlap
	if cut <= 0 {
		s.tail = text
		return false
	}
	if m.opts.Hex {
		// Raw bytes: no rune alignment, and no word boundaries to remember.
		s.tail = text[cut:]
		return false
	}
	if m.re != nil {
		if i := strings.IndexByte(text[cut:], '\n'); i >= 0 {
			cut += i + 1
		}
	}
	for cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut++
	}
	s.prev, _ = utf8.DecodeLastRuneInString(text[:cut])
	s.tail = text[cut:]
	return false
}

// find searches one piece of decoded text. prev is the rune before text (-1 at the
// start of the file). A match touching the end of the text while more data follows
// is "not sure yet" when the right boundary matters; it lies inside the tail and will
// be seen again with more context.
func (m *contentMatcher) find(text string, prev rune, eof bool) bool {
	if m.re != nil {
		for _, loc := range m.re.FindAllStringIndex(text, -1) {
			if loc[1] == len(text) && !eof {
				continue
			}
			if m.boundaryOK(text, loc[0], loc[1], prev, eof) {
				return true
			}
		}
		return false
	}
	hay := text
	if m.fold {
		hay = strings.ToLower(text)
		if len(hay) != len(text) {
			// Rare: lower-casing changed byte lengths, so boundary positions must be
			// judged in the folded text itself.
			text = hay
		}
	}
	if m.lit == "" {
		return false
	}
	if !m.wordL && !m.wordR {
		return strings.Contains(hay, m.lit)
	}
	for start := 0; ; {
		i := strings.Index(hay[start:], m.lit)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(m.lit)
		if !(end == len(hay) && !eof && m.wordR) && m.boundaryOK(text, i, end, prev, eof) {
			return true
		}
		_, w := utf8.DecodeRuneInString(hay[i:])
		start = i + max(w, 1)
	}
}

// boundaryOK applies the whole-word test to text[i:end].
func (m *contentMatcher) boundaryOK(text string, i, end int, prev rune, eof bool) bool {
	if m.wordL {
		var before rune
		if i == 0 {
			before = prev
		} else {
			before, _ = utf8.DecodeLastRuneInString(text[:i])
		}
		if before >= 0 && isWordRune(before) {
			return false
		}
	}
	if m.wordR && end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(after) {
			return false
		}
	}
	return true
}
