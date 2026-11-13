// This file decodes and encodes the small binary fields of the zip format
// that the standard library either hides or does not support: MS-DOS dates,
// the "extra field" blocks (Zip64 sizes, Unix and NTFS timestamps, the
// Info-ZIP Unicode path that rescues names in old code-page archives, and the
// WinZip AES header), the external attributes that carry Unix permissions and
// DOS attribute bits, and the compression method names shown in the packed
// file info dialog. Keeping them together makes the reader and writer easy to
// check against the PKWARE APPNOTE.
package ziparc

import (
	"encoding/binary"
	"hash/crc32"
	"io/fs"
	"strconv"
	"time"
	"unicode/utf8"

	"mormicommander/internal/archive"
)

// Extra field header IDs.
const (
	extraZip64       = 0x0001
	extraNTFS        = 0x000a
	extraExtTime     = 0x5455
	extraUnicodePath = 0x7075
	extraUnicodeCmt  = 0x6375
	extraAES         = 0x9901
)

// Compression methods.
const (
	methodStore   = 0
	methodDeflate = 8
	methodDef64   = 9
	methodBzip2   = 12
	methodLZMA    = 14
	methodZstd    = 93
	methodXZ      = 95
	methodPPMd    = 98
	methodAES     = 99
)

// Host systems in the high byte of "version made by".
const (
	hostFAT  = 0
	hostUnix = 3
	hostNTFS = 10
	hostOSX  = 19
)

// Flag bits.
const (
	flagEncrypted  = 0x0001
	flagDescriptor = 0x0008
	flagStrong     = 0x0040
	flagUTF8       = 0x0800
)

func methodName(m uint16) string {
	switch m {
	case methodStore:
		return "Store"
	case 1:
		return "Shrink"
	case 2, 3, 4, 5:
		return "Reduce"
	case 6:
		return "Implode"
	case methodDeflate:
		return "Deflate"
	case methodDef64:
		return "Deflate64"
	case methodBzip2:
		return "BZip2"
	case methodLZMA:
		return "LZMA"
	case methodZstd:
		return "Zstandard"
	case methodXZ:
		return "XZ"
	case 96:
		return "JPEG"
	case 97:
		return "WavPack"
	case methodPPMd:
		return "PPMd"
	case methodAES:
		return "AES"
	}
	return "Method " + strconv.Itoa(int(m))
}

// dosToTime converts MS-DOS date and time (local time, two-second steps).
func dosToTime(date, tm uint16) time.Time {
	if date == 0 && tm == 0 {
		return time.Time{}
	}
	return time.Date(
		int(date>>9)+1980, time.Month(date>>5&0xf), int(date&0x1f),
		int(tm>>11), int(tm>>5&0x3f), int(tm&0x1f)*2, 0, time.Local)
}

// timeToDOS converts a time to MS-DOS local date and time. Times before 1980
// are clamped to 1980-01-01 because DOS dates cannot express them.
func timeToDOS(t time.Time) (date, tm uint16) {
	t = t.Local()
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local)
	}
	if t.Year() > 2107 {
		t = time.Date(2107, 12, 31, 23, 59, 58, 0, time.Local)
	}
	date = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	tm = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return
}

// filetimeToTime converts a Windows FILETIME (100 ns since 1601).
func filetimeToTime(ft uint64) time.Time {
	const epochDiff = 116444736000000000 // 1601 -> 1970 in 100 ns units
	if ft < epochDiff {
		return time.Time{}
	}
	ns := (ft - epochDiff) * 100
	return time.Unix(0, int64(ns))
}

// aesInfo is the WinZip AES extra field.
type aesInfo struct {
	vendorVersion uint16 // 1 = AE-1 (CRC kept), 2 = AE-2 (CRC zero)
	strength      byte   // 1 = 128, 2 = 192, 3 = 256 bits
	method        uint16 // the real compression method
}

func (a *aesInfo) keyLen() int  { return 8 + 8*int(a.strength) }
func (a *aesInfo) saltLen() int { return a.keyLen() / 2 }

// extraFields is what parseExtra extracts from a record's extra data.
type extraFields struct {
	mtime       time.Time
	unicodePath string
	unicodeCmt  string
	aes         *aesInfo
}

// eachExtra calls fn for every well-formed block in an extra field.
func eachExtra(extra []byte, fn func(id uint16, data []byte)) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			return
		}
		fn(id, extra[4:4+size])
		extra = extra[4+size:]
	}
}

// parseExtra reads the blocks we understand. The Zip64 block is handled by
// the caller because its layout depends on which fixed fields overflowed.
func parseExtra(extra, nameRaw, commentRaw []byte) extraFields {
	var x extraFields
	var ntfs, unix time.Time
	eachExtra(extra, func(id uint16, d []byte) {
		switch id {
		case extraNTFS:
			if len(d) < 4 {
				return
			}
			d = d[4:]
			for len(d) >= 4 {
				tag := binary.LittleEndian.Uint16(d)
				size := int(binary.LittleEndian.Uint16(d[2:]))
				if 4+size > len(d) {
					return
				}
				if tag == 1 && size >= 8 {
					ntfs = filetimeToTime(binary.LittleEndian.Uint64(d[4:]))
				}
				d = d[4+size:]
			}
		case extraExtTime:
			if len(d) >= 5 && d[0]&1 != 0 {
				unix = time.Unix(int64(int32(binary.LittleEndian.Uint32(d[1:]))), 0)
			}
		case extraUnicodePath:
			if len(d) >= 5 && d[0] == 1 && binary.LittleEndian.Uint32(d[1:]) == crc32.ChecksumIEEE(nameRaw) && utf8.Valid(d[5:]) {
				x.unicodePath = string(d[5:])
			}
		case extraUnicodeCmt:
			if len(d) >= 5 && d[0] == 1 && binary.LittleEndian.Uint32(d[1:]) == crc32.ChecksumIEEE(commentRaw) && utf8.Valid(d[5:]) {
				x.unicodeCmt = string(d[5:])
			}
		case extraAES:
			if len(d) >= 7 && d[2] == 'A' && d[3] == 'E' {
				x.aes = &aesInfo{
					vendorVersion: binary.LittleEndian.Uint16(d),
					strength:      d[4],
					method:        binary.LittleEndian.Uint16(d[5:]),
				}
			}
		}
	})
	switch {
	case !ntfs.IsZero():
		x.mtime = ntfs.Local()
	case !unix.IsZero():
		x.mtime = unix.Local()
	}
	return x
}

// stripExtra returns extra without the blocks whose IDs are listed.
func stripExtra(extra []byte, ids ...uint16) []byte {
	var out []byte
	eachExtra(extra, func(id uint16, d []byte) {
		for _, s := range ids {
			if id == s {
				return
			}
		}
		var h [4]byte
		binary.LittleEndian.PutUint16(h[:], id)
		binary.LittleEndian.PutUint16(h[2:], uint16(len(d)))
		out = append(out, h[:]...)
		out = append(out, d...)
	})
	return out
}

func extTimeExtra(t time.Time) []byte {
	b := make([]byte, 9)
	binary.LittleEndian.PutUint16(b, extraExtTime)
	binary.LittleEndian.PutUint16(b[2:], 5)
	b[4] = 1
	binary.LittleEndian.PutUint32(b[5:], uint32(t.Unix()))
	return b
}

func aesExtra(a aesInfo) []byte {
	b := make([]byte, 11)
	binary.LittleEndian.PutUint16(b, extraAES)
	binary.LittleEndian.PutUint16(b[2:], 7)
	binary.LittleEndian.PutUint16(b[4:], a.vendorVersion)
	b[6], b[7] = 'A', 'E'
	b[8] = a.strength
	binary.LittleEndian.PutUint16(b[9:], a.method)
	return b
}

// Unix mode type bits as stored in the high 16 bits of external attributes.
const (
	sIFMT   = 0o170000
	sIFDIR  = 0o040000
	sIFREG  = 0o100000
	sIFLNK  = 0o120000
	sIFIFO  = 0o010000
	sIFCHR  = 0o020000
	sIFBLK  = 0o060000
	sIFSOCK = 0o140000
)

// modeFromAttrs computes the Go file mode and DOS attributes of a record.
func modeFromAttrs(creator uint16, ext uint32, isDir bool) (fs.FileMode, uint8) {
	dos := uint8(ext & 0xff)
	host := creator >> 8
	unixMode := ext >> 16
	var m fs.FileMode
	if (host == hostUnix || host == hostOSX) && unixMode != 0 {
		m = fs.FileMode(unixMode & 0o777)
		switch unixMode & sIFMT {
		case sIFDIR:
			m |= fs.ModeDir
		case sIFLNK:
			m |= fs.ModeSymlink
		case sIFIFO:
			m |= fs.ModeNamedPipe
		case sIFCHR:
			m |= fs.ModeDevice | fs.ModeCharDevice
		case sIFBLK:
			m |= fs.ModeDevice
		case sIFSOCK:
			m |= fs.ModeSocket
		}
		if unixMode&0o4000 != 0 {
			m |= fs.ModeSetuid
		}
		if unixMode&0o2000 != 0 {
			m |= fs.ModeSetgid
		}
		if unixMode&0o1000 != 0 {
			m |= fs.ModeSticky
		}
		if dos == 0 && m.Perm()&0o200 == 0 {
			dos |= archive.AttrReadOnly
		}
	} else {
		m = 0o644
		if dos&archive.AttrReadOnly != 0 {
			m = 0o444
		}
	}
	if isDir || dos&archive.AttrDir != 0 || m&fs.ModeDir != 0 {
		m = m&^fs.ModeType | fs.ModeDir
		if m.Perm() == 0o644 || m.Perm() == 0o444 {
			m = m&^0o777 | 0o755
		}
		dos |= archive.AttrDir
	}
	return m, dos
}

// attrsFromMode builds external attributes for a new entry: Unix mode in the
// high 16 bits and DOS bits in the low byte, as Info-ZIP writes them.
func attrsFromMode(m fs.FileMode, dos uint8) uint32 {
	u := uint32(m.Perm())
	switch {
	case m&fs.ModeDir != 0:
		u |= sIFDIR
		dos |= archive.AttrDir
	case m&fs.ModeSymlink != 0:
		u |= sIFLNK
	default:
		u |= sIFREG
	}
	if m&fs.ModeSetuid != 0 {
		u |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		u |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		u |= 0o1000
	}
	if m.Perm()&0o200 == 0 {
		dos |= archive.AttrReadOnly
	}
	return u<<16 | uint32(dos)
}
