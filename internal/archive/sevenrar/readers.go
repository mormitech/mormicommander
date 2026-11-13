// This file holds the small reader wrappers shared by the 7z and RAR
// backends. Every entry's data goes through checkedReader, which does three
// jobs: it stops promptly when the operation is cancelled (Total Commander's
// "Abort" button), it translates library errors into the package's own
// errors, and it compares the CRC-32 of what was read with the checksum
// stored in the archive, so a damaged archive or a wrong password is never
// silently turned into a broken file.

package sevenrar

import (
	"context"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
)

type checkedReader struct {
	ctx       context.Context
	r         io.Reader
	c         io.Closer // may be nil
	size      int64     // expected size, -1 if unknown
	crc       uint32
	check     bool // compare the CRC at the end
	encrypted bool
	mapErr    func(error) error

	h    hash.Hash32
	n    int64
	done bool
}

func (cr *checkedReader) Read(p []byte) (int, error) {
	if cr.done {
		return 0, io.EOF
	}
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	if cr.h == nil {
		cr.h = crc32.NewIEEE()
	}
	if cr.size >= 0 && int64(len(p)) > cr.size-cr.n {
		p = p[:cr.size-cr.n]
	}
	var n int
	var err error
	if len(p) > 0 {
		n, err = cr.r.Read(p)
	} else if cr.size >= 0 {
		err = io.EOF
	}
	cr.h.Write(p[:n])
	cr.n += int64(n)
	if err == io.EOF || err == nil && cr.size >= 0 && cr.n == cr.size {
		if cr.size >= 0 && cr.n < cr.size {
			return n, cr.wrap(io.ErrUnexpectedEOF)
		}
		cr.done = true
		if err == nil {
			// All expected bytes are in; read once more so the library
			// reaches its own end of file and runs its checks there
			// (rardecode verifies checksums only at EOF).
			var one [1]byte
			if _, e2 := cr.r.Read(one[:]); e2 != nil && e2 != io.EOF {
				return n, cr.wrap(e2)
			}
		}
		if cr.check && cr.h.Sum32() != cr.crc {
			if cr.encrypted {
				return n, fmt.Errorf("%w (wrong password?)", ErrCRC)
			}
			return n, ErrCRC
		}
		if n > 0 {
			return n, nil
		}
		return 0, io.EOF
	}
	if err != nil {
		return n, cr.wrap(err)
	}
	return n, nil
}

func (cr *checkedReader) wrap(err error) error {
	if cr.mapErr != nil {
		return cr.mapErr(err)
	}
	return err
}

func (cr *checkedReader) Close() error {
	if cr.c != nil {
		return cr.c.Close()
	}
	return nil
}

// errReader returns its error on every Read; walk hands it to the callback
// when an entry cannot be opened, so the error is reported per entry.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// multiCloser closes several things in order.
type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var first error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func statSize(p string) (int64, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
