// Lazy file access for the Lister. Total Commander opens multi-gigabyte
// files instantly because it never reads them whole: it only reads the part
// on screen. A File here does the same with positioned reads (ReadAt) through
// a small cache of 64 KiB blocks, so scrolling back and forth over the same
// screenful costs no disk access, memory stays at a few megabytes whatever
// the file size, and the same code works on every platform (memory mapping
// would need per-OS code and breaks when a followed log file is truncated).
// Reload re-checks the size on disk, which is what "follow tail" and the F2
// reload key are built on. Any io.ReaderAt (an archive entry, a buffer) can
// be wrapped as a Source too.

package lister

import (
	"container/list"
	"errors"
	"io"
	"os"
	"sync"
)

// Source is what the Lister reads from: random access plus a known size.
type Source interface {
	io.ReaderAt
	Size() int64
}

// Change describes what Reload found on disk.
type Change struct {
	OldSize, NewSize int64
	Grown            bool // the file got longer (new data appended)
	Truncated        bool // the file got shorter, so old offsets may be invalid
	Replaced         bool // the path now points to a different file (log rotation, editor save)
	Modified         bool // the modification time changed without a size change
}

// Changed reports whether anything at all differs.
func (c Change) Changed() bool { return c.Grown || c.Truncated || c.Replaced || c.Modified }

const (
	blockSize = 64 * 1024
	maxBlocks = 64 // 4 MiB of cache per open file
)

type block struct {
	idx  int64
	data []byte
}

// File is a lazily read file on disk. It is safe for concurrent use.
type File struct {
	path string

	mu     sync.Mutex
	f      *os.File // nil while no handle is held (always, between reads, on Windows)
	closed bool
	info   os.FileInfo
	size   int64
	lru    *list.List // of *block, most recently used first
	index  map[int64]*list.Element
}

// Open opens path for lazy reading. Nothing beyond the file header of the
// OS is read until the first ReadAt.
func Open(path string) (*File, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, errors.New("lister: " + path + " is a folder")
	}
	if !keepOpen {
		f.Close()
		f = nil
	}
	return &File{path: path, f: f, info: info, size: info.Size(), lru: list.New(), index: map[int64]*list.Element{}}, nil
}

// Path returns the path the file was opened with.
func (f *File) Path() string { return f.path }

// Size returns the size seen at Open or at the last Reload.
func (f *File) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.size
}

// Info returns the file information seen at Open or at the last Reload.
func (f *File) Info() os.FileInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info
}

// Close releases the file handle.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	var err error
	if f.f != nil {
		err = f.f.Close()
		f.f = nil
	}
	f.lru.Init()
	clear(f.index)
	return err
}

// ReadAt reads len(p) bytes at off through the block cache. Reads are
// limited to the size known since the last Reload, so a file that grows
// while it is shown does not change under the reader until it asks.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("lister: negative offset")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	lh := lazyHandle{f: f}
	defer lh.release()
	n := 0
	for n < len(p) {
		pos := off + int64(n)
		if pos >= f.size {
			return n, io.EOF
		}
		b, err := f.blockLocked(pos/blockSize, &lh)
		if err != nil {
			return n, err
		}
		in := int(pos % blockSize)
		if in >= len(b) {
			return n, io.EOF
		}
		c := copy(p[n:], b[in:])
		n += c
	}
	return n, nil
}

// readDirect bypasses the cache; long sequential scans (line counting,
// search) use it so they do not evict the blocks on screen.
func (f *File) readDirect(p []byte, off int64) (int, error) {
	f.mu.Lock()
	h, size, closed := f.f, f.size, f.closed
	f.mu.Unlock()
	if closed {
		return 0, os.ErrClosed
	}
	if off >= size {
		return 0, io.EOF
	}
	if h == nil {
		nh, err := openShared(f.path)
		if err != nil {
			return 0, err
		}
		defer nh.Close()
		h = nh
	}
	if rem := size - off; int64(len(p)) > rem {
		p = p[:rem]
		n, err := h.ReadAt(p, off)
		if err == nil {
			err = io.EOF
		}
		return n, err
	}
	return h.ReadAt(p, off)
}

func (f *File) blockLocked(idx int64, lh *lazyHandle) ([]byte, error) {
	if e, ok := f.index[idx]; ok {
		f.lru.MoveToFront(e)
		return e.Value.(*block).data, nil
	}
	h, err := lh.get()
	if err != nil {
		return nil, err
	}
	start := idx * blockSize
	n := int64(blockSize)
	if start+n > f.size {
		n = f.size - start
	}
	var buf []byte
	if f.lru.Len() >= maxBlocks {
		e := f.lru.Back()
		old := e.Value.(*block)
		delete(f.index, old.idx)
		f.lru.Remove(e)
		buf = old.data[:cap(old.data)]
	} else {
		buf = make([]byte, blockSize)
	}
	buf = buf[:n]
	m, err := h.ReadAt(buf, start)
	if m == 0 && err != nil {
		return nil, err
	}
	buf = buf[:m] // a short block means the file shrank under us; keep what exists
	f.index[idx] = f.lru.PushFront(&block{idx: idx, data: buf})
	return buf, nil
}

// Reload checks the file on disk again. It notices growth (new data at the
// end), truncation, a changed modification time and replacement of the file
// by a new one (log rotation, or an editor that saves via rename), reopening
// the path in the last case. The cache is dropped whenever something changed.
func (f *File) Reload() (Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return Change{}, os.ErrClosed
	}
	ch := Change{OldSize: f.size, NewSize: f.size}
	pathInfo, err := os.Stat(f.path)
	if err != nil {
		return ch, err
	}
	if !os.SameFile(pathInfo, f.info) {
		nf, err := openShared(f.path)
		if err != nil {
			return ch, err
		}
		ni, err := nf.Stat()
		if err != nil {
			nf.Close()
			return ch, err
		}
		if f.f != nil {
			f.f.Close()
			f.f = nf
		} else {
			nf.Close() // Windows: hold no handle between reads
		}
		f.info, f.size = ni, ni.Size()
		ch.Replaced = true
		ch.NewSize = f.size
	} else {
		lh := lazyHandle{f: f}
		h, err := lh.get()
		if err != nil {
			return ch, err
		}
		ni, err := h.Stat()
		lh.release()
		if err != nil {
			return ch, err
		}
		ch.NewSize = ni.Size()
		ch.Modified = !ni.ModTime().Equal(f.info.ModTime()) && ni.Size() == f.size
		f.info, f.size = ni, ni.Size()
	}
	ch.Grown = ch.NewSize > ch.OldSize
	ch.Truncated = ch.NewSize < ch.OldSize
	if ch.Changed() {
		if ch.Grown && !ch.Replaced && !ch.Modified {
			// Only the last, possibly partial block can be stale.
			last := ch.OldSize / blockSize
			if e, ok := f.index[last]; ok {
				f.lru.Remove(e)
				delete(f.index, last)
			}
		} else {
			f.lru.Init()
			clear(f.index)
		}
	}
	return ch, nil
}

// lazyHandle hands out the File's open handle, or on platforms where none is
// kept between reads opens one on first use; release closes it again. The
// caller holds f.mu.
type lazyHandle struct {
	f   *File
	h   *os.File
	own bool
}

func (l *lazyHandle) get() (*os.File, error) {
	if l.h != nil {
		return l.h, nil
	}
	if l.f.f != nil {
		l.h = l.f.f
		return l.h, nil
	}
	h, err := openShared(l.f.path)
	if err != nil {
		return nil, err
	}
	l.h, l.own = h, true
	return h, nil
}

func (l *lazyHandle) release() {
	if l.own && l.h != nil {
		l.h.Close()
	}
	l.h, l.own = nil, false
}

// memSource wraps an io.ReaderAt with a fixed size.
type memSource struct {
	r    io.ReaderAt
	size int64
}

func (m memSource) ReadAt(p []byte, off int64) (int, error) {
	if off >= m.size {
		return 0, io.EOF
	}
	if rem := m.size - off; int64(len(p)) > rem {
		n, err := m.r.ReadAt(p[:rem], off)
		if err == nil {
			err = io.EOF
		}
		return n, err
	}
	return m.r.ReadAt(p, off)
}

func (m memSource) Size() int64 { return m.size }

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// NewBytesSource shows an in-memory buffer (for example text produced by
// HTMLToText, or a small extracted archive entry).
func NewBytesSource(b []byte) Source { return memSource{bytesReaderAt(b), int64(len(b))} }

// NewReaderSource wraps any random-access reader of known size.
func NewReaderSource(r io.ReaderAt, size int64) Source { return memSource{r, size} }

// readFull reads len(p) bytes at off from src, accepting a short read at the
// end of the data. It returns the number of bytes read.
func readFull(src Source, p []byte, off int64) (int, error) {
	if off >= src.Size() {
		return 0, nil
	}
	var n int
	var err error
	if f, ok := src.(*File); ok && len(p) > blockSize {
		n, err = f.readDirect(p, off)
	} else {
		n, err = src.ReadAt(p, off)
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return n, err
}
