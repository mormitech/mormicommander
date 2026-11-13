// Every operation in this package can take a while on big files, so they all
// report progress the same way and can all be cancelled the same way. This
// file holds the shared Progress type and a copy helper that reads in large
// blocks, checks the context between blocks and calls the progress callback.
// A dialog shows "Done / Total" bytes plus the name of the current file, and
// the Cancel button simply cancels the context passed in.

package fileutil

import (
	"context"
	"errors"
	"io"
	"os"
)

// Progress describes how far an operation has got. Total is the number of
// bytes the whole operation will read (0 when it is not known up front), and
// Done is how many of them have been processed so far. File is the file being
// worked on right now, FileDone/FileTotal the position inside that file.
type Progress struct {
	File      string
	FileDone  int64
	FileTotal int64
	Done      int64
	Total     int64
}

// ProgressFunc receives progress updates. It is called from the goroutine that
// runs the operation, so it must be quick; a UI should just store the values
// and repaint on its own timer.
type ProgressFunc func(Progress)

// ErrCanceled is returned (wrapped) when the context is cancelled. Partial
// output files are removed before it is returned.
var ErrCanceled = errors.New("operation canceled")

// copyBufSize is the read block size. One MiB keeps the callback rate low on
// fast disks while still giving smooth progress on slow ones.
const copyBufSize = 1 << 20

// tracker carries the running totals for one operation so the per-file copy
// helper can report overall progress too.
type tracker struct {
	ctx   context.Context
	fn    ProgressFunc
	done  int64
	total int64
}

func newTracker(ctx context.Context, fn ProgressFunc, total int64) *tracker {
	if ctx == nil {
		ctx = context.Background()
	}
	return &tracker{ctx: ctx, fn: fn, total: total}
}

// check returns ErrCanceled once the context has been cancelled.
func (t *tracker) check() error {
	if err := t.ctx.Err(); err != nil {
		return errors.Join(ErrCanceled, err)
	}
	return nil
}

func (t *tracker) report(file string, fileDone, fileTotal int64) {
	if t.fn != nil {
		t.fn(Progress{File: file, FileDone: fileDone, FileTotal: fileTotal, Done: t.done, Total: t.total})
	}
}

// copyN copies up to n bytes (n < 0 means until EOF) from r to w in large
// blocks, checking for cancellation and reporting progress after each block.
// It returns the number of bytes copied. Hitting EOF before n bytes is not an
// error here; callers compare the count when they need an exact size.
func (t *tracker) copyN(w io.Writer, r io.Reader, n int64, file string, fileDone, fileTotal int64) (int64, error) {
	buf := make([]byte, copyBufSize)
	var copied int64
	for n < 0 || copied < n {
		if err := t.check(); err != nil {
			return copied, err
		}
		want := int64(len(buf))
		if n >= 0 && n-copied < want {
			want = n - copied
		}
		k, rerr := r.Read(buf[:want])
		if k > 0 {
			if _, werr := w.Write(buf[:k]); werr != nil {
				return copied, werr
			}
			copied += int64(k)
			t.done += int64(k)
			t.report(file, fileDone+copied, fileTotal)
		}
		if rerr == io.EOF {
			return copied, nil
		}
		if rerr != nil {
			return copied, rerr
		}
	}
	return copied, nil
}

// fileSize returns the size of a regular file, or an error for folders and
// missing files.
func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, &os.PathError{Op: "open", Path: path, Err: errors.New("is a folder")}
	}
	return fi.Size(), nil
}

// createOutput opens a new output file. Unless overwrite is set it refuses to
// replace an existing file, so a split or decode never silently destroys data.
func createOutput(path string, overwrite bool) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	return os.OpenFile(path, flags, 0o644)
}

// ErrExists is returned (wrapped in an *os.PathError) when an output file is
// already there and overwriting was not allowed.
var ErrExists = os.ErrExist

// removeAll deletes the given files, ignoring errors. It is used to clean up
// partial output after a failure or a cancel.
func removeAll(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}
