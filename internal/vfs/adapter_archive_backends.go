// This file connects the three archive engines to the archive file system.
// The engines were written independently and each has its own entry type and
// calling style: internal/archive/ziparc implements the rich archive.Archive
// interface and can rewrite zips, internal/archive/tarfmt reads and rewrites
// the whole tar family (tar, tgz, tbz, txz, tzst and the single-file gz, bz2,
// xz, zst, lzma packers), and internal/archive/sevenrar reads 7z and RAR.
// Here each one is wrapped in a tiny arcBackend so ArchiveFS can list, stat,
// open and (where the engine can) modify any of them the same way. Choosing
// the backend follows Total Commander: the content decides (a renamed
// archive still opens, a .jar is a zip), and only when the content says
// nothing, for example for a .lzma stream that has no magic bytes, the file
// name decides.
package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/arcapi"
	"mormicommander/internal/archive/sevenrar"
	"mormicommander/internal/archive/tarfmt"
	"mormicommander/internal/archive/ziparc"
)

// arcEntry is the backend-neutral description of one archive member.
type arcEntry struct {
	Path     string // slash path inside the archive, "" for the root
	Name     string
	Size     int64 // -1 when unknown
	ModTime  time.Time
	Mode     fs.FileMode
	IsDir    bool
	IsLink   bool
	Hidden   bool // DOS hidden attribute
	System   bool // DOS system attribute
	ReadOnly bool // DOS read-only attribute
}

// arcChanges is one batch of edits for arcBackend.update.
type arcChanges struct {
	Delete []string
	Rename [][2]string
	Mkdir  []string
	Add    []arcAdd
}

// arcAdd packs the local file or folder Source as Dest inside the archive.
type arcAdd struct {
	Source string
	Dest   string
}

// arcBackend is what ArchiveFS needs from an engine.
type arcBackend interface {
	format() string
	readDir(ctx context.Context, dir string) ([]arcEntry, error)
	stat(ctx context.Context, p string) (arcEntry, error)
	open(ctx context.Context, p string) (io.ReadCloser, error)
	// writable reports whether update can work on this archive.
	writable() bool
	// update applies ch to the archive file at host. The backend must be
	// closed by the caller first, because the file is replaced.
	update(ctx context.Context, host string, ch arcChanges) error
	close() error
}

// PasswordRequest is passed to ArchiveFS.Password when an encrypted archive
// or entry needs a password.
type PasswordRequest struct {
	Archive string // the archive file (first volume)
	Entry   string // the entry being read, "" when the file list is encrypted
	Attempt int    // 1 for the first question, more after a wrong password
}

// openOptions carries the ArchiveFS settings an engine needs when opening.
type openOptions struct {
	password func(PasswordRequest) (string, bool)
	encoding string
}

// errNotArchive is returned by openBackend for files no engine can read.
var errNotArchive = errors.New("vfs: not a supported archive")

// arcKind names the engine for a file: "zip", "tar", "7z" or "" (none).
func arcKind(file string) string {
	d, err := sevenrar.DetectFile(file)
	if err == nil {
		switch d.Format {
		case sevenrar.Format7z, sevenrar.FormatRar:
			return "7z"
		case sevenrar.FormatZip:
			return "zip"
		case sevenrar.FormatTar, sevenrar.FormatGzip, sevenrar.FormatBzip2, sevenrar.FormatXz, sevenrar.FormatZstd:
			return "tar"
		}
	}
	name := filepath.Base(file)
	switch {
	case tarfmt.DetectFormat(name) != tarfmt.FormatUnknown:
		return "tar"
	case archive.IsArchiveName(name):
		return "zip"
	}
	return ""
}

// openBackend opens the archive file with the engine that can read it.
func openBackend(file string, o openOptions) (arcBackend, error) {
	switch arcKind(file) {
	case "zip":
		opts := archive.OpenOptions{Encoding: o.encoding}
		if o.password != nil {
			opts.Password = func(arc, entry string, attempt int) (string, error) {
				pw, ok := o.password(PasswordRequest{Archive: arc, Entry: entry, Attempt: attempt})
				if !ok {
					return "", archive.ErrCanceled
				}
				return pw, nil
			}
		}
		r, err := ziparc.Open(file, opts)
		if err != nil {
			return nil, err
		}
		return &zipBackend{r: r}, nil
	case "tar":
		r, err := tarfmt.Open(file)
		if err != nil {
			return nil, err
		}
		return &tarBackend{r: r}, nil
	case "7z":
		opts := sevenrar.Options{}
		if o.password != nil {
			opts.PasswordFunc = func(req sevenrar.PasswordRequest) (string, bool) {
				return o.password(PasswordRequest{Archive: req.Archive, Entry: req.Entry, Attempt: req.Attempt})
			}
		}
		a, err := sevenrar.Open(sevenrar.FirstVolume(file), opts)
		if err != nil {
			return nil, err
		}
		return &sevenBackend{a: a}, nil
	}
	return nil, fmt.Errorf("%s: %w", file, errNotArchive)
}

// notExist makes engine-specific "no such entry" errors match
// fs.ErrNotExist, so callers need only one check.
func notExist(err error, p string) error {
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if errors.Is(err, arcapi.ErrNotFound) || errors.Is(err, sevenrar.ErrNotFound) {
		return &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	return err
}

// ctxReader stops a long read from an archive member when ctx is cancelled.
type ctxReader struct {
	ctx context.Context
	io.ReadCloser
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.ReadCloser.Read(p)
}

// ---- zip --------------------------------------------------------------

type zipBackend struct{ r *ziparc.Reader }

func (z *zipBackend) format() string { return "zip" }

func zipEntry(e archive.Entry) arcEntry {
	return arcEntry{
		Path: e.Path, Name: e.Name, Size: e.Size, ModTime: e.ModTime, Mode: e.Mode,
		IsDir: e.IsDir, IsLink: e.IsSymlink(),
		Hidden:   e.DOSAttrs&archive.AttrHidden != 0,
		System:   e.DOSAttrs&archive.AttrSystem != 0,
		ReadOnly: e.DOSAttrs&archive.AttrReadOnly != 0,
	}
}

func (z *zipBackend) readDir(_ context.Context, dir string) ([]arcEntry, error) {
	es, err := z.r.ReadDir(dir)
	if err != nil {
		return nil, notExist(err, dir)
	}
	out := make([]arcEntry, len(es))
	for i, e := range es {
		out[i] = zipEntry(e)
	}
	return out, nil
}

func (z *zipBackend) stat(_ context.Context, p string) (arcEntry, error) {
	e, err := z.r.Stat(p)
	if err != nil {
		return arcEntry{}, notExist(err, p)
	}
	return zipEntry(e), nil
}

func (z *zipBackend) open(ctx context.Context, p string) (io.ReadCloser, error) {
	e, err := z.r.Stat(p)
	if err != nil {
		return nil, notExist(err, p)
	}
	if e.IsDir {
		return nil, ErrIsFolder
	}
	rc, err := z.r.Open(e)
	if err != nil {
		return nil, err
	}
	return ctxReader{ctx, rc}, nil
}

func (z *zipBackend) writable() bool { return !z.r.Spanned() }

func (z *zipBackend) update(ctx context.Context, host string, ch arcChanges) error {
	c := archive.Changes{Delete: ch.Delete, Mkdir: ch.Mkdir}
	for _, r := range ch.Rename {
		c.Rename = append(c.Rename, archive.Rename{From: r[0], To: r[1]})
	}
	for _, a := range ch.Add {
		c.Add = append(c.Add, archive.AddItem{Source: a.Source, Dest: a.Dest})
	}
	_, err := ziparc.Packer{}.Update(ctx, host, c, archive.PackOptions{Policy: archive.PolicyOverwrite})
	return err
}

func (z *zipBackend) close() error { return z.r.Close() }

// ---- tar family -----------------------------------------------------------

type tarBackend struct{ r *tarfmt.Reader }

func (t *tarBackend) format() string { return t.r.Format().String() }

func tarEntry(e arcapi.Entry) arcEntry {
	return arcEntry{
		Path: e.Path, Name: path.Base(e.Path), Size: e.Size, ModTime: e.ModTime, Mode: e.Mode,
		IsDir: e.IsDir, IsLink: e.Mode&fs.ModeSymlink != 0,
	}
}

func (t *tarBackend) readDir(ctx context.Context, dir string) ([]arcEntry, error) {
	es, err := t.r.ReadDir(ctx, dir)
	if err != nil {
		if e, serr := t.r.Stat(ctx, dir); serr == nil && !e.IsDir {
			return nil, ErrNotFolder
		}
		return nil, notExist(err, dir)
	}
	out := make([]arcEntry, len(es))
	for i, e := range es {
		out[i] = tarEntry(e)
	}
	return out, nil
}

func (t *tarBackend) stat(ctx context.Context, p string) (arcEntry, error) {
	if p == "" {
		return arcEntry{IsDir: true, Mode: fs.ModeDir | 0o755}, nil
	}
	e, err := t.r.Stat(ctx, p)
	if err != nil {
		return arcEntry{}, notExist(err, p)
	}
	return tarEntry(e), nil
}

func (t *tarBackend) open(ctx context.Context, p string) (io.ReadCloser, error) {
	e, err := t.r.Stat(ctx, p)
	if err != nil {
		return nil, notExist(err, p)
	}
	if e.IsDir {
		return nil, ErrIsFolder
	}
	rc, err := t.r.OpenEntry(ctx, p)
	if err != nil {
		return nil, err
	}
	return ctxReader{ctx, rc}, nil
}

// writable: tarfmt rewrites real tar archives; single-file packers (.gz of
// one file) hold exactly one member and cannot be edited, as in TC.
func (t *tarBackend) writable() bool { return t.r.Format().IsTar() }

func (t *tarBackend) update(ctx context.Context, host string, ch arcChanges) error {
	c := tarfmt.Changes{Delete: ch.Delete, Mkdir: ch.Mkdir}
	for _, r := range ch.Rename {
		c.Rename = append(c.Rename, tarfmt.Rename{From: r[0], To: r[1]})
	}
	// Every addition names its full target, so the whole batch, deletes
	// and renames included, is one archive rewrite: it lands completely or
	// not at all.
	for _, a := range ch.Add {
		c.AddTo = append(c.AddTo, tarfmt.AddItem{Source: a.Source, Dest: a.Dest})
	}
	return tarfmt.Modify(ctx, host, c)
}

func (t *tarBackend) close() error { return t.r.Close() }

// ---- 7z and RAR (read-only) ---------------------------------------------

type sevenBackend struct{ a *sevenrar.Archive }

func (s *sevenBackend) format() string { return s.a.Format().String() }

func sevenEntry(e sevenrar.Entry) arcEntry {
	return arcEntry{
		Path: e.Path, Name: e.Name, Size: e.Size, ModTime: e.ModTime, Mode: e.Mode,
		IsDir: e.IsDir, IsLink: e.Symlink,
	}
}

func (s *sevenBackend) readDir(_ context.Context, dir string) ([]arcEntry, error) {
	es, err := s.a.ReadDir(dir)
	if err != nil {
		if errors.Is(err, sevenrar.ErrNotDir) {
			return nil, ErrNotFolder
		}
		return nil, notExist(err, dir)
	}
	out := make([]arcEntry, len(es))
	for i, e := range es {
		out[i] = sevenEntry(e)
	}
	return out, nil
}

func (s *sevenBackend) stat(_ context.Context, p string) (arcEntry, error) {
	if p == "" {
		return arcEntry{IsDir: true, Mode: fs.ModeDir | 0o755}, nil
	}
	e, err := s.a.Stat(p)
	if err != nil {
		return arcEntry{}, notExist(err, p)
	}
	return sevenEntry(e), nil
}

func (s *sevenBackend) open(ctx context.Context, p string) (io.ReadCloser, error) {
	e, err := s.a.Stat(p)
	if err != nil {
		return nil, notExist(err, p)
	}
	if e.IsDir {
		return nil, ErrIsFolder
	}
	rc, err := s.a.OpenContext(ctx, p)
	if err != nil {
		return nil, err
	}
	return ctxReader{ctx, rc}, nil
}

func (s *sevenBackend) writable() bool { return false }

func (s *sevenBackend) update(context.Context, string, arcChanges) error { return ErrUnsupported }

func (s *sevenBackend) close() error { return s.a.Close() }
