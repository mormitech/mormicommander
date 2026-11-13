// This file is the archive detector: given any file, it tells which archive
// format it is by looking at its first bytes (the "magic number"), never at
// its extension. Total Commander opens a .jar, .apk, .docx or .epub as a zip
// folder with Ctrl+PgDn and recognises a renamed .rar, and mormiCommander must
// do the same. The detector knows every format the app deals with, even the
// ones this package does not unpack itself: zip, 7z, RAR (versions 4 and 5),
// tar (ustar, GNU and old v7), gzip, bzip2, xz, zstd, lz4, compress (.Z),
// cab, ISO 9660/UDF images, ar/deb, cpio, ARJ and LZH. 7z and RAR are opened
// by this package; for the others the result is a report the UI uses to route
// the file to the zip or tar engine, or to say "not supported".
//
// Besides the format it also reports what can be learned cheaply from the
// headers: self-extracting archives (an .exe with an archive appended), RAR
// multi-volume sets and whether a volume is the first one, RAR solid and
// header-encrypted ("encrypt file names") archives, and whether a gzip/bzip2/
// xz/zstd stream contains a tar archive (so .tgz files can be shown as one
// folder instead of a single compressed file).

package sevenrar

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// Format identifies an archive or compression format.
type Format int

// The known formats. FormatUnknown means no signature matched.
const (
	FormatUnknown Format = iota
	FormatZip
	Format7z
	FormatRar
	FormatTar
	FormatGzip
	FormatBzip2
	FormatXz
	FormatZstd
	FormatLz4
	FormatCompress // Unix compress, .Z
	FormatCab
	FormatISO
	FormatAr // ar archives, including Debian .deb packages
	FormatCpio
	FormatArj
	FormatLzh
)

var formatNames = map[Format]string{
	FormatUnknown:  "unknown",
	FormatZip:      "zip",
	Format7z:       "7z",
	FormatRar:      "rar",
	FormatTar:      "tar",
	FormatGzip:     "gzip",
	FormatBzip2:    "bzip2",
	FormatXz:       "xz",
	FormatZstd:     "zstd",
	FormatLz4:      "lz4",
	FormatCompress: "compress",
	FormatCab:      "cab",
	FormatISO:      "iso",
	FormatAr:       "ar",
	FormatCpio:     "cpio",
	FormatArj:      "arj",
	FormatLzh:      "lzh",
}

// String returns a short lower-case name such as "7z" or "rar".
func (f Format) String() string {
	if s, ok := formatNames[f]; ok {
		return s
	}
	return "unknown"
}

// Compressor reports whether the format is a single-stream compressor (gzip,
// bzip2, xz, zstd, lz4, compress) rather than a multi-file archive.
func (f Format) Compressor() bool {
	switch f {
	case FormatGzip, FormatBzip2, FormatXz, FormatZstd, FormatLz4, FormatCompress:
		return true
	}
	return false
}

// VolumeInfo describes where a file sits in a multi-volume set.
type VolumeInfo struct {
	// Multi is true when the file is one volume of a multi-volume set.
	Multi bool
	// Number is the zero-based volume index when known (0 = first volume),
	// or -1 when it cannot be told.
	Number int
	// First is true for the volume the set must be opened from.
	First bool
}

// Detection is the result of Detect / DetectFile.
type Detection struct {
	Format Format
	// Variant refines Format: "ustar", "gnu" or "v7" for tar, "udf" for
	// UDF disc images, "spanned" for split zip parts, "newc"/"odc"/"binary"
	// for cpio, "deb" for Debian packages.
	Variant string
	// Offset is where the archive starts inside the file; it is non-zero
	// for self-extracting archives.
	Offset int64
	// SFX is true when the archive is appended to an executable stub.
	SFX bool
	// RarVersion is 4 (RAR 1.5-4.x format) or 5 (RAR 5 format) for RAR.
	RarVersion int
	// Volume tells whether the file is part of a multi-volume set.
	Volume VolumeInfo
	// HeadersEncrypted is true when even the file list is encrypted (RAR
	// "encrypt file names"); a password is needed just to browse.
	HeadersEncrypted bool
	// Solid is true for RAR solid archives (known from the main header).
	Solid bool
	// Locked is true for locked RAR archives (cannot be modified by RAR).
	Locked bool
	// Inner is the format inside a compressor stream: FormatTar for .tgz,
	// .tbz, .txz and .tzst files, FormatUnknown otherwise.
	Inner Format
}

// Supported reports whether this package can open the archive (7z or RAR).
func (d Detection) Supported() bool {
	return d.Format == Format7z || d.Format == FormatRar
}

// Signatures, kept together so the tests can refer to them.
var (
	sig7z     = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}
	sigRar4   = []byte{'R', 'a', 'r', '!', 0x1A, 0x07, 0x00}
	sigRar5   = []byte{'R', 'a', 'r', '!', 0x1A, 0x07, 0x01, 0x00}
	sigXz     = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	sigZstd   = []byte{0x28, 0xB5, 0x2F, 0xFD}
	sigLz4    = []byte{0x04, 0x22, 0x4D, 0x18}
	sigCab    = []byte{'M', 'S', 'C', 'F', 0, 0, 0, 0}
	sigAr     = []byte("!<arch>\n")
	sigZipLoc = []byte{'P', 'K', 3, 4}
	sigZipEnd = []byte{'P', 'K', 5, 6}
	sigZipSpn = []byte{'P', 'K', 7, 8}
)

const (
	headSize = 4096
	// sfxScanLimit bounds how far into an executable we look for an
	// appended 7z or RAR archive. Real SFX stubs are well below this.
	sfxScanLimit = 4 << 20
)

// DetectFile detects the format of the file at path. In addition to the
// content checks of Detect it uses the file name to recognise volumes that
// carry no signature of their own (the second and later parts of a split
// 7z, "archive.7z.002"), and to fill in the volume number.
func DetectFile(path string) (Detection, error) {
	f, err := os.Open(path)
	if err != nil {
		return Detection{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Detection{}, err
	}
	if st.IsDir() {
		return Detection{}, errors.New("sevenrar: is a directory: " + path)
	}
	d, err := Detect(f, st.Size())
	if err != nil {
		return d, err
	}
	vn := parseVolumeName(filepath.Base(path))
	switch {
	case d.Format == FormatUnknown && vn.kind != volNone && vn.index > 0:
		// A later volume of a split set: identify the set by its first
		// volume, and report this file's position in it.
		first := FirstVolume(path)
		if first != path {
			if fd, ferr := DetectFile(first); ferr == nil && fd.Format != FormatUnknown {
				fd.Volume = VolumeInfo{Multi: true, Number: vn.index, First: false}
				fd.Offset, fd.SFX = 0, false
				return fd, nil
			}
		}
	case d.Format == FormatRar:
		if d.Volume.Multi && d.Volume.Number < 0 && (vn.kind == volRarNew || vn.kind == volRarOld) {
			d.Volume.Number = vn.index
			d.Volume.First = vn.index == 0
		}
	case d.Format == Format7z && vn.kind == volSplit:
		if vn.index == 0 {
			if _, err := os.Stat(nextVolumeName(path, vn, 1)); err == nil {
				d.Volume = VolumeInfo{Multi: true, Number: 0, First: true}
			}
		}
	case d.Format == FormatZip && vn.kind == volZipSplit:
		d.Volume = VolumeInfo{Multi: true, Number: vn.index, First: vn.index == 0}
	}
	return d, nil
}

// Detect identifies the archive format of the size bytes readable from r.
func Detect(r io.ReaderAt, size int64) (Detection, error) {
	d := Detection{Volume: VolumeInfo{Number: -1}}
	head := make([]byte, headSize)
	n, err := r.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return d, err
	}
	head = head[:n]

	if ok := detectAt(r, size, head, 0, &d); ok {
		return d, nil
	}

	// Formats whose signature is not at offset 0.
	if isTar(head) {
		d.Format = FormatTar
		d.Variant = tarVariant(head)
		return d, nil
	}
	if v, ok := isISO(r, size); ok {
		d.Format, d.Variant = FormatISO, v
		return d, nil
	}

	// Self-extracting archives: an executable (or anything else unknown)
	// with a 7z or RAR archive appended. Each candidate signature is
	// validated by its header checksum, because executables such as 7-Zip
	// itself contain the signature bytes as plain constants.
	if off, ok := scanForArchive(r, size, &d); ok {
		d.Offset = off
		d.SFX = true
		return d, nil
	}
	// Zip SFX: the end-of-central-directory record is at the end.
	if hasZipEnd(r, size) {
		d.Format = FormatZip
		d.SFX = len(head) >= 2 && head[0] == 'M' && head[1] == 'Z'
		return d, nil
	}
	return d, nil
}

// detectAt checks the signatures that live at the start of the archive
// (offset off in the file, head holding bytes from off on).
func detectAt(r io.ReaderAt, size int64, head []byte, off int64, d *Detection) bool {
	switch {
	case bytes.HasPrefix(head, sig7z):
		if off > 0 && !valid7zHeader(head) {
			return false
		}
		d.Format = Format7z
		return true
	case bytes.HasPrefix(head, sigRar5):
		if !parseRar5Main(r, off+int64(len(sigRar5)), d) && off > 0 {
			return false
		}
		d.Format, d.RarVersion = FormatRar, 5
		return true
	case bytes.HasPrefix(head, sigRar4):
		if !parseRar4Main(head[len(sigRar4):], d) && off > 0 {
			return false
		}
		d.Format, d.RarVersion = FormatRar, 4
		return true
	}
	if off > 0 {
		return false
	}
	switch {
	case bytes.HasPrefix(head, sigZipLoc), bytes.HasPrefix(head, sigZipEnd):
		d.Format = FormatZip
	case bytes.HasPrefix(head, sigZipSpn):
		d.Format, d.Variant = FormatZip, "spanned"
		d.Volume = VolumeInfo{Multi: true, Number: 0, First: true}
	case len(head) >= 3 && head[0] == 0x1F && head[1] == 0x8B && head[2] == 8:
		d.Format = FormatGzip
		d.Inner = innerFormat(r, size, FormatGzip)
	case len(head) >= 4 && head[0] == 'B' && head[1] == 'Z' && head[2] == 'h' && head[3] >= '1' && head[3] <= '9':
		d.Format = FormatBzip2
		d.Inner = innerFormat(r, size, FormatBzip2)
	case bytes.HasPrefix(head, sigXz):
		d.Format = FormatXz
		d.Inner = innerFormat(r, size, FormatXz)
	case bytes.HasPrefix(head, sigZstd):
		d.Format = FormatZstd
		d.Inner = innerFormat(r, size, FormatZstd)
	case bytes.HasPrefix(head, sigLz4):
		d.Format = FormatLz4
	case len(head) >= 2 && head[0] == 0x1F && head[1] == 0x9D:
		d.Format = FormatCompress
	case bytes.HasPrefix(head, sigCab):
		d.Format = FormatCab
	case bytes.HasPrefix(head, sigAr):
		d.Format = FormatAr
		if bytes.Contains(head[8:min(len(head), 80)], []byte("debian-binary")) {
			d.Variant = "deb"
		}
	case bytes.HasPrefix(head, []byte("070701")), bytes.HasPrefix(head, []byte("070702")):
		d.Format, d.Variant = FormatCpio, "newc"
	case bytes.HasPrefix(head, []byte("070707")):
		d.Format, d.Variant = FormatCpio, "odc"
	case len(head) >= 2 && (head[0] == 0xC7 && head[1] == 0x71 || head[0] == 0x71 && head[1] == 0xC7):
		d.Format, d.Variant = FormatCpio, "binary"
	case len(head) >= 4 && head[0] == 0x60 && head[1] == 0xEA && isArj(head):
		d.Format = FormatArj
	case len(head) >= 7 && head[2] == '-' && head[3] == 'l' && (head[4] == 'h' || head[4] == 'z') && head[6] == '-':
		d.Format = FormatLzh
	default:
		return false
	}
	return true
}

// valid7zHeader checks the CRC of the 7z start header, which follows the
// 6-byte signature and 2-byte version.
func valid7zHeader(b []byte) bool {
	if len(b) < 32 {
		return false
	}
	return binary.LittleEndian.Uint32(b[8:12]) == crc32.ChecksumIEEE(b[12:32])
}

// readVint reads a RAR 5 variable-length integer.
func readVint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7F) << (7 * i)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}

// RAR 5 header types and flags used by the detector.
const (
	rar5HeadMain    = 1
	rar5HeadEncrypt = 4
	rar5ArcVolume   = 0x0001
	rar5ArcVolNum   = 0x0002
	rar5ArcSolid    = 0x0004
	rar5ArcLocked   = 0x0010
	rar5HasExtra    = 0x0001
	rar5HasData     = 0x0002
)

// parseRar5Main reads the first RAR 5 header after the signature at off. It
// is either the main archive header or, for archives with encrypted file
// names, the archive encryption header (in which case nothing else can be
// read without the password). It returns false if the header CRC is wrong.
func parseRar5Main(r io.ReaderAt, off int64, d *Detection) bool {
	buf := make([]byte, 64)
	n, _ := r.ReadAt(buf, off)
	buf = buf[:n]
	if len(buf) < 6 {
		return false
	}
	crc := binary.LittleEndian.Uint32(buf)
	hsize, k := readVint(buf[4:])
	if k == 0 || hsize == 0 || hsize > 1<<20 {
		return false
	}
	start := 4 + k
	end := start + int(hsize)
	if end > len(buf) {
		// Main headers are tiny; a bigger one carries extra records we
		// do not need. Read it fully to check the CRC.
		big := make([]byte, end)
		if m, _ := r.ReadAt(big, off); m < end {
			return false
		}
		buf = big
	}
	if crc32.ChecksumIEEE(buf[4:end]) != crc {
		return false
	}
	p := buf[start:end]
	htype, k := readVint(p)
	p = p[k:]
	hflags, k := readVint(p)
	p = p[k:]
	switch htype {
	case rar5HeadEncrypt:
		d.HeadersEncrypted = true
		return true
	case rar5HeadMain:
	default:
		return false
	}
	if hflags&rar5HasExtra != 0 {
		_, k = readVint(p)
		p = p[k:]
	}
	if hflags&rar5HasData != 0 {
		_, k = readVint(p)
		p = p[k:]
	}
	aflags, k := readVint(p)
	p = p[k:]
	d.Solid = aflags&rar5ArcSolid != 0
	d.Locked = aflags&rar5ArcLocked != 0
	if aflags&rar5ArcVolume != 0 {
		d.Volume.Multi = true
		d.Volume.Number = 0
		if aflags&rar5ArcVolNum != 0 {
			v, _ := readVint(p)
			d.Volume.Number = int(v)
		}
		d.Volume.First = d.Volume.Number == 0
	}
	return true
}

// RAR 4 main header flags.
const (
	rar4HeadMain      = 0x73
	rar4MainVolume    = 0x0001
	rar4MainLock      = 0x0004
	rar4MainSolid     = 0x0008
	rar4MainPassword  = 0x0080
	rar4MainFirstVol  = 0x0100
	rar4MainNewNaming = 0x0010
)

// parseRar4Main reads the RAR 4 main header that follows the marker block.
// The header CRC is the low 16 bits of the CRC-32 of the header from the
// type byte on.
func parseRar4Main(b []byte, d *Detection) bool {
	if len(b) < 13 {
		return false
	}
	size := int(binary.LittleEndian.Uint16(b[5:7]))
	if b[2] != rar4HeadMain || size < 13 || size > len(b) {
		return false
	}
	if uint16(crc32.ChecksumIEEE(b[2:size])) != binary.LittleEndian.Uint16(b[0:2]) {
		return false
	}
	flags := binary.LittleEndian.Uint16(b[3:5])
	d.Solid = flags&rar4MainSolid != 0
	d.Locked = flags&rar4MainLock != 0
	d.HeadersEncrypted = flags&rar4MainPassword != 0
	if flags&rar4MainVolume != 0 {
		d.Volume.Multi = true
		if flags&rar4MainFirstVol != 0 {
			d.Volume.First = true
			d.Volume.Number = 0
		} else {
			// Old RAR versions did not set the first-volume flag, and
			// later volumes carry no number: the file name decides.
			d.Volume.Number = -1
		}
	}
	return true
}

// isTar checks for a ustar/GNU magic at offset 257, or an old v7 header with a
// valid checksum.
func isTar(h []byte) bool {
	if len(h) < 512 {
		return false
	}
	if bytes.Equal(h[257:262], []byte("ustar")) {
		return true
	}
	return h[0] != 0 && tarChecksumOK(h[:512])
}

func tarVariant(h []byte) string {
	switch {
	case bytes.Equal(h[257:265], []byte("ustar  \x00")):
		return "gnu"
	case bytes.Equal(h[257:262], []byte("ustar")):
		return "ustar"
	}
	return "v7"
}

// tarChecksumOK verifies the header checksum: the sum of all header bytes
// with the checksum field itself counted as spaces.
func tarChecksumOK(h []byte) bool {
	field := bytes.Trim(h[148:156], " \x00")
	if len(field) == 0 {
		return false
	}
	var want int64
	for _, c := range field {
		if c < '0' || c > '7' {
			return false
		}
		want = want*8 + int64(c-'0')
	}
	var sum int64
	for i, c := range h {
		if i >= 148 && i < 156 {
			c = ' '
		}
		sum += int64(c)
	}
	return sum == want
}

// isISO looks for an ISO 9660 primary volume descriptor or a UDF volume
// recognition sequence in the first sectors after the 32 KiB system area.
func isISO(r io.ReaderAt, size int64) (string, bool) {
	if size < 0x8000+2048 {
		return "", false
	}
	buf := make([]byte, 6)
	iso, udf := false, false
	for sector := int64(16); sector < 24; sector++ {
		if n, _ := r.ReadAt(buf, sector*2048); n < 6 {
			break
		}
		switch string(buf[1:6]) {
		case "CD001":
			iso = true
		case "BEA01", "NSR02", "NSR03":
			udf = true
		}
	}
	switch {
	case udf:
		return "udf", true
	case iso:
		return "", true
	}
	return "", false
}

// isArj checks the ARJ main header size field (at most 2600 bytes).
func isArj(h []byte) bool {
	n := binary.LittleEndian.Uint16(h[2:4])
	return n > 0 && n <= 2600
}

// scanForArchive searches the first sfxScanLimit bytes for a 7z or RAR
// signature with a valid header. It fills d and returns the offset.
func scanForArchive(r io.ReaderAt, size int64, d *Detection) (int64, bool) {
	limit := min(size, sfxScanLimit)
	const chunk = 64 << 10
	buf := make([]byte, chunk+64)
	for off := int64(1); off < limit; off += chunk {
		n, _ := r.ReadAt(buf, off)
		if n == 0 {
			break
		}
		b := buf[:n]
		for i := 0; i < len(b) && off+int64(i) < limit; i++ {
			if b[i] != '7' && b[i] != 'R' {
				continue
			}
			if rest := b[i:]; len(rest) >= 6 && !bytes.HasPrefix(rest, sig7z) && !bytes.HasPrefix(rest, sigRar4[:6]) {
				continue
			}
			pos := off + int64(i)
			head := make([]byte, 128)
			m, _ := r.ReadAt(head, pos)
			cand := *d
			if detectAt(r, size, head[:m], pos, &cand) {
				*d = cand
				return pos, true
			}
		}
	}
	return 0, false
}

// hasZipEnd looks for a zip end-of-central-directory record in the last
// 64 KiB + 22 bytes (the record plus the longest possible comment).
func hasZipEnd(r io.ReaderAt, size int64) bool {
	n := min(size, 65535+22)
	if n < 22 {
		return false
	}
	buf := make([]byte, n)
	if m, _ := r.ReadAt(buf, size-n); int64(m) < n {
		return false
	}
	i := bytes.LastIndex(buf, sigZipEnd)
	if i < 0 || len(buf)-i < 22 {
		return false
	}
	// The central directory must start before the record.
	cdOff := int64(binary.LittleEndian.Uint32(buf[i+16:]))
	cdSize := int64(binary.LittleEndian.Uint32(buf[i+12:]))
	return cdOff+cdSize <= size-n+int64(i)
}

// innerFormat decompresses the first tar block of a compressor stream to
// find out whether it holds a tar archive.
func innerFormat(r io.ReaderAt, size int64, f Format) Format {
	src := io.NewSectionReader(r, 0, size)
	var dec io.Reader
	switch f {
	case FormatGzip:
		zr, err := gzip.NewReader(src)
		if err != nil {
			return FormatUnknown
		}
		dec = zr
	case FormatBzip2:
		dec = bzip2.NewReader(src)
	case FormatXz:
		xr, err := xz.NewReader(src)
		if err != nil {
			return FormatUnknown
		}
		dec = xr
	case FormatZstd:
		zr, err := zstd.NewReader(src, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			return FormatUnknown
		}
		defer zr.Close()
		dec = zr
	default:
		return FormatUnknown
	}
	block := make([]byte, 512)
	if _, err := io.ReadFull(dec, block); err != nil {
		return FormatUnknown
	}
	if isTar(block) {
		return FormatTar
	}
	return FormatUnknown
}
