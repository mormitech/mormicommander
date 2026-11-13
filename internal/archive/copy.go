// This file has the small copy helper every long operation uses: it moves
// bytes in chunks, stops promptly when the job's context is cancelled (the
// user pressed Cancel in the progress dialog), and reports progress after
// each chunk so the UI can draw both the per-file and the total bar. Formats
// use it too when packing, so cancel and progress behave the same everywhere.
package archive

import (
	"context"
	"io"
)

const copyChunk = 256 << 10

// Tracker accumulates job-wide progress and forwards it to a ProgressFunc.
// A nil *Tracker is valid and does nothing, so callers need no nil checks.
type Tracker struct {
	fn ProgressFunc
	p  Progress
}

// NewTracker starts progress reporting for an operation.
func NewTracker(op string, fn ProgressFunc, totalBytes int64, totalFiles int) *Tracker {
	if fn == nil {
		return nil
	}
	return &Tracker{fn: fn, p: Progress{Op: op, Total: totalBytes, TotalFiles: totalFiles}}
}

// StartFile announces the next file.
func (t *Tracker) StartFile(path string, size int64) {
	if t == nil {
		return
	}
	t.p.Path, t.p.FileDone, t.p.FileTotal = path, 0, size
	t.fn(t.p)
}

// Add records n more bytes of the current file.
func (t *Tracker) Add(n int64) {
	if t == nil {
		return
	}
	t.p.FileDone += n
	t.p.Done += n
	t.fn(t.p)
}

// FinishFile marks the current file as done.
func (t *Tracker) FinishFile() {
	if t == nil {
		return
	}
	t.p.Files++
	t.fn(t.p)
}

// CopyContext copies src to dst like io.Copy, checking ctx between chunks
// and reporting each chunk to t.
func CopyContext(ctx context.Context, dst io.Writer, src io.Reader, t *Tracker) (int64, error) {
	buf := make([]byte, copyChunk)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			w, werr := dst.Write(buf[:n])
			total += int64(w)
			t.Add(int64(w))
			if werr != nil {
				return total, werr
			}
			if w != n {
				return total, io.ErrShortWrite
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}
