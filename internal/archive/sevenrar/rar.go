// This file adapts github.com/nwaples/rardecode/v2, a pure-Go RAR reader, to
// the engine's backend interface. It reads both the RAR 1.5-4.x format and
// the RAR 5 format, solid archives, multi-volume sets in both naming schemes
// (name.part1.rar and name.rar/name.r00), AES encryption of file contents and
// of file names ("encrypt file names"), and symbolic/hard links. The library
// checks file checksums itself; we only translate its errors.
//
// Random access to one file (viewing it with F3) uses the library's File.Open
// for normal archives. In a solid archive each file can only be decoded after
// all files before it, so there a single file is reached by streaming the
// archive from the start, and extraction always streams.

package sevenrar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/nwaples/rardecode/v2"
)

type rarArchive struct {
	path      string
	det       Detection
	mu        sync.Mutex
	files     []*rardecode.File
	password  string
	entries   []Entry
	headerEnc bool
}

// rarOptions returns the library options for password pw. For a
// self-extracting archive the first volume is opened from the archive's
// offset, so signature-like bytes inside the executable stub cannot confuse
// the library's own signature search.
func rarOptions(path string, det Detection, pw string) []rardecode.Option {
	var opts []rardecode.Option
	if pw != "" {
		opts = append(opts, rardecode.Password(pw))
	}
	if det.SFX && det.Offset > 0 {
		opts = append(opts, rardecode.FileSystem(offsetFS{path: path, off: det.Offset}))
	}
	return opts
}

// offsetFS opens files from the OS, starting the file named path at off.
type offsetFS struct {
	path string
	off  int64
}

func (o offsetFS) Open(name string) (fs.File, error) {
	f, err := os.Open(name)
	if err != nil || name != o.path {
		return f, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &sectionFile{SectionReader: io.NewSectionReader(f, o.off, st.Size()-o.off), f: f}, nil
}

type sectionFile struct {
	*io.SectionReader
	f *os.File
}

func (s *sectionFile) Stat() (fs.FileInfo, error) { return s.f.Stat() }
func (s *sectionFile) Close() error               { return s.f.Close() }

// openRar lists the RAR archive (all volumes) starting at path.
func openRar(path string, det Detection, pw string) (*rarArchive, error) {
	files, err := rardecode.List(path, rarOptions(path, det, pw)...)
	if err != nil {
		return nil, mapRarErr(err, pw, det.HeadersEncrypted)
	}
	r := &rarArchive{path: path, det: det, files: files, password: pw, headerEnc: det.HeadersEncrypted}
	r.entries = make([]Entry, len(files))
	for i, f := range files {
		r.entries[i] = rarEntry(&f.FileHeader)
		if f.HeaderEncrypted {
			r.headerEnc = true
		}
	}
	return r, nil
}

func rarEntry(h *rardecode.FileHeader) Entry {
	mode := h.Mode()
	e := Entry{
		RawName:    h.Name,
		IsDir:      h.IsDir,
		Size:       h.UnPackedSize,
		PackedSize: h.PackedSize,
		ModTime:    h.ModificationTime,
		Mode:       mode,
		Attributes: uint32(h.Attributes),
		Encrypted:  h.Encrypted && !h.IsDir,
		Solid:      h.Solid,
	}
	switch h.LinkType {
	case rardecode.LinkTypeUnixSymlink, rardecode.LinkTypeWindowsSymlink, rardecode.LinkTypeWindowsJunction:
		e.Symlink = true
		e.LinkTarget = h.LinkTarget
		e.Mode |= fs.ModeSymlink
	case rardecode.LinkTypeHardLink, rardecode.LinkTypeFileCopy:
		e.HardLink = true
		e.LinkTarget = h.LinkTarget
		e.fileCopy = h.LinkType == rardecode.LinkTypeFileCopy
	default:
		if mode&fs.ModeSymlink != 0 {
			e.Symlink = true
		}
	}
	if h.UnKnownSize {
		e.Size = -1
	}
	return e
}

// mapRarErr turns rardecode errors into the package's sentinel errors.
func mapRarErr(err error, pw string, headerEnc bool) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted):
		if pw != "" {
			return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
		}
		return fmt.Errorf("%w (%v)", ErrPasswordRequired, err)
	case errors.Is(err, rardecode.ErrBadPassword):
		return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
	case errors.Is(err, rardecode.ErrBadFileChecksum):
		if pw != "" {
			return fmt.Errorf("%w (wrong password?)", ErrCRC)
		}
		return ErrCRC
	case headerEnc && pw != "" && (errors.Is(err, rardecode.ErrBadHeaderCRC) || errors.Is(err, rardecode.ErrCorruptBlockHeader)):
		// RAR 4 has no password check value: a wrong key turns the
		// encrypted headers into garbage.
		return fmt.Errorf("%w (%v)", ErrWrongPassword, err)
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(err, fs.ErrNotExist) {
		return &MissingVolumeError{Path: pe.Path, Err: err}
	}
	if errors.Is(err, rardecode.ErrMultiVolume) {
		return &MissingVolumeError{Path: "", Err: err}
	}
	return err
}

func (r *rarArchive) list() []Entry { return append([]Entry(nil), r.entries...) }

func (r *rarArchive) current() ([]*rardecode.File, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.files, r.password
}

func (r *rarArchive) wrapReader(ctx context.Context, rd io.Reader, c io.Closer, i int, pw string) io.ReadCloser {
	e := r.entries[i]
	return &checkedReader{ctx: ctx, r: rd, c: c, size: e.Size, encrypted: e.Encrypted,
		mapErr: func(err error) error { return mapRarErr(err, pw, r.headerEnc) }}
}

func (r *rarArchive) open(ctx context.Context, i int) (io.ReadCloser, error) {
	files, pw := r.current()
	if i < 0 || i >= len(files) {
		return nil, ErrNotFound
	}
	f, err := files[i].Open()
	if err == nil {
		return r.wrapReader(ctx, f, f, i, pw), nil
	}
	if !errors.Is(err, rardecode.ErrSolidOpen) {
		return nil, mapRarErr(err, pw, r.headerEnc)
	}
	// Solid file: stream from the start of the archive.
	rc, err := rardecode.OpenReader(r.path, rarOptions(r.path, r.det, pw)...)
	if err != nil {
		return nil, mapRarErr(err, pw, r.headerEnc)
	}
	for n := 0; ; n++ {
		if err := ctx.Err(); err != nil {
			rc.Close()
			return nil, err
		}
		if _, err := rc.Next(); err != nil {
			rc.Close()
			if err == io.EOF {
				return nil, ErrNotFound
			}
			return nil, mapRarErr(err, pw, r.headerEnc)
		}
		if n == i {
			return r.wrapReader(ctx, rc, rc, i, pw), nil
		}
	}
}

func (r *rarArchive) walk(ctx context.Context, want func(i int) bool, fn func(i int, rd io.Reader) error) error {
	_, pw := r.current()
	rc, err := rardecode.OpenReader(r.path, rarOptions(r.path, r.det, pw)...)
	if err != nil {
		return mapRarErr(err, pw, r.headerEnc)
	}
	defer rc.Close()
	last := -1
	for i := len(r.entries) - 1; i >= 0; i-- {
		if want(i) {
			last = i
			break
		}
	}
	for i := 0; i <= last; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := rc.Next(); err != nil {
			if err == io.EOF {
				return fmt.Errorf("sevenrar: archive ended after %d of %d entries", i, len(r.entries))
			}
			err = mapRarErr(err, pw, r.headerEnc)
			if want(i) {
				if ferr := fn(i, errReader{err}); ferr != nil {
					return ferr
				}
			}
			// The stream position is lost after a header error.
			return err
		}
		if !want(i) {
			continue
		}
		if err := fn(i, r.wrapReader(ctx, rc, nil, i, pw)); err != nil {
			return err
		}
	}
	return nil
}

func (r *rarArchive) setPassword(pw string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pw == r.password {
		return nil
	}
	files, err := rardecode.List(r.path, rarOptions(r.path, r.det, pw)...)
	if err != nil {
		return mapRarErr(err, pw, r.headerEnc)
	}
	r.files, r.password = files, pw
	return nil
}

// verifyPassword reads the start of entry i. RAR 5 stores a password check
// value, so a wrong password fails at once; for RAR 4 small files are read
// completely so the checksum can catch it.
func (r *rarArchive) verifyPassword(ctx context.Context, i int) error {
	rd, err := r.open(ctx, i)
	if err != nil {
		return asWrongPassword(err)
	}
	defer rd.Close()
	limit := int64(64 << 10)
	if r.entries[i].Size >= 0 && r.entries[i].Size <= 1<<20 {
		limit = r.entries[i].Size + 1
	}
	_, err = io.Copy(io.Discard, io.LimitReader(rd, limit))
	return asWrongPassword(err)
}

func (r *rarArchive) close() error { return nil }
