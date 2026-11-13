// This file creates and updates zip archives: Alt+F5 "Pack", F5 into an
// archive panel, F8 delete, Shift+F6 rename and F7 make folder inside an
// archive, and archive/file comments. The zip format cannot be edited in
// place, so every update writes a complete new archive into a temporary file
// in the same folder and renames it over the old one only after everything
// succeeded; a cancel or error leaves the original untouched. Entries that
// did not change are copied as raw compressed (and possibly encrypted) bytes,
// so updating a large archive is fast and never needs the password of
// entries it does not touch. New files are compressed with Store or Deflate
// at the chosen level and optionally encrypted with ZipCrypto or WinZip AES;
// names are always written as flagged UTF-8.
package ziparc

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"
	"unicode/utf8"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/arcapi"
)

// Packer implements archive.Packer for zip.
type Packer struct{}

type planKind int

const (
	planCopy planKind = iota
	planDir
	planSource
)

// planned is one entry of the archive being written.
type planned struct {
	kind    planKind
	path    string // clean path without trailing slash; "" for unlisted raw records
	isDir   bool
	rec     *record
	entry   archive.Entry // decoded view of rec
	renamed bool
	src     *archive.SourceFile
	modTime time.Time
	comment *string
	added   bool // came from Changes.Add (for MoveSources verification)
}

type plan struct {
	items  []*planned
	byPath map[string]*planned
	dirs   map[string]bool // every folder that exists implicitly or explicitly
}

func (p *plan) rebuild() {
	p.byPath = make(map[string]*planned, len(p.items))
	p.dirs = make(map[string]bool)
	for _, it := range p.items {
		p.index(it)
	}
}

func (p *plan) index(it *planned) {
	if it.path == "" {
		return
	}
	p.byPath[it.path] = it
	if it.isDir {
		p.dirs[it.path] = true
	}
	for d := path.Dir(it.path); d != "." && d != ""; d = path.Dir(d) {
		p.dirs[d] = true
	}
}

func (p *plan) exists(name string) bool {
	_, ok := p.byPath[name]
	return ok || p.dirs[name]
}

func (p *plan) removeIf(match func(*planned) bool) int {
	out := p.items[:0]
	n := 0
	for _, it := range p.items {
		if match(it) {
			n++
			continue
		}
		out = append(out, it)
	}
	p.items = out
	return n
}

// Update applies ch to the zip at archivePath, creating it when missing.
func (Packer) Update(ctx context.Context, archivePath string, ch archive.Changes, opts archive.PackOptions) (archive.PackResult, error) {
	var res archive.PackResult
	archivePath, err := filepath.Abs(archivePath)
	if err != nil {
		return res, err
	}
	// Replace the archive a symbolic link points to, not the link itself.
	archivePath = arcapi.ReplaceTarget(archivePath)
	var old *Reader
	oldInfo, statErr := os.Stat(archivePath)
	if statErr == nil {
		if oldInfo.IsDir() {
			return res, fmt.Errorf("ziparc: %s is a folder", archivePath)
		}
		old, err = Open(archivePath, archive.OpenOptions{})
		if err != nil {
			return res, err
		}
		defer func() {
			if old != nil {
				old.Close()
			}
		}()
		if old.Spanned() {
			return res, fmt.Errorf("%w: multi-volume archives cannot be modified", archive.ErrReadOnly)
		}
	} else if !os.IsNotExist(statErr) {
		return res, statErr
	}

	p := &plan{}
	if old != nil {
		byID := make(map[int]archive.Entry)
		for _, e := range old.Entries() {
			if e.ID >= 0 {
				byID[e.ID] = e
			}
		}
		for i := range old.dir.records {
			r := &old.dir.records[i]
			e := byID[i]
			p.items = append(p.items, &planned{kind: planCopy, path: e.Path, isDir: e.IsDir, rec: r, entry: e})
		}
	}
	p.rebuild()

	if err := applyDeletes(p, ch.Delete, &res); err != nil {
		return res, err
	}
	if err := applyRenames(p, ch.Rename, &res); err != nil {
		return res, err
	}
	for _, d := range ch.Mkdir {
		c, _ := archive.CleanPath(d)
		if c == "" {
			return res, fmt.Errorf("ziparc: invalid folder name %q", d)
		}
		if p.exists(c) {
			return res, fmt.Errorf("%w: %s", archive.ErrExist, c)
		}
		it := &planned{kind: planDir, path: c, isDir: true, modTime: time.Now()}
		p.items = append(p.items, it)
		p.index(it)
	}

	dir := filepath.Dir(archivePath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(archivePath)+".*.tmp")
	if err != nil {
		return res, err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	sources, err := archive.CollectSources(ch.Add, opts, archivePath, tmpName)
	if err != nil {
		return res, err
	}
	var written []archive.SourceFile
	conflicts := archive.NewConflictResolver(opts.Policy, opts.OnConflict)
	for i := range sources {
		sf := &sources[i]
		ok, err := addSource(p, sf, conflicts, &res)
		if err != nil {
			return res, err
		}
		if ok {
			written = append(written, *sf)
		}
	}
	for name, c := range ch.EntryComments {
		cp, _ := archive.CleanPath(name)
		it, ok := p.byPath[cp]
		if !ok {
			return res, fmt.Errorf("%w: %s", archive.ErrNotFound, name)
		}
		c := c
		it.comment = &c
	}

	w := &zipWriter{ctx: ctx, opts: opts, old: old, tmpDir: dir}
	if err := w.write(tmp, p, ch.Comment); err != nil {
		return res, err
	}
	res.Bytes = w.bytes
	if err := tmp.Close(); err != nil {
		return res, err
	}
	perm := fs.FileMode(0o644)
	if oldInfo != nil {
		perm = oldInfo.Mode().Perm()
	}
	_ = os.Chmod(tmpName, perm)
	if old != nil {
		old.Close() // Windows cannot replace a file that is still open
		old = nil
	}

	if opts.VolumeSize > 0 {
		st, err := os.Stat(tmpName)
		if err != nil {
			return res, err
		}
		if st.Size() > opts.VolumeSize {
			vols, err := splitVolumes(ctx, tmpName, archivePath, opts.VolumeSize)
			if err != nil {
				return res, err
			}
			res.Volumes = vols
			os.Remove(tmpName)
			committed = true
		}
	}
	if !committed {
		if err := os.Rename(tmpName, archivePath); err != nil {
			return res, err
		}
		committed = true
		if opts.VolumeSize > 0 {
			res.Volumes = []string{archivePath}
		}
	}

	if opts.MoveSources && len(written) > 0 {
		if err := verifyAdded(ctx, archivePath, p, opts.Password); err != nil {
			return res, fmt.Errorf("archive written but sources kept: %w", err)
		}
		moved, err := archive.RemoveSources(written)
		res.Moved = moved
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func applyDeletes(p *plan, dels []string, res *archive.PackResult) error {
	for _, d := range dels {
		c, _ := archive.CleanPath(d)
		if c == "" {
			return fmt.Errorf("ziparc: refusing to delete the archive root")
		}
		n := p.removeIf(func(it *planned) bool { return it.path != "" && archive.IsUnder(it.path, c) })
		if n == 0 {
			return fmt.Errorf("%w: %s", archive.ErrNotFound, c)
		}
		res.Deleted += n
		p.rebuild()
	}
	return nil
}

func applyRenames(p *plan, rens []archive.Rename, res *archive.PackResult) error {
	for _, rn := range rens {
		from, _ := archive.CleanPath(rn.From)
		to, _ := archive.CleanPath(rn.To)
		if from == "" || to == "" {
			return fmt.Errorf("ziparc: invalid rename %q -> %q", rn.From, rn.To)
		}
		if from == to {
			continue
		}
		if archive.IsUnder(to, from) {
			return fmt.Errorf("ziparc: cannot move %s into itself", from)
		}
		if !p.exists(from) {
			return fmt.Errorf("%w: %s", archive.ErrNotFound, from)
		}
		// Names are case-sensitive here, so a rename that only changes
		// case passes this check unless "to" is another, existing entry.
		if p.exists(to) {
			return fmt.Errorf("%w: %s", archive.ErrExist, to)
		}
		// Every parent of the new name must be a folder (or not exist yet):
		// an entry below a file would make that name both at once.
		for d := path.Dir(to); d != "." && d != ""; d = path.Dir(d) {
			if it, ok := p.byPath[d]; ok && !it.isDir {
				return fmt.Errorf("%w: %s is a file, not a folder", archive.ErrExist, d)
			}
		}
		for _, it := range p.items {
			if it.path != "" && archive.IsUnder(it.path, from) {
				it.path = to + it.path[len(from):]
				it.renamed = true
				res.Renamed++
			}
		}
		p.rebuild()
	}
	return nil
}

// addSource puts one collected source into the plan, resolving conflicts
// with entries that already exist. It reports whether the source will be
// written.
func addSource(p *plan, sf *archive.SourceFile, cr *archive.ConflictResolver, res *archive.PackResult) (bool, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name := sf.ArchivePath
		existing, ok := p.byPath[name]
		if sf.IsDir {
			if ok && !existing.isDir {
				return false, fmt.Errorf("%w: a file named %s exists in the archive", archive.ErrExist, name)
			}
			if ok {
				return false, nil // folder already there: merge
			}
			it := &planned{kind: planSource, path: name, isDir: true, src: sf, added: true}
			p.items = append(p.items, it)
			p.index(it)
			return true, nil
		}
		if !ok && p.dirs[name] {
			return false, fmt.Errorf("%w: a folder named %s exists in the archive", archive.ErrExist, name)
		}
		if !ok {
			it := &planned{kind: planSource, path: name, src: sf, added: true}
			p.items = append(p.items, it)
			p.index(it)
			res.Added++
			return true, nil
		}
		if existing.isDir {
			return false, fmt.Errorf("%w: a folder named %s exists in the archive", archive.ErrExist, name)
		}
		info := archive.ConflictInfo{
			Path:     name,
			Existing: existingStat(existing),
			Incoming: archive.FileStat{Size: sf.Size, ModTime: sf.ModTime, ReadOnly: sf.Mode.Perm()&0o200 == 0},
		}
		dir := path.Dir(name)
		action, newName := cr.Resolve(info, func(n string) bool { return p.exists(path.Join(dir, n)) })
		switch action {
		case archive.ResolveSkip:
			res.Skipped++
			return false, nil
		case archive.ResolveCancel:
			return false, archive.ErrCanceled
		case archive.ResolveRename:
			if dir == "." {
				sf.ArchivePath = newName
			} else {
				sf.ArchivePath = dir + "/" + newName
			}
			continue
		case archive.ResolveOverwrite:
			existing.kind, existing.src, existing.rec, existing.renamed, existing.added = planSource, sf, nil, false, true
			res.Replaced++
			return true, nil
		}
	}
	return false, fmt.Errorf("ziparc: no free name for %s", sf.ArchivePath)
}

func existingStat(it *planned) archive.FileStat {
	switch {
	case it.src != nil:
		return archive.FileStat{Size: it.src.Size, ModTime: it.src.ModTime}
	case it.rec != nil:
		return archive.FileStat{Size: it.entry.Size, ModTime: it.entry.ModTime, ReadOnly: it.entry.DOSAttrs&archive.AttrReadOnly != 0}
	}
	return archive.FileStat{}
}

// zipWriter writes a plan into a new zip file.
type zipWriter struct {
	ctx    context.Context
	opts   archive.PackOptions
	old    *Reader
	tmpDir string
	zw     *zip.Writer
	track  *archive.Tracker
	level  int
	store  bool
	bytes  int64
}

func (w *zipWriter) write(out io.Writer, p *plan, comment *string) error {
	w.zw = zip.NewWriter(out)
	w.level = w.opts.Level
	if w.level <= 0 || w.level > 9 {
		w.level = 6
	}
	w.store = w.opts.Method == archive.MethodStore
	lvl := w.level
	w.zw.RegisterCompressor(zip.Deflate, func(o io.Writer) (io.WriteCloser, error) { return flate.NewWriter(o, lvl) })

	switch {
	case comment != nil:
		if len(*comment) > 0xffff {
			return errors.New("ziparc: archive comment longer than 65535 bytes")
		}
		if err := w.zw.SetComment(*comment); err != nil {
			return err
		}
	case w.old != nil:
		if err := w.zw.SetComment(string(w.old.dir.commentRaw)); err != nil {
			return err
		}
	}

	var total int64
	files := 0
	for _, it := range p.items {
		switch {
		case it.kind == planCopy && it.rec != nil:
			total += int64(it.rec.csize)
		case it.kind == planSource && it.src != nil && !it.isDir:
			total += it.src.Size
			files++
		}
	}
	w.track = archive.NewTracker("pack", w.opts.Progress, total, files)

	for _, it := range p.items {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		var err error
		switch it.kind {
		case planCopy:
			err = w.writeCopy(it)
		case planDir:
			err = w.writeDir(it.path, it.modTime, fs.ModeDir|0o755, archive.AttrDir, it.comment)
		case planSource:
			if it.isDir {
				err = w.writeDir(it.path, it.src.ModTime, it.src.Mode|fs.ModeDir, it.src.DOSAttrs, it.comment)
			} else {
				err = w.writeSource(it)
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", it.path, err)
		}
	}
	return w.zw.Close()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func utf8Flag(flags uint16, texts ...string) uint16 {
	for _, t := range texts {
		if !isASCII(t) && utf8.ValidString(t) {
			return flags | flagUTF8
		}
	}
	return flags &^ flagUTF8
}

// writeCopy copies an unchanged (or only renamed / re-commented) entry as raw
// bytes.
func (w *zipWriter) writeCopy(it *planned) error {
	r := it.rec
	pos, err := w.old.dataPos(r)
	if err != nil {
		return err
	}
	name := string(r.nameRaw)
	cmt := string(r.commentRaw)
	flags := r.flags
	extra := stripExtra(r.extra, extraZip64)
	if it.renamed || it.comment != nil {
		// The name or comment becomes UTF-8; re-encode the other one from
		// its decoded form so both match the flag.
		name = it.entry.RawName
		if it.renamed {
			name = it.path
			if it.isDir {
				name += "/"
			}
		}
		cmt = it.entry.Comment
		if it.comment != nil {
			cmt = *it.comment
		}
		flags = utf8Flag(flags, name, cmt)
		extra = stripExtra(extra, extraUnicodePath, extraUnicodeCmt)
	}
	fh := &zip.FileHeader{
		Name: name, Comment: cmt,
		CreatorVersion: r.creatorVersion, ReaderVersion: r.readerVersion,
		Flags: flags, Method: r.method,
		ModifiedTime: r.dosTime, ModifiedDate: r.dosDate,
		CRC32: r.crc, CompressedSize64: r.csize, UncompressedSize64: r.usize,
		Extra: extra, ExternalAttrs: r.externalAttr,
	}
	zf, err := w.zw.CreateRaw(fh)
	if err != nil {
		return err
	}
	w.track.StartFile(it.path, int64(r.csize))
	_, err = archive.CopyContext(w.ctx, zf, io.NewSectionReader(w.old.vol, pos, int64(r.csize)), w.track)
	return err
}

func (w *zipWriter) writeDir(name string, mt time.Time, mode fs.FileMode, dos uint8, comment *string) error {
	fh := &zip.FileHeader{Name: name + "/", Method: zip.Store, Modified: mt}
	fh.CreatorVersion = hostUnix << 8
	fh.ExternalAttrs = attrsFromMode(mode|fs.ModeDir, dos)
	if comment != nil {
		fh.Comment = *comment
	}
	_, err := w.zw.CreateHeader(fh)
	return err
}

func (w *zipWriter) method() uint16 {
	if w.store {
		return zip.Store
	}
	return zip.Deflate
}

func (w *zipWriter) writeSource(it *planned) error {
	sf := it.src
	mt := sf.ModTime
	if mt.IsZero() {
		mt = time.Now()
	}
	cmt := ""
	if it.comment != nil {
		cmt = *it.comment
	}
	w.track.StartFile(sf.ArchivePath, sf.Size)
	defer w.track.FinishFile()
	if sf.IsLink {
		fh := &zip.FileHeader{Name: it.path, Method: zip.Store, Modified: mt, Comment: cmt}
		fh.CreatorVersion = hostUnix << 8
		fh.ExternalAttrs = attrsFromMode(fs.ModeSymlink|0o777, 0)
		zf, err := w.zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		_, err = io.WriteString(zf, sf.LinkTarget)
		return err
	}
	if w.opts.Password != "" {
		return w.writeEncrypted(it, mt, cmt)
	}
	fh := &zip.FileHeader{Name: it.path, Method: w.method(), Modified: mt, Comment: cmt}
	fh.CreatorVersion = hostUnix << 8
	fh.ExternalAttrs = attrsFromMode(sf.Mode&^fs.ModeType, sf.DOSAttrs)
	zf, err := w.zw.CreateHeader(fh)
	if err != nil {
		return err
	}
	rc, err := sf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	n, err := archive.CopyContext(w.ctx, zf, rc, w.track)
	w.bytes += n
	return err
}

// writeEncrypted compresses the file into a spool first, because the
// encrypted entry is written raw and its sizes must be known up front.
func (w *zipWriter) writeEncrypted(it *planned, mt time.Time, cmt string) error {
	sf := it.src
	sp := &spool{dir: w.tmpDir, limit: 8 << 20}
	defer sp.Close()
	crc := crc32.NewIEEE()
	var cw io.WriteCloser = nopWriteCloser{sp}
	if !w.store {
		fw, err := flate.NewWriter(sp, w.level)
		if err != nil {
			return err
		}
		cw = fw
	}
	rc, err := sf.Open()
	if err != nil {
		return err
	}
	n, err := archive.CopyContext(w.ctx, io.MultiWriter(cw, crc), rc, w.track)
	rc.Close()
	w.bytes += n
	if err != nil {
		return err
	}
	if err := cw.Close(); err != nil {
		return err
	}
	if err := sp.err; err != nil {
		return err
	}
	date, tm := timeToDOS(mt)
	fh := &zip.FileHeader{
		Name: it.path, Comment: cmt,
		Flags:              utf8Flag(flagEncrypted, it.path, cmt),
		Method:             w.method(),
		ModifiedDate:       date,
		ModifiedTime:       tm,
		CRC32:              crc.Sum32(),
		UncompressedSize64: uint64(n),
		CreatorVersion:     hostUnix<<8 | 20,
		ReaderVersion:      20,
		ExternalAttrs:      attrsFromMode(sf.Mode&^fs.ModeType, sf.DOSAttrs),
		Extra:              extTimeExtra(mt),
	}
	enc := w.opts.Encryption
	if enc == archive.EncryptZipCrypto {
		fh.CompressedSize64 = uint64(sp.size) + 12
		zf, err := w.zw.CreateRaw(fh)
		if err != nil {
			return err
		}
		ew, err := newZipCryptoWriter(zf, []byte(w.opts.Password), fh.CRC32)
		if err != nil {
			return err
		}
		return sp.copyTo(w.ctx, ew)
	}
	strength, err := strengthFor(enc)
	if err != nil {
		return err
	}
	info := aesInfo{vendorVersion: 1, strength: strength, method: w.method()}
	if n < 20 {
		// WinZip's rule: tiny files use AE-2 and omit the CRC, which could
		// otherwise leak information about very short contents.
		info.vendorVersion = 2
		fh.CRC32 = 0
	}
	fh.Method = methodAES
	fh.ReaderVersion = 51
	fh.CreatorVersion = hostUnix<<8 | 51
	fh.Extra = append(fh.Extra, aesExtra(info)...)
	fh.CompressedSize64 = uint64(sp.size) + uint64(info.saltLen()+2+aesAuthLen)
	zf, err := w.zw.CreateRaw(fh)
	if err != nil {
		return err
	}
	aw, err := newAESWriter(zf, []byte(w.opts.Password), strength)
	if err != nil {
		return err
	}
	if err := sp.copyTo(w.ctx, aw); err != nil {
		return err
	}
	return aw.Close()
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// spool buffers data in memory up to limit bytes and in a temporary file
// beyond that.
type spool struct {
	dir   string
	limit int
	buf   bytes.Buffer
	f     *os.File
	size  int64
	err   error
}

func (s *spool) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.f == nil && s.buf.Len()+len(p) > s.limit {
		f, err := os.CreateTemp(s.dir, ".mc-spool-*.tmp")
		if err != nil {
			s.err = err
			return 0, err
		}
		s.f = f
		if _, err := f.Write(s.buf.Bytes()); err != nil {
			s.err = err
			return 0, err
		}
		s.buf = bytes.Buffer{}
	}
	var n int
	var err error
	if s.f != nil {
		n, err = s.f.Write(p)
	} else {
		n, err = s.buf.Write(p)
	}
	s.size += int64(n)
	if err != nil {
		s.err = err
	}
	return n, err
}

func (s *spool) copyTo(ctx context.Context, w io.Writer) error {
	if s.f == nil {
		_, err := archive.CopyContext(ctx, w, bytes.NewReader(s.buf.Bytes()), nil)
		return err
	}
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := archive.CopyContext(ctx, w, s.f, nil)
	return err
}

func (s *spool) Close() {
	if s.f != nil {
		name := s.f.Name()
		s.f.Close()
		os.Remove(name)
	}
}

// verifyAdded reopens the written archive and tests the entries that were
// added, before "Move to archive" deletes the originals.
func verifyAdded(ctx context.Context, archivePath string, p *plan, password string) error {
	r, err := Open(archivePath, archive.OpenOptions{})
	if err != nil {
		return err
	}
	defer r.Close()
	if password != "" {
		r.SetPassword(password)
	}
	var paths []string
	for _, it := range p.items {
		if it.added && !it.isDir {
			paths = append(paths, it.path)
		}
	}
	rep, err := archive.Verify(ctx, r, archive.VerifyOptions{Paths: paths})
	if err != nil {
		return err
	}
	if !rep.OK() {
		return rep.Failed[0]
	}
	return nil
}
