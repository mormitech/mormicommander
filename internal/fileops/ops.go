package fileops

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"mormicommander/internal/trash"
)

// item is one thing a job works on: a folder, a file or a link, with where it goes.
type item struct {
	src, dst string
	info     fs.FileInfo // from Lstat: links are not followed
	top      int         // index of the source it belongs to
}

// plainErr makes a system error a short plain phrase for the UI ("permission denied", "the disk is full").
func plainErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "it is not there any more"
	case errors.Is(err, syscall.ENOSPC):
		return "the disk is full"
	}
	return err.Error()
}

// inside says whether p is the folder dir itself or anything under it.
func inside(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// samePath compares two cleaned paths; on case-insensitive systems (macOS, Windows default) case is ignored, which
// errs on the safe side: "copy a.txt onto A.TXT" is refused instead of truncating the source.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	if si, err := os.Stat(a); err == nil {
		if di, err := os.Stat(b); err == nil {
			return os.SameFile(si, di)
		}
	}
	return false
}

// plan lists src and everything under it (folders before their contents), with their places under dst.
func (j *job) plan(top int, src, dst string) ([]item, error) {
	var out []item
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if j.ctx.Err() != nil {
			return j.ctx.Err()
		}
		if err != nil {
			j.fail("Can't read %s: %s.", p, plainErr(err))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			j.fail("Can't read %s: %s.", p, plainErr(err))
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		out = append(out, item{src: p, dst: filepath.Join(dst, rel), info: info, top: top})
		return nil
	})
	return out, err
}

func (j *job) targetOf(src string) string {
	name := filepath.Base(src)
	if j.req.Rename != "" {
		name = j.req.Rename
	}
	return filepath.Join(j.req.Target, name)
}

// copyAll copies (or moves) every source into the target folder.
func (j *job) copyAll(move bool) {
	verb := "copy"
	if move {
		verb = "move"
	}
	var items []item
	tops := map[int]string{} // the sources that are copied (and, for a move, removed after)
	for i, src := range j.req.Sources {
		src = filepath.Clean(src)
		dst := j.targetOf(src)
		si, err := os.Lstat(src)
		if err != nil {
			j.fail("Can't %s %s: %s.", verb, src, plainErr(err))
			continue
		}
		if samePath(src, dst) {
			if !move {
				j.fail("Can't copy %s onto itself.", filepath.Base(src))
			}
			continue
		}
		if si.IsDir() && inside(dst, src) {
			j.fail("Can't %s the folder %s into itself.", verb, filepath.Base(src))
			continue
		}
		if move {
			if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
				if os.Rename(src, dst) == nil { // same volume: a rename, done at once
					j.update(func(p *Progress) { p.Files++; p.FilesDone++ })
					continue
				}
			}
		}
		its, err := j.plan(i, src, dst)
		if err != nil {
			return // cancelled
		}
		items = append(items, its...)
		tops[i] = src
	}
	var total int64
	files := 0
	for _, it := range items {
		if !it.info.IsDir() {
			files++
			if it.info.Mode().IsRegular() {
				total += it.info.Size()
			}
		}
	}
	j.update(func(p *Progress) { p.Files += files; p.Bytes = total })
	j.tick(true)

	bad := map[int]bool{} // sources with a skipped or failed file: a move keeps them
	var copied []item     // what was copied, for a move to remove
	var dirs []item       // folders made, to give them their dates at the end
	for _, it := range items {
		if j.ctx.Err() != nil {
			return
		}
		if j.q.beforeFile != nil {
			j.q.beforeFile(it.src)
		}
		j.update(func(p *Progress) { p.Current = it.src })
		ok := j.copyItem(it, verb, &dirs)
		if !ok {
			bad[it.top] = true
			continue
		}
		copied = append(copied, it)
		if !it.info.IsDir() {
			j.update(func(p *Progress) { p.FilesDone++ })
		}
		j.tick(false)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Chtimes(dirs[i].dst, dirs[i].info.ModTime(), dirs[i].info.ModTime())
	}
	if !move || j.ctx.Err() != nil {
		return
	}
	for top, src := range tops {
		if !bad[top] {
			if err := os.RemoveAll(src); err != nil {
				j.fail("Copied %s, but can't remove the original: %s.", filepath.Base(src), plainErr(err))
			}
		}
	}
	// sources with problems: remove only what was copied, then the folders that became empty
	for i := len(copied) - 1; i >= 0; i-- {
		it := copied[i]
		if bad[it.top] {
			os.Remove(it.src) // (a folder that still has files left in it stays)
		}
	}
}

// copyItem copies one folder, file or link. It returns false when the item was skipped or failed.
func (j *job) copyItem(it item, verb string, dirs *[]item) bool {
	mode := it.info.Mode()
	if mode.IsDir() {
		if di, err := os.Lstat(it.dst); err == nil {
			if di.IsDir() {
				return true // merge into the folder that is there
			}
			j.fail("Can't %s the folder %s: a file of that name is in the way.", verb, filepath.Base(it.src))
			return false
		}
		if err := os.MkdirAll(it.dst, mode.Perm()|0o700); err != nil {
			j.fail("Can't make the folder %s: %s.", it.dst, plainErr(err))
			return false
		}
		*dirs = append(*dirs, it)
		return true
	}
	dst, ok := j.resolve(it)
	if !ok {
		j.update(func(p *Progress) { p.Skipped++; p.BytesDone += it.info.Size() })
		return false
	}
	if mode&fs.ModeSymlink != 0 {
		target, err := os.Readlink(it.src)
		if err == nil {
			os.Remove(dst)
			err = os.Symlink(target, dst)
		}
		if err != nil {
			j.fail("Can't %s the link %s: %s.", verb, filepath.Base(it.src), plainErr(err))
			return false
		}
		return true
	}
	if !mode.IsRegular() {
		j.fail("Skipped %s: it is not a regular file.", filepath.Base(it.src))
		return false
	}
	if err := j.copyFile(it.src, dst, it.info); err != nil {
		if j.ctx.Err() == nil {
			j.fail("Can't %s %s: %s.", verb, filepath.Base(it.src), plainErr(err))
		}
		return false
	}
	return true
}

// resolve decides what to do when the target exists: the path to write to, or false to skip.
func (j *job) resolve(it item) (string, bool) {
	dst := it.dst
	for {
		di, err := os.Lstat(dst)
		if err != nil {
			return dst, true // nothing in the way
		}
		if di.IsDir() {
			j.fail("Can't overwrite the folder %s with a file.", dst)
			return "", false
		}
		older := di.ModTime().Before(it.info.ModTime())
		switch j.policy {
		case Overwrite:
			return dst, true
		case Skip:
			return "", false
		case Older:
			return dst, older
		}
		a := j.ask(Conflict{Source: it.src, Target: dst, SrcSize: it.info.Size(), DstSize: di.Size(),
			SrcTime: it.info.ModTime().UnixMilli(), DstTime: di.ModTime().UnixMilli()})
		switch a.Choice {
		case ChoiceOverwrite:
			return dst, true
		case ChoiceOverwriteAll:
			j.policy = Overwrite
			return dst, true
		case ChoiceSkipAll:
			j.policy = Skip
			return "", false
		case ChoiceOlderAll:
			j.policy = Older
			return dst, older
		case ChoiceRename:
			name := filepath.Base(a.NewName) // a name, never a path somewhere else
			if name == "" || name == "." || name == ".." {
				return "", false
			}
			dst = filepath.Join(filepath.Dir(dst), name)
			continue // the new name may be taken too
		case ChoiceCancel:
			j.cancel()
			return "", false
		default: // skip, or an answer we don't know
			return "", false
		}
	}
}

// copyFile copies the bytes, then the permissions and the date. A file left half-written by a cancel is removed.
func (j *job) copyFile(src, dst string, info fs.FileInfo) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()|0o200)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dst)
			return
		}
		os.Chmod(dst, info.Mode().Perm())
		os.Chtimes(dst, info.ModTime(), info.ModTime())
	}()
	buf := make([]byte, 1<<20)
	for {
		if err := j.ctx.Err(); err != nil {
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			j.update(func(p *Progress) { p.BytesDone += int64(n) })
			j.tick(false)
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// deleteAll removes every source for good: files first, then the folders from the deepest up. Links are removed, never
// what they point to.
func (j *job) deleteAll() {
	var items []item
	for i, src := range j.req.Sources {
		its, err := j.plan(i, filepath.Clean(src), "")
		if err != nil {
			return
		}
		items = append(items, its...)
	}
	files := 0
	for _, it := range items {
		if !it.info.IsDir() {
			files++
		}
	}
	j.update(func(p *Progress) { p.Files = files })
	j.tick(true)
	for i := len(items) - 1; i >= 0; i-- { // children come after their folder in the plan, so backwards is depth first
		it := items[i]
		if j.ctx.Err() != nil {
			return
		}
		if j.q.beforeFile != nil {
			j.q.beforeFile(it.src)
		}
		j.update(func(p *Progress) { p.Current = it.src })
		err := os.Remove(it.src)
		if err != nil && errors.Is(err, fs.ErrPermission) && !it.info.IsDir() {
			os.Chmod(it.src, 0o644) // a read-only file on Windows
			err = os.Remove(it.src)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			j.fail("Can't delete %s: %s.", it.src, plainErr(err))
			continue
		}
		if !it.info.IsDir() {
			j.update(func(p *Progress) { p.FilesDone++ })
		}
		j.tick(false)
	}
}

// trashAll moves each source to the system's trash, one by one.
func (j *job) trashAll() {
	j.update(func(p *Progress) { p.Files = len(j.req.Sources) })
	j.tick(true)
	for _, src := range j.req.Sources {
		if j.ctx.Err() != nil {
			return
		}
		j.update(func(p *Progress) { p.Current = src })
		if err := trashMove(src); err != nil {
			j.fail("Can't move %s to the trash: %s.", filepath.Base(src), plainErr(err))
			continue
		}
		j.update(func(p *Progress) { p.FilesDone++ })
		j.tick(false)
	}
}

// trashMove is trash.Move; the tests replace it, so they never touch the real trash.
var trashMove = trash.Move
