// This file reads the zip central directory and turns it into archive
// entries. We parse the directory ourselves instead of using archive/zip's
// reader because Total Commander parity needs things the standard library
// does not expose: multi-volume archives whose offsets are relative to each
// part, the raw name bytes of old archives so the user can pick the code
// page, the Info-ZIP Unicode path field, NTFS and Unix timestamps, AES and
// ZipCrypto encrypted entries, and archives with data in front of them
// (self-extracting .exe files, which are found by measuring how far the
// directory moved). Zip64 end records are supported so archives larger than
// 4 GB or with more than 65535 entries open normally.
package ziparc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"unicode/utf8"

	"mormicommander/internal/archive"
)

const (
	sigLocal      = 0x04034b50
	sigCentral    = 0x02014b50
	sigEOCD       = 0x06054b50
	sigZip64EOCD  = 0x06064b50
	sigZip64Loc   = 0x07064b50
	sigDescriptor = 0x08074b50
	sigSpanSingle = 0x30304b50 // "PK00": split archive that fit on one part

	eocdLen       = 22
	zip64LocLen   = 20
	zip64EOCDLen  = 56
	centralLen    = 46
	localLen      = 30
	maxCommentLen = 0xffff
)

// record is one central directory entry with its raw fields, kept so that
// unchanged entries can be copied byte-for-byte when the archive is updated.
type record struct {
	creatorVersion uint16
	readerVersion  uint16
	flags          uint16
	method         uint16
	dosTime        uint16
	dosDate        uint16
	crc            uint32
	csize          uint64
	usize          uint64
	diskStart      uint32
	internalAttr   uint16
	externalAttr   uint32
	localOffset    uint64
	nameRaw        []byte
	extra          []byte
	commentRaw     []byte

	x extraFields
}

func (r *record) encrypted() bool { return r.flags&flagEncrypted != 0 }

func (r *record) isDirName() bool {
	return bytes.HasSuffix(r.nameRaw, []byte("/")) || bytes.HasSuffix(r.nameRaw, []byte("\\"))
}

// realMethod is the compression method, looking through the AES wrapper.
func (r *record) realMethod() uint16 {
	if r.method == methodAES && r.x.aes != nil {
		return r.x.aes.method
	}
	return r.method
}

// directory is the parsed end of a zip file.
type directory struct {
	records    []record
	commentRaw []byte
	cdStart    int64 // logical offset of the central directory
	spanned    bool  // offsets are relative to volumes
	base       int64 // bytes in front of the archive (non-spanned only)
	lastDisk   uint32
	zip64      bool // the archive has a Zip64 end record
}

// eocdInfo holds the values of the (Zip64) end of central directory record.
type eocdInfo struct {
	pos        int64
	diskNum    uint32
	cdDisk     uint32
	total      uint64
	cdSize     uint64
	cdOffset   uint64
	commentRaw []byte
	zip64Pos   int64 // -1 when there is no Zip64 record
}

// findEOCD locates the end record within the last 64 KiB + 22 bytes.
func findEOCD(r io.ReaderAt, size int64) (eocdInfo, error) {
	var e eocdInfo
	e.zip64Pos = -1
	n := int64(maxCommentLen + eocdLen)
	if n > size {
		n = size
	}
	if n < eocdLen {
		return e, archive.ErrFormat
	}
	buf := make([]byte, n)
	if _, err := r.ReadAt(buf, size-n); err != nil && err != io.EOF {
		return e, err
	}
	pos := -1
	for i := len(buf) - eocdLen; i >= 0; i-- {
		if binary.LittleEndian.Uint32(buf[i:]) == sigEOCD {
			clen := int(binary.LittleEndian.Uint16(buf[i+20:]))
			if i+eocdLen+clen <= len(buf) {
				pos = i
				break
			}
		}
	}
	if pos < 0 {
		return e, archive.ErrFormat
	}
	b := buf[pos:]
	e.pos = size - n + int64(pos)
	e.diskNum = uint32(binary.LittleEndian.Uint16(b[4:]))
	e.cdDisk = uint32(binary.LittleEndian.Uint16(b[6:]))
	e.total = uint64(binary.LittleEndian.Uint16(b[10:]))
	e.cdSize = uint64(binary.LittleEndian.Uint32(b[12:]))
	e.cdOffset = uint64(binary.LittleEndian.Uint32(b[16:]))
	clen := int(binary.LittleEndian.Uint16(b[20:]))
	e.commentRaw = append([]byte(nil), b[eocdLen:eocdLen+clen]...)
	return e, nil
}

// readZip64 reads the Zip64 end record if a locator precedes the end record.
// volStart maps a disk number to its offset in the logical stream.
func readZip64(r io.ReaderAt, e *eocdInfo, volStart func(uint32) (int64, bool)) error {
	if e.pos < zip64LocLen {
		return nil
	}
	loc := make([]byte, zip64LocLen)
	if _, err := r.ReadAt(loc, e.pos-zip64LocLen); err != nil {
		return nil
	}
	if binary.LittleEndian.Uint32(loc) != sigZip64Loc {
		return nil
	}
	disk := binary.LittleEndian.Uint32(loc[4:])
	off := int64(binary.LittleEndian.Uint64(loc[8:]))
	rec := make([]byte, zip64EOCDLen)
	try := func(p int64) bool {
		if p < 0 {
			return false
		}
		if _, err := r.ReadAt(rec, p); err != nil {
			return false
		}
		return binary.LittleEndian.Uint32(rec) == sigZip64EOCD
	}
	var at int64 = -1
	if s, ok := volStart(disk); ok && try(s+off) {
		at = s + off
	} else if try(e.pos - zip64LocLen - zip64EOCDLen) {
		// Data in front of the archive shifts every stored offset; the
		// record normally sits right before the locator.
		at = e.pos - zip64LocLen - zip64EOCDLen
	}
	if at < 0 {
		return fmt.Errorf("%w: Zip64 end record not found", archive.ErrFormat)
	}
	e.zip64Pos = at
	e.diskNum = binary.LittleEndian.Uint32(rec[16:])
	e.cdDisk = binary.LittleEndian.Uint32(rec[20:])
	e.total = binary.LittleEndian.Uint64(rec[32:])
	e.cdSize = binary.LittleEndian.Uint64(rec[40:])
	e.cdOffset = binary.LittleEndian.Uint64(rec[48:])
	return nil
}

// readDirectory parses the whole central directory of a volume set.
func readDirectory(v *volumeSet, numbered bool) (*directory, error) {
	// The end record lives in the last volume.
	e, err := findEOCD(v, v.total)
	if err != nil {
		return nil, err
	}
	spanned := e.diskNum > 0 && !numbered
	volStart := func(d uint32) (int64, bool) {
		if !spanned {
			return 0, true
		}
		if int(d) >= len(v.starts) {
			return 0, false
		}
		return v.starts[d], true
	}
	if err := readZip64(v, &e, volStart); err != nil {
		return nil, err
	}
	spanned = e.diskNum > 0 && !numbered
	d := &directory{commentRaw: e.commentRaw, spanned: spanned, lastDisk: e.diskNum, zip64: e.zip64Pos >= 0}
	if spanned && int(e.diskNum)+1 != len(v.names) {
		have := len(v.names) - 1
		missing := volumeNameFor(v.names[len(v.names)-1], have+1)
		if have > int(e.diskNum) {
			missing = v.names[0]
		}
		return nil, fmt.Errorf("%w: expected %d parts, found %d (missing %s)",
			archive.ErrMissingVolume, e.diskNum+1, len(v.names), shortName(missing))
	}

	var cdStart int64
	if spanned {
		s, ok := volStart(e.cdDisk)
		if !ok {
			return nil, archive.ErrMissingVolume
		}
		cdStart = s + int64(e.cdOffset)
	} else {
		end := e.pos
		if e.zip64Pos >= 0 {
			end = e.zip64Pos
		}
		d.base = end - int64(e.cdSize) - int64(e.cdOffset)
		if d.base < 0 {
			d.base = 0
		}
		cdStart = d.base + int64(e.cdOffset)
		if e.total > 0 && !sigAt(v, cdStart, sigCentral) && sigAt(v, int64(e.cdOffset), sigCentral) {
			d.base = 0
			cdStart = int64(e.cdOffset)
		}
	}
	if e.cdSize > uint64(v.total) || cdStart < 0 || cdStart+int64(e.cdSize) > v.total {
		return nil, fmt.Errorf("%w: central directory out of range", archive.ErrFormat)
	}
	d.cdStart = cdStart
	cd := make([]byte, e.cdSize)
	if _, err := v.ReadAt(cd, cdStart); err != nil && err != io.EOF {
		return nil, err
	}
	for len(cd) >= centralLen && binary.LittleEndian.Uint32(cd) == sigCentral {
		rec, n, err := parseCentral(cd)
		if err != nil {
			return nil, err
		}
		d.records = append(d.records, rec)
		cd = cd[n:]
	}
	if uint64(len(d.records))&0xffff != e.total&0xffff {
		return nil, fmt.Errorf("%w: central directory has %d entries, end record says %d", archive.ErrFormat, len(d.records), e.total)
	}
	return d, nil
}

func shortName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func sigAt(r io.ReaderAt, off int64, sig uint32) bool {
	var b [4]byte
	if off < 0 {
		return false
	}
	if _, err := r.ReadAt(b[:], off); err != nil {
		return false
	}
	return binary.LittleEndian.Uint32(b[:]) == sig
}

// parseCentral decodes one central directory record and returns its length.
func parseCentral(b []byte) (record, int, error) {
	var r record
	r.creatorVersion = binary.LittleEndian.Uint16(b[4:])
	r.readerVersion = binary.LittleEndian.Uint16(b[6:])
	r.flags = binary.LittleEndian.Uint16(b[8:])
	r.method = binary.LittleEndian.Uint16(b[10:])
	r.dosTime = binary.LittleEndian.Uint16(b[12:])
	r.dosDate = binary.LittleEndian.Uint16(b[14:])
	r.crc = binary.LittleEndian.Uint32(b[16:])
	c32 := binary.LittleEndian.Uint32(b[20:])
	u32 := binary.LittleEndian.Uint32(b[24:])
	nlen := int(binary.LittleEndian.Uint16(b[28:]))
	mlen := int(binary.LittleEndian.Uint16(b[30:]))
	klen := int(binary.LittleEndian.Uint16(b[32:]))
	d16 := binary.LittleEndian.Uint16(b[34:])
	r.internalAttr = binary.LittleEndian.Uint16(b[36:])
	r.externalAttr = binary.LittleEndian.Uint32(b[38:])
	o32 := binary.LittleEndian.Uint32(b[42:])
	total := centralLen + nlen + mlen + klen
	if total > len(b) {
		return r, 0, fmt.Errorf("%w: truncated central directory", archive.ErrFormat)
	}
	r.nameRaw = append([]byte(nil), b[centralLen:centralLen+nlen]...)
	r.extra = append([]byte(nil), b[centralLen+nlen:centralLen+nlen+mlen]...)
	r.commentRaw = append([]byte(nil), b[centralLen+nlen+mlen:total]...)
	r.csize, r.usize, r.localOffset, r.diskStart = uint64(c32), uint64(u32), uint64(o32), uint32(d16)
	eachExtra(r.extra, func(id uint16, d []byte) {
		if id != extraZip64 {
			return
		}
		next := func() (uint64, bool) {
			if len(d) < 8 {
				return 0, false
			}
			v := binary.LittleEndian.Uint64(d)
			d = d[8:]
			return v, true
		}
		if u32 == 0xffffffff {
			if v, ok := next(); ok {
				r.usize = v
			}
		}
		if c32 == 0xffffffff {
			if v, ok := next(); ok {
				r.csize = v
			}
		}
		if o32 == 0xffffffff {
			if v, ok := next(); ok {
				r.localOffset = v
			}
		}
		if d16 == 0xffff && len(d) >= 4 {
			r.diskStart = binary.LittleEndian.Uint32(d)
		}
	})
	r.x = parseExtra(r.extra, r.nameRaw, r.commentRaw)
	return r, total, nil
}

// Reader is an open zip archive. It implements archive.Archive and
// archive.Sectioner.
type Reader struct {
	path string
	vol  *volumeSet
	dir  *directory
	ix   *archive.Index
	opts archive.OpenOptions

	mu       sync.Mutex
	password []byte
	havePW   bool
	comment  string
}

// newArchive builds the entry index from a parsed directory.
func newArchive(path string, vol *volumeSet, dir *directory, opts archive.OpenOptions) *Reader {
	z := &Reader{path: path, vol: vol, dir: dir, opts: opts}
	var samples [][]byte
	for i := range dir.records {
		r := &dir.records[i]
		if r.flags&flagUTF8 == 0 && r.x.unicodePath == "" && !isASCIIBytes(r.nameRaw) {
			samples = append(samples, r.nameRaw)
		}
		if r.flags&flagUTF8 == 0 && r.x.unicodeCmt == "" && !isASCIIBytes(r.commentRaw) {
			samples = append(samples, r.commentRaw)
		}
	}
	if !isASCIIBytes(dir.commentRaw) && !utf8.Valid(dir.commentRaw) {
		samples = append(samples, dir.commentRaw)
	}
	dec := archive.NewNameDecoder(opts.Encoding, samples)
	text := func(r *record, raw []byte, unicode string) string {
		switch {
		case unicode != "":
			return unicode
		case r.flags&flagUTF8 != 0:
			return strings.ToValidUTF8(string(raw), "�")
		}
		return dec(raw)
	}
	entries := make([]archive.Entry, 0, len(dir.records))
	for i := range dir.records {
		r := &dir.records[i]
		name := text(r, r.nameRaw, r.x.unicodePath)
		isDir := r.isDirName()
		mode, dos := modeFromAttrs(r.creatorVersion, r.externalAttr, isDir)
		if !isDir && mode.IsDir() {
			// Only the attributes say "folder". Some old Windows tools mark
			// folders that way without a trailing slash; trust it only when
			// there is no data.
			if r.usize == 0 {
				isDir = true
			} else {
				mode &^= fs.ModeDir
				dos &^= archive.AttrDir
			}
		}
		mt := r.x.mtime
		if mt.IsZero() {
			mt = dosToTime(r.dosDate, r.dosTime)
		}
		e := archive.Entry{
			ID: i, RawName: name, IsDir: isDir,
			Size: int64(r.usize), PackedSize: int64(r.csize),
			ModTime: mt, Mode: mode, DOSAttrs: dos,
			Method:  methodName(r.realMethod()),
			Comment: text(r, r.commentRaw, r.x.unicodeCmt),
		}
		if !isDir {
			e.CRC32 = r.crc
			e.HasCRC = !(r.x.aes != nil && r.x.aes.vendorVersion == 2)
		}
		if r.encrypted() {
			e.Encrypted = true
			switch {
			case r.method == methodAES && r.x.aes != nil:
				e.Encryption = fmt.Sprintf("AES-%d", r.x.aes.keyLen()*8)
			case r.flags&flagStrong != 0:
				e.Encryption = "PKWARE strong encryption"
			default:
				e.Encryption = "ZipCrypto"
			}
		}
		entries = append(entries, e)
	}
	z.ix = archive.NewIndex(entries)
	if utf8.Valid(dir.commentRaw) {
		z.comment = string(dir.commentRaw)
	} else {
		z.comment = dec(dir.commentRaw)
	}
	return z
}

func isASCIIBytes(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

func (z *Reader) Format() string                              { return "zip" }
func (z *Reader) Path() string                                { return z.path }
func (z *Reader) Comment() string                             { return z.comment }
func (z *Reader) Entries() []archive.Entry                    { return z.ix.Entries() }
func (z *Reader) ReadDir(dir string) ([]archive.Entry, error) { return z.ix.ReadDir(dir) }
func (z *Reader) Stat(name string) (archive.Entry, error)     { return z.ix.Stat(name) }
func (z *Reader) Close() error                                { return z.vol.Close() }

// Volumes returns the files that make up the archive, in order. A normal
// archive has one.
func (z *Reader) Volumes() []string { return append([]string(nil), z.vol.names...) }

func (z *Reader) dirEndIsZip64() bool { return z.dir.zip64 }

// Spanned reports whether the archive is a split multi-volume set.
func (z *Reader) Spanned() bool { return z.dir.spanned }

// localHeaderPos maps a record's stored offset to the logical stream.
func (z *Reader) localHeaderPos(r *record) (int64, error) {
	if z.dir.spanned {
		if int(r.diskStart) >= len(z.vol.starts) {
			return 0, archive.ErrMissingVolume
		}
		return z.vol.starts[r.diskStart] + int64(r.localOffset), nil
	}
	return z.dir.base + int64(r.localOffset), nil
}

// dataPos returns where the (possibly encrypted, compressed) data of a
// record starts, by reading its local header.
func (z *Reader) dataPos(r *record) (int64, error) {
	pos, err := z.localHeaderPos(r)
	if err != nil {
		return 0, err
	}
	var h [localLen]byte
	if _, err := z.vol.ReadAt(h[:], pos); err != nil {
		return 0, fmt.Errorf("%w: cannot read local header: %v", archive.ErrFormat, err)
	}
	if binary.LittleEndian.Uint32(h[:]) != sigLocal {
		return 0, fmt.Errorf("%w: bad local header signature", archive.ErrFormat)
	}
	n := int64(binary.LittleEndian.Uint16(h[26:]))
	m := int64(binary.LittleEndian.Uint16(h[28:]))
	start := pos + localLen + n + m
	if start+int64(r.csize) > z.vol.total {
		return 0, fmt.Errorf("%w: entry data runs past the end of the archive", archive.ErrFormat)
	}
	return start, nil
}

func (z *Reader) record(e archive.Entry) (*record, error) {
	if e.ID < 0 || e.ID >= len(z.dir.records) {
		return nil, fmt.Errorf("%w: %s", archive.ErrNotFound, e.Path)
	}
	return &z.dir.records[e.ID], nil
}

// Section implements archive.Sectioner for stored, unencrypted entries, so
// nested archives can be opened in place.
func (z *Reader) Section(e archive.Entry) (io.ReaderAt, int64, bool) {
	r, err := z.record(e)
	if err != nil || r.encrypted() || r.method != methodStore || e.IsDir {
		return nil, 0, false
	}
	pos, err := z.dataPos(r)
	if err != nil {
		return nil, 0, false
	}
	return io.NewSectionReader(z.vol, pos, int64(r.csize)), int64(r.csize), true
}
