// This file adapts github.com/bodgit/sevenzip, a pure-Go 7z reader, to the
// engine's backend interface. It supports the usual 7z methods (LZMA, LZMA2,
// PPMd, BZip2, Deflate, BCJ/BCJ2 and other filters, zstd, brotli, lz4),
// AES-256 encryption, and split archives (name.7z.001, .002, ...).
//
// Two gaps of the library are filled in here. First, it does not verify file
// checksums, so every reader is wrapped in a CRC-32 check, which is also how a
// wrong password is noticed for files stored without compression. Second, it
// does not say which files are encrypted when the file list itself is not.
// We find out by reading the start of the first file with two different
// dummy passwords: unencrypted data reads the same both times, encrypted data
// does not (or fails to decompress). 7-Zip always encrypts all streams of an
// archive with the same password, so one probe answers for all files.

package sevenrar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/bodgit/sevenzip"
)

type sevenZip struct {
	path      string
	mu        sync.Mutex
	rc        *sevenzip.ReadCloser
	password  string
	entries   []Entry
	encrypted bool // file contents are encrypted
	headerEnc bool // the file list is encrypted
}

// open7z opens a 7z archive (or split set starting at .001) with password pw.
func open7z(path string, det Detection, pw string) (*sevenZip, error) {
	rc, err := sevenzip.OpenReaderWithPassword(path, pw)
	if err != nil {
		return nil, map7zErr(err, pw, det.HeadersEncrypted)
	}
	z := &sevenZip{path: path, rc: rc, password: pw}
	headerEnc := false
	if pw != "" {
		// If the archive cannot be opened without the password, the file
		// list is encrypted (and then so are all contents).
		if probe, perr := sevenzip.OpenReader(path); perr != nil {
			var re *sevenzip.ReadError
			if errors.As(perr, &re) && re.Encrypted {
				headerEnc = true
			}
		} else {
			_ = probe.Close()
		}
	}
	z.headerEnc = headerEnc
	z.encrypted = headerEnc || probe7zEncrypted(path)

	perStream := map[int]int{}
	for _, f := range rc.File {
		if f.UncompressedSize > 0 {
			perStream[f.Stream]++
		}
	}
	z.entries = make([]Entry, len(rc.File))
	for i, f := range rc.File {
		mode := f.Mode()
		e := Entry{
			RawName:    f.Name,
			IsDir:      mode.IsDir(),
			Size:       int64(f.UncompressedSize),
			PackedSize: -1,
			ModTime:    f.Modified,
			Mode:       mode,
			Attributes: f.Attributes,
			CRC32:      f.CRC32,
			HasCRC:     f.CRC32 != 0,
			Symlink:    mode&fs.ModeSymlink != 0,
		}
		if !e.IsDir && e.Size > 0 {
			e.Encrypted = z.encrypted
			e.Solid = perStream[f.Stream] > 1
		}
		z.entries[i] = e
	}
	return z, nil
}

// probe7zEncrypted reads the first bytes of the first non-empty file with two
// different passwords and reports whether they differ (or fail to decode).
func probe7zEncrypted(path string) bool {
	read := func(pw string) ([]byte, bool, error) {
		rc, err := sevenzip.OpenReaderWithPassword(path, pw)
		if err != nil {
			var re *sevenzip.ReadError
			return nil, errors.As(err, &re) && re.Encrypted, err
		}
		defer rc.Close()
		for _, f := range rc.File {
			if f.UncompressedSize == 0 || f.Mode().IsDir() {
				continue
			}
			r, err := f.Open()
			if err != nil {
				var re *sevenzip.ReadError
				return nil, errors.As(err, &re) && re.Encrypted, err
			}
			buf := make([]byte, min(int64(f.UncompressedSize), 64))
			_, err = io.ReadFull(r, buf)
			_ = r.Close()
			if err != nil {
				var re *sevenzip.ReadError
				return nil, errors.As(err, &re) && re.Encrypted, err
			}
			return buf, false, nil
		}
		return nil, false, nil
	}
	a, encA, errA := read("\x01mormi-probe-a")
	b, encB, errB := read("\x02mormi-probe-b")
	if encA || encB {
		return true
	}
	if errA != nil || errB != nil {
		return false
	}
	return !bytes.Equal(a, b)
}

// map7zErr turns library errors into the package's sentinel errors.
func map7zErr(err error, pw string, encrypted bool) error {
	if err == nil {
		return nil
	}
	var re *sevenzip.ReadError
	if errors.As(err, &re) && re.Encrypted || encrypted && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCRC) && !errors.Is(err, io.ErrUnexpectedEOF) {
		if pw == "" {
			return fmt.Errorf("%w (%v)", ErrPasswordRequired, err)
		}
		return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(err, fs.ErrNotExist) {
		return &MissingVolumeError{Path: pe.Path, Err: err}
	}
	return err
}

func (z *sevenZip) list() []Entry { return append([]Entry(nil), z.entries...) }

func (z *sevenZip) current() (*sevenzip.ReadCloser, string) {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.rc, z.password
}

func (z *sevenZip) openFile(ctx context.Context, rc *sevenzip.ReadCloser, pw string, i int) (io.ReadCloser, error) {
	if i < 0 || i >= len(rc.File) {
		return nil, ErrNotFound
	}
	e := z.entries[i]
	r, err := rc.File[i].Open()
	if err != nil {
		return nil, map7zErr(err, pw, e.Encrypted)
	}
	return &checkedReader{ctx: ctx, r: r, c: r, size: e.Size, crc: e.CRC32, check: e.HasCRC,
		encrypted: e.Encrypted, mapErr: func(err error) error { return map7zErr(err, pw, e.Encrypted) }}, nil
}

func (z *sevenZip) open(ctx context.Context, i int) (io.ReadCloser, error) {
	rc, pw := z.current()
	return z.openFile(ctx, rc, pw, i)
}

func (z *sevenZip) walk(ctx context.Context, want func(i int) bool, fn func(i int, r io.Reader) error) error {
	rc, pw := z.current()
	for i := range rc.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !want(i) {
			continue
		}
		r, err := z.openFile(ctx, rc, pw, i)
		if err != nil {
			if ferr := fn(i, errReader{err}); ferr != nil {
				return ferr
			}
			continue
		}
		err = fn(i, r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (z *sevenZip) setPassword(pw string) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if pw == z.password {
		return nil
	}
	rc, err := sevenzip.OpenReaderWithPassword(z.path, pw)
	if err != nil {
		return map7zErr(err, pw, true)
	}
	_ = z.rc.Close()
	z.rc, z.password = rc, pw
	return nil
}

// verifyPassword reads the start of entry i (or all of it when it is small,
// so the checksum can be compared). Compressed data decoded with a wrong key
// fails almost at once; stored data is only caught by the checksum.
func (z *sevenZip) verifyPassword(ctx context.Context, i int) error {
	r, err := z.open(ctx, i)
	if err != nil {
		return asWrongPassword(err)
	}
	defer r.Close()
	limit := int64(64 << 10)
	if z.entries[i].Size <= 1<<20 {
		limit = z.entries[i].Size + 1
	}
	_, err = io.Copy(io.Discard, io.LimitReader(r, limit))
	return asWrongPassword(err)
}

// asWrongPassword turns read errors during password verification into
// ErrWrongPassword; context errors pass through.
func asWrongPassword(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrMissingVolume):
		return err
	case errors.Is(err, ErrWrongPassword), errors.Is(err, ErrPasswordRequired), errors.Is(err, ErrCRC):
		return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
	}
	return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
}

func (z *sevenZip) close() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.rc.Close()
}
