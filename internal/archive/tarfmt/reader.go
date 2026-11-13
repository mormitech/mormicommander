// This file opens tar-family archives for reading. Tar has no central
// directory, so the only way to know what is inside is to decompress and
// walk the whole stream once; the Reader therefore lists lazily, caches the
// result, and works on an io.ReaderAt so it can restart the stream from the
// beginning whenever it needs the data again (extracting, viewing one file,
// testing). Folders that the archive only implies (it stores "a/b/c.txt" but
// no "a/" entry) are added to the listing so the panel can browse the
// archive like a normal folder tree, and when the same name is stored twice
// (tar's way of updating a file) the later copy wins, as with GNU tar.
package tarfmt

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/archive"
)

// ErrFormat is returned when a file is not an archive this package handles.
var ErrFormat = errors.New("tarfmt: not a tar, gz, bz2, xz, zst or lzma archive")

// Reader is an opened tar-family archive. It is safe for concurrent use;
// every operation runs its own pass over the data.
type Reader struct {
	ra      io.ReaderAt
	size    int64
	closer  io.Closer
	name    string
	modTime time.Time
	format  Format

	mu      sync.Mutex
	listed  bool
	entries []archive.Entry
	index   map[string]int         // path -> position in entries
	headers map[string]*tar.Header // last header stored for each path
	occ     map[string]int         // how many times each path is stored
	inner   string                 // single-file formats: the inner name
}

var _ archive.Reader = (*Reader)(nil)

// Open opens the archive file at p. The format is taken from the content
// first and the file name second, so a mis-named file still opens.
func Open(p string) (*Reader, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if fi.IsDir() {
		f.Close()
		return nil, ErrFormat
	}
	r, err := NewReader(f, fi.Size(), filepath.Base(p), fi.ModTime())
	if err != nil {
		f.Close()
		return nil, err
	}
	r.closer = f
	return r, nil
}

// NewReader opens an archive held in any io.ReaderAt, for example a file
// extracted from another archive (nested archives). name is used for format
// hints and the inner name of single-file archives; modTime is used as the
// date of a single-file archive's content when the format stores none.
func NewReader(ra io.ReaderAt, size int64, name string, modTime time.Time) (*Reader, error) {
	r := &Reader{ra: ra, size: size, name: name, modTime: modTime}
	f, err := r.detect()
	if err != nil {
		return nil, err
	}
	r.format = f
	return r, nil
}

// Format returns the detected format.
func (r *Reader) Format() Format { return r.format }

// Size returns the size of the archive file itself.
func (r *Reader) Size() int64 { return r.size }

// Close releases the underlying file when the Reader was made by Open.
func (r *Reader) Close() error {
	if r.closer != nil {
		err := r.closer.Close()
		r.closer = nil
		return err
	}
	return nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// detect works out the format from magic bytes, the decompressed content and
// the name, in that order of trust.
func (r *Reader) detect() (Format, error) {
	byName := DetectFormat(r.name)
	head := make([]byte, 512)
	n, err := r.ra.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return FormatUnknown, err
	}
	head = head[:n]
	comp, ok := sniffCompression(head)
	if !ok {
		if looksLikeTar(head) {
			return FormatTar, nil
		}
		switch {
		case byName == FormatTar && (n == 0 || allZero(head)):
			return FormatTar, nil // empty archive
		case byName.Compression() == CompLzma:
			comp = CompLzma
		default:
			return FormatUnknown, ErrFormat
		}
	}
	dec, err := decompress(comp, io.NewSectionReader(r.ra, 0, r.size))
	if err != nil {
		return FormatUnknown, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	defer dec.Close()
	block := make([]byte, 512)
	m, err := io.ReadFull(dec, block)
	// The single-file format with the compression actually found.
	var single Format
	for f := FormatGz; f <= FormatLzma; f++ {
		if f.Compression() == comp {
			single = f
		}
	}
	switch {
	case err == nil && looksLikeTar(block):
		return single.withTar(), nil
	case err == nil && allZero(block) && byName.IsTar():
		return single.withTar(), nil
	case (err == io.EOF || err == io.ErrUnexpectedEOF) && m == 0 && byName.IsTar():
		return single.withTar(), nil
	case err != nil && err != io.EOF && err != io.ErrUnexpectedEOF:
		return FormatUnknown, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	return single, nil
}

// countReader sits between the raw archive and the decompressor. It stops
// the pass as soon as the context is cancelled and counts how much of the
// archive has been consumed, for progress bars.
type countReader struct {
	ctx context.Context
	r   io.Reader
	n   int64
}

func (c *countReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// pass is one run over the decompressed data.
type pass struct {
	raw *countReader
	dec io.ReadCloser
}

func (r *Reader) newPass(ctx context.Context) (*pass, error) {
	raw := &countReader{ctx: ctx, r: io.NewSectionReader(r.ra, 0, r.size)}
	dec, err := decompress(r.format.Compression(), raw)
	if err != nil {
		return nil, err
	}
	return &pass{raw: raw, dec: dec}, nil
}

func (p *pass) Close() error { return p.dec.Close() }

// errStop ends a walk early without reporting an error.
var errStop = errors.New("stop")

// walk calls fn for every real member of a tar stream (PAX global headers
// are skipped). The tar reader is positioned at the member's data.
func (r *Reader) walk(ctx context.Context, p *pass, fn func(h *tar.Header, name string, tr *tar.Reader) error) error {
	tr := tar.NewReader(p.dec)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := cleanName(h.Name)
		if name == "" {
			continue
		}
		if err := fn(h, name, tr); err != nil {
			if err == errStop {
				return nil
			}
			return err
		}
	}
}

func isDirHeader(h *tar.Header) bool {
	return h.Typeflag == tar.TypeDir || strings.HasSuffix(h.Name, "/")
}

func isRegHeader(h *tar.Header) bool {
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeRegA, tar.TypeCont, tar.TypeGNUSparse:
		return !strings.HasSuffix(h.Name, "/")
	}
	return false
}

func entryFromHeader(name string, h *tar.Header) archive.Entry {
	e := archive.Entry{
		Path:    name,
		ModTime: h.ModTime,
		Mode:    h.FileInfo().Mode(),
		IsDir:   isDirHeader(h),
	}
	if e.IsDir {
		e.Mode = e.Mode&^fs.ModeType | fs.ModeDir
	}
	if isRegHeader(h) {
		e.Size, e.Packed = h.Size, h.Size
	}
	return e
}

// Entries lists the archive. The first call reads the whole archive; later
// calls return the cached list. Callers must not modify the returned slice.
func (r *Reader) Entries(ctx context.Context) ([]archive.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listed {
		return r.entries, nil
	}
	if err := r.list(ctx); err != nil {
		return nil, err
	}
	r.listed = true
	return r.entries, nil
}

func (r *Reader) list(ctx context.Context) error {
	if !r.format.IsTar() {
		return r.listSingle()
	}
	p, err := r.newPass(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	var entries []archive.Entry
	index := map[string]int{}
	headers := map[string]*tar.Header{}
	occ := map[string]int{}
	err = r.walk(ctx, p, func(h *tar.Header, name string, _ *tar.Reader) error {
		e := entryFromHeader(name, h)
		occ[name]++
		headers[name] = h
		if i, ok := index[name]; ok {
			entries[i] = e
		} else {
			index[name] = len(entries)
			entries = append(entries, e)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Add folders that are only implied by deeper names.
	// Names that climb out ("../x") get no implied parents.
	for i := 0; i < len(entries); i++ {
		if !isLocalName(entries[i].Path) {
			continue
		}
		for dir := path.Dir(entries[i].Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if _, ok := index[dir]; ok {
				break
			}
			index[dir] = len(entries)
			entries = append(entries, archive.Entry{Path: dir, IsDir: true, Mode: fs.ModeDir | 0o755})
		}
	}
	r.entries, r.index, r.headers, r.occ = entries, index, headers, occ
	return nil
}

// listSingle builds the one-entry listing of a .gz/.bz2/.xz/.zst/.lzma file.
// Only gzip records the unpacked size (modulo 4 GiB, in its trailer); for
// the others Size is -1 rather than decompressing everything just to list.
func (r *Reader) listSingle() error {
	name := trimExt(r.name)
	mod := r.modTime
	size := int64(-1)
	if r.format.Compression() == CompGzip {
		if zr, err := gzip.NewReader(io.NewSectionReader(r.ra, 0, r.size)); err == nil {
			if hn := cleanName(zr.Name); hn != "" {
				name = path.Base(hn)
			}
			if !zr.ModTime.IsZero() {
				mod = zr.ModTime
			}
			zr.Close()
		}
		if r.size >= 18 {
			var tail [4]byte
			if _, err := r.ra.ReadAt(tail[:], r.size-4); err == nil {
				size = int64(binary.LittleEndian.Uint32(tail[:]))
			}
		}
	}
	if name == "" || !isLocalName(name) {
		name = "data"
	}
	r.inner = name
	r.entries = []archive.Entry{{Path: name, Size: size, Packed: r.size, ModTime: mod, Mode: 0o644}}
	r.index = map[string]int{name: 0}
	return nil
}

// Stat returns the entry with the given path.
func (r *Reader) Stat(ctx context.Context, name string) (archive.Entry, error) {
	if _, err := r.Entries(ctx); err != nil {
		return archive.Entry{}, err
	}
	name = cleanName(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.index[name]
	if !ok {
		return archive.Entry{}, &archive.EntryError{Name: name, Err: archive.ErrNotFound}
	}
	return r.entries[i], nil
}

// ReadDir lists the direct children of an archive folder ("" is the root),
// sorted by name, which is what a panel showing the archive needs.
func (r *Reader) ReadDir(ctx context.Context, dir string) ([]archive.Entry, error) {
	all, err := r.Entries(ctx)
	if err != nil {
		return nil, err
	}
	dir = cleanName(dir)
	if dir != "" {
		e, err := r.Stat(ctx, dir)
		if err != nil {
			return nil, err
		}
		if !e.IsDir {
			return nil, &archive.EntryError{Name: dir, Err: errors.New("not a folder")}
		}
	}
	var out []archive.Entry
	for _, e := range all {
		parent := path.Dir(e.Path)
		if parent == "." {
			parent = ""
		}
		if parent == dir && isLocalName(e.Path) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Header returns a copy of the tar header of a member, for details the
// common Entry does not carry: link target, owner and group, uid/gid,
// PAX records. Implied folders and single-file archives have no header.
func (r *Reader) Header(ctx context.Context, name string) (*tar.Header, error) {
	if _, err := r.Entries(ctx); err != nil {
		return nil, err
	}
	name = cleanName(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.headers[name]
	if !ok {
		return nil, &archive.EntryError{Name: name, Err: archive.ErrNotFound}
	}
	c := *h
	return &c, nil
}

// readCloser bundles a member's data with the pass that produces it.
type readCloser struct {
	io.Reader
	p *pass
}

func (rc *readCloser) Close() error { return rc.p.Close() }

// OpenEntry streams the content of one file, for the viewer, for "info on
// packed file" or to open an archive nested inside this one. Hard links are
// followed to the data they share. The caller must close the result.
func (r *Reader) OpenEntry(ctx context.Context, name string) (io.ReadCloser, error) {
	e, err := r.Stat(ctx, name)
	if err != nil {
		return nil, err
	}
	if e.IsDir {
		return nil, &archive.EntryError{Name: e.Path, Err: errors.New("is a folder")}
	}
	p, err := r.newPass(ctx)
	if err != nil {
		return nil, err
	}
	if !r.format.IsTar() {
		return &readCloser{Reader: p.dec, p: p}, nil
	}
	return r.openTarMember(ctx, p, e.Path, 0)
}

func (r *Reader) openTarMember(ctx context.Context, p *pass, name string, depth int) (io.ReadCloser, error) {
	r.mu.Lock()
	want := r.occ[name]
	r.mu.Unlock()
	var found *tar.Header
	var data io.Reader
	seen := 0
	err := r.walk(ctx, p, func(h *tar.Header, n string, tr *tar.Reader) error {
		if n != name {
			return nil
		}
		seen++
		if seen < want {
			return nil
		}
		found, data = h, tr
		return errStop
	})
	if err != nil {
		p.Close()
		return nil, err
	}
	if found == nil {
		p.Close()
		return nil, &archive.EntryError{Name: name, Err: archive.ErrNotFound}
	}
	switch {
	case isRegHeader(found):
		return &readCloser{Reader: data, p: p}, nil
	case found.Typeflag == tar.TypeLink && depth < 8:
		p.Close()
		target := cleanName(found.Linkname)
		np, err := r.newPass(ctx)
		if err != nil {
			return nil, err
		}
		return r.openTarMember(ctx, np, target, depth+1)
	}
	p.Close()
	return nil, &archive.EntryError{Name: name, Err: errors.New("not a regular file")}
}

// Test reads every byte of the archive, which makes the decompressor check
// its checksums (gzip CRC32, xz CRC64, bzip2 block CRCs, zstd frame
// checksums) and the tar reader check the structure. A nil result means the
// archive is intact; progress, when set, receives regular updates.
func (r *Reader) Test(ctx context.Context, progress func(archive.Progress)) error {
	p, err := r.newPass(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	prog := progressFn(progress, r.size, p.raw)
	if r.format.IsTar() {
		err = r.walk(ctx, p, func(h *tar.Header, name string, tr *tar.Reader) error {
			prog.file(name)
			_, err := io.Copy(prog.writer(io.Discard), tr)
			return err
		})
		if err != nil {
			return err
		}
	}
	// Drain what is left (tar padding, trailing data) so the codec reaches
	// its end and verifies the final checksum.
	if _, err := io.Copy(prog.writer(io.Discard), p.dec); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		return err
	}
	prog.flush()
	return ctx.Err()
}

// Test opens the archive at p and tests it (TC's Alt+Shift+F9).
func Test(ctx context.Context, p string, progress func(archive.Progress)) error {
	r, err := Open(p)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.Test(ctx, progress)
}

// progress throttles callbacks to roughly every 256 KiB.
type progress struct {
	fn      func(archive.Progress)
	p       archive.Progress
	raw     *countReader
	lastRep int64
}

func progressFn(fn func(archive.Progress), size int64, raw *countReader) *progress {
	return &progress{fn: fn, raw: raw, p: archive.Progress{ArchiveSize: size}}
}

func (pr *progress) file(name string) {
	pr.p.Name = name
	pr.p.Files++
	pr.report(true)
}

func (pr *progress) add(n int64) {
	pr.p.Bytes += n
	pr.report(false)
}

func (pr *progress) report(force bool) {
	if pr.fn == nil {
		return
	}
	if !force && pr.p.Bytes-pr.lastRep < 256<<10 {
		return
	}
	pr.lastRep = pr.p.Bytes
	if pr.raw != nil {
		pr.p.ArchiveRead = pr.raw.n
	}
	pr.fn(pr.p)
}

func (pr *progress) flush() { pr.report(true) }

type progressWriter struct {
	w  io.Writer
	pr *progress
}

func (pw progressWriter) Write(b []byte) (int, error) {
	n, err := pw.w.Write(b)
	pw.pr.add(int64(n))
	return n, err
}

func (pr *progress) writer(w io.Writer) io.Writer { return progressWriter{w: w, pr: pr} }
