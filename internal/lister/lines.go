// Line numbers in huge files. "Go to line" and the line number in the
// Lister's status bar need to know where line N starts, but counting the
// lines of a 10 GB log means reading all 10 GB. LineIndex counts lazily: it
// only scans as far as the deepest question asked so far, and it remembers
// the offset of every 1024th line (a sparse index), so later questions about
// any line before that point cost at most 1024 lines of scanning. Memory stays
// tiny (8 bytes per 1024 lines). Line breaks are LF, CRLF and lone CR (old
// Mac files), counted in the file's code units, so UTF-16 text works too.
// When the file grows (follow tail), scanning simply continues from where it
// stopped.

package lister

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
)

// ErrBeyondEnd is returned when a line number is past the last line.
var ErrBeyondEnd = errors.New("lister: line is beyond the end of the file")

const (
	lineEvery = 1024        // checkpoint interval in lines
	scanChunk = 1024 * 1024 // read size for sequential scans
)

// unitAt returns the code unit at buf[i:] for 1- or 2-byte units.
func unitAt(buf []byte, i, unit int, be bool) int {
	if unit == 1 {
		return int(buf[i])
	}
	if be {
		return int(buf[i])<<8 | int(buf[i+1])
	}
	return int(buf[i+1])<<8 | int(buf[i])
}

// scanBreaks calls fn with the file offset of every line start that follows
// a terminator beginning in buf[0:limit) (buf starts at file offset base).
// buf may hold lookahead bytes beyond limit so a CR at the edge can see a
// following LF. fn returns false to stop.
// The result is the offset where the next scan should continue (it can be a
// little past limit when a CRLF straddles it). A CR that is the very last
// unit of the data is left unprocessed, so it pairs correctly with an LF that
// a growing file may append later.
func scanBreaks(buf []byte, limit int, base int64, unit int, be bool, fn func(int64) bool) (next int64, stopped bool) {
	i := 0
	for i+unit <= limit {
		c := unitAt(buf, i, unit, be)
		switch c {
		case '\n':
			if !fn(base + int64(i+unit)) {
				return base + int64(i+unit), true
			}
		case '\r':
			j := i + unit
			if j+unit > len(buf) {
				// No lookahead: only happens for a CR at the end of the data.
				return base + int64(i), false
			}
			if unitAt(buf, j, unit, be) == '\n' {
				j += unit
			}
			if !fn(base + int64(j)) {
				return base + int64(j), true
			}
			i = j
			continue
		}
		i += unit
	}
	return base + int64(i), false
}

// LineIndex finds line starts and line numbers lazily. It is safe for
// concurrent use; long scans hold its lock, so callers that must stay
// responsive should ask from a goroutine and pass a cancellable context.
type LineIndex struct {
	src   Source
	unit  int
	be    bool
	start int64 // offset of line 1 (after a byte order mark)

	mu          sync.Mutex
	checkpoints []int64 // checkpoints[k] = offset of line k*lineEvery+1
	lines       int64   // line starts found so far (including line 1)
	lastStart   int64   // offset of the last line start found
	scanned     atomic.Int64
	done        bool // scanned reached the end of the data (as of its current size)
}

// NewLineIndex creates an index over src for text in encoding enc whose
// first line starts at start.
func NewLineIndex(src Source, enc Encoding, start int64) *LineIndex {
	ix := &LineIndex{src: src, unit: enc.unitSize(), be: enc == UTF16BE, start: start}
	ix.resetLocked()
	return ix
}

func (ix *LineIndex) resetLocked() {
	ix.checkpoints = []int64{ix.start}
	ix.lines = 1
	ix.lastStart = ix.start
	ix.scanned.Store(ix.start)
	ix.done = false
}

// Reset forgets everything (used after truncation or replacement).
func (ix *LineIndex) Reset() {
	ix.mu.Lock()
	ix.resetLocked()
	ix.mu.Unlock()
}

// Grew tells the index the data got longer, so a finished scan may continue.
func (ix *LineIndex) Grew() {
	ix.mu.Lock()
	ix.done = false
	ix.mu.Unlock()
}

// Progress returns how far the background count has scanned and the size
// of the data, for a "counting lines…" indicator.
func (ix *LineIndex) Progress() (scanned, size int64) {
	return ix.scanned.Load(), ix.src.Size()
}

// extendLocked scans forward until enough(...) is true or the data ends.
func (ix *LineIndex) extendLocked(ctx context.Context, enough func() bool) error {
	var buf []byte
	for !ix.done && !enough() {
		if err := ctx.Err(); err != nil {
			return err
		}
		pos := ix.scanned.Load()
		size := ix.src.Size()
		if pos >= size {
			ix.done = true
			break
		}
		if buf == nil {
			buf = make([]byte, scanChunk+ix.unit)
		}
		n, err := readFull(ix.src, buf, pos)
		if err != nil {
			return err
		}
		limit := min(n, scanChunk)
		limit -= limit % ix.unit
		if limit == 0 {
			ix.done = true
			break
		}
		next, _ := scanBreaks(buf[:n], limit, pos, ix.unit, ix.be, func(s int64) bool {
			ix.lines++
			ix.lastStart = s
			if (ix.lines-1)%lineEvery == 0 {
				ix.checkpoints = append(ix.checkpoints, s)
			}
			return true
		})
		if next <= pos {
			ix.done = true // a lone CR at the very end; wait for more data
			break
		}
		ix.scanned.Store(next)
	}
	return nil
}

// walk scans from a known line start and calls fn for each later line start.
func (ix *LineIndex) walk(ctx context.Context, from int64, fn func(int64) bool) error {
	buf := make([]byte, 64*1024+ix.unit)
	pos := from
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		size := ix.src.Size()
		if pos >= size {
			return nil
		}
		n, err := readFull(ix.src, buf, pos)
		if err != nil {
			return err
		}
		limit := min(n, 64*1024)
		limit -= limit % ix.unit
		if limit == 0 {
			return nil
		}
		next, stopped := scanBreaks(buf[:n], limit, pos, ix.unit, ix.be, fn)
		if stopped || next <= pos {
			return nil
		}
		pos = next
	}
}

// LineOffset returns the offset where line n (1-based) starts. For a line
// past the end it returns ErrBeyondEnd.
func (ix *LineIndex) LineOffset(ctx context.Context, n int64) (int64, error) {
	if n < 1 {
		n = 1
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	k := int((n - 1) / lineEvery)
	if err := ix.extendLocked(ctx, func() bool { return len(ix.checkpoints) > k }); err != nil {
		return 0, err
	}
	if k >= len(ix.checkpoints) {
		return 0, ErrBeyondEnd
	}
	off := ix.checkpoints[k]
	rest := (n - 1) % lineEvery
	if rest > 0 {
		found := int64(0)
		err := ix.walk(ctx, off, func(s int64) bool {
			found++
			off = s
			return found < rest
		})
		if err != nil {
			return 0, err
		}
		if found < rest {
			return 0, ErrBeyondEnd
		}
	}
	if size := ix.src.Size(); off >= size && size > ix.start {
		return 0, ErrBeyondEnd // the empty "line" after a final newline
	}
	return off, nil
}

// LineAt returns the 1-based number of the line containing offset off.
func (ix *LineIndex) LineAt(ctx context.Context, off int64) (int64, error) {
	if off <= ix.start {
		return 1, nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.extendLocked(ctx, func() bool { return ix.scanned.Load() > off }); err != nil {
		return 0, err
	}
	k := sort.Search(len(ix.checkpoints), func(i int) bool { return ix.checkpoints[i] > off }) - 1
	if k < 0 {
		k = 0
	}
	count := int64(0)
	err := ix.walk(ctx, ix.checkpoints[k], func(s int64) bool {
		if s > off {
			return false
		}
		count++
		return true
	})
	if err != nil {
		return 0, err
	}
	return int64(k)*lineEvery + 1 + count, nil
}

// Count scans to the end and returns the number of lines. A final line
// break does not start an extra (empty) line; an empty file has one line.
func (ix *LineIndex) Count(ctx context.Context) (int64, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.extendLocked(ctx, func() bool { return false }); err != nil {
		return 0, err
	}
	n := ix.lines
	if size := ix.src.Size(); ix.lastStart >= size && size > ix.start && n > 1 {
		n--
	}
	return n, nil
}

// Known returns the number of lines found so far without scanning further,
// and whether that is the final count.
func (ix *LineIndex) Known() (lines int64, complete bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	n := ix.lines
	if ix.done {
		if size := ix.src.Size(); ix.lastStart >= size && size > ix.start && n > 1 {
			n--
		}
	}
	return n, ix.done
}
