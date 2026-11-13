// BLAKE3 is a modern, very fast cryptographic hash that the b3sum tool made
// popular for checking big downloads and backups. Total Commander does not
// offer it, but the checklist (F-332) asks for it as our own addition. The Go
// standard library has no BLAKE3, and the third-party packages pull in CPU
// feature detection and assembly, so this file is a compact, portable port of
// the official reference implementation: one 256-bit output, unkeyed, which is
// exactly what b3sum prints. Speed is secondary here because reading the file
// from disk is usually the slower part.

package fileutil

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const (
	b3OutLen   = 32
	b3BlockLen = 64
	b3ChunkLen = 1024

	b3ChunkStart = 1 << 0
	b3ChunkEnd   = 1 << 1
	b3Parent     = 1 << 2
	b3Root       = 1 << 3
)

var b3IV = [8]uint32{
	0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
	0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
}

var b3Perm = [16]int{2, 6, 3, 10, 7, 0, 4, 13, 1, 11, 12, 5, 9, 14, 15, 8}

func b3g(s *[16]uint32, a, b, c, d int, mx, my uint32) {
	s[a] = s[a] + s[b] + mx
	s[d] = bits.RotateLeft32(s[d]^s[a], -16)
	s[c] = s[c] + s[d]
	s[b] = bits.RotateLeft32(s[b]^s[c], -12)
	s[a] = s[a] + s[b] + my
	s[d] = bits.RotateLeft32(s[d]^s[a], -8)
	s[c] = s[c] + s[d]
	s[b] = bits.RotateLeft32(s[b]^s[c], -7)
}

func b3Round(s *[16]uint32, m *[16]uint32) {
	b3g(s, 0, 4, 8, 12, m[0], m[1])
	b3g(s, 1, 5, 9, 13, m[2], m[3])
	b3g(s, 2, 6, 10, 14, m[4], m[5])
	b3g(s, 3, 7, 11, 15, m[6], m[7])
	b3g(s, 0, 5, 10, 15, m[8], m[9])
	b3g(s, 1, 6, 11, 12, m[10], m[11])
	b3g(s, 2, 7, 8, 13, m[12], m[13])
	b3g(s, 3, 4, 9, 14, m[14], m[15])
}

func b3Compress(cv *[8]uint32, block *[16]uint32, counter uint64, blockLen, flags uint32) [16]uint32 {
	s := [16]uint32{
		cv[0], cv[1], cv[2], cv[3], cv[4], cv[5], cv[6], cv[7],
		b3IV[0], b3IV[1], b3IV[2], b3IV[3],
		uint32(counter), uint32(counter >> 32), blockLen, flags,
	}
	m := *block
	for r := 0; r < 7; r++ {
		b3Round(&s, &m)
		if r < 6 {
			var p [16]uint32
			for i := range p {
				p[i] = m[b3Perm[i]]
			}
			m = p
		}
	}
	for i := 0; i < 8; i++ {
		s[i] ^= s[i+8]
		s[i+8] ^= cv[i]
	}
	return s
}

func b3Words(b []byte) [16]uint32 {
	var full [b3BlockLen]byte
	copy(full[:], b)
	var w [16]uint32
	for i := range w {
		w[i] = binary.LittleEndian.Uint32(full[i*4:])
	}
	return w
}

// b3Output is a not-yet-compressed node: it can become either a chaining
// value for its parent or, for the root, the final hash bytes.
type b3Output struct {
	cv       [8]uint32
	block    [16]uint32
	counter  uint64
	blockLen uint32
	flags    uint32
}

func (o *b3Output) chainingValue() [8]uint32 {
	s := b3Compress(&o.cv, &o.block, o.counter, o.blockLen, o.flags)
	var cv [8]uint32
	copy(cv[:], s[:8])
	return cv
}

func (o *b3Output) rootBytes(out []byte) {
	s := b3Compress(&o.cv, &o.block, 0, o.blockLen, o.flags|b3Root)
	for i := 0; i < 8 && i*4 < len(out); i++ {
		binary.LittleEndian.PutUint32(out[i*4:], s[i])
	}
}

type b3Chunk struct {
	cv        [8]uint32
	counter   uint64
	block     [b3BlockLen]byte
	blockLen  int
	compCount int
}

func newB3Chunk(counter uint64) b3Chunk {
	return b3Chunk{cv: b3IV, counter: counter}
}

func (c *b3Chunk) len() int { return b3BlockLen*c.compCount + c.blockLen }

func (c *b3Chunk) startFlag() uint32 {
	if c.compCount == 0 {
		return b3ChunkStart
	}
	return 0
}

func (c *b3Chunk) update(p []byte) {
	for len(p) > 0 {
		if c.blockLen == b3BlockLen {
			w := b3Words(c.block[:])
			s := b3Compress(&c.cv, &w, c.counter, b3BlockLen, c.startFlag())
			copy(c.cv[:], s[:8])
			c.compCount++
			c.block = [b3BlockLen]byte{}
			c.blockLen = 0
		}
		n := copy(c.block[c.blockLen:], p)
		c.blockLen += n
		p = p[n:]
	}
}

func (c *b3Chunk) output() b3Output {
	return b3Output{
		cv:       c.cv,
		block:    b3Words(c.block[:c.blockLen]),
		counter:  c.counter,
		blockLen: uint32(c.blockLen),
		flags:    c.startFlag() | b3ChunkEnd,
	}
}

func b3ParentOutput(left, right [8]uint32) b3Output {
	var block [16]uint32
	copy(block[:8], left[:])
	copy(block[8:], right[:])
	return b3Output{cv: b3IV, block: block, blockLen: b3BlockLen, flags: b3Parent}
}

// blake3Hasher implements hash.Hash for unkeyed BLAKE3 with 32-byte output.
type blake3Hasher struct {
	chunk   b3Chunk
	stack   [54][8]uint32
	stackN  int
	written uint64
}

// newBLAKE3 returns a new BLAKE3-256 hash.
func newBLAKE3() hash.Hash {
	h := &blake3Hasher{}
	h.Reset()
	return h
}

func (h *blake3Hasher) Reset() {
	h.chunk = newB3Chunk(0)
	h.stackN = 0
	h.written = 0
}

func (h *blake3Hasher) Size() int      { return b3OutLen }
func (h *blake3Hasher) BlockSize() int { return b3BlockLen }

func (h *blake3Hasher) addChunkCV(cv [8]uint32, total uint64) {
	for total&1 == 0 {
		h.stackN--
		p := b3ParentOutput(h.stack[h.stackN], cv)
		cv = p.chainingValue()
		total >>= 1
	}
	h.stack[h.stackN] = cv
	h.stackN++
}

func (h *blake3Hasher) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if h.chunk.len() == b3ChunkLen {
			o := h.chunk.output()
			total := h.chunk.counter + 1
			h.addChunkCV(o.chainingValue(), total)
			h.chunk = newB3Chunk(total)
		}
		take := b3ChunkLen - h.chunk.len()
		if take > len(p) {
			take = len(p)
		}
		h.chunk.update(p[:take])
		p = p[take:]
	}
	h.written += uint64(n)
	return n, nil
}

func (h *blake3Hasher) Sum(b []byte) []byte {
	o := h.chunk.output()
	for i := h.stackN - 1; i >= 0; i-- {
		o = b3ParentOutput(h.stack[i], o.chainingValue())
	}
	var out [b3OutLen]byte
	o.rootBytes(out[:])
	return append(b, out[:]...)
}
