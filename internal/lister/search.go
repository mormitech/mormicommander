// Searching in the Lister (F7, then F3 / Shift+F3 for next and previous).
// Total Commander's search box offers plain text, case sensitivity, whole
// words, hexadecimal byte patterns, regular expressions and searching
// backwards, and it works on files of any size. We search in windows of a
// few megabytes that overlap by the longest possible match, so nothing is
// missed at window edges and memory stays bounded. Text is decoded in the
// file's encoding before matching (so "árvíztűrő" is found in a UTF-16 or
// Windows-1250 file alike), and every match is mapped back to the exact byte
// offset and byte length the UI must highlight. Hex patterns match raw
// bytes. Regular expressions use Go's RE2 syntax; a single regex match is
// limited to 64 KiB, which is plenty for an interactive viewer.

package lister

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"mormicommander/internal/rxflags"
)

// Query describes one Lister search.
type Query struct {
	Pattern       string
	Hex           bool // Pattern is hex bytes, e.g. "4D 5A 90" or "4d5a90"
	Regex         bool // Pattern is a regular expression (RE2 syntax)
	CaseSensitive bool
	WholeWords    bool
	Backward      bool     // find the previous match instead of the next
	Encoding      Encoding // text encoding of the data; "" means UTF-8
}

// Match is a found occurrence, in bytes.
type Match struct {
	Offset int64
	Length int64
}

// End returns the offset just after the match.
func (m Match) End() int64 { return m.Offset + m.Length }

// ErrEmptyPattern is returned for an empty search text.
var ErrEmptyPattern = errors.New("lister: empty search pattern")

const (
	searchWindow    = 4 << 20
	regexMaxMatch   = 64 * 1024
	searchLeadBytes = 8
)

// Searcher is a compiled Query. It can be reused for repeated F3 presses
// and is safe for concurrent use (each Find allocates its own buffers).
type Searcher struct {
	q       Query
	raw     []byte         // hex pattern
	text    string         // folded plain pattern
	re      *regexp.Regexp // regex pattern
	overlap int
	// Progress, if set, is called with the current position during long
	// searches so the UI can show a progress bar and offer to cancel.
	Progress func(pos, size int64)
}

// Compile validates and prepares q.
func Compile(q Query) (*Searcher, error) {
	if q.Pattern == "" {
		return nil, ErrEmptyPattern
	}
	if !q.Encoding.Valid() {
		q.Encoding = UTF8
	}
	s := &Searcher{q: q}
	switch {
	case q.Hex:
		b, err := ParseHex(q.Pattern)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			return nil, ErrEmptyPattern
		}
		s.raw = b
		s.overlap = len(b)
	case q.Regex:
		flags := "(?m)"
		if !q.CaseSensitive {
			flags = "(?i)" + flags
		}
		re, err := rxflags.Compile(flags, q.Pattern)
		if err != nil {
			return nil, err
		}
		s.re = re
		s.overlap = regexMaxMatch
	default:
		s.text = q.Pattern
		if !q.CaseSensitive {
			s.text = foldString(q.Pattern)
		}
		s.overlap = 4*len(q.Pattern) + 8
	}
	return s, nil
}

// ParseHex reads a hex byte pattern. Spaces, commas, dashes and an optional
// "0x" prefix per byte are ignored, so "4D 5A", "4d5a" and "0x4D,0x5A" all
// give the same two bytes.
func ParseHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(strings.ToLower(s), "0x", "")
	var clean strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == ',' || r == '-' || r == '\t' || r == ':':
		default:
			clean.WriteRune(r)
		}
	}
	return hex.DecodeString(clean.String())
}

func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			return r + 32
		}
		return r
	}
	return unicode.ToLower(unicode.ToUpper(r))
}

func foldString(s string) string {
	var sb strings.Builder
	for _, r := range s {
		sb.WriteRune(foldRune(r))
	}
	return sb.String()
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

// Find searches src for the next match starting at or after from (or, for
// a backward query, the last match starting before from). dataStart is the
// first byte of text (after a byte order mark). found is false when there
// is no further match.
func (s *Searcher) Find(ctx context.Context, src Source, from, dataStart int64) (m Match, found bool, err error) {
	size := src.Size()
	if from < dataStart {
		from = dataStart
	}
	if s.q.Backward {
		for hi := min(from, size); hi > dataStart; {
			if err := ctx.Err(); err != nil {
				return Match{}, false, err
			}
			lo := max(dataStart, hi-searchWindow)
			all, err := s.window(src, lo, hi, dataStart)
			if err != nil {
				return Match{}, false, err
			}
			if len(all) > 0 {
				return all[len(all)-1], true, nil
			}
			if s.Progress != nil {
				s.Progress(lo, size)
			}
			hi = lo
		}
		return Match{}, false, nil
	}
	for lo := from; lo < size; {
		if err := ctx.Err(); err != nil {
			return Match{}, false, err
		}
		hi := min(size, lo+searchWindow)
		all, err := s.window(src, lo, hi, dataStart)
		if err != nil {
			return Match{}, false, err
		}
		if len(all) > 0 {
			return all[0], true, nil
		}
		if s.Progress != nil {
			s.Progress(hi, size)
		}
		lo = hi
	}
	return Match{}, false, nil
}

// FindAll returns every match in [from, to) in order, up to limit matches
// (0 = no limit). It is meant for highlighting all matches on screen.
func (s *Searcher) FindAll(ctx context.Context, src Source, from, to, dataStart int64, limit int) ([]Match, error) {
	var out []Match
	to = min(to, src.Size())
	from = max(from, dataStart)
	for lo := from; lo < to; {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		hi := min(to, lo+searchWindow)
		all, err := s.window(src, lo, hi, dataStart)
		if err != nil {
			return out, err
		}
		for _, m := range all {
			if len(out) > 0 && m.Offset < out[len(out)-1].End() {
				continue // keep highlights non-overlapping
			}
			out = append(out, m)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
		lo = hi
	}
	return out, nil
}

// window returns all matches whose start lies in [lo, hi), in order.
func (s *Searcher) window(src Source, lo, hi, dataStart int64) ([]Match, error) {
	size := src.Size()
	readLo := max(dataStart, lo-searchLeadBytes)
	if s.q.Encoding.IsUTF16() {
		readLo -= (readLo - dataStart) % 2
	}
	readHi := min(size, hi+int64(s.overlap))
	buf := make([]byte, readHi-readLo)
	n, err := readFull(src, buf, readLo)
	if err != nil {
		return nil, err
	}
	buf = buf[:n]
	if s.raw != nil {
		var out []Match
		for i := 0; ; {
			j := bytes.Index(buf[i:], s.raw)
			if j < 0 {
				break
			}
			at := readLo + int64(i+j)
			if at >= hi {
				break
			}
			if at >= lo {
				out = append(out, Match{at, int64(len(s.raw))})
			}
			i += j + 1
		}
		return out, nil
	}
	if entryFor(s.q.Encoding).kind == kindMulti && readLo > dataStart {
		// Double-byte encodings: start decoding at a byte that can never be
		// the second half of a character (all trail bytes are >= 0x40).
		for k := 0; k < len(buf) && k < searchLeadBytes; k++ {
			if buf[k] < 0x40 {
				buf = buf[k:]
				readLo += int64(k)
				break
			}
		}
	}
	text, offs := decodeForSearch(s.q.Encoding, buf, s.re == nil && !s.q.CaseSensitive)
	fileOff := func(i int) int64 { return readLo + int64(offs[i]) }
	okWord := func(i, j int) bool {
		if !s.q.WholeWords {
			return true
		}
		if i > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:i])
			if isWordRune(r) {
				return false
			}
		}
		if j < len(text) {
			r, _ := utf8.DecodeRuneInString(text[j:])
			if isWordRune(r) {
				return false
			}
		}
		return true
	}
	var out []Match
	add := func(i, j int) bool {
		at := fileOff(i)
		if at >= hi {
			return false
		}
		if at >= lo && j > i && okWord(i, j) {
			out = append(out, Match{at, fileOff(j) - at})
		}
		return true
	}
	if s.re != nil {
		for _, loc := range s.re.FindAllStringIndex(text, -1) {
			if !add(loc[0], loc[1]) {
				break
			}
		}
		return out, nil
	}
	for i := 0; i < len(text); {
		j := strings.Index(text[i:], s.text)
		if j < 0 {
			break
		}
		if !add(i+j, i+j+len(s.text)) {
			break
		}
		_, sz := utf8.DecodeRuneInString(text[i+j:])
		i += j + max(sz, 1)
	}
	return out, nil
}

// decodeForSearch decodes buf into a UTF-8 string, optionally case-folded,
// and returns for every byte of that string (plus one past the end) the
// offset in buf of the character it came from.
func decodeForSearch(e Encoding, buf []byte, fold bool) (string, []int32) {
	d := newRuneDecoder(e)
	var sb strings.Builder
	sb.Grow(len(buf))
	offs := make([]int32, 0, len(buf)+1)
	var tmp [utf8.UTFMax]byte
	for pos := 0; pos < len(buf); {
		r, n := d.decode(buf[pos:])
		if n <= 0 {
			n = 1
		}
		if fold {
			r = foldRune(r)
		}
		w := utf8.EncodeRune(tmp[:], r)
		sb.Write(tmp[:w])
		for k := 0; k < w; k++ {
			offs = append(offs, int32(pos))
		}
		pos += n
	}
	offs = append(offs, int32(len(buf)))
	return sb.String(), offs
}

// Search is a convenience wrapper: it compiles q with the viewer's
// encoding and searches from the given offset.
func (v *Viewer) Search(ctx context.Context, q Query, from int64) (Match, bool, error) {
	if q.Encoding == "" {
		q.Encoding = v.Encoding()
	}
	s, err := Compile(q)
	if err != nil {
		return Match{}, false, err
	}
	return s.Find(ctx, v.src, from, v.DataStart())
}
