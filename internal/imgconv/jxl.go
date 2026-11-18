package imgconv

import (
	"encoding/binary"
	"io"
)

// JXLSize reads the picture size from the start of a JPEG XL file (bare codestream or ISO box container), without
// decoding anything: the SizeHeader right after the FF 0A signature (ISO/IEC 18181-1, A.3.1). It is the size as
// stored, before any EXIF-like orientation. ok is false for anything else.
func JXLSize(r io.ReaderAt, size int64) (w, h int, ok bool) {
	head := make([]byte, 64)
	n, _ := r.ReadAt(head, 0)
	head = head[:n]
	if len(head) >= 2 && head[0] == 0xFF && head[1] == 0x0A {
		return jxlSizeHeader(head[2:])
	}
	if Sniff(head) != JXL {
		return 0, 0, false
	}
	// the container: find the jxlc box (the whole codestream) or the first jxlp box (a part, after a 4-byte index)
	off := int64(0)
	for i := 0; i < 64 && off+8 <= size; i++ {
		var b [16]byte
		if k, _ := r.ReadAt(b[:], off); k < 8 {
			return 0, 0, false
		}
		boxLen, hdr := int64(binary.BigEndian.Uint32(b[:4])), int64(8)
		typ := string(b[4:8])
		switch boxLen {
		case 1:
			boxLen, hdr = int64(binary.BigEndian.Uint64(b[8:16])), 16
		case 0:
			boxLen = size - off
		}
		if boxLen < hdr {
			return 0, 0, false
		}
		data := off + hdr
		if typ == "jxlp" {
			data += 4
		}
		if typ == "jxlc" || typ == "jxlp" {
			cs := make([]byte, 48)
			k, _ := r.ReadAt(cs, data)
			cs = cs[:k]
			if len(cs) < 2 || cs[0] != 0xFF || cs[1] != 0x0A {
				return 0, 0, false
			}
			return jxlSizeHeader(cs[2:])
		}
		off += boxLen
	}
	return 0, 0, false
}

// jxlBits reads bits least significant first, as JPEG XL stores them.
type jxlBits struct {
	b   []byte
	pos int
	bad bool
}

func (r *jxlBits) u(n int) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		if r.pos/8 >= len(r.b) {
			r.bad = true
			return 0
		}
		v |= uint64(r.b[r.pos/8]>>(r.pos%8)&1) << i
		r.pos++
	}
	return v
}

// u32 is the spec's U32(1+u(9), 1+u(13), 1+u(18), 1+u(30)) used for picture edges.
func (r *jxlBits) u32() uint64 {
	return 1 + r.u([]int{9, 13, 18, 30}[r.u(2)])
}

func jxlSizeHeader(b []byte) (w, h int, ok bool) {
	r := &jxlBits{b: b}
	var hh, ww uint64
	small := r.u(1) == 1
	if small {
		hh = (r.u(5) + 1) * 8
	} else {
		hh = r.u32()
	}
	ratio := r.u(3)
	switch ratio {
	case 0:
		if small {
			ww = (r.u(5) + 1) * 8
		} else {
			ww = r.u32()
		}
	default:
		num := []uint64{0, 1, 12, 4, 3, 16, 5, 2}[ratio]
		den := []uint64{0, 1, 10, 3, 2, 9, 4, 1}[ratio]
		ww = hh * num / den
	}
	if r.bad || ww == 0 || hh == 0 || ww > 1<<30 || hh > 1<<30 {
		return 0, 0, false
	}
	return int(ww), int(hh), true
}
