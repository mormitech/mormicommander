// This file builds 7z and RAR archives for the tests. No 7z or RAR tool can
// be assumed on the build machines, and the interesting cases (hostile names
// such as "../evil", links pointing out of the target folder, encrypted file
// names, multi-volume sets) are exactly the ones nobody ships as fixtures. So
// the tests write their own archives: the files are stored uncompressed,
// which keeps the writers short while exercising the real header parsing,
// checksum, encryption and volume code of the two libraries. Compressed and
// AES-encrypted 7z archives come from the library's own test data instead
// (testdata/7z, BSD licensed, see LICENSE.bodgit-sevenzip there).

package sevenrar

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"
)

// tFile is one entry for the test archive writers.
type tFile struct {
	name   string
	data   []byte
	dir    bool
	mode   fs.FileMode // permission bits; fs.ModeSymlink makes data the link target
	mtime  time.Time
	link   int    // RAR redirection type (1 unix symlink, 4 hard link, 5 file copy)
	target string // RAR link target
}

var testTime = time.Date(2023, 5, 17, 10, 20, 30, 0, time.UTC)

func (f tFile) modTime() time.Time {
	if f.mtime.IsZero() {
		return testTime
	}
	return f.mtime
}

// ---------------------------------------------------------------- 7z ----

func sz7Number(v uint64) []byte {
	for n := 0; n < 8; n++ {
		if v < uint64(1)<<(7*(n+1)) {
			first := byte(0xFF<<(8-n)) | byte(v>>(8*n))
			b := []byte{first}
			for i := 0; i < n; i++ {
				b = append(b, byte(v>>(8*i)))
			}
			return b
		}
	}
	b := []byte{0xFF}
	return binary.LittleEndian.AppendUint64(b, v)
}

func bitVector(bits []bool) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b {
			out[i/8] |= 0x80 >> (i % 8)
		}
	}
	return out
}

// write7z writes a 7z archive with every non-empty file in its own
// uncompressed ("copy") folder.
func write7z(t testing.TB, path string, files []tFile) {
	t.Helper()
	var packed bytes.Buffer
	var sizes []uint64
	var crcs []uint32
	empty := make([]bool, len(files))
	var emptyFile []bool
	anyEmpty := false
	for i, f := range files {
		if f.dir || len(f.data) == 0 {
			empty[i] = true
			anyEmpty = true
			emptyFile = append(emptyFile, !f.dir)
			continue
		}
		packed.Write(f.data)
		sizes = append(sizes, uint64(len(f.data)))
		crcs = append(crcs, crc32.ChecksumIEEE(f.data))
	}

	var h bytes.Buffer
	h.WriteByte(0x01) // kHeader
	if len(sizes) > 0 {
		h.WriteByte(0x04) // kMainStreamsInfo
		h.WriteByte(0x06) // kPackInfo
		h.Write(sz7Number(0))
		h.Write(sz7Number(uint64(len(sizes))))
		h.WriteByte(0x09) // kSize
		for _, s := range sizes {
			h.Write(sz7Number(s))
		}
		h.WriteByte(0x00)
		h.WriteByte(0x07) // kUnPackInfo
		h.WriteByte(0x0B) // kFolder
		h.Write(sz7Number(uint64(len(sizes))))
		h.WriteByte(0x00) // not external
		for range sizes {
			h.Write(sz7Number(1)) // one coder
			h.WriteByte(0x01)     // simple coder, 1-byte id
			h.WriteByte(0x00)     // Copy
		}
		h.WriteByte(0x0C) // kCodersUnPackSize
		for _, s := range sizes {
			h.Write(sz7Number(s))
		}
		h.WriteByte(0x00) // end unpack info
		h.WriteByte(0x08) // kSubStreamsInfo
		h.WriteByte(0x0D) // kNumUnPackStream
		for range sizes {
			h.Write(sz7Number(1))
		}
		h.WriteByte(0x0A) // kCRC
		h.WriteByte(0x01) // all defined
		for _, c := range crcs {
			h.Write(binary.LittleEndian.AppendUint32(nil, c))
		}
		h.WriteByte(0x00) // end substreams info
		h.WriteByte(0x00) // end streams info
	}
	h.WriteByte(0x05) // kFilesInfo
	h.Write(sz7Number(uint64(len(files))))
	prop := func(id byte, data []byte) {
		h.WriteByte(id)
		h.Write(sz7Number(uint64(len(data))))
		h.Write(data)
	}
	if anyEmpty {
		prop(0x0E, bitVector(empty))
		prop(0x0F, bitVector(emptyFile))
	}
	var names bytes.Buffer
	names.WriteByte(0) // not external
	for _, f := range files {
		for _, u := range utf16.Encode([]rune(f.name)) {
			names.Write(binary.LittleEndian.AppendUint16(nil, u))
		}
		names.Write([]byte{0, 0})
	}
	prop(0x11, names.Bytes())
	var times bytes.Buffer
	times.Write([]byte{1, 0})
	for _, f := range files {
		ft := uint64(f.modTime().UnixNano()/100) + 116444736000000000
		times.Write(binary.LittleEndian.AppendUint64(nil, ft))
	}
	prop(0x14, times.Bytes())
	var attrs bytes.Buffer
	attrs.Write([]byte{1, 0})
	for _, f := range files {
		perm := uint32(f.mode.Perm())
		var unix uint32
		switch {
		case f.dir:
			if perm == 0 {
				perm = 0o755
			}
			unix = 0o040000 | perm
		case f.mode&fs.ModeSymlink != 0:
			unix = 0o120000 | 0o777
		default:
			if perm == 0 {
				perm = 0o644
			}
			unix = 0o100000 | perm
		}
		a := uint32(0x8000) | unix<<16
		if f.dir {
			a |= 0x10
		}
		attrs.Write(binary.LittleEndian.AppendUint32(nil, a))
	}
	prop(0x15, attrs.Bytes())
	h.WriteByte(0x00) // end files info
	h.WriteByte(0x00) // end header

	start := make([]byte, 20)
	binary.LittleEndian.PutUint64(start[0:], uint64(packed.Len()))
	binary.LittleEndian.PutUint64(start[8:], uint64(h.Len()))
	binary.LittleEndian.PutUint32(start[16:], crc32.ChecksumIEEE(h.Bytes()))
	var out bytes.Buffer
	out.Write(sig7z)
	out.Write([]byte{0, 4})
	out.Write(binary.LittleEndian.AppendUint32(nil, crc32.ChecksumIEEE(start)))
	out.Write(start)
	out.Write(packed.Bytes())
	out.Write(h.Bytes())
	writeFile(t, path, out.Bytes())
}

func writeFile(t testing.TB, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// -------------------------------------------------------------- RAR 5 ----

func vint(v uint64) []byte {
	var b []byte
	for {
		c := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b = append(b, c|0x80)
			continue
		}
		return append(b, c)
	}
}

// rar5Block builds one RAR 5 header block (without data).
func rar5Block(htype, flags uint64, body, extra []byte, dataSize int64) []byte {
	if len(extra) > 0 {
		flags |= rar5HasExtra
	}
	if dataSize >= 0 {
		flags |= rar5HasData
	}
	var h []byte
	h = append(h, vint(htype)...)
	h = append(h, vint(flags)...)
	if len(extra) > 0 {
		h = append(h, vint(uint64(len(extra)))...)
	}
	if dataSize >= 0 {
		h = append(h, vint(uint64(dataSize))...)
	}
	h = append(h, body...)
	h = append(h, extra...)
	sized := append(vint(uint64(len(h))), h...)
	return append(binary.LittleEndian.AppendUint32(nil, crc32.ChecksumIEEE(sized)), sized...)
}

func extraRecord(rtype uint64, data []byte) []byte {
	rec := append(vint(rtype), data...)
	return append(vint(uint64(len(rec))), rec...)
}

// rar5Keys mirrors RAR 5's PBKDF2-HMAC-SHA256 key derivation: key, hash
// key and the 12-byte password check value.
func rar5Keys(pass string, salt []byte, kdfLog int) (key, check []byte) {
	prf := hmac.New(sha256.New, []byte(pass))
	prf.Write(salt)
	prf.Write([]byte{0, 0, 0, 1})
	t := prf.Sum(nil)
	u := append([]byte(nil), t...)
	var keys [][]byte
	for _, iter := range []int{1<<kdfLog - 1, 16, 16} {
		for ; iter > 0; iter-- {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range u {
				t[j] ^= u[j]
			}
		}
		keys = append(keys, append([]byte(nil), t...))
	}
	pw := keys[2]
	for i, v := range pw[8:] {
		pw[i&7] ^= v
	}
	pw = pw[:8]
	sum := sha256.Sum256(pw)
	return keys[0], append(pw, sum[:4]...)
}

func aesCBC(key, iv, plain []byte) []byte {
	p := append([]byte(nil), plain...)
	for len(p)%16 != 0 {
		p = append(p, 0)
	}
	block, _ := aes.NewCipher(key)
	out := make([]byte, len(p))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, p)
	return out
}

type rarOpts struct {
	password   string // encrypt file contents
	encHeaders bool   // also encrypt the headers ("encrypt file names")
	volumeSize int    // split stored data into volumes of this many bytes
	oldNaming  bool   // name.rar, name.r00 instead of name.part1.rar
	solid      bool   // set the solid flags
}

const testKdfLog = 4 // tiny PBKDF2 count keeps the tests fast

// writeRar5 writes a RAR 5 archive (or volume set) named base+".rar" /
// base+".partN.rar" and returns the volume paths.
func writeRar5(t testing.TB, base string, files []tFile, o rarOpts) []string {
	t.Helper()
	salt := bytes.Repeat([]byte{0x5A}, 16)
	var key, check []byte
	if o.password != "" {
		key, check = rar5Keys(o.password, salt, testKdfLog)
	}

	type piece struct {
		f          tFile
		data       []byte // stored (possibly encrypted) bytes of this part
		first      bool
		last       bool
		crc        uint32
		unpacked   int64
		encryptRec []byte
	}
	// Split every file's stored data into pieces that fit the volumes.
	var vols [][]piece
	cur := []piece{}
	room := o.volumeSize
	for i, f := range files {
		stored := f.data
		var encRec []byte
		if o.password != "" && !f.dir && f.mode&fs.ModeSymlink == 0 && f.link == 0 {
			iv := bytes.Repeat([]byte{byte(i + 1)}, 16)
			stored = aesCBC(key, iv, f.data)
			var r []byte
			r = append(r, vint(0)...) // AES-256
			r = append(r, vint(1)...) // check value present
			r = append(r, testKdfLog)
			r = append(r, salt...)
			r = append(r, iv...)
			r = append(r, check...)
			encRec = extraRecord(1, r)
		}
		whole := crc32.ChecksumIEEE(f.data)
		first := true
		for {
			n := len(stored)
			if o.volumeSize > 0 && n > room {
				n = room
			}
			p := piece{f: f, data: stored[:n], first: first, unpacked: int64(len(f.data)), encryptRec: encRec}
			stored = stored[n:]
			p.last = len(stored) == 0
			if p.last {
				p.crc = whole
			} else {
				p.crc = crc32.ChecksumIEEE(p.data)
			}
			cur = append(cur, p)
			room -= n
			first = false
			if p.last {
				break
			}
			vols = append(vols, cur)
			cur = []piece{}
			room = o.volumeSize
		}
		if o.volumeSize > 0 && room <= 0 && i < len(files)-1 {
			vols = append(vols, cur)
			cur = []piece{}
			room = o.volumeSize
		}
	}
	vols = append(vols, cur)

	multi := len(vols) > 1
	var paths []string
	for vi, pieces := range vols {
		var out bytes.Buffer
		out.Write(sigRar5)
		var hk []byte // header key
		writeHeader := func(b []byte) {
			if hk == nil {
				out.Write(b)
				return
			}
			iv := bytes.Repeat([]byte{0xA0 + byte(out.Len()%16)}, 16)
			out.Write(iv)
			out.Write(aesCBC(hk, iv, b))
		}
		if o.encHeaders {
			var body []byte
			body = append(body, vint(0)...)
			body = append(body, vint(1)...)
			body = append(body, testKdfLog)
			body = append(body, salt...)
			body = append(body, check...)
			out.Write(rar5Block(rar5HeadEncrypt, 0, body, nil, -1))
			hk = key
		}
		var aflags uint64
		var abody []byte
		if multi {
			aflags |= rar5ArcVolume
			if vi > 0 {
				aflags |= rar5ArcVolNum
			}
		}
		if o.solid {
			aflags |= rar5ArcSolid
		}
		abody = append(abody, vint(aflags)...)
		if vi > 0 {
			abody = append(abody, vint(uint64(vi))...)
		}
		writeHeader(rar5Block(rar5HeadMain, 0, abody, nil, -1))
		for _, p := range pieces {
			f := p.f
			var fflags uint64 = 0x0002 | 0x0004 // mtime, CRC
			if f.dir {
				fflags |= 0x0001
			}
			mode := uint64(f.mode.Perm())
			switch {
			case f.dir && mode == 0:
				mode = 0o755
			case f.mode&fs.ModeSymlink != 0:
				mode = 0o120777
			case mode == 0:
				mode = 0o644
			}
			var body []byte
			body = append(body, vint(fflags)...)
			body = append(body, vint(uint64(p.unpacked))...)
			body = append(body, vint(mode)...)
			body = binary.LittleEndian.AppendUint32(body, uint32(f.modTime().Unix()))
			body = binary.LittleEndian.AppendUint32(body, p.crc)
			var comp uint64 // version 0, method 0 (store)
			if o.solid && !f.dir {
				comp |= 0x40
			}
			body = append(body, vint(comp)...)
			body = append(body, vint(1)...) // host OS: Unix
			body = append(body, vint(uint64(len(f.name)))...)
			body = append(body, f.name...)
			var extra []byte
			extra = append(extra, p.encryptRec...)
			if f.link != 0 || f.mode&fs.ModeSymlink != 0 {
				lt := uint64(f.link)
				target := f.target
				if lt == 0 {
					lt, target = 1, string(f.data)
				}
				var r []byte
				r = append(r, vint(lt)...)
				r = append(r, vint(0)...)
				r = append(r, vint(uint64(len(target)))...)
				r = append(r, target...)
				extra = append(extra, extraRecord(5, r)...)
			}
			var hflags uint64
			if !p.first {
				hflags |= 0x0008
			}
			if !p.last {
				hflags |= 0x0010
			}
			data := p.data
			if f.link != 0 || f.mode&fs.ModeSymlink != 0 {
				data = nil
			}
			writeHeader(rar5Block(2, hflags, body, extra, int64(len(data))))
			out.Write(data)
		}
		var eflags uint64
		if vi < len(vols)-1 {
			eflags = 1
		}
		writeHeader(rar5Block(5, 0, vint(eflags), nil, -1))
		var name string
		switch {
		case !multi:
			name = base + ".rar"
		case o.oldNaming && vi == 0:
			name = base + ".rar"
		case o.oldNaming:
			name = fmt.Sprintf("%s.r%02d", base, vi-1)
		default:
			name = fmt.Sprintf("%s.part%d.rar", base, vi+1)
		}
		writeFile(t, name, out.Bytes())
		paths = append(paths, name)
	}
	return paths
}

// -------------------------------------------------------------- RAR 4 ----

func dosTime(t time.Time) uint32 {
	return uint32(t.Year()-1980)<<25 | uint32(t.Month())<<21 | uint32(t.Day())<<16 |
		uint32(t.Hour())<<11 | uint32(t.Minute())<<5 | uint32(t.Second()/2)
}

// rar4Block builds a RAR 4 block: CRC16, type, flags, size, then body.
func rar4Block(htype byte, flags uint16, body []byte) []byte {
	h := []byte{htype}
	h = binary.LittleEndian.AppendUint16(h, flags)
	h = binary.LittleEndian.AppendUint16(h, uint16(7+len(body)))
	h = append(h, body...)
	crc := uint16(crc32.ChecksumIEEE(h))
	return append(binary.LittleEndian.AppendUint16(nil, crc), h...)
}

func rar4Main(flags uint16) []byte { return rar4Block(rar4HeadMain, flags, make([]byte, 6)) }

// writeRar4 writes a RAR 4 archive with stored files; volumeSize > 0 splits
// it into old-style volumes (name.rar, name.r00, ...).
func writeRar4(t testing.TB, base string, files []tFile, volumeSize int) []string {
	t.Helper()
	type piece struct {
		f           tFile
		data        []byte
		first, last bool
		crc         uint32
	}
	var vols [][]piece
	cur := []piece{}
	room := volumeSize
	for _, f := range files {
		stored := f.data
		first := true
		for {
			n := len(stored)
			if volumeSize > 0 && n > room {
				n = room
			}
			p := piece{f: f, data: stored[:n], first: first}
			stored = stored[n:]
			p.last = len(stored) == 0
			p.crc = crc32.ChecksumIEEE(f.data)
			if !p.last {
				p.crc = crc32.ChecksumIEEE(p.data)
			}
			cur = append(cur, p)
			room -= n
			first = false
			if p.last {
				break
			}
			vols = append(vols, cur)
			cur = []piece{}
			room = volumeSize
		}
	}
	vols = append(vols, cur)
	multi := len(vols) > 1
	var paths []string
	for vi, pieces := range vols {
		var out bytes.Buffer
		out.Write(sigRar4)
		var mflags uint16
		if multi {
			mflags |= rar4MainVolume
			if vi == 0 {
				mflags |= rar4MainFirstVol
			}
		}
		out.Write(rar4Main(mflags))
		for _, p := range pieces {
			f := p.f
			flags := uint16(0x8000)
			if f.dir {
				flags |= 0x00E0
			}
			if !p.first {
				flags |= 0x0001
			}
			if !p.last {
				flags |= 0x0002
			}
			mode := uint32(f.mode.Perm())
			if mode == 0 {
				mode = 0o644
				if f.dir {
					mode = 0o40755
				}
			}
			var body []byte
			body = binary.LittleEndian.AppendUint32(body, uint32(len(p.data)))
			body = binary.LittleEndian.AppendUint32(body, uint32(len(f.data)))
			body = append(body, 3) // host OS: Unix
			body = binary.LittleEndian.AppendUint32(body, p.crc)
			body = binary.LittleEndian.AppendUint32(body, dosTime(f.modTime().In(time.Local)))
			body = append(body, 20, 0x30) // version 2.0, stored
			body = binary.LittleEndian.AppendUint16(body, uint16(len(f.name)))
			body = binary.LittleEndian.AppendUint32(body, mode)
			body = append(body, f.name...)
			out.Write(rar4Block(0x74, flags, body))
			out.Write(p.data)
		}
		var eflags uint16
		if vi < len(vols)-1 {
			eflags = 1
		}
		out.Write(rar4Block(0x7B, eflags, nil))
		name := base + ".rar"
		if vi > 0 {
			name = fmt.Sprintf("%s.r%02d", base, vi-1)
		}
		writeFile(t, name, out.Bytes())
		paths = append(paths, name)
	}
	return paths
}
