// Package branch lists a folder tree as one flat list, for the branch view of the panels (F-086 cm_DirBranch,
// Ctrl+B; F-087 cm_DirBranchSel, Ctrl+Shift+B). Every file under the folder becomes one entry whose name is its path
// relative to the folder ("sub/deeper/a.txt"), with the real file's size, date and flags, so a panel can show the
// whole tree at once and every file command can map a name back to the real file by joining it to the folder.
//
// Only files are listed: a folder has no use in a flat list, and TC's branch view leaves them out as well. Links to
// folders are not followed, so a link that points back up the tree cannot make the walk endless. A file inside a
// hidden folder (".git/config") is marked hidden too, so the panel's "show hidden files" switch hides the insides of
// hidden folders along with the folders themselves. Big trees are cut at a limit, so a branch view of a whole disk
// stays usable and the caller can say the list is incomplete; the walk honours its context, so Esc stops it at once.
package branch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"mormicommander/internal/vfs"
)

// Result is a listed tree.
type Result struct {
	Entries   []vfs.Entry // the files; Name is the path relative to the base folder
	Truncated bool        // the limit was reached, so files are missing
	Skipped   int         // subfolders that could not be read (no permission, gone while listed)
}

// progressEvery is how many files go by between two progress calls.
const progressEvery = 500

// Walk lists every file under base. roots limits the walk to some names of base (the selected files and folders of
// the branch view of a selection); nil or empty walks all of base. limit is the most files listed (0: no limit).
// progress, when given, is called now and then with the number of files found so far.
func Walk(ctx context.Context, base string, roots []string, limit int, progress func(files int)) (Result, error) {
	base = vfs.Local{}.Clean(base)
	w := walker{ctx: ctx, limit: limit, progress: progress}
	if len(roots) == 0 {
		// the base folder itself must be readable: a branch view of a folder that is not there is an error
		l, err := vfs.Local{}.List(ctx, base)
		if err != nil {
			return Result{}, err
		}
		if err := w.add(base, "", false, l.Entries); err != nil {
			return w.res, err
		}
		return w.done()
	}
	for _, r := range roots {
		if w.full() {
			// selected items are left that were not looked at, so the list is incomplete
			w.res.Truncated = true
			break
		}
		full := filepath.Join(base, r)
		li, err := os.Lstat(full)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // gone since it was selected
			}
			w.res.Skipped++
			continue
		}
		hidden := isHiddenPath(r)
		e, err := vfs.Local{}.Stat(full)
		if err != nil {
			continue
		}
		if li.IsDir() {
			// a selected hidden folder hides its files, as a hidden folder deeper down does
			if err := w.dir(full, r, hidden || e.Flags&vfs.FlagHidden != 0); err != nil {
				return w.res, err
			}
			continue
		}
		if e.Flags&vfs.FlagDir != 0 {
			continue // a link to a folder is not followed
		}
		e.Name = r
		w.file(e, hidden)
	}
	return w.done()
}

type walker struct {
	ctx      context.Context
	limit    int
	progress func(int)
	res      Result
}

func (w *walker) full() bool { return w.limit > 0 && len(w.res.Entries) >= w.limit }

func (w *walker) done() (Result, error) {
	if err := w.ctx.Err(); err != nil {
		return w.res, err
	}
	if w.progress != nil {
		w.progress(len(w.res.Entries))
	}
	return w.res, nil
}

// dir lists the folder full, whose path relative to the base is rel.
func (w *walker) dir(full, rel string, hidden bool) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if w.full() {
		return nil
	}
	l, err := vfs.Local{}.List(w.ctx, full)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		w.res.Skipped++
		return nil
	}
	return w.add(full, rel, hidden, l.Entries)
}

// add takes the entries of one folder: files are listed, folders are walked in turn (depth first).
func (w *walker) add(full, rel string, hidden bool, es []vfs.Entry) error {
	var dirs []vfs.Entry
	for _, e := range es {
		if e.Flags&vfs.FlagDir != 0 {
			if e.Flags&vfs.FlagLink == 0 {
				dirs = append(dirs, e)
			}
			continue
		}
		if w.full() {
			w.res.Truncated = true
			return nil
		}
		name := e.Name
		if rel != "" {
			e.Name = filepath.Join(rel, name)
		}
		w.file(e, hidden)
	}
	for _, d := range dirs {
		if w.full() {
			w.res.Truncated = true
			return nil
		}
		sub := d.Name
		if rel != "" {
			sub = filepath.Join(rel, d.Name)
		}
		if err := w.dir(filepath.Join(full, d.Name), sub, hidden || d.Flags&vfs.FlagHidden != 0); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) file(e vfs.Entry, hidden bool) {
	if hidden {
		e.Flags |= vfs.FlagHidden
	}
	w.res.Entries = append(w.res.Entries, e)
	if w.progress != nil && len(w.res.Entries)%progressEvery == 0 {
		w.progress(len(w.res.Entries))
	}
}

// isHiddenPath says whether a relative path goes through a dot folder ("a/.git/x"); the last part does not count,
// because the file's own flags already say whether it is hidden.
func isHiddenPath(rel string) bool {
	dir := filepath.Dir(rel)
	for dir != "." && dir != string(filepath.Separator) && dir != "" {
		if b := filepath.Base(dir); len(b) > 1 && b[0] == '.' {
			return true
		}
		up := filepath.Dir(dir)
		if up == dir {
			break
		}
		dir = up
	}
	return false
}
