// This file implements "Test archive" (Alt+Shift+F9 in Total Commander):
// every file in the archive is unpacked into nowhere and its checksum
// verified, and the user gets a per-archive report of what is fine and what
// is damaged. Because Archive.Open already verifies checksums (CRC32 for zip,
// the HMAC for AES-encrypted zip entries) when a reader reaches the end,
// testing is the same loop for every format.
package archive

import (
	"context"
	"errors"
	"io"
)

// VerifyOptions configure Verify.
type VerifyOptions struct {
	// Paths limits the test to these entries (folders include subtrees);
	// empty tests everything.
	Paths []string
	// Progress receives progress reports with Op "test".
	Progress ProgressFunc
}

// VerifyReport is the outcome of testing one archive.
type VerifyReport struct {
	Archive string
	Files   int
	Bytes   int64
	Failed  []FileError
}

// OK reports whether every tested file was intact.
func (r VerifyReport) OK() bool { return len(r.Failed) == 0 }

// Verify tests the archive. Damaged entries are collected in the report; the
// returned error is only set when the test itself was stopped (cancelled,
// or the user dismissed a password prompt).
func Verify(ctx context.Context, a Archive, opts VerifyOptions) (VerifyReport, error) {
	rep := VerifyReport{Archive: a.Path()}
	var list []Entry
	var total int64
	for _, e := range Selection(a, opts.Paths, "") {
		if e.IsDir {
			continue
		}
		list = append(list, e)
		total += e.Size
	}
	t := NewTracker("test", opts.Progress, total, len(list))
	for _, e := range list {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		t.StartFile(e.Path, e.Size)
		n, err := verifyOne(ctx, a, e, t)
		rep.Bytes += n
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrCanceled) {
				return rep, err
			}
			rep.Failed = append(rep.Failed, FileError{e.Path, err})
			continue
		}
		rep.Files++
		t.FinishFile()
	}
	return rep, nil
}

func verifyOne(ctx context.Context, a Archive, e Entry, t *Tracker) (int64, error) {
	rc, err := a.Open(e)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return CopyContext(ctx, io.Discard, rc, t)
}
