// This file is the public face of the 7z/RAR engine: Open an archive, browse
// it like a folder, read single files from it and (in extract.go) unpack or
// test it. Total Commander shows a 7z or RAR file as a folder when the user
// presses Enter on it, lets him copy files out with F5, view them with F3 and
// test the archive with Alt+Shift+F9; the UI does all of that through the
// Archive type below and never sees the two underlying libraries.
//
// Both formats are read-only here (creating 7z or RAR needs the external
// tools, see FEATURES.md F-607/F-608). Encrypted archives are handled the way
// Total Commander handles them: if even the file list is encrypted the
// password is asked for when opening; if only the file contents are
// encrypted the archive can be browsed freely and the password is asked for
// the first time data has to be read. The question is put to the caller
// through Options.PasswordFunc, so the UI can show its own dialog, and a
// wrong password is asked for again a few times before giving up.

package sevenrar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry describes one file, folder or link in the archive.
type Entry struct {
	// Path is the clean, slash-separated path inside the archive (no
	// leading slash, no drive letter, no "." components). It is the key
	// for Stat, Open, ReadDir and ExtractOptions.Paths.
	Path string
	// Name is the last element of Path.
	Name string
	// RawName is the name exactly as stored in the archive.
	RawName string
	IsDir   bool
	// Size is the unpacked size in bytes.
	Size int64
	// PackedSize is the compressed size, or -1 when the format does not
	// keep it per file (7z packs many files into one stream).
	PackedSize int64
	ModTime    time.Time
	// Mode holds the permission bits and the type (dir, symlink).
	Mode fs.FileMode
	// Attributes are the raw host attributes (DOS attributes for archives
	// made on Windows, Unix mode bits in the high word for 7z, Unix mode
	// for RAR archives made on Unix).
	Attributes uint32
	// CRC32 is the stored checksum when HasCRC is true.
	CRC32  uint32
	HasCRC bool
	// Encrypted is true when the contents need a password.
	Encrypted bool
	// Solid is true for files in a solid block (RAR solid archives, and 7z
	// files sharing a compressed stream with other files). Reading a single
	// solid file means decompressing the files before it.
	Solid bool
	// Symlink is true for symbolic links (and Windows junctions);
	// LinkTarget is where they point.
	Symlink    bool
	LinkTarget string
	// HardLink is true for RAR hard links and file copies, whose
	// LinkTarget names another entry in the archive.
	HardLink bool
	// Unsafe is true when the name contains ".." (or a NUL byte) and could
	// escape the target folder. Unsafe entries are listed by Entries but
	// not shown in ReadDir and never extracted.
	Unsafe bool
	// Synthetic is true for folders that are not stored in the archive but
	// implied by the paths of the files in it.
	Synthetic bool
	index     int  // position in the backend's list, -1 for synthetic folders
	fileCopy  bool // RAR "file copy" link: copy the data instead of hard linking
}

// Ratio returns the packed size as a percentage of the unpacked size (the
// "ratio" column of Total Commander's packed file info), or -1 when unknown.
func (e Entry) Ratio() int {
	if e.PackedSize < 0 || e.Size <= 0 {
		return -1
	}
	return int((e.PackedSize*100 + e.Size/2) / e.Size)
}

// PasswordRequest is passed to Options.PasswordFunc.
type PasswordRequest struct {
	Archive string // path of the archive (first volume)
	// Entry is the first encrypted entry that needs the password, or ""
	// when the file list itself is encrypted.
	Entry string
	// HeadersEncrypted is true when the password is needed just to list
	// the archive.
	HeadersEncrypted bool
	// Attempt counts from 1; Attempt > 1 means the previous password was
	// wrong, so the dialog can say so.
	Attempt int
}

// Options control Open.
type Options struct {
	// Password is tried first, before asking PasswordFunc.
	Password string
	// PasswordFunc is asked for a password when one is needed. It returns
	// ok=false when the user cancelled. When nil, ErrPasswordRequired is
	// returned instead of asking.
	PasswordFunc func(PasswordRequest) (password string, ok bool)
	// MaxPasswordAttempts bounds how many times PasswordFunc is asked
	// after a wrong password (default 3).
	MaxPasswordAttempts int
}

// Info summarises an archive (Total Commander's archive properties).
type Info struct {
	Path             string // first volume
	Format           Format
	RarVersion       int
	Volumes          []string
	SFX              bool
	Solid            bool
	HeadersEncrypted bool
	Encrypted        bool // some entries have encrypted contents
	Files, Dirs      int
	UnpackedSize     int64
	PackedSize       int64 // total size of all volume files
	Unsafe           int   // entries skipped for unsafe paths
}

// backend is what each format implementation provides.
type backend interface {
	// list returns the entries in archive order (Path/Name/Unsafe are
	// filled in by the Archive).
	list() []Entry
	// open returns the contents of entry i (random access; may have to
	// decompress earlier files of a solid block).
	open(ctx context.Context, i int) (io.ReadCloser, error)
	// walk calls fn for every entry i with want(i) true, in archive
	// order, with a reader for its contents. It is the efficient way to
	// read many files of a solid archive.
	walk(ctx context.Context, want func(i int) bool, fn func(i int, r io.Reader) error) error
	// setPassword reopens the archive with another password.
	setPassword(p string) error
	// verifyPassword checks the current password by reading a little of
	// entry i (an encrypted entry). It returns ErrWrongPassword or nil.
	verifyPassword(ctx context.Context, i int) error
	close() error
}

// Archive is an open 7z or RAR archive. Its methods are safe for concurrent
// use; reads that need a password are serialised while it is asked for.
type Archive struct {
	path      string // first volume
	det       Detection
	opts      Options
	volumes   []string
	mu        sync.Mutex
	pwMu      sync.Mutex // held while asking for / verifying a password
	be        backend
	password  string
	pwOK      bool // the password has been verified (or is not needed)
	headerEnc bool
	entries   []Entry
	byPath    map[string]int   // clean path -> index into all
	children  map[string][]int // folder path -> indexes into all
	all       []Entry          // entries plus synthetic folders
	closed    bool
}

// Open opens the 7z or RAR archive at path. If path is a later volume of a
// multi-volume set, the set is opened from its first volume. Files of other
// formats give an error wrapping ErrUnsupportedFormat; use DetectFile to
// find out what they are.
func Open(path string, opts Options) (*Archive, error) {
	return OpenContext(context.Background(), path, opts)
}

// OpenContext is Open with a context, which can cancel the password prompt
// loop.
func OpenContext(ctx context.Context, path string, opts Options) (*Archive, error) {
	if opts.MaxPasswordAttempts <= 0 {
		opts.MaxPasswordAttempts = 3
	}
	first := FirstVolume(path)
	det, err := DetectFile(first)
	if err != nil {
		if first != path {
			return nil, &MissingVolumeError{Path: first, Err: err}
		}
		return nil, err
	}
	if !det.Supported() {
		return nil, fmt.Errorf("%w: %s is %s", ErrUnsupportedFormat, filepath.Base(path), det.Format)
	}
	vols, err := FindVolumes(first)
	if err != nil {
		return nil, err
	}
	a := &Archive{path: first, det: det, opts: opts, volumes: vols, password: opts.Password}

	openBackend := func(pw string) (backend, error) {
		if det.Format == Format7z {
			return open7z(first, det, pw)
		}
		return openRar(first, det, pw)
	}

	be, err := openBackend(a.password)
	tried := 0 // passwords tried so far
	if a.password != "" {
		tried = 1
	}
	for asked := 0; errors.Is(err, ErrPasswordRequired) || errors.Is(err, ErrWrongPassword); asked++ {
		a.headerEnc = true
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if opts.PasswordFunc == nil {
			return nil, err
		}
		if asked >= opts.MaxPasswordAttempts {
			return nil, ErrWrongPassword
		}
		pw, ok := opts.PasswordFunc(PasswordRequest{Archive: first, HeadersEncrypted: true, Attempt: tried + 1})
		if !ok {
			return nil, ErrPasswordCancelled
		}
		tried++
		a.password = pw
		be, err = openBackend(pw)
	}
	if err != nil {
		return nil, err
	}
	a.be = be
	if det.HeadersEncrypted {
		a.headerEnc = true
	}
	if r, ok := be.(*rarArchive); ok && r.headerEnc {
		a.headerEnc = true
	}
	if z, ok := be.(*sevenZip); ok && z.headerEnc {
		a.headerEnc = true
	}
	// When the list was encrypted, the password that opened it is the
	// right one for the contents too.
	a.pwOK = a.headerEnc && a.password != ""
	a.build(be.list())
	return a, nil
}

// build fills the path index and the folder tree.
func (a *Archive) build(list []Entry) {
	a.entries = list
	a.byPath = map[string]int{}
	a.children = map[string][]int{}
	a.all = nil
	for i := range a.entries {
		e := &a.entries[i]
		e.index = i
		clean, unsafe := cleanEntryName(e.RawName)
		e.Path, e.Unsafe = clean, unsafe
		e.Name = path.Base(clean)
		if clean == "" {
			e.Name = ""
		}
	}
	var ensureDir func(p string)
	ensureDir = func(p string) {
		if p == "." || p == "" {
			return
		}
		if _, ok := a.byPath[p]; ok {
			return
		}
		parent := path.Dir(p)
		ensureDir(parent)
		a.all = append(a.all, Entry{Path: p, Name: path.Base(p), RawName: p, IsDir: true,
			Mode: fs.ModeDir | 0o755, PackedSize: -1, Synthetic: true, index: -1})
		idx := len(a.all) - 1
		a.byPath[p] = idx
		a.children[dirKey(parent)] = append(a.children[dirKey(parent)], idx)
	}
	for _, e := range a.entries {
		if e.Unsafe || e.Path == "" {
			continue
		}
		parent := path.Dir(e.Path)
		ensureDir(parent)
		if old, ok := a.byPath[e.Path]; ok {
			// A later entry with the same path replaces the earlier
			// one (updated files, or a stored folder after an implied
			// one), like the last write wins when extracting.
			a.all[old] = e
			continue
		}
		a.all = append(a.all, e)
		idx := len(a.all) - 1
		a.byPath[e.Path] = idx
		a.children[dirKey(parent)] = append(a.children[dirKey(parent)], idx)
	}
	for k := range a.children {
		c := a.children[k]
		sort.Slice(c, func(i, j int) bool { return a.all[c[i]].Name < a.all[c[j]].Name })
	}
}

func dirKey(p string) string {
	if p == "." {
		return ""
	}
	return p
}

// Path returns the path of the archive (its first volume).
func (a *Archive) Path() string { return a.path }

// Format returns Format7z or FormatRar.
func (a *Archive) Format() Format { return a.det.Format }

// Detection returns what the detector found out about the archive.
func (a *Archive) Detection() Detection { return a.det }

// Volumes returns the volume files of the archive in order (just the archive
// file itself for single-volume archives).
func (a *Archive) Volumes() []string { return append([]string(nil), a.volumes...) }

// HeadersEncrypted reports whether the file list itself is encrypted.
func (a *Archive) HeadersEncrypted() bool { return a.headerEnc }

// Encrypted reports whether any entry has encrypted contents.
func (a *Archive) Encrypted() bool {
	for _, e := range a.entries {
		if e.Encrypted {
			return true
		}
	}
	return false
}

// Entries returns every entry stored in the archive, in archive order,
// including unsafe ones (but not synthetic folders).
func (a *Archive) Entries() []Entry { return append([]Entry(nil), a.entries...) }

// Info returns a summary of the archive.
func (a *Archive) Info() Info {
	in := Info{Path: a.path, Format: a.det.Format, RarVersion: a.det.RarVersion, Volumes: a.Volumes(),
		SFX: a.det.SFX, Solid: a.det.Solid, HeadersEncrypted: a.headerEnc, Encrypted: a.Encrypted()}
	for _, e := range a.entries {
		switch {
		case e.Unsafe:
			in.Unsafe++
		case e.IsDir:
			in.Dirs++
		default:
			in.Files++
			in.UnpackedSize += e.Size
		}
		if e.Solid {
			in.Solid = true
		}
	}
	for _, v := range a.volumes {
		if st, err := statSize(v); err == nil {
			in.PackedSize += st
		}
	}
	return in
}

// Stat returns the entry at the clean path p ("" is the root folder).
func (a *Archive) Stat(p string) (Entry, error) {
	p = normalizeQuery(p)
	if p == "" {
		return Entry{IsDir: true, Mode: fs.ModeDir | 0o755, PackedSize: -1, Synthetic: true, index: -1}, nil
	}
	i, ok := a.byPath[p]
	if !ok {
		return Entry{}, fmt.Errorf("%w: %s", ErrNotFound, p)
	}
	return a.all[i], nil
}

// ReadDir lists the folder at the clean path p ("" is the root), sorted by
// name, with folders implied by file paths included as Synthetic entries.
func (a *Archive) ReadDir(p string) ([]Entry, error) {
	p = normalizeQuery(p)
	if p != "" {
		e, err := a.Stat(p)
		if err != nil {
			return nil, err
		}
		if !e.IsDir {
			return nil, fmt.Errorf("%w: %s", ErrNotDir, p)
		}
	}
	idx := a.children[p]
	out := make([]Entry, len(idx))
	for i, j := range idx {
		out[i] = a.all[j]
	}
	return out, nil
}

// Walk calls fn for every browsable entry (files, folders and synthetic
// folders, without unsafe entries) below the folder p, depth first in name
// order. It is what "find files inside archives" and "show as branch" use.
func (a *Archive) Walk(p string, fn func(Entry) error) error {
	list, err := a.ReadDir(p)
	if err != nil {
		return err
	}
	for _, e := range list {
		if err := fn(e); err != nil {
			return err
		}
		if e.IsDir {
			if err := a.Walk(e.Path, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeQuery(p string) string {
	c, _ := cleanEntryName(p)
	return c
}

// SetPassword sets the password used for encrypted contents and checks it
// against the first encrypted entry. It returns ErrWrongPassword when it is
// rejected. Use it when the UI asks for the password itself rather than
// through Options.PasswordFunc.
func (a *Archive) SetPassword(pw string) error {
	a.pwMu.Lock()
	defer a.pwMu.Unlock()
	return a.trySetPassword(context.Background(), pw, a.firstEncrypted(nil))
}

func (a *Archive) trySetPassword(ctx context.Context, pw string, probe int) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return ErrClosed
	}
	if err := a.be.setPassword(pw); err != nil {
		a.mu.Unlock()
		return err
	}
	a.password = pw
	a.pwOK = false
	be := a.be
	a.mu.Unlock()
	if probe < 0 {
		a.pwOK = true
		return nil
	}
	if err := be.verifyPassword(ctx, probe); err != nil {
		return err
	}
	a.pwOK = true
	return nil
}

// firstEncrypted returns the backend index of the first encrypted entry
// (among those selected by want, or all when want is nil), or -1.
func (a *Archive) firstEncrypted(want func(i int) bool) int {
	for i, e := range a.entries {
		if e.Encrypted && !e.IsDir && (want == nil || want(i)) {
			return i
		}
	}
	return -1
}

// ensurePassword makes sure a verified password is set before entries
// selected by want are read, asking PasswordFunc as needed.
func (a *Archive) ensurePassword(ctx context.Context, want func(i int) bool) error {
	probe := a.firstEncrypted(want)
	if probe < 0 {
		return nil
	}
	a.pwMu.Lock()
	defer a.pwMu.Unlock()
	if a.pwOK {
		return nil
	}
	tried := 0
	if a.password != "" {
		err := a.trySetPassword(ctx, a.password, probe)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrWrongPassword) {
			return err
		}
		tried = 1
	}
	if a.opts.PasswordFunc == nil {
		if tried > 0 {
			return ErrWrongPassword
		}
		return &EntryError{Path: a.entries[probe].Path, Err: ErrPasswordRequired}
	}
	for asked := 0; asked < a.opts.MaxPasswordAttempts; asked++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		pw, ok := a.opts.PasswordFunc(PasswordRequest{Archive: a.path, Entry: a.entries[probe].Path, Attempt: tried + 1})
		if !ok {
			return ErrPasswordCancelled
		}
		tried++
		err := a.trySetPassword(ctx, pw, probe)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrWrongPassword) {
			return err
		}
	}
	return ErrWrongPassword
}

// Open returns a reader for the contents of the file at the clean path p, for
// viewing (F3) or copying a single file. The data is checked against the
// stored checksum: the final Read returns an error wrapping ErrCRC if it does
// not match. For a solid archive this decompresses everything before the
// file, so use Extract for many files.
func (a *Archive) Open(p string) (io.ReadCloser, error) {
	return a.OpenContext(context.Background(), p)
}

// OpenContext is Open with a context for the password prompt and for the
// skipping of earlier files in solid archives.
func (a *Archive) OpenContext(ctx context.Context, p string) (io.ReadCloser, error) {
	e, err := a.Stat(p)
	if err != nil {
		return nil, err
	}
	if e.IsDir {
		return nil, fmt.Errorf("%w: %s", ErrIsDir, p)
	}
	if err := a.ensurePassword(ctx, func(i int) bool { return i == e.index }); err != nil {
		return nil, err
	}
	be, err := a.backend()
	if err != nil {
		return nil, err
	}
	rc, err := be.open(ctx, e.index)
	if err != nil {
		return nil, &EntryError{Path: e.Path, Err: err}
	}
	return rc, nil
}

func (a *Archive) backend() (backend, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, ErrClosed
	}
	return a.be, nil
}

// Close releases the archive. It is safe to call more than once.
func (a *Archive) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	return a.be.close()
}

// pathSelected reports whether the clean entry path p is selected by the
// list of clean selection paths (a selected folder selects everything below
// it). An empty list selects everything.
func pathSelected(p string, sel []string) bool {
	if len(sel) == 0 {
		return true
	}
	for _, s := range sel {
		if s == "" || p == s || strings.HasPrefix(p, s+"/") {
			return true
		}
	}
	return false
}
