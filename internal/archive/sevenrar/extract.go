// This file unpacks and tests archives: Total Commander's "Unpack files"
// (Alt+F9), copying files out of an archive panel with F5, and "Test
// archives" (Alt+Shift+F9). The options mirror the unpack dialog: a list of
// selected files and folders, a file mask such as "*.txt | *.bak", "unpack
// path names" (or flatten everything into the target folder), and what to do
// when a file already exists (ask, overwrite, skip, overwrite older, rename
// the new file, rename the existing file, cancel), with "apply to all".
//
// Safety comes first. Entry names are cleaned so nothing is written outside
// the target folder; entries with ".." are skipped and reported; nothing is
// ever written through an existing symbolic link; links stored in the archive
// are created last and only if they point inside the target folder. Every
// file is written to a temporary name and renamed into place only after its
// checksum matched, so a cancelled or failed extraction never leaves a
// half-written file behind and never destroys the existing file it would
// have replaced. Progress is reported per chunk and the context cancels the
// operation promptly.

package sevenrar

import (
	"context"
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

	"mormicommander/internal/archive/arcapi"
)

// OverwriteMode is the default answer to "file exists".
type OverwriteMode int

const (
	// OverwriteAsk calls ExtractOptions.OnConflict for every existing
	// file (skipping when OnConflict is nil).
	OverwriteAsk OverwriteMode = iota
	// OverwriteAll replaces existing files.
	OverwriteAll
	// OverwriteSkip keeps existing files.
	OverwriteSkip
	// OverwriteOlder replaces existing files only when they are older
	// than the archived file.
	OverwriteOlder
	// OverwriteRename writes the new file under a free name ("a (2).txt").
	OverwriteRename
	// OverwriteRenameExisting renames the existing file to a free name and
	// writes the new file under the original name.
	OverwriteRenameExisting
)

// ConflictAction is the answer to one "file exists" question.
type ConflictAction int

const (
	ActionOverwrite ConflictAction = iota
	ActionSkip
	ActionOverwriteOlder
	ActionRename         // write the new file as NewName (or a free name)
	ActionRenameExisting // move the existing file aside first
	ActionCancel         // abort the whole operation
)

// Conflict describes an existing file that is in the way.
type Conflict struct {
	Entry           Entry
	Target          string // full local path
	ExistingSize    int64
	ExistingModTime time.Time
	ExistingIsLink  bool
}

// ConflictDecision answers a Conflict.
type ConflictDecision struct {
	Action ConflictAction
	// NewName is the file name (not a path) for ActionRename; empty means
	// pick a free name automatically.
	NewName string
	// ApplyToAll makes the action the answer for all further conflicts.
	ApplyToAll bool
}

// SymlinkPolicy says what to do with symbolic links stored in the archive.
type SymlinkPolicy int

const (
	// SymlinksSafe creates links whose target stays inside the target
	// folder and skips (and reports) all others.
	SymlinksSafe SymlinkPolicy = iota
	// SymlinksSkip never creates links.
	SymlinksSkip
	// SymlinksAsFiles writes each link as a small text file holding the
	// target, as on file systems without links.
	SymlinksAsFiles
)

// Progress is reported while extracting or testing.
type Progress struct {
	Entry      string // archive path of the current entry
	Target     string // local path being written ("" when testing)
	FileBytes  int64  // bytes done of the current entry
	FileSize   int64  // size of the current entry
	Bytes      int64  // bytes done overall
	TotalBytes int64  // total bytes of all selected entries
	Files      int    // entries finished
	TotalFiles int    // entries selected
}

// ExtractOptions control Extract.
type ExtractOptions struct {
	// Paths selects entries by clean archive path; a folder selects
	// everything below it. Empty means the whole archive.
	Paths []string
	// BaseDir is the archive folder the user is browsing; output paths
	// are made relative to it (copying "docs/a.txt" while inside "docs"
	// writes dest/a.txt).
	BaseDir string
	// Mask is a Total Commander file mask ("*.txt *.doc | *.bak") applied
	// to file names. Empty selects all files.
	Mask string
	// FlattenPaths drops the folder structure (Total Commander's "unpack
	// path names" unchecked): all files land directly in dest.
	FlattenPaths bool
	// Overwrite is the default answer when a file exists.
	Overwrite OverwriteMode
	// OnConflict is asked when Overwrite is OverwriteAsk.
	OnConflict func(Conflict) ConflictDecision
	// Symlinks chooses how stored links are handled.
	Symlinks SymlinkPolicy
	// KeepBroken keeps files whose checksum did not match (renamed into
	// place anyway), like 7-Zip's "keep broken files". By default they are
	// deleted.
	KeepBroken bool
	// NoPermissions skips restoring permission bits; NoTimes skips
	// restoring modification times.
	NoPermissions bool
	NoTimes       bool
	// OnError is asked after a per-entry error; returning false aborts.
	// When nil, extraction continues with the next entry and all errors
	// are returned at the end.
	OnError func(*EntryError) bool
	// Progress, when set, is called after every chunk written.
	Progress func(Progress)
}

// Skipped records an entry that was deliberately not extracted.
type Skipped struct {
	Path   string
	Reason string
}

// Result reports what Extract did.
type Result struct {
	Files, Dirs, Links int
	Bytes              int64
	Skipped            []Skipped
	Errors             []*EntryError
}

// errAbort stops a walk after OnError said so; the entry error itself is
// what Extract returns.
var errAbort = errors.New("sevenrar: aborted")

type deferredLink struct {
	e      Entry
	rel    string // clean output path
	target string // link target (symlink) or archive path (hard link)
}

type dirTimes struct {
	path string
	e    Entry
}

type extractor struct {
	a       *Archive
	ctx     context.Context
	dest    string
	opts    ExtractOptions
	mode    OverwriteMode
	res     Result
	prog    Progress
	created map[string]string // local path written in this run -> archive path that wrote it
	written map[string]string // clean archive path -> local path, for hard links
	links   []deferredLink
	dirs    []dirTimes
	ours    map[string]bool // clean output paths of links we created
	buf     []byte
}

// Extract unpacks the selected entries into the folder dest (created if
// missing). It returns the result and, if anything went wrong, an error:
// the context's error when cancelled, ErrPasswordCancelled and friends when
// no password could be set, or all per-entry errors joined (test with
// errors.Is, e.g. errors.Is(err, ErrCRC)).
func (a *Archive) Extract(ctx context.Context, dest string, opts ExtractOptions) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.backend(); err != nil {
		return Result{}, err
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return Result{}, err
	}
	x := &extractor{a: a, ctx: ctx, dest: absDest, opts: opts, mode: opts.Overwrite,
		created: map[string]string{}, written: map[string]string{}, ours: map[string]bool{},
		buf: make([]byte, 256<<10)}

	mask := ParseMask(opts.Mask)
	maskAll := strings.TrimSpace(opts.Mask) == ""
	sel := make([]string, 0, len(opts.Paths))
	for _, p := range opts.Paths {
		sel = append(sel, normalizeQuery(p))
	}
	base := normalizeQuery(opts.BaseDir)

	want := make([]bool, len(a.entries))
	rels := make([]string, len(a.entries))
	for i, e := range a.entries {
		if e.Path == "" || !pathSelected(e.Path, sel) {
			continue
		}
		if e.Unsafe {
			x.res.Skipped = append(x.res.Skipped, Skipped{Path: e.RawName, Reason: "unsafe path (would be written outside the target folder)"})
			continue
		}
		if !e.IsDir && !mask.Match(e.Name) || e.IsDir && !maskAll {
			continue
		}
		rel := e.Path
		if base != "" {
			if rel == base {
				continue
			}
			if strings.HasPrefix(rel, base+"/") {
				rel = rel[len(base)+1:]
			}
		}
		if opts.FlattenPaths {
			if e.IsDir {
				continue
			}
			rel = e.Name
		}
		want[i], rels[i] = true, rel
		if !e.IsDir && !e.Symlink && !e.HardLink {
			x.prog.TotalFiles++
			if e.Size > 0 {
				x.prog.TotalBytes += e.Size
			}
		}
	}
	wantFn := func(i int) bool { return want[i] }
	if err := a.ensurePassword(ctx, wantFn); err != nil {
		return x.res, err
	}
	be, err := a.backend()
	if err != nil {
		return x.res, err
	}

	werr := be.walk(ctx, wantFn, func(i int, r io.Reader) error {
		return x.entry(a.entries[i], rels[i], r)
	})
	switch {
	case werr == nil:
		x.finishLinks()
	case errors.Is(werr, errAbort):
		// OnError asked to stop; the entry error is already recorded.
	case ctx.Err() != nil || isFatal(werr):
		x.finishDirs()
		if ctx.Err() != nil {
			return x.res, ctx.Err()
		}
		return x.res, werr
	default:
		x.res.Errors = append(x.res.Errors, &EntryError{Err: werr})
		x.finishLinks()
	}
	x.finishDirs()
	if err := ctx.Err(); err != nil {
		return x.res, err
	}
	return x.res, joinEntryErrors(x.res.Errors)
}

// isFatal reports errors that end the operation instead of being per-entry.
func isFatal(err error) bool {
	return errors.Is(err, ErrPasswordCancelled) || errors.Is(err, ErrMissingVolume) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func joinEntryErrors(list []*EntryError) error {
	if len(list) == 0 {
		return nil
	}
	errs := make([]error, len(list))
	for i, e := range list {
		errs[i] = e
	}
	return errors.Join(errs...)
}

// fail records a per-entry error and returns errAbort if OnError says so.
func (x *extractor) fail(p string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	ee := &EntryError{Path: p, Err: err}
	x.res.Errors = append(x.res.Errors, ee)
	if x.opts.OnError != nil && !x.opts.OnError(ee) {
		return errAbort
	}
	return nil
}

func (x *extractor) skip(p, reason string) {
	x.res.Skipped = append(x.res.Skipped, Skipped{Path: p, Reason: reason})
}

// entry handles one selected entry during the walk.
func (x *extractor) entry(e Entry, rel string, r io.Reader) error {
	if err := x.ctx.Err(); err != nil {
		return err
	}
	switch {
	case e.IsDir:
		target, err := targetPath(x.dest, rel)
		if err != nil {
			x.skip(e.Path, err.Error())
			return nil
		}
		if err := mkdirNoFollow(x.dest, target, 0o755); err != nil {
			return x.fail(e.Path, err)
		}
		x.dirs = append(x.dirs, dirTimes{path: target, e: e})
		x.res.Dirs++
		return nil
	case e.Symlink:
		target := e.LinkTarget
		if target == "" {
			// 7z stores the link target as the file's contents.
			b, err := io.ReadAll(io.LimitReader(r, 4096))
			if err != nil {
				return x.fail(e.Path, err)
			}
			target = string(b)
		}
		x.links = append(x.links, deferredLink{e: e, rel: rel, target: target})
		return nil
	case e.HardLink:
		x.links = append(x.links, deferredLink{e: e, rel: rel, target: e.LinkTarget})
		return nil
	}
	return x.file(e, rel, r)
}

// prepareTarget resolves the local path for rel, creates its parent folders
// and settles any conflict. It returns the final path, or "" to skip.
func (x *extractor) prepareTarget(e Entry, rel string) (string, error) {
	target, err := targetPath(x.dest, rel)
	if err != nil {
		x.skip(e.Path, err.Error())
		return "", nil
	}
	if err := mkdirNoFollow(x.dest, filepath.Dir(target), 0o755); err != nil {
		if errors.Is(err, ErrUnsafePath) {
			x.skip(e.Path, err.Error())
			return "", nil
		}
		return "", x.fail(e.Path, err)
	}
	st, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return target, nil
	}
	if err != nil {
		return "", x.fail(e.Path, err)
	}
	if st.IsDir() {
		return "", x.fail(e.Path, fmt.Errorf("a folder named %s already exists", target))
	}
	if by, ok := x.created[target]; ok && by == e.Path {
		// Written earlier in this run by the same archive path (the
		// archive holds that path twice): the later entry wins, as when
		// packing. A different entry landing on the same file (two
		// "same.txt" from different folders with "unpack path names"
		// off) is a real conflict and goes through the normal rules
		// instead of silently replacing the file written a moment ago.
		return target, nil
	}
	return x.resolveConflict(e, target, st)
}

func (x *extractor) resolveConflict(e Entry, target string, st fs.FileInfo) (string, error) {
	var action ConflictAction
	var newName string
	switch x.mode {
	case OverwriteAll:
		action = ActionOverwrite
	case OverwriteSkip:
		action = ActionSkip
	case OverwriteOlder:
		action = ActionOverwriteOlder
	case OverwriteRename:
		action = ActionRename
	case OverwriteRenameExisting:
		action = ActionRenameExisting
	default:
		if x.opts.OnConflict == nil {
			action = ActionSkip
			break
		}
		d := x.opts.OnConflict(Conflict{Entry: e, Target: target, ExistingSize: st.Size(),
			ExistingModTime: st.ModTime(), ExistingIsLink: st.Mode()&fs.ModeSymlink != 0})
		action, newName = d.Action, d.NewName
		if d.ApplyToAll {
			switch d.Action {
			case ActionOverwrite:
				x.mode = OverwriteAll
			case ActionSkip:
				x.mode = OverwriteSkip
			case ActionOverwriteOlder:
				x.mode = OverwriteOlder
			case ActionRename:
				if d.NewName == "" {
					x.mode = OverwriteRename
				}
			case ActionRenameExisting:
				x.mode = OverwriteRenameExisting
			}
		}
	}
	switch action {
	case ActionOverwrite:
		return target, nil
	case ActionSkip:
		x.skip(e.Path, "target exists")
		return "", nil
	case ActionOverwriteOlder:
		if !e.ModTime.IsZero() && st.ModTime().Before(e.ModTime) {
			return target, nil
		}
		x.skip(e.Path, "target exists and is not older")
		return "", nil
	case ActionRename:
		if newName != "" {
			clean, unsafe := cleanEntryName(newName)
			if unsafe || clean == "" || strings.Contains(clean, "/") {
				return "", x.fail(e.Path, fmt.Errorf("%w: invalid new name %q", ErrUnsafePath, newName))
			}
			t := filepath.Join(filepath.Dir(target), legalComponent(clean, windowsPaths))
			if _, err := os.Lstat(t); err == nil {
				return x.resolveConflict(e, t, mustLstat(t))
			}
			return t, nil
		}
		return freeName(target), nil
	case ActionRenameExisting:
		aside := freeName(target)
		if err := os.Rename(target, aside); err != nil {
			return "", x.fail(e.Path, err)
		}
		return target, nil
	case ActionCancel:
		return "", context.Canceled
	}
	return "", x.fail(e.Path, fmt.Errorf("unknown conflict action %d", action))
}

func mustLstat(p string) fs.FileInfo {
	st, _ := os.Lstat(p)
	return st
}

// freeName returns "name (2).ext", "name (3).ext", ... whichever does not
// exist yet next to p.
func freeName(p string) string {
	dir, base := filepath.Split(p)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem, ext = base, ""
	}
	for n := 2; ; n++ {
		c := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		if _, err := os.Lstat(c); errors.Is(err, fs.ErrNotExist) {
			return c
		}
	}
}

// file writes one regular file.
func (x *extractor) file(e Entry, rel string, r io.Reader) error {
	target, err := x.prepareTarget(e, rel)
	if err != nil || target == "" {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".mormi-*.tmp")
	if err != nil {
		return x.fail(e.Path, err)
	}
	tmpName := tmp.Name()
	x.prog.Entry, x.prog.Target, x.prog.FileBytes, x.prog.FileSize = e.Path, target, 0, e.Size
	n, copyErr := x.copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil && (!x.opts.KeepBroken || !errors.Is(copyErr, ErrCRC)) {
		os.Remove(tmpName)
		if errors.Is(copyErr, context.Canceled) || errors.Is(copyErr, context.DeadlineExceeded) {
			return copyErr
		}
		return x.fail(e.Path, copyErr)
	}
	x.applyMeta(tmpName, e, false)
	if err := renameReplace(tmpName, target); err != nil {
		os.Remove(tmpName)
		return x.fail(e.Path, err)
	}
	x.created[target] = e.Path
	x.written[e.Path] = target
	x.res.Files++
	x.res.Bytes += n
	x.prog.Files++
	x.report()
	if copyErr != nil {
		return x.fail(e.Path, copyErr) // broken file kept on request
	}
	return nil
}

func (x *extractor) report() {
	if x.opts.Progress != nil {
		x.opts.Progress(x.prog)
	}
}

// copy copies r to w in chunks, reporting progress and honouring the
// context between chunks.
func (x *extractor) copy(w io.Writer, r io.Reader) (int64, error) {
	var total int64
	for {
		if err := x.ctx.Err(); err != nil {
			return total, err
		}
		n, err := r.Read(x.buf)
		if n > 0 {
			if _, werr := w.Write(x.buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			x.prog.FileBytes += int64(n)
			x.prog.Bytes += int64(n)
			x.report()
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// renameReplace renames src over dst, clearing a read-only flag on dst
// first if the plain rename is refused (Windows refuses to replace
// read-only files).
func renameReplace(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if st, serr := os.Lstat(dst); serr == nil && st.Mode().IsRegular() {
		_ = os.Chmod(dst, 0o666)
		if err2 := os.Rename(src, dst); err2 == nil {
			return nil
		}
	}
	return err
}

// applyMeta restores permissions and modification time.
func (x *extractor) applyMeta(p string, e Entry, dir bool) {
	if !(dir && x.opts.NoPermissions) {
		// The stored bits minus what the process umask forbids (it used
		// to be a fixed 022), as 7-Zip and unzip do.
		perm := arcapi.ApplyUmask(e.Mode.Perm())
		if x.opts.NoPermissions || perm == 0 {
			perm = 0o644
			if dir {
				perm = 0o755
			}
		}
		if dir {
			perm |= 0o700
		}
		_ = os.Chmod(p, perm)
	}
	if !x.opts.NoTimes && !e.ModTime.IsZero() {
		_ = os.Chtimes(p, e.ModTime, e.ModTime)
	}
}

// finishLinks creates the deferred symbolic and hard links.
func (x *extractor) finishLinks() {
	for _, l := range x.links {
		if x.ctx.Err() != nil {
			return
		}
		if l.e.HardLink {
			x.hardLink(l)
			continue
		}
		switch x.opts.Symlinks {
		case SymlinksSkip:
			x.skip(l.e.Path, "symbolic links are not extracted")
			continue
		case SymlinksAsFiles:
			e := l.e
			e.Symlink, e.Size = false, int64(len(l.target))
			e.Mode = e.Mode &^ fs.ModeSymlink
			if err := x.file(e, l.rel, strings.NewReader(l.target)); err != nil {
				return
			}
			continue
		}
		if !symlinkTargetOK(x.dest, l.rel, l.target, x.ours) {
			x.skip(l.e.Path, "symbolic link points outside the target folder: "+l.target)
			continue
		}
		target, err := x.prepareTarget(l.e, l.rel)
		if err != nil || target == "" {
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			if err := os.Remove(target); err != nil {
				_ = x.fail(l.e.Path, err)
				continue
			}
		}
		if err := os.Symlink(filepath.FromSlash(strings.ReplaceAll(l.target, "\\", "/")), target); err != nil {
			x.skip(l.e.Path, "cannot create symbolic link: "+err.Error())
			continue
		}
		x.ours[l.rel] = true
		x.created[target] = l.e.Path
		x.res.Links++
	}
}

// hardLink creates a RAR hard link or file copy to an entry extracted
// earlier in this run, copying the data when linking is not possible.
func (x *extractor) hardLink(l deferredLink) {
	src, ok := x.written[normalizeQuery(l.target)]
	if !ok {
		x.skip(l.e.Path, "link target was not extracted: "+l.target)
		return
	}
	target, err := x.prepareTarget(l.e, l.rel)
	if err != nil || target == "" {
		return
	}
	if target == src {
		return
	}
	if _, err := os.Lstat(target); err == nil {
		_ = os.Remove(target)
	}
	if !l.e.fileCopy && os.Link(src, target) == nil {
		x.created[target] = l.e.Path
		x.res.Links++
		return
	}
	in, err := os.Open(src)
	if err != nil {
		_ = x.fail(l.e.Path, err)
		return
	}
	defer in.Close()
	e := l.e
	e.HardLink = false
	if err := x.file(e, l.rel, in); err != nil {
		return
	}
}

// finishDirs restores folder permissions and times, deepest first so that
// setting a child's time does not change its parent's time afterwards.
func (x *extractor) finishDirs() {
	sort.SliceStable(x.dirs, func(i, j int) bool {
		return strings.Count(x.dirs[i].path, string(filepath.Separator)) > strings.Count(x.dirs[j].path, string(filepath.Separator))
	})
	for _, d := range x.dirs {
		x.applyMeta(d.path, d.e, true)
	}
}

// TestResult reports what Test found.
type TestResult struct {
	Files  int
	Bytes  int64
	Errors []*EntryError
}

// OK reports whether every file tested fine.
func (t TestResult) OK() bool { return len(t.Errors) == 0 }

// Test decompresses every file and checks its checksum without writing
// anything, like Total Commander's "Test archives". Per-file errors are
// collected (the archive is tested to the end where possible) and also
// returned joined; fatal problems (cancel, missing volume, no password)
// are returned at once.
func (a *Archive) Test(ctx context.Context, progress func(Progress)) (TestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var res TestResult
	var prog Progress
	want := func(i int) bool {
		e := a.entries[i]
		return !e.IsDir && !e.HardLink && !e.Symlink
	}
	for i, e := range a.entries {
		if want(i) {
			prog.TotalFiles++
			if e.Size > 0 {
				prog.TotalBytes += e.Size
			}
		}
	}
	if err := a.ensurePassword(ctx, want); err != nil {
		return res, err
	}
	be, err := a.backend()
	if err != nil {
		return res, err
	}
	buf := make([]byte, 256<<10)
	werr := be.walk(ctx, want, func(i int, r io.Reader) error {
		e := a.entries[i]
		prog.Entry, prog.FileBytes, prog.FileSize = e.Path, 0, e.Size
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, err := r.Read(buf)
			prog.FileBytes += int64(n)
			prog.Bytes += int64(n)
			res.Bytes += int64(n)
			if n > 0 && progress != nil {
				progress(prog)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				if isFatal(err) {
					return err
				}
				res.Errors = append(res.Errors, &EntryError{Path: displayPath(e), Err: err})
				break
			}
		}
		res.Files++
		prog.Files++
		if progress != nil {
			progress(prog)
		}
		return nil
	})
	if werr != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if isFatal(werr) {
			return res, werr
		}
		res.Errors = append(res.Errors, &EntryError{Err: werr})
	}
	return res, joinEntryErrors(res.Errors)
}

func displayPath(e Entry) string {
	if e.Path != "" {
		return e.Path
	}
	return path.Clean(e.RawName)
}
