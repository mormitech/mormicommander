package fileops

// Delete, trash and wipe. A permanent delete lists everything and removes it depth first, files before the folder
// that holds them; links are removed, never what they point to. TC's confirmations can be switched on per job: one
// per non-empty folder ("Delete the folder with all its contents?" Yes / All / Skip / Skip all) and one per read-only
// file. When something can't be removed, the folders above it stay without a second, noisy error.
//
// Trash hands each selected item to the system's trash (internal/trash), so it can be put back with the system's own
// tools. On a volume without a trash (some USB sticks and network shares) the move fails, and with OnError set to ask
// the user can choose to delete that item permanently instead (TC's F8 on such volumes).
//
// On macOS the trash goes through Finder (for "Put Back"), and a Finder call can fail or hang while macOS waits for
// the automation permission. The call stops when the job is cancelled, and the error question then also offers to
// trash the item (or every remaining item) without Finder, through the file system's own trash call (issue #36).
//
// Wipe (TC's "secure delete") overwrites the contents of every file with random bytes, as many passes as asked,
// flushes each pass to the disk, cuts the file to zero length, renames it to a random name so the old name doesn't
// linger in the folder either, and only then removes it. On SSDs and copy-on-write file systems (APFS, Btrfs) the old
// blocks may survive elsewhere on the device; this is the same limit every file-level wipe has.

import (
	crand "crypto/rand"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"mormicommander/internal/trash"
)

// trashMove, trashDirect and trashDirectOK are the trash package's calls; the tests replace them, so they never touch
// the real trash.
var (
	trashMove     = trash.MoveContext
	trashDirect   = trash.MoveDirect
	trashDirectOK = trash.DirectSupported
)

// TrashSupported says whether this platform has a trash the engine can use at all.
func TrashSupported() bool { return trash.Supported() }

// deleteAll removes every source permanently (wipe: overwriting the files first).
func (j *job) deleteAll(wipe bool) {
	verb := "delete"
	if wipe {
		verb = "wipe"
	}
	passes := int64(max(j.req.WipePasses, 1))
	var items []item
	for i, src := range j.req.Sources {
		if j.ctx.Err() != nil {
			return
		}
		src = filepath.Clean(src)
		si, err := lstat(src)
		if err != nil {
			j.fail("Can't %s %s: %s.", verb, src, plainErr(err))
			continue
		}
		if si.IsDir() && j.req.ConfirmDirs && !emptyDir(src) && !j.confirmDir(src) {
			if j.ctx.Err() != nil {
				return
			}
			continue
		}
		items = append(items, j.walk(i, src, "", si, walkOpts{})...)
	}
	var total int64
	files := 0
	for _, it := range items {
		if !it.info.IsDir() {
			files++
			if wipe && it.info.Mode().IsRegular() {
				total += it.info.Size() * passes
			}
		}
	}
	j.update(func(p *Progress) { p.Files = files; p.Bytes = total })
	j.tick(true)
	kept := map[string]bool{}              // folders that keep something, because an item under them stayed
	for i := len(items) - 1; i >= 0; i-- { // children come after their folder, so backwards is depth first
		it := items[i]
		if kept[it.src] {
			continue
		}
		size := int64(0)
		if wipe && it.info.Mode().IsRegular() {
			size = it.info.Size() * passes
		}
		if !j.at(it.src, size) {
			return
		}
		if !j.removeItem(it, wipe, verb) {
			for d := filepath.Dir(it.src); inside(d, it.topSrc); d = filepath.Dir(d) {
				kept[d] = true
				if d == it.topSrc {
					break
				}
			}
			j.passBytes(size - j.snapshot().CurrentDone)
			continue
		}
		if !it.info.IsDir() {
			j.update(func(p *Progress) { p.FilesDone++ })
		}
		j.tick(false)
	}
}

// confirmDir asks before a non-empty folder is removed. It returns true to go ahead.
func (j *job) confirmDir(src string) bool {
	if j.delDirsAll {
		return true
	}
	if j.delDirSkip {
		j.skipped()
		return false
	}
	a := j.ask(Conflict{Type: AskDeleteDir, Source: src,
		Choices: []Choice{ChoiceYes, ChoiceAll, ChoiceSkip, ChoiceSkipAll, ChoiceCancel}})
	switch a.Choice {
	case ChoiceYes:
		return true
	case ChoiceAll:
		j.delDirsAll = true
		return true
	case ChoiceSkipAll:
		j.delDirSkip = true
	case ChoiceCancel:
		return false
	}
	j.skipped()
	return false
}

func emptyDir(p string) bool {
	f, err := open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == io.EOF
}

// removeItem removes (or wipes) one file, link or folder. It returns false when the item stays.
func (j *job) removeItem(it item, wipe bool, verb string) bool {
	if j.req.ConfirmReadOnly && readOnly(it.info) {
		switch j.delROAll {
		case -1:
			j.skipped()
			return false
		case 0:
			a := j.ask(Conflict{Type: AskDeleteReadOnly, Source: it.src, SrcSize: it.info.Size(),
				SrcTime: it.info.ModTime().UnixMilli(),
				Choices: []Choice{ChoiceYes, ChoiceAll, ChoiceSkip, ChoiceSkipAll, ChoiceCancel}})
			switch a.Choice {
			case ChoiceYes:
			case ChoiceAll:
				j.delROAll = 1
			case ChoiceSkipAll:
				j.delROAll = -1
				j.skipped()
				return false
			case ChoiceCancel:
				return false
			default:
				j.skipped()
				return false
			}
		}
	}
	err, _ := j.try("Can't "+verb+" "+it.src, it.src, "", nil, func() error {
		if wipe {
			return j.wipeOne(it)
		}
		return removePath(it.src, it.info)
	})
	if err != nil {
		return false
	}
	j.log(verb, it.src, "", nil)
	return true
}

// removePath removes one file, link or empty folder; a file that is gone already counts as removed.
func removePath(p string, info fs.FileInfo) error {
	err := remove(p)
	if err != nil && isPerm(err) && !info.IsDir() && !isLink(info) {
		makeWritable(p, info) // a read-only file on Windows can't be removed until the flag is off
		err = remove(p)
	}
	if isNotExist(err) {
		return nil
	}
	return err
}

// wipeOne overwrites a regular file, then hides its name and removes it. Folders and links are renamed and removed.
//
// A file with more than one name (a hard link elsewhere) is only unlinked: overwriting it would destroy the data that
// the other name still shows, which is not what wiping this one name means. If the removal fails after the random
// rename, the item gets its old name back, so a folder that is not empty doesn't stay behind under a garbage name.
func (j *job) wipeOne(it item) error {
	p := it.src
	if it.info.Mode().IsRegular() {
		if linkCount(p, it.info) > 1 {
			j.addBytes(it.info.Size() * int64(max(j.req.WipePasses, 1))) // nothing to overwrite; keep the totals right
		} else if err := j.overwrite(p, it.info); err != nil {
			return err
		}
	}
	q, err := renameRandom(p)
	if err != nil {
		q = p
	}
	if err := removePath(q, it.info); err != nil {
		if q != p {
			rename(q, p)
		}
		return err
	}
	return nil
}

// overwrite writes random bytes over the whole file, pass after pass, then cuts it to zero length.
func (j *job) overwrite(p string, info fs.FileInfo) (err error) {
	if readOnly(info) {
		if err := makeWritable(p, info); err != nil {
			return err
		}
	}
	f, err := openFile(p, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	var done int64
	defer func() {
		if err != nil {
			f.Close()
			j.addBytes(-done)
		}
	}()
	var seed [32]byte
	crand.Read(seed[:])
	rng := rand.NewChaCha8(seed)
	size := info.Size()
	buf := make([]byte, min(int64(defaultBuffer), max(size, 1)))
	for pass := 0; pass < max(j.req.WipePasses, 1); pass++ {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		for left := size; left > 0; {
			if !j.gate() {
				return j.ctx.Err()
			}
			n := min(int64(len(buf)), left)
			rng.Read(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			left -= n
			done += n
			j.addBytes(n)
			j.tick(false)
		}
		if err := f.Sync(); err != nil {
			return err
		}
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	f.Sync()
	return f.Close()
}

// renameRandom renames p to a random name of the same length in the same folder and returns the new path.
func renameRandom(p string) (string, error) {
	dir, name := filepath.Dir(p), filepath.Base(p)
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	for try := 0; try < 5; try++ {
		var b strings.Builder
		for range max(len(name), 8) {
			b.WriteByte(letters[rand.IntN(len(letters))])
		}
		q := filepath.Join(dir, b.String())
		if _, err := lstat(q); err == nil {
			continue
		}
		if err := rename(p, q); err != nil {
			return p, err
		}
		return q, nil
	}
	return p, os.ErrExist
}

// trashAll moves each source to the system's trash, one by one.
func (j *job) trashAll() {
	j.update(func(p *Progress) { p.Files = len(j.req.Sources) })
	j.tick(true)
	for i, src := range j.req.Sources {
		src = filepath.Clean(src)
		if !j.at(src, 0) {
			return
		}
		err, extra := j.trashOne(src)
		if extra == ChoiceDelete {
			if j.deleteTree(i, src) {
				j.update(func(p *Progress) { p.FilesDone++ })
			}
			continue
		}
		if err != nil {
			continue
		}
		j.log("trash", src, "", nil)
		j.update(func(p *Progress) { p.FilesDone++ })
		j.tick(false)
	}
}

// trashOne trashes one item, asking on errors as try does. Where the platform has a route around Finder (macOS) the
// question also offers it, for this item or for the rest of the job; once chosen, the item is tried that way at once.
func (j *job) trashOne(src string) (error, Choice) {
	what := "Can't move " + filepath.Base(src) + " to the trash"
	direct := func() error { return trashDirect(j.ctx, src) }
	if j.trashDirect {
		return j.try(what, src, "", []Choice{ChoiceDelete}, direct)
	}
	choices := []Choice{ChoiceDelete}
	if trashDirectOK() {
		choices = []Choice{ChoiceTrashDirect, ChoiceTrashDirectAll, ChoiceDelete}
	}
	err, extra := j.try(what, src, "", choices, func() error { return trashMove(j.ctx, src) })
	switch extra {
	case ChoiceTrashDirectAll:
		j.trashDirect = true
		fallthrough
	case ChoiceTrashDirect:
		return j.try(what, src, "", []Choice{ChoiceDelete}, direct)
	}
	return err, extra
}

// deleteTree removes src and everything under it permanently (the fallback when the trash can't take it).
func (j *job) deleteTree(top int, src string) bool {
	si, err := lstat(src)
	if err != nil {
		j.fail("Can't delete %s: %s.", src, plainErr(err))
		return false
	}
	items := j.walk(top, src, "", si, walkOpts{})
	ok := true
	for i := len(items) - 1; i >= 0; i-- {
		if j.ctx.Err() != nil {
			return false
		}
		if err := removePath(items[i].src, items[i].info); err != nil {
			j.fail("Can't delete %s: %s.", items[i].src, plainErr(err))
			ok = false
		}
	}
	if ok {
		j.log("delete", src, "", nil)
	}
	return ok
}
