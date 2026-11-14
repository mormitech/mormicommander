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
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf8"
)

// sameVolume is SameVolume; the tests replace it to drive the copy-and-delete path of a move on one disk.
var sameVolume = SameVolume

const defaultBuffer = 1 << 20

// allIntoItself says whether every source is a folder that is the target or holds it, so the whole job would only
// copy folders into themselves.
func (j *job) allIntoItself(target string) bool {
	if len(j.req.Sources) == 0 {
		return false
	}
	for _, src := range j.req.Sources {
		src = filepath.Clean(src)
		si, err := lstat(src)
		if err != nil || !si.IsDir() || !intoItself(target, src, si) {
			return false
		}
	}
	return true
}

// copyAll copies (or moves) every source into the target folder.
func (j *job) copyAll(move bool) {
	verb := "copy"
	if move {
		verb = "move"
	}
	target := filepath.Clean(j.req.Target)
	// A folder copied into itself is refused before anything is made: when every source is a folder that holds the
	// target, the target (say dir/sub) must not be created just to be left behind empty.
	if j.allIntoItself(target) {
		for i, src := range j.req.Sources {
			j.fail("Can't %s the folder %s into itself.", verb, filepath.Base(filepath.Clean(src)))
			j.markLeft(i)
		}
		return
	}
	if err, _ := j.try("Can't make the target folder "+target, "", target, nil, func() error { return mkdirAll(target, 0o777) }); err != nil {
		j.leaveFrom(nil, 0)
		return
	}
	var items []item
	// a source that ran into an error or a skip while it was looked at (or renamed, for a move) did not get across
	// whole; prev is the problem count before the source being looked at
	prev := j.problemCount()
	for i, src := range j.req.Sources {
		if i > 0 {
			if n := j.problemCount(); n > prev {
				j.markLeft(i - 1)
				prev = n
			}
		}
		if j.ctx.Err() != nil {
			j.leaveFrom(items, i)
			return
		}
		src = filepath.Clean(src)
		si, err := lstat(src)
		if err != nil {
			j.failAt(src, filepath.Join(target, ApplyMask(j.req.Rename, filepath.Base(src))), err, "Can't %s %s: %s.", verb, src, plainErr(err))
			continue
		}
		name := ApplyMask(j.req.Rename, filepath.Base(src))
		if err := checkName(name); err != nil { // a mask must not produce "", "." or "..", which would escape the target
			j.fail("Can't %s %s as %q: %s.", verb, filepath.Base(src), name, plainErr(err))
			continue
		}
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
		if si.IsDir() && intoItself(dst, src, si) {
			j.fail("Can't %s the folder %s into itself.", verb, filepath.Base(src))
			continue
		}
		same := move && sameVolume(src, target)
		if same && j.filter.Empty() {
			if _, err := lstat(dst); isNotExist(err) {
				if !j.at(src, 0) {
					j.leaveFrom(items, i)
					return
				}
				err := renameNew(src, dst)
				if err != nil && !crossDevice(err) {
					err, _ = j.try("Can't move "+filepath.Base(src), src, dst, nil, func() error { return renameNew(src, dst) })
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
	if n := len(j.req.Sources); n > 0 && j.problemCount() > prev {
		j.markLeft(n - 1)
	}
	if j.ctx.Err() != nil {
		j.leaveFrom(items, len(j.req.Sources))
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
		j.leaveFrom(items, len(j.req.Sources))
		return
	}

	var dirs []item // folders made or merged, to give them their dates and rights at the end
	var srcDirs []item
	for k, it := range items {
		size := int64(0)
		if it.info.Mode().IsRegular() {
			size = it.info.Size()
		}
		if !j.at(it.src, size) {
			j.leaveFrom(items[k:], len(j.req.Sources))
			break
		}
		before := j.problemCount()
		if it.info.IsDir() {
			made, ok := j.copyDir(it, verb)
			if !ok || j.problemCount() > before {
				j.markLeft(it.top)
			}
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
		copied := j.copyEntry(it, verb, move)
		if copied {
			j.update(func(p *Progress) { p.FilesDone++ })
		}
		if !copied || j.problemCount() > before {
			j.markLeft(it.top)
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

// leaveFrom notes, when a copy or move stops early, that the sources it did not finish did not get across whole:
// those of the listed items not yet done, and every source from index from on (not looked at yet).
func (j *job) leaveFrom(items []item, from int) {
	for _, it := range items {
		j.markLeft(it.top)
	}
	for i := from; i < len(j.req.Sources); i++ {
		j.markLeft(i)
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
		j.passBytes(size)
		return false
	}
	dst, wm, out := j.resolve(it)
	if out != outWrite {
		if out == outSkip {
			j.skipped()
			j.log("skip", it.src, dst, nil)
		}
		j.passBytes(size)
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
			return replaceWithLink(target, dst)
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
		err := renameNew(it.src, dst)
		if err == nil {
			j.addBytes(size)
			j.log(verb, it.src, dst, nil)
			return true
		}
		if !crossDevice(err) {
			err, _ = j.try("Can't move "+name, it.src, dst, nil, func() error { return renameNew(it.src, dst) })
			if err != nil {
				j.passBytes(size)
				return false
			}
			j.addBytes(size)
			j.log(verb, it.src, dst, nil)
			return true
		}
	}
	err, _ := j.try("Can't "+verb+" "+name, it.src, dst, nil, func() error { return j.copyFile(it.src, dst, it.info, wm) })
	if err != nil {
		j.passBytes(size) // counted as done so the bar still reaches the end; copyFile took back what it wrote
		return false
	}
	j.log(verb, it.src, dst, nil)
	return !move || j.removeSource(it)
}

// replaceWithLink makes the link dst pointing to target. Something already at dst (an overwrite) is replaced only
// once the new link exists: the link is made under a temporary name first and renamed over it, so a volume that
// can't hold links (FAT, exFAT) or any other failure leaves the old target as it was.
func replaceWithLink(target, dst string) error {
	if _, err := lstat(dst); isNotExist(err) {
		return symlink(target, dst)
	}
	tmp := filepath.Join(filepath.Dir(dst), fmt.Sprintf(".%s.mc-link-%d", tempStem(filepath.Base(dst)), time.Now().UnixNano()))
	if err := symlink(target, tmp); err != nil {
		return err
	}
	if err := rename(tmp, dst); err != nil {
		// Windows can't rename a link over some entries (a link to a folder); remove the old one and try once more
		if rerr := remove(dst); rerr != nil && !isNotExist(rerr) {
			remove(tmp)
			return err
		}
		if err := rename(tmp, dst); err != nil {
			remove(tmp)
			return err
		}
	}
	return nil
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
//
// An overwrite (wmCreate onto an existing file) does not truncate the old file and write into it: the bytes go to a
// hidden temporary file next to it, which is renamed over the old file only once the copy (and the verify, if asked)
// has succeeded. A cancel, a read error or a full disk partway through therefore leaves the old target untouched,
// where Total Commander would leave a half-written file or none. The price is that the disk briefly needs room for
// both, and that other hard links to the old target keep the old contents.
func (j *job) copyFile(src, dst string, info fs.FileInfo, wm writeMode) (err error) {
	in, err := open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// A link in the way is overwritten itself, never the file it points to. It is replaced like an old file, by the
	// rename of the finished temporary file, so a failed copy leaves the link as it was.
	overLink := false
	if di, err := lstat(dst); err == nil && isLink(di) && wm == wmCreate {
		overLink = true
	}
	var oldSize int64
	var oldInfo fs.FileInfo
	if di, err := stat(dst); err == nil && !overLink {
		oldSize = di.Size()
		oldInfo = di
	}
	final := dst // where the bytes end up; dst is where they are written
	if overLink || (wm == wmCreate && oldInfo != nil && oldInfo.Mode().IsRegular()) {
		dst = filepath.Join(filepath.Dir(final), fmt.Sprintf(".%s.mc-copy-%d", tempStem(filepath.Base(final)), time.Now().UnixNano()))
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
		j.log("verify", src, final, nil)
	}
	if dst != final {
		if j.req.NoPerms && oldInfo != nil {
			chmod(dst, oldInfo.Mode().Perm()) // without permission copying an overwrite keeps the old file's mode
		}
		err := rename(dst, final)
		if err != nil && overLink {
			// Windows can't rename a file over a link to a folder; that link goes first, then the rename is tried again
			if rerr := remove(final); rerr == nil || isNotExist(rerr) {
				err = rename(dst, final)
			}
		}
		if err != nil {
			return err // the deferred cleanup removes the temporary file; the old target is still whole
		}
		dst = final
	}
	if wm != wmAppend {
		j.copyMeta(src, dst, info)
	}
	return nil
}

// tempStem is the part of a file name that goes into the name of a temporary file next to it. The whole name plus the
// temporary file's prefix and suffix would pass the 255-byte limit of file names for names near that limit, so such a
// file could never be overwritten or renamed; the stem keeps only the start of a long name (cut between characters),
// which is still enough to tell what the temporary file belongs to.
func tempStem(name string) string {
	const max = 64
	if len(name) <= max {
		return name
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
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
