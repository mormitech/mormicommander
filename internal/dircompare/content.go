// This file compares two files byte by byte for the "by content" option. Dates alone mislead after a copy tool that
// does not keep times, or after extracting an archive, so TC can open both files and look at the bytes: identical
// content makes the pair equal whatever the dates say. Both files are read side by side in fixed-size blocks so memory stays
// small for huge files, the first differing block stops the read, and the context is checked between blocks so the
// user can cancel a long comparison with Esc. Pipes, sockets and devices are never opened here (judgeFiles skips them
// and OS.Open refuses them): opening a pipe without a writer would block inside the open call, where no context can
// reach it.
package dircompare

import (
	"bytes"
	"context"
	"io"
)

// blockSize is how much of each file is read at a time.
const blockSize = 256 << 10

// sameContent reports whether the two files hold the same bytes.
func (c *comparer) sameContent(lpath, rpath, rel string) (bool, error) {
	lf, err := c.l.Open(c.ctx, lpath)
	if err != nil {
		return false, err
	}
	defer lf.Close()
	rf, err := c.r.Open(c.ctx, rpath)
	if err != nil {
		return false, err
	}
	defer rf.Close()
	return sameReaders(c.ctx, lf, rf, func(n int64) {
		c.bytes += n
		c.progress(rel)
	})
}

// SameContent reports whether two readers deliver the same bytes. It is exported for the file compare tool and the
// overwrite dialog's "Compare by content" button, which need the same answer for two files.
func SameContent(ctx context.Context, a, b io.Reader) (bool, error) {
	return sameReaders(ctx, a, b, nil)
}

func sameReaders(ctx context.Context, a, b io.Reader, read func(int64)) (bool, error) {
	ba, bb := make([]byte, blockSize), make([]byte, blockSize)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		na, ea := io.ReadFull(a, ba)
		nb, eb := io.ReadFull(b, bb)
		if read != nil {
			read(int64(na + nb))
		}
		if ea != nil && ea != io.EOF && ea != io.ErrUnexpectedEOF {
			return false, ea
		}
		if eb != nil && eb != io.EOF && eb != io.ErrUnexpectedEOF {
			return false, eb
		}
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		if ea != nil || eb != nil {
			// Both short reads of the same length: both ended (ReadFull returns an error only at the end).
			return ea != nil && eb != nil, nil
		}
	}
}
