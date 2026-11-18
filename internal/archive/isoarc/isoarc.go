// Package isoarc reads ISO 9660 disc images (.iso) so the panel can browse
// them like a folder and copy files out of them (FEATURES.md F-610). It is
// read-only and needs no external tool. Names come from the Joliet
// supplementary descriptor when the image has one (long Unicode names, the
// usual case for images made on Windows or macOS), else from Rock Ridge NM
// entries (images made on Linux), else from the plain ISO 9660 names with
// their ";1" version suffix removed. Files split over several extents
// (multi-extent, used for files above 4 GB) are read as one file.
//
// It implements archive.Archive, so archive.Extract, archive.Verify and
// archive.FS work on it without further code. ISO 9660 stores no checksums;
// Verify therefore only checks that every file can be read to its end.
package isoarc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
	"unicode/utf16"

	"mormicommander/internal/archive"
)

const sector = 2048

// maxEntries and maxDepth bound what a damaged or hostile image can make the
// reader do (directory loops, endless records).
const (
	maxEntries = 1 << 20
	maxDepth   = 64
)

// extent is one contiguous run of a file's data.
type extent struct {
	off, size int64
}

// Reader is an opened ISO image.
type Reader struct {
	f      *os.File
	r      io.ReaderAt
	size   int64
	path   string
	label  string
	ix     *archive.Index
	extent map[int][]extent // by Entry.ID
}

// Detect reports whether the file is an ISO 9660 image: "CD001" at the start
// of sector 16.
func Detect(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return DetectReaderAt(f)
}

// DetectReaderAt is Detect for data already open.
func DetectReaderAt(r io.ReaderAt) bool {
	var b [7]byte
	if _, err := r.ReadAt(b[:], 16*sector); err != nil {
		return false
	}
	return string(b[1:6]) == "CD001"
}

// Open opens the image at path.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	r, err := OpenReaderAt(f, fi.Size(), path)
	if err != nil {
		f.Close()
		return nil, err
	}
	r.f = f
	return r, nil
}

// OpenReaderAt reads an image held in r (size bytes); name is used for
// messages and Path.
func OpenReaderAt(r io.ReaderAt, size int64, name string) (*Reader, error) {
	z := &Reader{r: r, size: size, path: name, extent: map[int][]extent{}}
	var pvd, joliet []byte
	for s := int64(16); s < 16+64; s++ {
		d := make([]byte, sector)
		if _, err := r.ReadAt(d, s*sector); err != nil {
			break
		}
		if string(d[1:6]) != "CD001" {
			break
		}
		switch d[0] {
		case 1:
			if pvd == nil {
				pvd = d
			}
		case 2:
			esc := d[88:91]
			if joliet == nil && esc[0] == '%' && esc[1] == '/' && (esc[2] == '@' || esc[2] == 'C' || esc[2] == 'E') {
				joliet = d
			}
		}
		if d[0] == 255 {
			break
		}
	}
	if pvd == nil {
		return nil, fmt.Errorf("%s: %w (no ISO 9660 volume descriptor)", name, archive.ErrFormat)
	}
	desc, isJoliet := pvd, false
	if joliet != nil {
		desc, isJoliet = joliet, true
	}
	if bs := binary.LittleEndian.Uint16(desc[128:]); bs != 0 && bs != sector {
		return nil, fmt.Errorf("%s: %w (block size %d)", name, archive.ErrFormat, bs)
	}
	z.label = strings.TrimRight(string(pvd[40:72]), " \x00")
	if isJoliet {
		z.label = strings.TrimRight(ucs2(desc[40:72]), " \x00")
	}
	root := desc[156 : 156+34]
	w := walker{z: z, joliet: isJoliet, seen: map[int64]bool{}}
	if !isJoliet {
		w.rockRidge = w.hasRockRidge(root)
	}
	if err := w.walk(root, "", 0); err != nil {
		return nil, err
	}
	z.ix = archive.NewIndex(w.records)
	return z, nil
}

type walker struct {
	z         *Reader
	joliet    bool
	rockRidge bool
	records   []archive.Entry
	seen      map[int64]bool
}

type dirRec struct {
	off, size int64
	flags     byte
	name      string
	mtime     time.Time
	mode      fs.FileMode
	hasMode   bool
	rrName    bool // the name came from Rock Ridge and is used as is
}

func (w *walker) parse(rec []byte) (dirRec, bool) {
	if len(rec) < 34 {
		return dirRec{}, false
	}
	n := int(rec[32])
	if 33+n > len(rec) {
		return dirRec{}, false
	}
	d := dirRec{
		off:   int64(binary.LittleEndian.Uint32(rec[2:])) * sector,
		size:  int64(binary.LittleEndian.Uint32(rec[10:])),
		flags: rec[25],
		mtime: recTime(rec[18:25]),
	}
	raw := rec[33 : 33+n]
	switch {
	case n == 1 && (raw[0] == 0 || raw[0] == 1):
		return d, true // "." and ".."; skipped by the caller
	case w.joliet:
		d.name = ucs2(raw)
	default:
		d.name = string(raw)
	}
	if !w.joliet {
		if w.rockRidge {
			su := 33 + n
			if n%2 == 0 {
				su++
			}
			if su < len(rec) {
				if nm, mode, ok := rockRidge(rec[su:]); nm != "" || ok {
					if nm != "" {
						d.name, d.rrName = nm, true
					}
					if ok {
						d.mode, d.hasMode = mode, true
					}
				}
			}
		}
	}
	if !d.rrName {
		if i := strings.LastIndexByte(d.name, ';'); i >= 0 {
			d.name = d.name[:i]
		}
		if d.flags&0x02 == 0 {
			d.name = strings.TrimSuffix(d.name, ".")
		}
	}
	return d, true
}

func (w *walker) hasRockRidge(root []byte) bool {
	r, ok := w.parse(root)
	if !ok || r.size <= 0 {
		return false
	}
	b := make([]byte, sector)
	if _, err := w.z.r.ReadAt(b, r.off); err != nil {
		return false
	}
	l := int(b[0])
	if l < 34 || l > len(b) {
		return false
	}
	su := 34 // the "." record has a one-byte name and one pad byte
	return bytes.Contains(b[su:l], []byte("SP\x07\x01\xbe\xef")) || bytes.Contains(b[su:l], []byte("RR")) || bytes.Contains(b[su:l], []byte("PX"))
}

func (w *walker) walk(rec []byte, dir string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("%s: %w (folders nested too deep)", w.z.path, archive.ErrFormat)
	}
	d, ok := w.parse(rec)
	if !ok {
		return fmt.Errorf("%s: %w (bad folder record)", w.z.path, archive.ErrFormat)
	}
	if w.seen[d.off] {
		return nil // a loop in a damaged image
	}
	w.seen[d.off] = true
	if d.off+d.size > w.z.size || d.size < 0 {
		return fmt.Errorf("%s: %w (folder outside the image)", w.z.path, archive.ErrFormat)
	}
	data := make([]byte, d.size)
	if _, err := w.z.r.ReadAt(data, d.off); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	var pending *archive.Entry // a multi-extent file being collected
	var pendingExt []extent
	flush := func() {
		if pending != nil {
			w.z.extent[pending.ID] = pendingExt
			w.records = append(w.records, *pending)
			pending, pendingExt = nil, nil
		}
	}
	type sub struct {
		rec  []byte
		path string
	}
	var subs []sub
	for i := 0; i < len(data); {
		l := int(data[i])
		if l == 0 {
			i = (i/sector + 1) * sector // records never cross a sector; the rest is padding
			continue
		}
		if i+l > len(data) || l < 34 {
			break
		}
		rec := data[i : i+l]
		i += l
		c, ok := w.parse(rec)
		if !ok || rec[32] == 1 && (rec[33] == 0 || rec[33] == 1) {
			continue
		}
		if len(w.records) > maxEntries {
			return fmt.Errorf("%s: %w (too many entries)", w.z.path, archive.ErrFormat)
		}
		p := c.name
		if dir != "" {
			p = dir + "/" + c.name
		}
		isDir := c.flags&0x02 != 0
		if pending != nil && pending.RawName == p && !isDir {
			pendingExt = append(pendingExt, extent{c.off, c.size})
			pending.Size += c.size
			pending.PackedSize = pending.Size
			if c.flags&0x80 == 0 {
				flush()
			}
			continue
		}
		flush()
		mode := fs.FileMode(0o644)
		if isDir {
			mode = 0o755
		}
		if c.hasMode {
			mode = c.mode
		}
		e := archive.Entry{ID: len(w.records), RawName: p, IsDir: isDir, ModTime: c.mtime, Mode: mode, Method: "ISO 9660"}
		if w.joliet {
			e.Method = "ISO 9660 (Joliet)"
		} else if w.rockRidge {
			e.Method = "ISO 9660 (Rock Ridge)"
		}
		if isDir {
			w.records = append(w.records, e)
			subs = append(subs, sub{append([]byte(nil), rec...), p})
			continue
		}
		e.Size, e.PackedSize = c.size, c.size
		if c.off+c.size > w.z.size {
			e.Comment = "data outside the image"
		}
		pending, pendingExt = &e, []extent{{c.off, c.size}}
		if c.flags&0x80 == 0 {
			flush()
		}
	}
	flush()
	for _, s := range subs {
		if err := w.walk(s.rec, s.path, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// rockRidge reads the NM (alternate name) and PX (POSIX mode) entries of a
// System Use area.
func rockRidge(su []byte) (name string, mode fs.FileMode, hasMode bool) {
	var nm strings.Builder
	for len(su) >= 4 {
		l := int(su[2])
		if l < 4 || l > len(su) {
			break
		}
		sig := string(su[:2])
		body := su[4:l]
		switch sig {
		case "NM":
			if len(body) >= 1 && body[0]&0x06 == 0 { // not "." or ".."
				nm.Write(body[1:])
			}
		case "PX":
			if len(body) >= 8 {
				m := binary.LittleEndian.Uint32(body[0:])
				mode = fs.FileMode(m & 0o777)
				if m&0o170000 == 0o040000 {
					mode |= fs.ModeDir
				}
				if m&0o170000 == 0o120000 {
					mode |= fs.ModeSymlink
				}
				hasMode = true
			}
		case "ST":
			return nm.String(), mode, hasMode
		}
		su = su[l:]
	}
	return nm.String(), mode, hasMode
}

// recTime reads the 7-byte directory record date.
func recTime(b []byte) time.Time {
	if len(b) < 7 || b[1] == 0 {
		return time.Time{}
	}
	loc := time.FixedZone("", int(int8(b[6]))*15*60)
	return time.Date(1900+int(b[0]), time.Month(b[1]), int(b[2]), int(b[3]), int(b[4]), int(b[5]), 0, loc)
}

func ucs2(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, binary.BigEndian.Uint16(b[i:]))
	}
	return string(utf16.Decode(u))
}

// ---- archive.Archive ----

// Format is "iso".
func (z *Reader) Format() string { return "iso" }

// Path is the image file.
func (z *Reader) Path() string { return z.path }

// Comment is the volume label.
func (z *Reader) Comment() string { return z.label }

// Label is the volume label.
func (z *Reader) Label() string { return z.label }

// Entries lists everything in the image.
func (z *Reader) Entries() []archive.Entry { return z.ix.Entries() }

// ReadDir lists one folder.
func (z *Reader) ReadDir(dir string) ([]archive.Entry, error) { return z.ix.ReadDir(dir) }

// Stat describes one entry.
func (z *Reader) Stat(name string) (archive.Entry, error) { return z.ix.Stat(name) }

// Open returns the contents of a file.
func (z *Reader) Open(e archive.Entry) (io.ReadCloser, error) {
	if e.IsDir {
		return nil, fmt.Errorf("%s: is a folder", e.Path)
	}
	exts, ok := z.extent[e.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", fs.ErrNotExist, e.Path)
	}
	rs := make([]io.Reader, 0, len(exts))
	for _, x := range exts {
		if x.off+x.size > z.size {
			return nil, fmt.Errorf("%s: %w (data outside the image)", e.Path, archive.ErrFormat)
		}
		rs = append(rs, io.NewSectionReader(z.r, x.off, x.size))
	}
	return io.NopCloser(io.MultiReader(rs...)), nil
}

// Close closes the image file.
func (z *Reader) Close() error {
	if z.f != nil {
		return z.f.Close()
	}
	return nil
}

var _ archive.Archive = (*Reader)(nil)
