// This file changes existing tar archives: copying files into an archive
// panel (F5), moving them in (Alt+Shift+F5), deleting (F8), renaming
// (Shift+F6), making a folder (F7) and putting back a file edited from
// inside the archive (F4). Tar cannot be edited in place, least of all when
// compressed, so every change rewrites the archive: the old members are
// streamed into a new temporary archive with the same compression, with
// deleted ones left out, renamed ones given their new name and replaced ones
// dropped, then folders and added files are appended and the new archive
// takes the old one's place in a single rename. If anything fails the
// original stays untouched.
package tarfmt

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	archive "mormicommander/internal/archive/arcapi"
)

// Rename is one rename inside an archive; renaming a folder moves
// everything below it.
type Rename struct{ From, To string }

// Changes lists what Modify should do in one rewrite.
type Changes struct {
	Delete []string
	Rename []Rename
	Mkdir  []string
	// Add packs these local paths, using the packing options in AddOptions
	// (BaseDir, Prefix, NoPaths, NoRecurse, FollowSymlinks, RemoveSources).
	// Members with the same name are replaced.
	Add        []string
	AddOptions CreateOptions
	// Level is the compression level for the rewritten archive.
	Level    int
	Progress func(archive.Progress)
}

// paxKeysOwnedByHeader are PAX records archive/tar derives from Header
// fields; copying them verbatim would clash with renamed names or turn
// sparse files back into a format the writer cannot produce.
var paxKeysOwnedByHeader = []string{"path", "linkpath", "size", "uid", "gid", "uname", "gname", "mtime", "atime", "ctime"}

func cleanPAX(rec map[string]string) map[string]string {
	if len(rec) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range rec {
		if strings.HasPrefix(k, "GNU.sparse.") {
			continue
		}
		owned := false
		for _, o := range paxKeysOwnedByHeader {
			if k == o {
				owned = true
				break
			}
		}
		if !owned {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Modify applies ch to the tar archive at archivePath. Single-file formats
// (.gz, .xz, ...) cannot hold more than one file and return
// archive.ErrNotSupported.
func Modify(ctx context.Context, archivePath string, ch Changes) error {
	r, err := Open(archivePath)
	if err != nil {
		return err
	}
	rClosed := false
	defer func() {
		if !rClosed {
			r.Close()
		}
	}()
	if !r.format.IsTar() {
		return archive.ErrNotSupported
	}
	if _, err := r.Entries(ctx); err != nil {
		return err
	}
	arcInfo, err := os.Stat(archivePath)
	if err != nil {
		return err
	}

	deletes := cleanAll(ch.Delete)
	for _, d := range deletes {
		if d == "" {
			return errors.New("tarfmt: cannot delete the archive root")
		}
		if _, err := r.Stat(ctx, d); err != nil {
			return err
		}
	}
	type ren struct{ from, to string }
	var renames []ren
	for _, rn := range ch.Rename {
		from, to := cleanName(rn.From), cleanName(rn.To)
		if from == "" || to == "" || !isLocalName(to) {
			return fmt.Errorf("tarfmt: invalid rename %q -> %q", rn.From, rn.To)
		}
		if _, err := r.Stat(ctx, from); err != nil {
			return err
		}
		if from == to {
			continue
		}
		if underDir(to, from) {
			return fmt.Errorf("tarfmt: cannot move %q into itself", from)
		}
		if _, err := r.Stat(ctx, to); err == nil {
			return &archive.EntryError{Name: to, Err: fs.ErrExist}
		}
		renames = append(renames, ren{from, to})
	}
	rename := func(name string) string {
		for _, rn := range renames {
			if underDir(name, rn.from) {
				return rn.to + name[len(rn.from):]
			}
		}
		return name
	}
	deleted := func(name string) bool { return len(deletes) > 0 && selected(name, deletes) }

	tmp, err := tempFor(archivePath)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	var adds []item
	if len(ch.Add) > 0 {
		skip := []fs.FileInfo{arcInfo}
		if fi, err := tmp.Stat(); err == nil {
			skip = append(skip, fi)
		}
		adds, err = collect(ctx, ch.Add, ch.AddOptions, skip)
		if err != nil {
			return err
		}
	}
	addNames := map[string]bool{}
	prog := progressFn(ch.Progress, r.size, nil)
	for _, it := range adds {
		addNames[it.name] = true
		if it.link == "" && it.fi.Mode().IsRegular() {
			prog.p.Total += it.fi.Size()
		}
	}
	dropped := func(name string) bool { return deleted(name) || addNames[rename(name)] }

	// Hard links whose data member is dropped must receive the data
	// themselves: the first surviving link becomes the data member and
	// later links point to it.
	r.mu.Lock()
	needData := map[string]bool{}
	for name, h := range r.headers {
		if h.Typeflag == tar.TypeLink && !dropped(name) && dropped(cleanName(h.Linkname)) {
			needData[cleanName(h.Linkname)] = true
		}
	}
	r.mu.Unlock()
	movedData := map[string]string{} // old data name -> new name holding it
	stash := map[string]string{}     // dropped data name -> temp file with its data
	defer func() {
		for _, p := range stash {
			os.Remove(p)
		}
	}()

	cw, err := compress(r.format.Compression(), tmp, ch.Level)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(cw)
	fail := func(err error) error {
		tw.Close()
		cw.Close()
		return err
	}

	p, err := r.newPass(ctx)
	if err != nil {
		return fail(err)
	}
	prog.raw = p.raw
	err = r.walk(ctx, p, func(h *tar.Header, name string, tr *tar.Reader) error {
		if dropped(name) {
			if needData[name] && isRegHeader(h) {
				// Keep the data aside until a surviving link claims it.
				f, err := os.CreateTemp(filepath.Dir(archivePath), ".mctar-link-*")
				if err != nil {
					return err
				}
				stash[name] = f.Name()
				_, err = io.Copy(f, tr)
				if cerr := f.Close(); err == nil {
					err = cerr
				}
				return err
			}
			return nil
		}
		nh := *h
		newName := rename(name)
		nh.Name = newName
		if isDirHeader(h) {
			nh.Name += "/"
		}
		nh.PAXRecords = cleanPAX(h.PAXRecords)
		nh.Format = tar.FormatUnknown
		nh.AccessTime, nh.ChangeTime = time.Time{}, time.Time{}
		var data io.Reader = tr
		if isRegHeader(h) {
			nh.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeLink {
			target := cleanName(h.Linkname)
			if moved, ok := movedData[target]; ok {
				nh.Linkname = moved
			} else if sp, ok := stash[target]; ok {
				f, err := os.Open(sp)
				if err != nil {
					return err
				}
				defer f.Close()
				fi, err := f.Stat()
				if err != nil {
					return err
				}
				nh.Typeflag = tar.TypeReg
				nh.Linkname = ""
				nh.Size = fi.Size()
				data = f
				movedData[target] = newName
			} else {
				nh.Linkname = rename(target)
			}
		}
		prog.file(newName)
		if err := tw.WriteHeader(&nh); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if nh.Typeflag == tar.TypeReg && nh.Size > 0 {
			if _, err := io.Copy(prog.writer(tw), data); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	})
	p.Close()
	if err != nil {
		return fail(err)
	}

	// New folders, unless they already exist after the other changes.
	exists := func(name string) bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, e := range r.entries {
			if !dropped(e.Path) && rename(e.Path) == name {
				return true
			}
		}
		return addNames[name]
	}
	now := time.Now()
	for _, m := range cleanAll(ch.Mkdir) {
		if !isLocalName(m) {
			return fail(fmt.Errorf("tarfmt: invalid folder name %q", m))
		}
		if exists(m) {
			continue
		}
		h := &tar.Header{Typeflag: tar.TypeDir, Name: m + "/", Mode: 0o755, ModTime: now}
		if err := tw.WriteHeader(h); err != nil {
			return fail(err)
		}
		addNames[m] = true
	}

	pk := newPacker(ctx, tw, prog)
	for _, it := range adds {
		if err := pk.add(it); err != nil {
			return fail(err)
		}
	}
	if err := tw.Close(); err != nil {
		cw.Close()
		return err
	}
	if err := cw.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if ch.AddOptions.RemoveSources && len(adds) > 0 {
		if err := Test(ctx, tmpName, nil); err != nil {
			return fmt.Errorf("tarfmt: rewritten archive failed its test, sources kept: %w", err)
		}
	}
	_ = os.Chmod(tmpName, arcInfo.Mode().Perm())
	r.Close()
	rClosed = true
	if err := os.Rename(tmpName, archivePath); err != nil {
		return err
	}
	ok = true
	prog.flush()
	if ch.AddOptions.RemoveSources && len(adds) > 0 {
		return removeItems(adds)
	}
	return nil
}
