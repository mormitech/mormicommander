// This file unpacks tar-family archives (TC's Alt+F9 and copying out of an
// archive panel). Safety comes first: all writing goes through an os.Root
// opened on the target folder, so no name, ".." trick or symlink planted by
// an earlier entry can make the engine write outside it; names that try to
// escape are refused up front, and symlinks are only created when they point
// somewhere inside the target folder. Files are written to a temporary name
// and renamed into place when complete, so cancelling never leaves a
// half-written file behind and an existing file is only replaced by a
// complete copy. Permissions and modification times are restored from the
// archive (setuid/setgid bits are dropped), and folder dates are set last
// because creating files inside a folder changes its date.
package tarfmt

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	archive "mormicommander/internal/archive/arcapi"
)

// extractor holds the state of one Extract call.
type extractor struct {
	r       *Reader
	ctx     context.Context
	root    *os.Root
	dest    string
	opt     archive.ExtractOptions
	names   []string
	prefix  string
	prog    *progress
	written map[string]string // local name -> archive name, for items created by this run
	dirs    map[string]dirMeta
	pending []pendingLink
	errs    []error
}

type dirMeta struct {
	mode  fs.FileMode
	mtime time.Time
	atime time.Time
}

// pendingLink is a hard link whose target was not unpacked in this run
// (for example only the link was selected); its data is fetched in a second
// pass.
type pendingLink struct {
	rel    string
	target string // archive name of the data
	entry  archive.Entry
	hdr    *tar.Header
}

// Extract unpacks the selected entries into dest, creating dest if needed.
// Problems with single entries (unsafe names, links that cannot be made) do
// not stop the run; they are returned together as joined *archive.EntryError
// values. Cancellation and archive corruption stop the run immediately.
func (r *Reader) Extract(ctx context.Context, names []string, dest string, opt archive.ExtractOptions) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	x := &extractor{
		r: r, ctx: ctx, root: root, dest: dest, opt: opt,
		names:   cleanAll(names),
		prefix:  cleanName(opt.StripPrefix),
		written: map[string]string{},
		dirs:    map[string]dirMeta{},
	}
	x.prog = progressFn(opt.Progress, r.size, nil)
	x.prog.p.Total = r.selectedTotal(x)
	if r.format.IsTar() {
		err = x.runTar()
	} else {
		err = x.runSingle()
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return err
	}
	x.finishDirs()
	x.prog.flush()
	return errors.Join(x.errs...)
}

// selectedTotal sums the sizes to unpack when the archive is already listed;
// otherwise the total is unknown (0) and ArchiveRead gives the progress.
func (r *Reader) selectedTotal(x *extractor) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.listed {
		return 0
	}
	var t int64
	for _, e := range r.entries {
		if !e.IsDir && e.Size > 0 && selected(e.Path, x.names) && x.wanted(e) {
			if _, ok := x.target(e.Path, false); ok {
				t += e.Size
			}
		}
	}
	return t
}

func (x *extractor) fail(name string, err error) {
	x.errs = append(x.errs, &archive.EntryError{Name: name, Err: err})
}

// wanted applies the user's file mask. Folders pass only without a mask:
// with "*.txt" the folders that hold matching files are still created as
// parents, but empty or non-matching folders are not.
func (x *extractor) wanted(e archive.Entry) bool {
	if x.opt.Filter == nil {
		return true
	}
	if e.IsDir {
		return false
	}
	return x.opt.Filter(e)
}

// target maps an archive name to the slash path under dest, applying
// StripPrefix and Flatten. ok is false when the entry is not to be written.
func (x *extractor) target(name string, isDir bool) (string, bool) {
	if !underDir(name, x.prefix) {
		return "", false
	}
	rel := name
	if x.prefix != "" {
		if name == x.prefix {
			return "", false
		}
		rel = name[len(x.prefix)+1:]
	}
	if x.opt.Flatten {
		if isDir {
			return "", false
		}
		rel = path.Base(rel)
	}
	return rel, true
}

func (x *extractor) runTar() error {
	p, err := x.r.newPass(x.ctx)
	if err != nil {
		return err
	}
	x.prog.raw = p.raw
	err = x.r.walk(x.ctx, p, func(h *tar.Header, name string, tr *tar.Reader) error {
		return x.member(h, name, tr)
	})
	if err == nil {
		err = x.checkTrailer(p)
	}
	p.Close()
	if err != nil {
		return err
	}
	if len(x.pending) > 0 {
		return x.resolvePending()
	}
	return nil
}

// checkTrailer reads what is left of the compressed stream after the tar
// end marker. The tar reader stops at that marker, so without this the
// gzip CRC32 and size trailer (and the xz index and checksums) at the very
// end of the file would never be checked, and a damaged archive would
// unpack without a word. The rest is normally only padding, so this costs
// next to nothing. A gzip archive followed by unrelated junk (some tape
// tools pad with garbage) is still accepted: the gzip member itself has
// been verified by then.
func (x *extractor) checkTrailer(p *pass) error {
	if x.r.format.Compression() == CompNone {
		return nil
	}
	_, err := io.Copy(io.Discard, p.dec)
	if err == nil || errors.Is(err, gzip.ErrHeader) {
		return nil
	}
	if cerr := x.ctx.Err(); cerr != nil {
		return cerr
	}
	return fmt.Errorf("tarfmt: archive is damaged (end of compressed data): %w", err)
}

// member handles one tar header during the main pass.
func (x *extractor) member(h *tar.Header, name string, tr io.Reader) error {
	if !selected(name, x.names) {
		return nil
	}
	e := entryFromHeader(name, h)
	if !x.wanted(e) {
		return nil
	}
	if !isLocalName(name) {
		x.fail(name, archive.ErrUnsafePath)
		return nil
	}
	rel, ok := x.target(name, e.IsDir)
	if !ok {
		return nil
	}
	if !isLocalName(rel) {
		x.fail(name, archive.ErrUnsafePath)
		return nil
	}
	switch {
	case e.IsDir:
		return x.makeDir(rel, h)
	case isRegHeader(h):
		x.prog.file(name)
		return x.writeFile(rel, e, tr, h.AccessTime)
	case h.Typeflag == tar.TypeSymlink:
		return x.makeSymlink(rel, e, h.Linkname)
	case h.Typeflag == tar.TypeLink:
		return x.makeHardlink(rel, e, h)
	}
	// Devices, FIFOs and other special members are not unpacked, like TC.
	return nil
}

func (x *extractor) runSingle() error {
	if _, err := x.r.Entries(x.ctx); err != nil {
		return err
	}
	e := x.r.entries[0]
	if !selected(e.Path, x.names) || !x.wanted(e) {
		return nil
	}
	rel, ok := x.target(e.Path, false)
	if !ok {
		return nil
	}
	p, err := x.r.newPass(x.ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	x.prog.raw = p.raw
	x.prog.file(e.Path)
	if err := x.writeFile(rel, e, p.dec, time.Time{}); err != nil {
		return err
	}
	return nil
}

// local turns a slash path into the OS form os.Root expects.
func local(rel string) string { return filepath.FromSlash(rel) }

// ensureParent creates the folders above rel. os.Root refuses to walk
// through a symlink that leaves the target folder.
func (x *extractor) ensureParent(rel string) error {
	dir := path.Dir(rel)
	if dir == "." {
		return nil
	}
	return x.root.MkdirAll(local(dir), 0o755)
}

func (x *extractor) makeDir(rel string, h *tar.Header) error {
	if fi, err := x.root.Lstat(local(rel)); err == nil && !fi.IsDir() {
		// A file or link is in the way of a folder.
		c := archive.Entry{Path: cleanName(h.Name), IsDir: true, ModTime: h.ModTime}
		ok, err := x.conflict(rel, c, fi)
		if err != nil || !ok {
			return err
		}
		if err := x.root.Remove(local(rel)); err != nil {
			x.fail(rel, err)
			return nil
		}
	}
	if err := x.root.MkdirAll(local(rel), 0o755); err != nil {
		x.fail(rel, err)
		return nil
	}
	x.written[rel] = cleanName(h.Name)
	x.dirs[rel] = dirMeta{mode: h.FileInfo().Mode().Perm(), mtime: h.ModTime, atime: h.AccessTime}
	return nil
}

// conflict decides whether an existing item at rel may be replaced. It
// returns ok=false to skip and a non-nil error only to abort the run.
func (x *extractor) conflict(rel string, e archive.Entry, existing fs.FileInfo) (bool, error) {
	if x.written[rel] == e.Path {
		// The same member was written earlier in this run: a newer copy
		// stored later in the archive replaces it silently, as tar does.
		// Different members landing on one name (Flatten) still ask.
		return true, nil
	}
	if existing.IsDir() && !e.IsDir {
		x.fail(e.Path, errors.New("a folder with this name already exists"))
		return false, nil
	}
	pol := x.opt.Conflict
	if pol == archive.ConflictAsk {
		if x.opt.OnConflict == nil {
			pol = archive.ConflictSkip
		} else {
			pol = x.opt.OnConflict(archive.Conflict{Entry: e, Path: filepath.Join(x.dest, local(rel)), Existing: existing})
		}
	}
	switch pol {
	case archive.ConflictOverwrite:
		return true, nil
	case archive.ConflictOverwriteOlder:
		return e.ModTime.Truncate(time.Second).After(existing.ModTime().Truncate(time.Second)), nil
	case archive.ConflictAbort:
		return false, archive.ErrAborted
	}
	return false, nil
}

// checkExisting runs the conflict rules for rel when something is there.
func (x *extractor) checkExisting(rel string, e archive.Entry) (bool, fs.FileInfo, error) {
	fi, err := x.root.Lstat(local(rel))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil, nil
		}
		x.fail(e.Path, err)
		return false, nil, nil
	}
	ok, err := x.conflict(rel, e, fi)
	return ok, fi, err
}

func tempSuffix() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ctxWriter stops a long copy promptly when the context is cancelled even
// if the data comes from a source that does not check it.
type ctxWriter struct {
	ctx context.Context
	w   io.Writer
}

func (c ctxWriter) Write(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.w.Write(b)
}

// writeFile writes src to rel through a temporary file and renames it into
// place. Errors that concern only this file are recorded; cancellation and
// read errors from the archive abort.
func (x *extractor) writeFile(rel string, e archive.Entry, src io.Reader, atime time.Time) error {
	if err := x.ensureParent(rel); err != nil {
		// The tar reader skips the unread data on its next step.
		x.fail(e.Path, err)
		return nil
	}
	ok, _, err := x.checkExisting(rel, e)
	if err != nil || !ok {
		return err
	}
	tmp := ""
	var f *os.File
	for i := 0; i < 10; i++ {
		tmp = path.Join(path.Dir(rel), "."+path.Base(rel)+".mctmp-"+tempSuffix())
		f, err = x.root.OpenFile(local(tmp), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil || !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		x.fail(e.Path, err)
		return nil
	}
	_, cerr := io.Copy(ctxWriter{x.ctx, x.prog.writer(f)}, src)
	if err := f.Close(); err != nil && cerr == nil {
		cerr = err
	}
	if cerr != nil {
		_ = x.root.Remove(local(tmp))
		if x.ctx.Err() != nil {
			return x.ctx.Err()
		}
		var pe *fs.PathError
		if errors.As(cerr, &pe) {
			// A local write problem (disk full, permissions): record and
			// go on with the next entry.
			x.fail(e.Path, cerr)
			return nil
		}
		return fmt.Errorf("%s: %w", e.Path, cerr)
	}
	// The stored bits minus what the umask forbids, as tar does for a
	// normal user.
	mode := archive.ApplyUmask(e.Mode.Perm())
	if e.Mode.Perm() == 0 {
		mode = archive.ApplyUmask(0o644)
	}
	_ = x.root.Chmod(local(tmp), mode)
	if !e.ModTime.IsZero() {
		if atime.IsZero() {
			atime = e.ModTime
		}
		_ = x.root.Chtimes(local(tmp), atime, e.ModTime)
	}
	if err := x.root.Rename(local(tmp), local(rel)); err != nil {
		// Windows refuses to replace a read-only file; make it writable
		// and try once more.
		_ = x.root.Chmod(local(rel), 0o600)
		if err2 := x.root.Rename(local(tmp), local(rel)); err2 != nil {
			_ = x.root.Remove(local(tmp))
			x.fail(e.Path, err)
			return nil
		}
	}
	x.written[rel] = e.Path
	return nil
}

// noLinkParents reports whether every folder above rel is a real folder,
// not a symlink. Symlink targets are checked as text, which is only
// meaningful when the folders they start from are real.
func (x *extractor) noLinkParents(rel string) bool {
	parts := strings.Split(rel, "/")
	cur := ""
	for _, p := range parts[:len(parts)-1] {
		cur = path.Join(cur, p)
		fi, err := x.root.Lstat(local(cur))
		if err != nil || fi.Mode()&fs.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// replaceable clears rel for a new link after the conflict rules said yes.
func (x *extractor) replaceable(rel string, e archive.Entry) (bool, error) {
	ok, fi, err := x.checkExisting(rel, e)
	if err != nil || !ok {
		return false, err
	}
	if fi != nil {
		if err := x.root.Remove(local(rel)); err != nil {
			x.fail(e.Path, err)
			return false, nil
		}
	}
	return true, nil
}

func (x *extractor) makeSymlink(rel string, e archive.Entry, target string) error {
	if !strictLinkSafe(rel, target) {
		x.fail(e.Path, fmt.Errorf("%w (-> %s)", archive.ErrUnsafeLink, target))
		return nil
	}
	if err := x.ensureParent(rel); err != nil {
		x.fail(e.Path, err)
		return nil
	}
	if !x.noLinkParents(rel) {
		x.fail(e.Path, archive.ErrUnsafeLink)
		return nil
	}
	ok, err := x.replaceable(rel, e)
	if err != nil || !ok {
		return err
	}
	if err := x.root.Symlink(filepath.FromSlash(target), local(rel)); err != nil {
		x.fail(e.Path, err)
		return nil
	}
	x.written[rel] = e.Path
	return nil
}

// strictLinkSafe accepts a symlink target only when it is relative, its
// ".." parts all come first and do not climb above the target folder, and
// nothing after them climbs again. That rules out targets such as "d/.."
// whose text looks harmless but which could leave the folder once "d" is
// itself a link.
func strictLinkSafe(rel, target string) bool {
	if !linkTargetSafe(rel, target) {
		return false
	}
	t := strings.ReplaceAll(target, "\\", "/")
	depth := strings.Count(rel, "/") // folders above the link
	up := 0
	leading := true
	for _, p := range strings.Split(t, "/") {
		switch p {
		case "", ".":
		case "..":
			if !leading {
				return false
			}
			up++
		default:
			leading = false
		}
	}
	return up <= depth
}

func (x *extractor) makeHardlink(rel string, e archive.Entry, h *tar.Header) error {
	target := cleanName(h.Linkname)
	if !isLocalName(target) {
		x.fail(e.Path, fmt.Errorf("%w (-> %s)", archive.ErrUnsafeLink, h.Linkname))
		return nil
	}
	trel, ok := x.target(target, false)
	if ok && x.written[trel] != "" && trel != rel {
		if fi, err := x.root.Lstat(local(trel)); err == nil && fi.Mode().IsRegular() {
			return x.linkOrCopy(trel, rel, e, fi)
		}
	}
	x.pending = append(x.pending, pendingLink{rel: rel, target: target, entry: e, hdr: h})
	return nil
}

// linkOrCopy makes rel a hard link to the already unpacked file trel, or a
// copy of it when the file system has no hard links.
func (x *extractor) linkOrCopy(trel, rel string, e archive.Entry, fi fs.FileInfo) error {
	if err := x.ensureParent(rel); err != nil {
		x.fail(e.Path, err)
		return nil
	}
	ok, err := x.replaceable(rel, e)
	if err != nil || !ok {
		return err
	}
	if err := x.root.Link(local(trel), local(rel)); err == nil {
		x.written[rel] = e.Path
		return nil
	}
	src, err := x.root.Open(local(trel))
	if err != nil {
		x.fail(e.Path, err)
		return nil
	}
	defer src.Close()
	e.Mode = fi.Mode()
	e.ModTime = fi.ModTime()
	return x.writeFile(rel, e, src, time.Time{})
}

// resolvePending runs a second pass to fetch the data of hard links whose
// target was not unpacked, writing it as an ordinary file.
func (x *extractor) resolvePending() error {
	want := map[string][]pendingLink{}
	for _, pl := range x.pending {
		want[pl.target] = append(want[pl.target], pl)
	}
	p, err := x.r.newPass(x.ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	x.prog.raw = p.raw
	done := map[string]bool{}
	err = x.r.walk(x.ctx, p, func(h *tar.Header, name string, tr *tar.Reader) error {
		links, ok := want[name]
		if !ok || !isRegHeader(h) || done[name] {
			return nil
		}
		done[name] = true
		first := links[0]
		e := first.entry
		e.Size = h.Size
		e.Mode = h.FileInfo().Mode()
		e.ModTime = h.ModTime
		x.prog.file(e.Path)
		if err := x.writeFile(first.rel, e, tr, h.AccessTime); err != nil {
			return err
		}
		fi, err := x.root.Lstat(local(first.rel))
		if err != nil || x.written[first.rel] == "" {
			return nil
		}
		for _, pl := range links[1:] {
			if err := x.linkOrCopy(first.rel, pl.rel, pl.entry, fi); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for name, links := range want {
		if !done[name] {
			for _, pl := range links {
				x.fail(pl.entry.Path, fmt.Errorf("hard link target %q not found in archive", name))
			}
		}
	}
	return nil
}

// finishDirs restores folder permissions and dates, deepest first, after
// every file is in place.
func (x *extractor) finishDirs() {
	rels := make([]string, 0, len(x.dirs))
	for rel := range x.dirs {
		rels = append(rels, rel)
	}
	sort.Slice(rels, func(i, j int) bool {
		di, dj := strings.Count(rels[i], "/"), strings.Count(rels[j], "/")
		if di != dj {
			return di > dj
		}
		return rels[i] > rels[j]
	})
	for _, rel := range rels {
		m := x.dirs[rel]
		if m.mode != 0 {
			_ = x.root.Chmod(local(rel), archive.ApplyUmask(m.mode))
		}
		if !m.mtime.IsZero() {
			at := m.atime
			if at.IsZero() {
				at = m.mtime
			}
			_ = x.root.Chtimes(local(rel), at, m.mtime)
		}
	}
}
