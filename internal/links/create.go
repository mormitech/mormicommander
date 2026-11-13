// Creating links is what TC's Files > Create symbolic link / Create hard link
// dialog and Ctrl+Shift+F5 (cm_CreateShortcut) do: the user picks a target
// (usually the file under the cursor) and a link name (usually the same name
// in the other panel's folder), and chooses the kind of link. This file holds
// the single-link functions for each kind plus a batch helper for a whole
// selection. The checks that matter to the user happen up front, with errors
// they can understand: a hard link to a folder or across volumes is refused
// with ErrIsDir / ErrCrossDevice instead of a raw errno, an existing name is
// reported as os.ErrExist so the dialog can ask "Overwrite?", and a real
// folder is never replaced even with Overwrite set. When an existing link is
// replaced, the new one is first made under a temporary name and then renamed
// over the old one, so a failure never leaves the user with neither.

package links

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CreateOptions controls how a link is made.
type CreateOptions struct {
	// Relative stores a symbolic link's target relative to the link's folder,
	// so the pair keeps working when the whole tree is moved or mounted
	// elsewhere. Ignored for other kinds and when no relative path exists
	// (different Windows drives).
	Relative bool
	// Overwrite replaces an existing file or link at the link path. A real
	// folder is never replaced.
	Overwrite bool
	// TargetIsDir forces a Windows symbolic link to be a directory link even
	// when the target does not exist yet. When the target exists its type is
	// used. Ignored on other platforms, where symlinks are untyped.
	TargetIsDir bool
}

// CreateSymlink makes linkPath a symbolic link pointing at target. target is
// stored as given unless opt.Relative asks for a relative path; a relative
// target is interpreted relative to linkPath's folder, as the OS does.
func CreateSymlink(target, linkPath string, opt CreateOptions) error {
	if target == "" || linkPath == "" {
		return &os.LinkError{Op: "symlink", Old: target, New: linkPath, Err: fs.ErrInvalid}
	}
	stored := target
	if opt.Relative {
		stored = relativeTarget(target, linkPath)
	}
	isDir := opt.TargetIsDir
	if fi, err := os.Stat(targetFromLink(stored, linkPath)); err == nil {
		isDir = fi.IsDir()
	}
	err := replaceWith(linkPath, targetFromLink(stored, linkPath), opt.Overwrite, func(p string) error {
		return platformSymlink(stored, p, isDir)
	})
	if err != nil && isPrivilegeErr(err) {
		return &os.LinkError{Op: "symlink", Old: target, New: linkPath, Err: fmt.Errorf("%w: %w", ErrPrivilege, unwrapLink(err))}
	}
	return err
}

// CreateHardLink gives the file target a second name, linkPath. Both names
// then refer to the same data; deleting one keeps the other. Folders and
// links across volumes are refused with ErrIsDir and ErrCrossDevice.
func CreateHardLink(target, linkPath string, opt CreateOptions) error {
	fail := func(err error) error { return &os.LinkError{Op: "link", Old: target, New: linkPath, Err: err} }
	if target == "" || linkPath == "" {
		return fail(fs.ErrInvalid)
	}
	fi, err := os.Stat(target)
	if err != nil {
		return fail(unwrapPath(err))
	}
	if fi.IsDir() {
		return fail(ErrIsDir)
	}
	// Hard-link the real file, not a symlink in front of it: link(2) on Linux
	// would otherwise link the symlink itself, unlike macOS and Windows.
	src := target
	if lfi, err := os.Lstat(target); err == nil && lfi.Mode()&fs.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(target); err == nil {
			src = real
		}
	}
	// When linkPath already is a name of the same file (the target itself or
	// an earlier hard link to it), the wanted result exists: replacing it
	// would only risk the data (rename of one hard link over another is a
	// no-op on POSIX and would leave the temporary name behind).
	if li, err := os.Lstat(linkPath); err == nil && opt.Overwrite && os.SameFile(fi, li) {
		return nil
	}
	err = replaceWith(linkPath, "", opt.Overwrite, func(p string) error { return os.Link(src, p) })
	if err != nil && isCrossDeviceErr(err) {
		return fail(fmt.Errorf("%w: %w", ErrCrossDevice, unwrapLink(err)))
	}
	return err
}

// CreateJunction makes linkPath an NTFS junction to the folder target
// (Windows only; elsewhere it returns an error wrapping ErrUnsupported).
// Junctions always store an absolute path and work without special rights.
func CreateJunction(target, linkPath string, opt CreateOptions) error {
	if target == "" || linkPath == "" {
		return &os.LinkError{Op: "junction", Old: target, New: linkPath, Err: fs.ErrInvalid}
	}
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
		return &os.LinkError{Op: "junction", Old: target, New: linkPath, Err: errors.New("junctions can only point to folders")}
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	return replaceWith(linkPath, abs, opt.Overwrite, func(p string) error { return platformJunction(abs, p) })
}

// Create makes a link of the given kind. Symlinks, hard links and junctions
// go to the functions above; .url, .webloc and .desktop shortcuts are written
// by CreateShortcut. KindLnk and KindAlias cannot be written.
func Create(kind Kind, target, linkPath string, opt CreateOptions) error {
	switch kind {
	case KindSymlink:
		return CreateSymlink(target, linkPath, opt)
	case KindHardLink:
		return CreateHardLink(target, linkPath, opt)
	case KindJunction:
		return CreateJunction(target, linkPath, opt)
	case KindURL, KindWebloc, KindDesktop:
		return CreateShortcut(kind, target, linkPath, opt.Overwrite)
	}
	return &os.LinkError{Op: "create " + kind.String(), Old: target, New: linkPath, Err: ErrUnsupported}
}

// SupportedKinds lists the link kinds this platform can create, in the order
// the Create link dialog should offer them.
func SupportedKinds(goos string) []Kind {
	switch goos {
	case "windows":
		return []Kind{KindSymlink, KindHardLink, KindJunction, KindURL}
	case "darwin":
		return []Kind{KindSymlink, KindHardLink, KindWebloc}
	default:
		return []Kind{KindSymlink, KindHardLink, KindDesktop}
	}
}

// BatchRequest asks for one link per source, all in DestDir (TC creates a
// link for every selected file when the dialog is opened on a selection).
type BatchRequest struct {
	Kind    Kind
	Sources []string
	DestDir string
	Options CreateOptions
	// Name returns the link file name for a source; nil uses DefaultLinkName.
	Name func(source string) string
}

// Result reports what happened to one source of a batch.
type Result struct {
	Source string
	Link   string
	Err    error
}

// CreateLinks creates the links of a batch, one Result per source in order.
// It stops early (remaining results carry ctx.Err()) when ctx is cancelled.
func CreateLinks(ctx context.Context, req BatchRequest) []Result {
	out := make([]Result, len(req.Sources))
	for i, src := range req.Sources {
		name := ""
		if req.Name != nil {
			name = req.Name(src)
		} else {
			name = DefaultLinkName(src, req.Kind)
		}
		out[i] = Result{Source: src, Link: filepath.Join(req.DestDir, name)}
		if err := ctx.Err(); err != nil {
			out[i].Err = err
			continue
		}
		out[i].Err = Create(req.Kind, src, out[i].Link, req.Options)
	}
	return out
}

// DefaultLinkName is the name the dialog proposes for a link to source: the
// source's own name, plus the shortcut extension for shortcut kinds.
func DefaultLinkName(source string, kind Kind) string {
	base := filepath.Base(filepath.Clean(source))
	if base == string(filepath.Separator) || base == "." || strings.HasSuffix(base, ":") || base == "" {
		base = "link"
		if v := filepath.VolumeName(source); v != "" {
			base = strings.TrimSuffix(strings.Trim(v, `\/`), ":")
		}
	}
	if ext := ShortcutExt(kind); ext != "" {
		base += ext
	}
	return base
}

// UniqueName returns name if it is free in dir, otherwise the first free
// "name(2).ext", "name(3).ext", ... so a batch never fails on collisions
// when the caller prefers renaming to asking. The numbering is Total
// Commander's auto-rename style, the same as fileops.UniqueName.
func UniqueName(dir, name string) string {
	if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, fs.ErrNotExist) {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		cand := stem + "(" + strconv.Itoa(i) + ")" + ext
		if _, err := os.Lstat(filepath.Join(dir, cand)); errors.Is(err, fs.ErrNotExist) {
			return cand
		}
	}
}

// relativeTarget turns target into a path relative to linkPath's folder when
// possible; both are made absolute first.
func relativeTarget(target, linkPath string) string {
	if !filepath.IsAbs(target) {
		return target
	}
	linkDir, err := filepath.Abs(filepath.Dir(linkPath))
	if err != nil {
		return target
	}
	// Compare real folders so that a link made through a symlinked folder
	// (macOS /var -> /private/var) gets a path that works physically.
	if r, err := filepath.EvalSymlinks(linkDir); err == nil {
		linkDir = r
	}
	tgt := filepath.Clean(target)
	if r, err := filepath.EvalSymlinks(filepath.Dir(tgt)); err == nil {
		tgt = filepath.Join(r, filepath.Base(tgt))
	}
	rel, err := filepath.Rel(linkDir, tgt)
	if err != nil {
		return target
	}
	return rel
}

// targetFromLink returns where a stored link target points, resolving a
// relative target against the link's folder.
func targetFromLink(stored, linkPath string) string {
	if filepath.IsAbs(stored) || filepath.VolumeName(stored) != "" {
		return stored
	}
	return filepath.Join(filepath.Dir(linkPath), stored)
}

// replaceWith runs make(linkPath) when linkPath is free. When it exists and
// overwrite is set (and it is not a real folder), make runs on a temporary
// name next to it which is then renamed over the old entry. target is where
// the new link will point (resolved against linkPath's folder); an overwrite
// is refused with ErrSelfLink when target is linkPath itself or leads to it
// through links, because the entry being replaced would be the very data the
// link is meant to reach.
func replaceWith(linkPath, target string, overwrite bool, make func(string) error) error {
	fi, err := os.Lstat(linkPath)
	if errors.Is(err, fs.ErrNotExist) {
		return make(linkPath)
	}
	if err != nil {
		return err
	}
	if !overwrite {
		return &os.LinkError{Op: "create link", New: linkPath, Err: fs.ErrExist}
	}
	if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 && !isJunction(linkPath, fi) {
		return &os.LinkError{Op: "create link", New: linkPath, Err: fmt.Errorf("a folder with this name exists: %w", fs.ErrExist)}
	}
	if target != "" && leadsTo(target, linkPath, fi) {
		return &os.LinkError{Op: "create link", Old: target, New: linkPath, Err: ErrSelfLink}
	}
	tmp := filepath.Join(filepath.Dir(linkPath), "."+filepath.Base(linkPath)+".mclink-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := make(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, linkPath); err != nil {
		// Windows cannot rename a directory link over another; remove the
		// old one first and try once more.
		if rmErr := os.Remove(linkPath); rmErr != nil {
			os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, linkPath); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}

// leadsTo says whether target is the entry li (the existing linkPath), or
// reaches it by following links one hop at a time. Each hop is compared with
// os.SameFile, so other spellings of the same path (case, "..", hard links)
// are caught too. A chain longer than MaxHops counts as leading there, to stay
// on the safe side.
func leadsTo(target, linkPath string, li fs.FileInfo) bool {
	p := target
	for range MaxHops {
		fi, err := os.Lstat(p)
		if err != nil {
			return false
		}
		if os.SameFile(fi, li) {
			return true
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return false
		}
		next, err := os.Readlink(p)
		if err != nil {
			return false
		}
		if !filepath.IsAbs(next) && filepath.VolumeName(next) == "" {
			next = filepath.Join(filepath.Dir(p), next)
		}
		p = next
	}
	return true
}

func unwrapLink(err error) error {
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}

func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
