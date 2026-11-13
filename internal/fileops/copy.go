package fileops

// Copy and move. A copy lists everything first (walk.go), checks that the target volume has room, then copies folder
// by folder and file by file, asking about files that are in the way (conflict.go). Bytes go through one buffer at a
// time so the job can be paused, cancelled and held to its speed limit between blocks; a file left half-written by a
// cancel or an error is removed (or cut back to its old size after an append), so no broken copy stays behind.
//
// A move on one volume renames: a whole source at once when nothing is in the way, otherwise file by file into the
// existing folders. Across volumes a move copies each file and removes its source right after that one copy worked
// (and was verified, when asked), so a cancel or an error in the middle leaves every file in exactly one complete
// place. Source folders that end up empty are removed at the end; a folder that still holds a skipped file stays.

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// sameVolume is SameVolume; the tests replace it to drive the copy-and-delete path of a move on one disk.
var sameVolume = SameVolume

const defaultBuffer = 1 << 20

// copyAll copies (or moves) every source into the target folder.
func (j *job) copyAll(move bool) {
	verb := "copy"
	if move {
		verb = "move"
	}
	target := filepath.Clean(j.req.Target)
	if err, _ := j.try("Can't make the target folder "+target, "", target, nil, func() error { return mkdirAll(target, 0o777) }); err != nil {
		return
	}
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
		name := ApplyMask(j.req.Rename, filepath.Base(src))
		dst := filepath.Join(target, name)
		if samePath(src, dst) {
			switch {
			case move && filepath.Base(src) != name: // only the case of the name changes
				if _, err := RenameItem(src, name); err != nil {
					j.fail("Can't rename %s: %s.", filepath.Base(src), plainErr(err))
				} else {
					j.update(func(p *Progress) { p.Files++; p.FilesDone++ })
					j.log("rename", src, dst, nil)
				}
				continue
			case move: // a move onto itself: nothing to do
				continue
			case j.policy == AutoRenameSource: // Shift+F5 into the same folder: the copy gets a free name
				dst = filepath.Join(target, UniqueName(target, name))
			default:
				j.fail("Can't copy %s onto itself.", filepath.Base(src))
				continue
			}
		}
		if si.IsDir() && inside(dst, src) {
			j.fail("Can't %s the folder %s into itself.", verb, filepath.Base(src))
			continue
		}
		same := move && sameVolume(src, target)
		if same && j.filter.Empty() {
			if _, err := lstat(dst); isNotExist(err) {
				if !j.at(src, 0) {
					return
				}
				err := rename(src, dst)
				if err != nil && !crossDevice(err) {
					err, _ = j.try("Can't move "+filepath.Base(src), src, dst, nil, func() error { return rename(src, dst) })
					if err != nil {
						continue
					}
				}
				if err == nil { // same volume: a rename, done at once
					j.update(func(p *Progress) { p.Files++; p.FilesDone++ })
					j.log("move", src, dst, nil)
					j.tick(false)
					continue
				}
				same = false // the volumes differ after all: copy and delete
			}
		}
		o := walkOpts{filter: true}
		if !move {
			o.follow = j.req.Links == LinksFollow
			o.skipLn = j.req.Links == LinksSkip
		}
		its := j.walk(i, src, dst, si, o)
		for k := range its {
			its[k].same = same
		}
		items = append(items, its...)
	}
	if j.ctx.Err() != nil {
		return
	}
	var total, need int64
	files := 0
	for _, it := range items {
		if !it.info.IsDir() {
			files++
			if it.info.Mode().IsRegular() {
				total += it.info.Size()
				if !it.same {
					need += it.info.Size()
				}
			}
		}
	}
	j.update(func(p *Progress) { p.Files += files; p.Bytes = total })
	j.tick(true)
	if !j.checkSpace(need) {
		return
	}

	var dirs []item // folders made or merged, to give them their dates and rights at the end
	var srcDirs []item
	for _, it := range items {
		size := int64(0)
		if it.info.Mode().IsRegular() {
			size = it.info.Size()
		}
		if !j.at(it.src, size) {
			break
		}
		if it.info.IsDir() {
			made, ok := j.copyDir(it, verb)
			if ok {
				if made {
					dirs = append(dirs, it)
				}
				if move {
					srcDirs = append(srcDirs, it)
				}
			}
			continue
		}
		if j.copyEntry(it, verb, move) {
			j.update(func(p *Progress) { p.FilesDone++ })
		}
		j.tick(false)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		j.copyMeta(dirs[i].src, dirs[i].dst, dirs[i].info)
	}
	if !move {
		return
	}
	// the source folders that are empty now go, deepest first; one that still holds something just stays
	for i := len(srcDirs) - 1; i >= 0; i-- {
		if remove(srcDirs[i].src) == nil {
			j.log("rmdir", srcDirs[i].src, "", nil)
		}
	}
}

// copyDir makes one folder in the target, or merges into the folder that is there. made says whether it was made
// (and so gets the source's dates and rights at the end).
func (j *job) copyDir(it item, verb string) (made, ok bool) {
	if di, err := stat(it.dst); err == nil {
		if di.IsDir() {
			return false, true // merge into the folder that is there
		}
		j.fail("Can't %s the folder %s: a file of that name is in the way.", verb, filepath.Base(it.src))
		return false, false
	}
	err, _ := j.try("Can't make the folder "+it.dst, it.src, it.dst, nil, func() error {
		err := mkdir(it.dst, it.info.Mode().Perm()|0o700)
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	})
	if err != nil {
		return false, false
	}
	j.log("mkdir", it.src, it.dst, nil)
	return true, true
}

// copyEntry copies (or moves) one file or link. It returns true when the item reached the target.
func (j *job) copyEntry(it item, verb string, move bool) bool {
	mode := it.info.Mode()
	size := int64(0)
	if mode.IsRegular() {
		size = it.info.Size()
	}
	if !mode.IsRegular() && !isLink(it.info) {
		j.fail("Skipped %s: it is not a regular file.", filepath.Base(it.src))
		j.addBytes(size)
		return false
	}
	dst, wm, out := j.resolve(it)
	if out != outWrite {
		if out == outSkip {
			j.skipped()
			j.log("skip", it.src, dst, nil)
		}
		j.addBytes(size)
		return false
	}
	name := filepath.Base(it.src)
	if isLink(it.info) {
		if it.same {
			if err := rename(it.src, dst); err == nil {
				j.log(verb, it.src, dst, nil)
				return true
			}
		}
		err, _ := j.try("Can't "+verb+" the link "+name, it.src, dst, nil, func() error {
			target, err := readlink(it.src)
			if err != nil {
				return err
			}
			if err := remove(dst); err != nil && !isNotExist(err) {
				return err
			}
			return symlink(target, dst)
		})
		if err != nil {
			return false
		}
		if j.req.Owner {
			copyOwner(dst, it.info)
		}
		j.log(verb, it.src, dst, nil)
		return !move || j.removeSource(it)
	}
	if it.same && wm == wmCreate {
		err := rename(it.src, dst)
		if err == nil {
			j.addBytes(size)
			j.log(verb, it.src, dst, nil)
			return true
		}
		if !crossDevice(err) {
			err, _ = j.try("Can't move "+name, it.src, dst, nil, func() error { return rename(it.src, dst) })
			if err != nil {
				j.addBytes(size)
				return false
			}
			j.addBytes(size)
			j.log(verb, it.src, dst, nil)
			return true
		}
	}
	err, _ := j.try("Can't "+verb+" "+name, it.src, dst, nil, func() error { return j.copyFile(it.src, dst, it.info, wm) })
	if err != nil {
		j.addBytes(size) // counted as done so the bar still reaches the end; copyFile took back what it wrote
		return false
	}
	j.log(verb, it.src, dst, nil)
	return !move || j.removeSource(it)
}

// removeSource removes the source of a file that was copied for a move.
func (j *job) removeSource(it item) bool {
	err, _ := j.try("Copied "+filepath.Base(it.src)+", but can't remove the original", it.src, "", nil, func() error {
		err := remove(it.src)
		if isPerm(err) && !isLink(it.info) {
			makeWritable(it.src, it.info) // a read-only file on Windows
			err = remove(it.src)
		}
		if isNotExist(err) {
			return nil
		}
		return err
	})
	return err == nil
}

// crossDevice says whether a rename failed only because source and target are on different volumes.
func crossDevice(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	return errors.Is(err, syscall.EXDEV) || isCrossDeviceOS(err)
}

// writeMode is how copyFile writes to the target.
type writeMode int

const (
	wmCreate writeMode = iota // a new file, or the old one truncated
	wmAppend                  // the whole source added to the end of the target
	wmResume                  // the source from the target's length on, added to its end
)

// copyFile copies the bytes, then the attributes and dates. On error or cancel the target is removed (wmCreate) or cut
// back to its old length (wmAppend, wmResume), and the bytes counted for it are taken back.
func (j *job) copyFile(src, dst string, info fs.FileInfo, wm writeMode) (err error) {
	in, err := open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if di, err := lstat(dst); err == nil && isLink(di) && wm == wmCreate {
		remove(dst) // overwrite the link itself, never the file it points to
	}
	var oldSize int64
	if di, err := stat(dst); err == nil {
		oldSize = di.Size()
	}
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	var offset int64
	switch wm {
	case wmAppend:
		flag = os.O_WRONLY | os.O_APPEND
	case wmResume:
		flag = os.O_WRONLY | os.O_APPEND
		offset = oldSize
		if offset > info.Size() {
			return errors.New("the target is larger than the source, so it can't be resumed")
		}
		if _, err := in.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}
	perm := fs.FileMode(0o666)
	if !j.req.NoPerms {
		perm = info.Mode().Perm() | 0o200
	}
	out, err := openFile(dst, flag, perm)
	if err != nil {
		return err
	}
	if j.req.NoCache {
		noCacheStart(in)
		noCacheStart(out)
	}
	var written int64
	defer func() {
		if err == nil {
			return
		}
		out.Close()
		if wm == wmCreate {
			remove(dst)
		} else {
			truncate(dst, oldSize)
		}
		j.addBytes(-written)
	}()
	var h hash.Hash
	if j.req.Verify {
		h = sha256.New()
	}
	bufSize := j.req.BufferSize
	if bufSize <= 0 {
		bufSize = defaultBuffer
	}
	buf := make([]byte, bufSize)
	for {
		if !j.gate() {
			return j.ctx.Err()
		}
		n, rerr := in.Read(buf[:j.chunk(len(buf))])
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			if h != nil {
				h.Write(buf[:n])
			}
			written += int64(n)
			j.addBytes(int64(n))
			j.throttle(n)
			j.tick(false)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if h != nil {
		if err := out.Sync(); err != nil {
			return err
		}
	}
	if j.req.NoCache {
		noCacheEnd(in)
		noCacheEnd(out)
	}
	if err := out.Close(); err != nil {
		return err
	}
	if h != nil {
		if err := verifyFile(dst, oldSize, written, wm, h.Sum(nil)); err != nil {
			return err
		}
		j.update(func(p *Progress) { p.Verified++ })
		j.log("verify", src, dst, nil)
	}
	if wm != wmAppend {
		j.copyMeta(src, dst, info)
	}
	return nil
}

// verifyFile re-reads the part of dst that was just written and compares its SHA-256 with want.
func verifyFile(dst string, oldSize, written int64, wm writeMode, want []byte) error {
	f, err := open(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	start := int64(0)
	if wm != wmCreate {
		start = oldSize
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, written))
	if err != nil {
		return err
	}
	if n != written || !bytes.Equal(h.Sum(nil), want) {
		return errVerify
	}
	return nil
}

// sameContent compares two files byte by byte (the overwrite dialog's "Compare").
func sameContent(a, b string) (bool, error) {
	fa, err := open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	ia, _ := fa.Stat()
	ib, _ := fb.Stat()
	if ia != nil && ib != nil && ia.Size() != ib.Size() {
		return false, nil
	}
	ba, bb := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		na, ea := io.ReadFull(fa, ba)
		nb, eb := io.ReadFull(fb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		if ea == io.EOF || ea == io.ErrUnexpectedEOF {
			return eb == io.EOF || eb == io.ErrUnexpectedEOF, nil
		}
		if ea != nil {
			return false, ea
		}
		if eb != nil {
			return false, eb
		}
	}
}
