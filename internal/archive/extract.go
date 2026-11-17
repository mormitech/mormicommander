// This file implements unpacking for every format: F5 out of an archive
// panel, Alt+F9 "Unpack specific files", and "Unpack all". It works only
// through the Archive interface, so zip, tar, 7z and RAR all get the same
// behavior. The rules follow Total Commander's Unpack dialog: extract the
// selected entries (a selected folder brings its whole subtree), relative to
// the archive folder currently shown in the panel, optionally without paths,
// filtered by a file mask, and ask before overwriting. Safety comes first:
// every target path passes SafeJoin (no absolute paths, no "..", no names the
// OS rejects), parent folders are checked after resolving symbolic links so a
// link created by the archive cannot redirect later writes, links themselves
// are created last and only when they point inside the target folder, and
// files are written to a temporary name and renamed into place so a
// cancelled or failed extraction never leaves a half-written file behind or
// destroys the file it was going to replace.
package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mormicommander/internal/archive/arcapi"
	"mormicommander/internal/robust"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SymlinkPolicy decides what happens to symbolic links stored in archives.
type SymlinkPolicy int

const (
	// SymlinksSafe creates links whose target stays inside the extraction
	// folder and skips (and reports) the others. This is the default.
	SymlinksSafe SymlinkPolicy = iota
	// SymlinksSkip never creates links.
	SymlinksSkip
	// SymlinksAsFiles writes each link as a small text file holding the
	// target, which is what Windows tools without link support do.
	SymlinksAsFiles
)

// ExtractOptions are the settings of the Unpack dialog.
type ExtractOptions struct {
	// Dest is the local target folder; it is created when missing.
	Dest string
	// Paths are the selected entries. Folders include everything below
	// them. Empty means everything below Base.
	Paths []string
	// Base is the archive folder the panel shows. Entries are written
	// relative to it, so extracting "docs/a/x.txt" while inside "docs"
	// creates Dest/a/x.txt.
	Base string
	// NoPaths extracts files without their folders ("Unpack path names"
	// switched off). Name collisions go through the conflict policy.
	NoPaths bool
	// Mask filters file names (see ParseMask); folders are created only as
	// needed for matching files when a mask is set.
	Mask string
	// Policy and OnConflict handle files that already exist.
	Policy     ConflictPolicy
	OnConflict ConflictFunc
	// OnError is called when one entry fails (wrong password, damaged
	// data, disk error). Returning true skips it and continues; returning
	// false, or a nil OnError, stops the job with that error.
	OnError func(path string, err error) bool
	// Symlinks decides how stored symbolic links are handled.
	Symlinks SymlinkPolicy
	// Progress receives progress reports.
	Progress ProgressFunc
}

// ExtractResult summarizes an extraction.
type ExtractResult struct {
	Files, Dirs, Links int
	Bytes              int64
	// Written lists the local paths of extracted files and links.
	Written []string
	Skipped []Skipped
	Failed  []FileError
}

// Skipped is an entry that was deliberately not extracted.
type Skipped struct {
	Path   string
	Reason string
}

// FileError is an entry that failed.
type FileError struct {
	Path string
	Err  error
}

func (e FileError) Error() string { return e.Path + ": " + e.Err.Error() }

// Selection returns the entries selected by paths (folders bring their
// subtree; empty paths select everything below base), in archive order and
// without duplicates.
func Selection(a Archive, paths []string, base string) []Entry {
	base, _ = CleanPath(base)
	var sel []string
	for _, p := range paths {
		c, _ := CleanPath(p)
		sel = append(sel, c)
	}
	if len(sel) == 0 {
		sel = []string{base}
	}
	seen := make(map[string]bool)
	var out []Entry
	for _, e := range a.Entries() {
		for _, s := range sel {
			if IsUnder(e.Path, s) {
				key := fmt.Sprint(e.ID, "\x00", e.Path)
				if !seen[key] {
					seen[key] = true
					out = append(out, e)
				}
				break
			}
		}
	}
	return out
}

// relTo returns the path of entry p as it should appear below the target
// folder when the panel shows archive folder base.
func relTo(p, base string) string {
	if base == "" {
		return p
	}
	if p == base {
		return path.Base(p)
	}
	if strings.HasPrefix(p, base+"/") {
		return p[len(base)+1:]
	}
	return p
}

type plannedEntry struct {
	e   Entry
	rel string
}

// Extract unpacks entries of a to opts.Dest. It returns the result so far
// together with the error that stopped it, if any; ctx cancellation returns
// ctx.Err() and the Cancel button in the conflict dialog returns ErrCanceled.
func Extract(ctx context.Context, a Archive, opts ExtractOptions) (ExtractResult, error) {
	var res ExtractResult
	if opts.Dest == "" {
		return res, errors.New("archive: no target folder")
	}
	if err := os.MkdirAll(opts.Dest, 0o755); err != nil {
		return res, err
	}
	dest, err := filepath.Abs(opts.Dest)
	if err != nil {
		return res, err
	}
	realDest := realDir(dest)
	mask := ParseMask(opts.Mask)
	base, _ := CleanPath(opts.Base)

	var files, dirs, links []plannedEntry
	var total int64
	for _, e := range Selection(a, opts.Paths, base) {
		if len(opts.Paths) == 0 && e.Path == base {
			continue // the folder the panel shows is the target itself
		}
		rel := relTo(e.Path, base)
		switch {
		case e.IsDir:
			if opts.NoPaths || !mask.IsEmpty() {
				continue
			}
			dirs = append(dirs, plannedEntry{e, rel})
		default:
			if !mask.Match(e.Name) {
				continue
			}
			if opts.NoPaths {
				rel = e.Name
			}
			if e.IsSymlink() {
				links = append(links, plannedEntry{e, rel})
				continue
			}
			files = append(files, plannedEntry{e, rel})
			total += e.Size
		}
	}

	x := &extractor{
		ctx: ctx, a: a, opts: opts, dest: dest, realDest: realDest,
		conflicts: NewConflictResolver(opts.Policy, opts.OnConflict),
		track:     NewTracker("extract", opts.Progress, total, len(files)),
		res:       &res,
	}

	type dirTime struct {
		path string
		t    time.Time
	}
	var dirTimes []dirTime
	for _, d := range dirs {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		target, err := SafeJoin(dest, d.rel)
		if err != nil {
			if stop := x.fail(d.e.Path, err); stop != nil {
				return res, stop
			}
			continue
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			if stop := x.fail(d.e.Path, err); stop != nil {
				return res, stop
			}
			continue
		}
		res.Dirs++
		if !d.e.Implied && !d.e.ModTime.IsZero() {
			dirTimes = append(dirTimes, dirTime{target, d.e.ModTime})
		}
	}

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := x.extractFile(f); err != nil {
			return res, err
		}
	}
	for _, l := range links {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := x.extractLink(l); err != nil {
			return res, err
		}
	}

	// Folder times last, deepest first, because writing files into a folder
	// updates its modification time.
	sort.Slice(dirTimes, func(i, j int) bool { return len(dirTimes[i].path) > len(dirTimes[j].path) })
	for _, d := range dirTimes {
		if !arcapi.NoFileTimes {
			_ = os.Chtimes(d.path, d.t, d.t)
		}
	}
	return res, nil
}

type extractor struct {
	ctx       context.Context
	a         Archive
	opts      ExtractOptions
	dest      string
	realDest  string
	conflicts *ConflictResolver
	track     *Tracker
	res       *ExtractResult
}

// fail records a per-entry error. It returns nil when the job continues and
// the error to stop with otherwise.
func (x *extractor) fail(p string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrCanceled) {
		return err
	}
	if x.opts.OnError != nil && x.opts.OnError(p, err) {
		x.res.Failed = append(x.res.Failed, FileError{p, err})
		return nil
	}
	return fmt.Errorf("%s: %w", p, err)
}

// prepareTarget computes the local path for rel, creates its parent folder,
// checks it against symlink redirection and resolves conflicts. It returns
// "" (and no error) when the entry is skipped.
func (x *extractor) prepareTarget(e Entry, rel string) (string, os.FileInfo, error) {
	target, err := SafeJoin(x.dest, rel)
	if err != nil {
		return "", nil, err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", nil, err
	}
	if !insideReal(x.realDest, parent) {
		return "", nil, fmt.Errorf("%w: %s (parent folder is a link to elsewhere)", ErrInsecurePath, rel)
	}
	for attempt := 0; attempt < 16; attempt++ {
		st, err := os.Lstat(target)
		if err != nil {
			return target, nil, nil
		}
		info := ConflictInfo{
			Path:     target,
			Existing: FileStat{Size: st.Size(), ModTime: st.ModTime(), IsDir: st.IsDir(), ReadOnly: st.Mode().Perm()&0o200 == 0},
			Incoming: FileStat{Size: e.Size, ModTime: e.ModTime, ReadOnly: e.DOSAttrs&AttrReadOnly != 0},
		}
		r, newName := x.conflicts.Resolve(info, func(n string) bool { return existsLocal(filepath.Join(parent, n)) })
		switch r {
		case ResolveSkip:
			x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "target exists"})
			return "", nil, nil
		case ResolveCancel:
			return "", nil, ErrCanceled
		case ResolveRename:
			target = filepath.Join(parent, SanitizeName(newName))
			continue
		case ResolveOverwrite:
			if st.IsDir() {
				return "", nil, fmt.Errorf("cannot replace folder %s with a file", target)
			}
			return target, st, nil
		}
	}
	return "", nil, fmt.Errorf("archive: no free name for %s", rel)
}

func (x *extractor) extractFile(f plannedEntry) error {
	e := f.e
	target, existing, err := x.prepareTarget(e, f.rel)
	if err != nil {
		return x.fail(e.Path, err)
	}
	if target == "" {
		return nil
	}
	x.track.StartFile(e.Path, e.Size)
	n, err := x.writeFile(e, target, existing)
	if err != nil {
		return x.fail(e.Path, err)
	}
	x.track.FinishFile()
	x.res.Files++
	x.res.Bytes += n
	x.res.Written = append(x.res.Written, target)
	return nil
}

// writeFile streams entry content into a temporary file next to target and
// renames it into place, then applies times, permissions and attributes.
func (x *extractor) writeFile(e Entry, target string, existing os.FileInfo) (int64, error) {
	rc, err := x.a.Open(e)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".mc-unpack-*.tmp")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	n, err := CopyContext(x.ctx, tmp, rc, x.track)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmpName)
		return n, err
	}
	if existing != nil && existing.Mode().Perm()&0o200 == 0 {
		// Replacing a read-only file: the user already agreed to overwrite,
		// and Windows refuses to rename over a read-only file.
		_ = os.Chmod(target, existing.Mode().Perm()|0o200)
	}
	if err := robust.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return n, err
	}
	applyMeta(target, e)
	return n, nil
}

// applyMeta sets permissions, attributes and the modification time.
func applyMeta(target string, e Entry) {
	perm := e.Mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	// Drop what the process umask forbids, as unzip and tar do, then make
	// sure the owner can always read and update the copy.
	perm = arcapi.ApplyUmask(perm) | 0o600
	if e.DOSAttrs&AttrReadOnly != 0 {
		perm &^= 0o222
	}
	_ = os.Chmod(target, perm)
	setDOSAttrs(target, e.DOSAttrs)
	if !e.ModTime.IsZero() && !arcapi.NoFileTimes {
		_ = os.Chtimes(target, e.ModTime, e.ModTime)
	}
}

func (x *extractor) extractLink(l plannedEntry) error {
	e := l.e
	rc, err := x.a.Open(e)
	if err != nil {
		return x.fail(e.Path, err)
	}
	raw, err := io.ReadAll(io.LimitReader(rc, 4097))
	rc.Close()
	if err != nil {
		return x.fail(e.Path, err)
	}
	if len(raw) > 4096 {
		x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "link target too long"})
		return nil
	}
	linkTarget := string(raw)
	switch x.opts.Symlinks {
	case SymlinksSkip:
		x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "symbolic links are not extracted"})
		return nil
	case SymlinksAsFiles:
		target, existing, err := x.prepareTarget(e, l.rel)
		if err != nil {
			return x.fail(e.Path, err)
		}
		if target == "" {
			return nil
		}
		if existing != nil {
			os.Remove(target)
		}
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return x.fail(e.Path, err)
		}
		x.res.Files++
		x.res.Written = append(x.res.Written, target)
		return nil
	}
	if !linkStaysInside(l.rel, linkTarget) {
		x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "link points outside the target folder: " + linkTarget})
		return nil
	}
	target, existing, err := x.prepareTarget(e, l.rel)
	if err != nil {
		return x.fail(e.Path, err)
	}
	if target == "" {
		return nil
	}
	// The textual depth of rel can differ from where the link really
	// lands when a parent folder is itself a link made earlier, so check
	// the target again against the real depth of the link's folder.
	if !linkTargetInside(realDepth(x.realDest, filepath.Dir(target)), linkTarget) {
		x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "link points outside the target folder: " + linkTarget})
		return nil
	}
	if existing != nil {
		if err := os.Remove(target); err != nil {
			return x.fail(e.Path, err)
		}
	}
	if err := os.Symlink(filepath.FromSlash(linkTarget), target); err != nil {
		// Windows without developer mode cannot create links; report it
		// instead of failing the whole job.
		x.res.Skipped = append(x.res.Skipped, Skipped{e.Path, "cannot create symbolic link: " + err.Error()})
		return nil
	}
	x.res.Links++
	x.res.Written = append(x.res.Written, target)
	return nil
}
