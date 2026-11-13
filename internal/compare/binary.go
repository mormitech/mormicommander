// This file implements the binary side of Compare by content. When either
// file is not text, Total Commander compares the two files byte by byte at
// the same offsets and lists the differing bytes in hex, stopping after a
// configurable number of differences so a completely different multi-gigabyte
// file does not flood the window. CompareBinary streams both inputs in large
// chunks (nothing is loaded whole), collects the first N differing bytes with
// their offsets and values, groups neighbouring differences into runs for a
// compact display, and reports a length mismatch separately. FilesEqual is
// the fast yes/no form used by "compare files with the same name" and by the
// folder synchronisation's "compare by content" option.
package compare

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
)

// DefaultMaxBinaryDiffs is the number of differing bytes collected when the
// caller passes 0.
const DefaultMaxBinaryDiffs = 1000

const binChunk = 64 * 1024

// ByteDiff is one differing byte at the same offset in both inputs.
type ByteDiff struct {
	Offset int64
	A, B   byte
}

// ByteRun is a run of consecutive differing offsets [Offset, Offset+Len).
type ByteRun struct {
	Offset int64
	Len    int64
}

// BinaryResult is the outcome of a binary comparison.
type BinaryResult struct {
	// SizeA and SizeB are the number of bytes read from each input. When
	// the comparison stopped early (Truncated) they are lower bounds unless
	// the sizes were known from the file system.
	SizeA, SizeB int64
	// Diffs holds the first differing bytes in offset order (at most the
	// requested maximum). Bytes beyond the end of the shorter input are not
	// listed; see SizeA/SizeB.
	Diffs []ByteDiff
	// Runs groups Diffs into runs of adjacent offsets.
	Runs []ByteRun
	// Truncated is true when more differences exist than were collected.
	Truncated bool
}

// Identical reports whether both inputs had the same length and content.
func (r *BinaryResult) Identical() bool {
	return !r.Truncated && len(r.Diffs) == 0 && r.SizeA == r.SizeB
}

// FirstDifference returns the offset of the first difference, including a
// length mismatch, or -1 when identical.
func (r *BinaryResult) FirstDifference() int64 {
	if len(r.Diffs) > 0 {
		return r.Diffs[0].Offset
	}
	if r.SizeA != r.SizeB {
		return min(r.SizeA, r.SizeB)
	}
	return -1
}

// CompareBinary compares a and b byte by byte. maxDiffs limits the number of
// differences collected (0 = DefaultMaxBinaryDiffs, negative = unlimited).
// Once the limit is exceeded the comparison stops and Truncated is set. The
// context is checked between chunks.
func CompareBinary(ctx context.Context, a, b io.Reader, maxDiffs int) (*BinaryResult, error) {
	if maxDiffs == 0 {
		maxDiffs = DefaultMaxBinaryDiffs
	}
	res := &BinaryResult{}
	bufA := make([]byte, binChunk)
	bufB := make([]byte, binChunk)
	var off int64
	eofA, eofB := false, false
	for !eofA || !eofB {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		na, errA := readChunk(a, bufA, &eofA)
		if errA != nil {
			return res, errA
		}
		nb, errB := readChunk(b, bufB, &eofB)
		if errB != nil {
			return res, errB
		}
		res.SizeA += int64(na)
		res.SizeB += int64(nb)
		n := min(na, nb)
		if !bytes.Equal(bufA[:n], bufB[:n]) {
			for i := 0; i < n; i++ {
				if bufA[i] == bufB[i] {
					continue
				}
				if maxDiffs > 0 && len(res.Diffs) >= maxDiffs {
					res.Truncated = true
					res.Runs = groupRuns(res.Diffs)
					return res, nil
				}
				res.Diffs = append(res.Diffs, ByteDiff{Offset: off + int64(i), A: bufA[i], B: bufB[i]})
			}
		}
		off += int64(n)
		if na != nb {
			// One input ended: count the rest of the longer one.
			rest := a
			if nb > na {
				rest = b
			}
			if (nb > na && !eofB) || (na > nb && !eofA) {
				m, err := remaining(ctx, rest)
				if err != nil {
					return res, err
				}
				if nb > na {
					res.SizeB += m
				} else {
					res.SizeA += m
				}
			}
			break
		}
	}
	res.Runs = groupRuns(res.Diffs)
	return res, nil
}

// remaining counts the bytes left in r after the shorter input ended. A
// seekable input (a file) is measured without reading it, so comparing a
// small file with a huge one stays instant; anything else is read in chunks
// with a cancel check between them, so Esc still works.
func remaining(ctx context.Context, r io.Reader) (int64, error) {
	if s, ok := r.(io.Seeker); ok {
		if cur, err := s.Seek(0, io.SeekCurrent); err == nil {
			if end, err := s.Seek(0, io.SeekEnd); err == nil && end >= cur {
				return end - cur, nil
			}
		}
	}
	buf := make([]byte, binChunk)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		m, err := r.Read(buf)
		n += int64(m)
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
	}
}

// readChunk fills buf as far as possible; it sets *eof when the reader is
// exhausted.
func readChunk(r io.Reader, buf []byte, eof *bool) (int, error) {
	if *eof {
		return 0, nil
	}
	n, err := io.ReadFull(r, buf)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		*eof = true
		return n, nil
	}
	return n, err
}

func groupRuns(d []ByteDiff) []ByteRun {
	var runs []ByteRun
	for _, x := range d {
		if n := len(runs); n > 0 && runs[n-1].Offset+runs[n-1].Len == x.Offset {
			runs[n-1].Len++
			continue
		}
		runs = append(runs, ByteRun{Offset: x.Offset, Len: 1})
	}
	return runs
}

// CompareBinaryFiles opens two files and compares them with CompareBinary.
// When the comparison is truncated the sizes are taken from the file system.
func CompareBinaryFiles(ctx context.Context, pathA, pathB string, maxDiffs int) (*BinaryResult, error) {
	fa, err := os.Open(pathA)
	if err != nil {
		return nil, err
	}
	defer fa.Close()
	fb, err := os.Open(pathB)
	if err != nil {
		return nil, err
	}
	defer fb.Close()
	res, err := CompareBinary(ctx, fa, fb, maxDiffs)
	if err != nil {
		return res, err
	}
	if res.Truncated {
		if st, err := fa.Stat(); err == nil {
			res.SizeA = st.Size()
		}
		if st, err := fb.Stat(); err == nil {
			res.SizeB = st.Size()
		}
	}
	return res, nil
}

// FilesEqual reports whether two files have identical content. Sizes are
// compared first so that files of different length are rejected without
// reading them.
func FilesEqual(ctx context.Context, pathA, pathB string) (bool, error) {
	sa, err := os.Stat(pathA)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(pathB)
	if err != nil {
		return false, err
	}
	if sa.Size() != sb.Size() {
		return false, nil
	}
	if os.SameFile(sa, sb) {
		return true, nil
	}
	fa, err := os.Open(pathA)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(pathB)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	return ReadersEqual(ctx, fa, fb)
}

// ReadersEqual reports whether two streams have identical content.
// It stops at the first difference.
func ReadersEqual(ctx context.Context, a, b io.Reader) (bool, error) {
	bufA := make([]byte, binChunk)
	bufB := make([]byte, binChunk)
	eofA, eofB := false, false
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		na, err := readChunk(a, bufA, &eofA)
		if err != nil {
			return false, err
		}
		nb, err := readChunk(b, bufB, &eofB)
		if err != nil {
			return false, err
		}
		if na != nb || !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false, nil
		}
		if na == 0 {
			return true, nil
		}
	}
}
