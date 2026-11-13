// The Lister's view model. A Viewer turns a Source into screen rows in one of
// Total Commander's three byte-based modes: text (key 1), with optional word
// wrap (W) and tab expansion; binary (key 2), fixed-width rows of raw bytes
// drawn as characters; and hex (key 3), offset + hex bytes + character
// column. Everything is addressed by byte offset, never by row number: the UI
// keeps "the offset of the top row", asks for the next screenful with Rows,
// scrolls with Forward/Back, jumps with RowStart (go to offset or percent)
// and LineIndex (go to line). That is how a 10 GB file opens instantly and
// scrolls smoothly: only the bytes of the visible rows, plus at most one long
// line around them, are ever read. Lines longer than MaxLineBytes (binary
// junk shown as text) are split there, like Total Commander does.

package lister

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"
)

// Mode is one of the Lister's byte-based display modes.
type Mode int

const (
	ModeText   Mode = iota // key 1
	ModeBinary             // key 2
	ModeHex                // key 3
)

func (m Mode) String() string {
	switch m {
	case ModeBinary:
		return "binary"
	case ModeHex:
		return "hex"
	}
	return "text"
}

// MaxLineBytes is the size of the pieces that over-long lines are cut
// into, so a file without line breaks still scrolls instantly. The cuts sit
// at fixed positions (multiples of MaxLineBytes from the start of the text,
// moved to the next character boundary), and the first cut of a line is at
// least half a piece after the line start. Fixed positions are what make
// scrolling up through such a line land on exactly the same rows as
// scrolling down, without ever searching for the line's real start.
const MaxLineBytes = 32 * 1024

// pieceEnd returns the offset where the piece of a line starting at s is
// forcibly cut (before character alignment).
func (v *Viewer) pieceEndLocked(s int64) int64 {
	const m = MaxLineBytes
	k := (s + m/2 - v.dataStart + m - 1) / m
	return v.dataStart + k*m
}

// alignForwardLocked moves a cut position to the next character start.
func (v *Viewer) alignForwardLocked(off int64) int64 {
	if v.enc != UTF8 {
		return off
	}
	var b [3]byte
	n, _ := readFull(v.src, b[:], off)
	for i := 0; i < n && b[i]&0xC0 == 0x80; i++ {
		off++
	}
	return off
}

// Options configures a Viewer. Zero values pick Total Commander's defaults.
type Options struct {
	Mode     Mode
	Encoding Encoding // "" = detect from the file
	ANSI     Encoding // fallback 8-bit code page for detection and binary mode; "" = DefaultANSI()
	Wrap     bool     // text mode: wrap long lines at Width columns
	Width    int      // text mode: wrap column (default 80); binary mode: bytes per row (default 80)
	TabWidth int      // text mode: tab stop distance (default 8)
	HexWidth int      // hex mode: bytes per row (default 16)
}

func (o Options) withDefaults() Options {
	if o.Width <= 0 {
		o.Width = 80
	}
	if o.TabWidth <= 0 {
		o.TabWidth = 8
	}
	if o.HexWidth <= 0 {
		o.HexWidth = 16
	}
	if !o.ANSI.Valid() {
		o.ANSI = DefaultANSI()
	}
	return o
}

// Row is one screen row.
type Row struct {
	Offset int64  // first byte of the row
	End    int64  // first byte after the row (the last row of a text line includes its line break)
	Text   string // what to draw: tabs expanded, control characters replaced
	// Map gives, for every rune of Text, its byte offset relative to Offset,
	// so the UI can place search highlights and selections exactly. A tab
	// expands to several spaces that all map to the tab's offset.
	Map []int32
	// LineStart is true when the row begins a logical line (false for the
	// continuation rows of a wrapped line).
	LineStart bool
	// Hex mode only: the hex byte column, e.g. "48 65 6C 6C 6F 20 57 6F  72 6C 64".
	Hex string
}

// Viewer lays out a Source for display. It is safe for concurrent use.
type Viewer struct {
	src Source

	mu        sync.Mutex
	opt       Options
	enc       Encoding
	detected  Detection
	dataStart int64 // first byte after a byte order mark that matches enc
	dec       runeDecoder
	glyphs    *[256]rune // binary mode glyph table
	hexChars  *[256]rune // hex mode character column
	lines     *LineIndex
}

// NewViewer prepares src for display. It reads only the first SampleSize
// bytes (for encoding detection).
func NewViewer(src Source, opt Options) (*Viewer, error) {
	v := &Viewer{src: src}
	sample := make([]byte, SampleSize)
	n, err := readFull(src, sample, 0)
	if err != nil {
		return nil, err
	}
	opt = opt.withDefaults()
	v.detected = Detect(sample[:n], opt.ANSI)
	v.applyLocked(opt)
	return v, nil
}

// OpenViewer opens a file on disk and wraps it in a Viewer. Closing the
// returned File is the caller's job.
func OpenViewer(path string, opt Options) (*Viewer, *File, error) {
	f, err := Open(path)
	if err != nil {
		return nil, nil, err
	}
	v, err := NewViewer(f, opt)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return v, f, nil
}

func (v *Viewer) applyLocked(opt Options) {
	opt = opt.withDefaults()
	enc := opt.Encoding
	if !enc.Valid() {
		enc = v.detected.Encoding
	}
	changedEnc := enc != v.enc || v.lines == nil
	v.opt, v.enc = opt, enc
	v.dataStart = 0
	if be, n := BOM(v.sampleHead()); n > 0 && be == enc {
		v.dataStart = int64(n)
	}
	v.dec = newRuneDecoder(enc)
	v.glyphs = glyphTable(enc, opt.ANSI)
	v.hexChars = hexCharTable(enc, opt.ANSI)
	if changedEnc {
		v.lines = NewLineIndex(v.src, enc, v.dataStart)
	}
}

func (v *Viewer) sampleHead() []byte {
	b := make([]byte, 3)
	n, _ := readFull(v.src, b, 0)
	return b[:n]
}

// SetOptions changes the mode, encoding, wrap or widths. Offsets kept by
// the UI stay valid; pass them through RowStart to re-align the top row.
func (v *Viewer) SetOptions(opt Options) {
	v.mu.Lock()
	v.applyLocked(opt)
	v.mu.Unlock()
}

// Options returns the current options with defaults filled in. Encoding is
// the effective encoding (the detected one when auto-detection is on).
func (v *Viewer) Options() Options {
	v.mu.Lock()
	defer v.mu.Unlock()
	o := v.opt
	if !o.Encoding.Valid() {
		o.Encoding = v.enc
	}
	return o
}

// Encoding returns the encoding in effect.
func (v *Viewer) Encoding() Encoding { v.mu.Lock(); defer v.mu.Unlock(); return v.enc }

// Detection returns what auto-detection found when the file was opened.
func (v *Viewer) Detection() Detection { v.mu.Lock(); defer v.mu.Unlock(); return v.detected }

// DataStart is the first offset shown in text mode (after a byte order mark).
func (v *Viewer) DataStart() int64 { v.mu.Lock(); defer v.mu.Unlock(); return v.dataStart }

// Size returns the size of the data.
func (v *Viewer) Size() int64 { return v.src.Size() }

// Source returns the underlying data source.
func (v *Viewer) Source() Source { return v.src }

// Lines returns the line index for "go to line" and line numbers.
func (v *Viewer) Lines() *LineIndex { v.mu.Lock(); defer v.mu.Unlock(); return v.lines }

// OffsetDigits returns how many hex digits offsets need (at least 8).
func (v *Viewer) OffsetDigits() int {
	d := 8
	for s := v.src.Size() >> 32; s > 0; s >>= 4 {
		d++
	}
	return d
}

// rowWidthLocked is the fixed row length in bytes for binary and hex mode.
func (v *Viewer) rowWidthLocked() int64 {
	if v.opt.Mode == ModeHex {
		return int64(v.opt.HexWidth)
	}
	return int64(v.opt.Width)
}

// RowStart aligns any offset to the start of the row containing it (go to
// offset, go to percent, or re-aligning after an option change).
func (v *Viewer) RowStart(off int64) (int64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.rowStartLocked(off)
}

func (v *Viewer) rowStartLocked(off int64) (int64, error) {
	size := v.src.Size()
	if v.opt.Mode != ModeText {
		if off >= size {
			return size, nil
		}
		if off < 0 {
			off = 0
		}
		w := v.rowWidthLocked()
		return off - off%w, nil
	}
	if off >= size {
		return size, nil
	}
	if off < v.dataStart {
		off = v.dataStart
	}
	rows, idx, _, err := v.rowsAroundLocked(off)
	if err != nil || len(rows) == 0 {
		return off, err
	}
	return rows[idx].Offset, nil
}

// Rows returns up to n rows starting with the row that contains off.
func (v *Viewer) Rows(off int64, n int) ([]Row, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if n <= 0 {
		return nil, nil
	}
	size := v.src.Size()
	if v.opt.Mode != ModeText {
		return v.fixedRowsLocked(off, n)
	}
	if off < v.dataStart {
		off = v.dataStart
	}
	if off >= size {
		return nil, nil
	}
	rows, idx, next, err := v.rowsAroundLocked(off)
	if err != nil {
		return nil, err
	}
	out := append([]Row(nil), rows[idx:]...)
	for len(out) < n && next < size {
		more, nx, err := v.layoutLineLocked(next)
		if err != nil {
			return out, err
		}
		if nx <= next {
			break
		}
		out = append(out, more...)
		next = nx
	}
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// Forward returns the offset of the row n rows below the row containing
// off, stopping at the last row.
func (v *Viewer) Forward(off int64, n int) (int64, error) {
	rows, err := v.Rows(off, n+1)
	if err != nil {
		return off, err
	}
	if len(rows) == 0 {
		return v.RowStart(off)
	}
	if len(rows) > n {
		return rows[n].Offset, nil
	}
	return rows[len(rows)-1].Offset, nil
}

// Back returns the offset of the row n rows above the row containing off.
// Back(Size(), n) is the top row of the last page (the End key).
func (v *Viewer) Back(off int64, n int) (int64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	size := v.src.Size()
	if v.opt.Mode != ModeText {
		w := v.rowWidthLocked()
		var r int64
		if off >= size {
			r = (size + w - 1) / w * w
		} else {
			r = max(off, 0) / w * w
		}
		r -= int64(n) * w
		return max(r, 0), nil
	}
	r, err := v.rowStartLocked(off)
	if err != nil {
		return r, err
	}
	for n > 0 && r > v.dataStart {
		prev := r - 1
		if v.enc.IsUTF16() {
			prev = r - 2
			if prev < v.dataStart {
				prev = v.dataStart
			}
		}
		rows, idx, _, err := v.rowsAroundLocked(prev)
		if err != nil {
			return r, err
		}
		if len(rows) == 0 {
			break
		}
		take := min(n, idx+1)
		r = rows[idx+1-take].Offset
		n -= take
	}
	return r, nil
}

// LastPage returns the top offset that shows the last n rows.
func (v *Viewer) LastPage(n int) (int64, error) { return v.Back(v.src.Size(), n) }

// PercentAt returns the position of off as a percentage of the data.
func (v *Viewer) PercentAt(off int64) float64 {
	start := v.DataStart()
	size := v.src.Size()
	if size <= start {
		return 100
	}
	p := float64(off-start) / float64(size-start) * 100
	return max(0, min(100, p))
}

// OffsetAtPercent is the inverse of PercentAt (not yet row aligned).
func (v *Viewer) OffsetAtPercent(p float64) int64 {
	start := v.DataStart()
	size := v.src.Size()
	p = max(0, min(100, p))
	return start + int64(float64(size-start)*p/100)
}

// --- text mode ---------------------------------------------------------

// align moves off back to a character boundary where that is knowable.
func (v *Viewer) alignLocked(off int64) int64 {
	if v.enc.IsUTF16() {
		return off - (off-v.dataStart)%2
	}
	if v.enc == UTF8 {
		// Step back over continuation bytes (10xxxxxx) to the lead byte.
		var b [4]byte
		lo := max(v.dataStart, off-3)
		n, _ := readFull(v.src, b[:off-lo+1], lo)
		for off > lo && int(off-lo) < n && b[off-lo]&0xC0 == 0x80 {
			off--
		}
	}
	return off
}

// lineStartLocked finds the start of the logical line containing off.
func (v *Viewer) lineStartLocked(off int64) (int64, error) {
	start := v.dataStart
	if off <= start {
		return start, nil
	}
	u := int64(v.enc.unitSize())
	be := v.enc == UTF16BE
	off = v.alignLocked(off)
	// If off is the LF of a CRLF pair it belongs to the line the pair ends.
	var pair [4]byte
	if off-u >= start {
		if n, _ := readFull(v.src, pair[:2*u], off-u); int64(n) == 2*u {
			if unitAt(pair[:], 0, int(u), be) == '\r' && unitAt(pair[:], int(u), int(u), be) == '\n' {
				off -= u
			}
		}
	}
	limit := max(start, off-MaxLineBytes-MaxLineBytes/2)
	buf := make([]byte, 16*1024)
	hi := off
	for hi > limit {
		lo := max(limit, hi-int64(len(buf)))
		lo += (hi - lo) % u // keep unit alignment relative to hi
		n, err := readFull(v.src, buf[:hi-lo], lo)
		if err != nil {
			return start, err
		}
		for i := int64(n) - u; i >= 0; i -= u {
			c := unitAt(buf, int(i), int(u), be)
			if c == '\n' || c == '\r' {
				return lo + i + u, nil
			}
		}
		hi = lo
	}
	if limit == start {
		return start, nil
	}
	// No line break within 1.5 pieces: off lies in a cut-up long line, and
	// its piece starts at the fixed cut position at or before off.
	cut := v.dataStart + (off-v.dataStart)/MaxLineBytes*MaxLineBytes
	return v.alignForwardLocked(cut), nil
}

type lchar struct {
	r   rune
	off int32 // relative to the line start
}

// layoutLineLocked lays out the logical line starting at s and returns its
// rows and the start of the next line.
func (v *Viewer) layoutLineLocked(s int64) ([]Row, int64, error) {
	size := v.src.Size()
	u := v.enc.unitSize()
	be := v.enc == UTF16BE
	cutAt := int(v.pieceEndLocked(s) - s)
	want := min(int64(cutAt+8), size-s)
	buf := make([]byte, want)
	n, err := readFull(v.src, buf, s)
	if err != nil {
		return nil, s, err
	}
	buf = buf[:n]
	chars := make([]lchar, 0, min(n, 256))
	pos := 0
	next := s + int64(n)
	for pos < n {
		if pos+u <= n {
			c := unitAt(buf, pos, u, be)
			if c == '\n' {
				next = s + int64(pos+u)
				break
			}
			if c == '\r' {
				end := pos + u
				if end+u <= n && unitAt(buf, end, u, be) == '\n' {
					end += u
				}
				next = s + int64(end)
				break
			}
		}
		if pos >= cutAt {
			next = s + int64(pos)
			break
		}
		r, sz := v.dec.decode(buf[pos:])
		if sz <= 0 {
			sz = 1
		}
		chars = append(chars, lchar{r: r, off: int32(pos)})
		pos += sz
	}
	endOfText := s + int64(pos)
	return v.wrapLocked(chars, s, endOfText, next), next, nil
}

// displayRune maps a decoded character to what the text mode draws.
func displayRune(r rune) rune {
	switch {
	case r == '\t':
		return r
	case r < 0x20 || r == 0x7F:
		return ' '
	case r >= 0x80 && r < 0xA0:
		return ' '
	case r == 0xFEFF:
		return ' '
	}
	return r
}

func (v *Viewer) wrapLocked(chars []lchar, s, endOfText, next int64) []Row {
	tab := v.opt.TabWidth
	width := v.opt.Width
	cell := func(r rune, col int) int {
		if r == '\t' {
			return tab - col%tab
		}
		return RuneWidth(displayRune(r))
	}
	var bounds [][2]int
	if !v.opt.Wrap {
		bounds = [][2]int{{0, len(chars)}}
	} else {
		i := 0
		for {
			rowStart, col, lastBreak := i, 0, -1
			for i < len(chars) {
				w := cell(chars[i].r, col)
				if col+w > width && i > rowStart {
					if lastBreak > rowStart {
						i = lastBreak
					}
					break
				}
				if r := chars[i].r; r == ' ' || r == '\t' || r == '-' {
					lastBreak = i + 1
				}
				col += w
				i++
			}
			bounds = append(bounds, [2]int{rowStart, i})
			if i >= len(chars) {
				break
			}
		}
	}
	rows := make([]Row, 0, len(bounds))
	for bi, b := range bounds {
		row := Row{LineStart: bi == 0}
		if b[0] < len(chars) {
			row.Offset = s + int64(chars[b[0]].off)
		} else if len(chars) > 0 {
			row.Offset = endOfText
		} else {
			row.Offset = s
		}
		if bi == len(bounds)-1 {
			row.End = next
		} else {
			row.End = s + int64(chars[b[1]].off)
		}
		var sb strings.Builder
		m := make([]int32, 0, b[1]-b[0])
		col := 0
		base := int32(row.Offset - s)
		for _, c := range chars[b[0]:b[1]] {
			if c.r == '\t' {
				w := tab - col%tab
				for k := 0; k < w; k++ {
					sb.WriteByte(' ')
					m = append(m, c.off-base)
				}
				col += w
				continue
			}
			d := displayRune(c.r)
			sb.WriteRune(d)
			m = append(m, c.off-base)
			col += RuneWidth(d)
		}
		row.Text = sb.String()
		row.Map = m
		rows = append(rows, row)
	}
	return rows
}

// rowsAroundLocked lays out the line containing off and returns its rows,
// the index of the row containing off, and the next line's start.
func (v *Viewer) rowsAroundLocked(off int64) ([]Row, int, int64, error) {
	s, err := v.lineStartLocked(off)
	if err != nil {
		return nil, 0, s, err
	}
	size := v.src.Size()
	for {
		rows, next, err := v.layoutLineLocked(s)
		if err != nil {
			return nil, 0, next, err
		}
		if off < next || next >= size || next <= s {
			idx := 0
			for i, r := range rows {
				if r.Offset <= off {
					idx = i
				}
			}
			return rows, idx, next, nil
		}
		s = next
	}
}

// --- binary and hex mode -------------------------------------------------

// cp437Controls are the glyphs DOS showed for bytes 0x00-0x1F, which is how
// Total Commander's binary mode draws control bytes.
var cp437Controls = []rune(" ☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼")

func glyphTable(enc, ansi Encoding) *[256]rune {
	base := enc
	if !enc.Info().SingleByte {
		base = ansi
	}
	src := tableCache[base]
	if src == nil {
		src = tableCache[Win1252]
	}
	t := *src
	for i := 0; i < 0x20; i++ {
		t[i] = cp437Controls[i]
	}
	t[0x7F] = '⌂'
	for i := 0x80; i < 0x100; i++ {
		if t[i] < 0xA0 && t[i] >= 0x80 || t[i] == 0xFFFD {
			t[i] = '·'
		}
	}
	return &t
}

func hexCharTable(enc, ansi Encoding) *[256]rune {
	t := *glyphTable(enc, ansi)
	for i := 0; i < 0x20; i++ {
		t[i] = '.'
	}
	t[0x7F] = '.'
	for i := 0x80; i < 0x100; i++ {
		if t[i] == '·' || !unicode.IsPrint(t[i]) {
			t[i] = '.'
		}
	}
	return &t
}

const hexDigits = "0123456789ABCDEF"

func (v *Viewer) fixedRowsLocked(off int64, n int) ([]Row, error) {
	size := v.src.Size()
	w := v.rowWidthLocked()
	if off < 0 {
		off = 0
	}
	off -= off % w
	if off >= size {
		return nil, nil
	}
	total := min(int64(n)*w, size-off)
	buf := make([]byte, total)
	m, err := readFull(v.src, buf, off)
	if err != nil {
		return nil, err
	}
	buf = buf[:m]
	rows := make([]Row, 0, n)
	for i := 0; i < len(buf); i += int(w) {
		chunk := buf[i:min(len(buf), i+int(w))]
		row := Row{Offset: off + int64(i), End: off + int64(i+len(chunk)), LineStart: true}
		var sb strings.Builder
		table := v.glyphs
		if v.opt.Mode == ModeHex {
			table = v.hexChars
			row.Hex = FormatHex(chunk, v.opt.HexWidth)
		}
		row.Map = make([]int32, len(chunk))
		for k, c := range chunk {
			sb.WriteRune(table[c])
			row.Map[k] = int32(k)
		}
		row.Text = sb.String()
		rows = append(rows, row)
	}
	return rows, nil
}

// FormatHex renders bytes as upper-case hex pairs separated by spaces, with
// an extra space after each group of 8 bytes when groupWidth is 16 or more
// (the Lister's hex column, and the "copy as hex" format).
func FormatHex(b []byte, groupWidth int) string {
	var sb strings.Builder
	sb.Grow(len(b) * 3)
	for i, c := range b {
		if i > 0 {
			sb.WriteByte(' ')
			if groupWidth >= 16 && i%8 == 0 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte(hexDigits[c>>4])
		sb.WriteByte(hexDigits[c&15])
	}
	return sb.String()
}

// FormatHexRow renders a hex-mode row as one line of text, Total Commander
// style: "0000001F: 48 65 6C 6C 6F ...  Hello", padding short last rows so
// the character column lines up.
func (v *Viewer) FormatHexRow(r Row) string {
	w := v.Options().HexWidth
	full := FormatHex(make([]byte, w), w)
	return fmt.Sprintf("%0*X: %-*s  %s", v.OffsetDigits(), r.Offset, len(full), r.Hex, r.Text)
}

// --- selection, copy and save --------------------------------------------

// ErrTooLarge is returned when a selection is bigger than the given limit.
var ErrTooLarge = errors.New("lister: selection is too large")

// Text decodes the bytes [start, end) as text in the current encoding, for
// copying a selection to the clipboard. Line breaks are kept as they are.
// limit caps the selection size in bytes (0 means 64 MiB).
func (v *Viewer) Text(start, end, limit int64) (string, error) {
	if limit <= 0 {
		limit = 64 << 20
	}
	start = max(start, 0)
	end = min(end, v.src.Size())
	if end <= start {
		return "", nil
	}
	if end-start > limit {
		return "", ErrTooLarge
	}
	buf := make([]byte, end-start)
	n, err := readFull(v.src, buf, start)
	if err != nil {
		return "", err
	}
	return DecodeString(v.Encoding(), buf[:n]), nil
}

// HexText returns [start, end) as "copy as hex" text: hex pairs separated
// by spaces, no offsets.
func (v *Viewer) HexText(start, end, limit int64) (string, error) {
	if limit <= 0 {
		limit = 16 << 20
	}
	start = max(start, 0)
	end = min(end, v.src.Size())
	if end <= start {
		return "", nil
	}
	if end-start > limit {
		return "", ErrTooLarge
	}
	buf := make([]byte, end-start)
	n, err := readFull(v.src, buf, start)
	if err != nil {
		return "", err
	}
	return FormatHex(buf[:n], 0), nil
}

// HexDump returns [start, end) as hex-mode lines (offset, bytes, chars).
func (v *Viewer) HexDump(start, end int64) (string, error) {
	v.mu.Lock()
	saved := v.opt.Mode
	v.opt.Mode = ModeHex // only for the duration of the locked reads below
	defer func() { v.opt.Mode = saved; v.mu.Unlock() }()
	w := int64(v.opt.HexWidth)
	full := len(FormatHex(make([]byte, w), int(w)))
	digits := v.OffsetDigits()
	var sb strings.Builder
	for off := max(start, 0) / w * w; off < end; {
		rows, err := v.fixedRowsLocked(off, 256)
		if err != nil {
			return sb.String(), err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if r.Offset >= end {
				break
			}
			fmt.Fprintf(&sb, "%0*X: %-*s  %s\n", digits, r.Offset, full, r.Hex, r.Text)
			off = r.End
		}
		if off < rows[len(rows)-1].End {
			break
		}
	}
	return sb.String(), nil
}

// CopyRange writes the raw bytes [start, end) to w ("save selection as").
// Use start=0, end=Size() to save the whole file.
func (v *Viewer) CopyRange(ctx context.Context, w io.Writer, start, end int64) (int64, error) {
	start = max(start, 0)
	end = min(end, v.src.Size())
	var done int64
	buf := make([]byte, scanChunk)
	for pos := start; pos < end; {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		n, err := readFull(v.src, buf[:min(int64(len(buf)), end-pos)], pos)
		if err != nil {
			return done, err
		}
		if n == 0 {
			break
		}
		m, err := w.Write(buf[:n])
		done += int64(m)
		if err != nil {
			return done, err
		}
		pos += int64(n)
	}
	return done, nil
}

// Reload re-reads the size of a File source (F2, and follow tail). After
// truncation or replacement the line index is rebuilt; after growth it
// continues. Sources that are not files report no change.
func (v *Viewer) Reload() (Change, error) {
	f, ok := v.src.(*File)
	if !ok {
		s := v.src.Size()
		return Change{OldSize: s, NewSize: s}, nil
	}
	ch, err := f.Reload()
	if err != nil {
		return ch, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	switch {
	case ch.Truncated || ch.Replaced || ch.Modified:
		sample := make([]byte, SampleSize)
		n, _ := readFull(v.src, sample, 0)
		v.detected = Detect(sample[:n], v.opt.ANSI)
		v.lines = nil
		v.applyLocked(v.opt)
	case ch.Grown:
		v.lines.Grew()
	}
	return ch, nil
}
