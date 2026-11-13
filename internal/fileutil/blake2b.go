// BLAKE2b-512 is the hash printed by the b2sum tool from GNU coreutils, so a
// ".blake2" checksum file made here can be checked with b2sum and the other
// way round. The Go standard library does not include BLAKE2 (it lives in
// golang.org/x/crypto), and pulling a module in for one small function is not
// worth it, so this file is a straightforward implementation of RFC 7693 for
// the unkeyed 64-byte variant only.

package fileutil

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const (
	b2Size      = 64
	b2BlockSize = 128
)

var b2IV = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

var b2Sigma = [12][16]byte{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
}

type blake2bHasher struct {
	h   [8]uint64
	t   [2]uint64
	buf [b2BlockSize]byte
	n   int
}

// newBLAKE2b returns a new unkeyed BLAKE2b-512 hash.
func newBLAKE2b() hash.Hash {
	d := &blake2bHasher{}
	d.Reset()
	return d
}

func (d *blake2bHasher) Reset() {
	d.h = b2IV
	d.h[0] ^= 0x01010000 ^ b2Size
	d.t = [2]uint64{}
	d.n = 0
}

func (d *blake2bHasher) Size() int      { return b2Size }
func (d *blake2bHasher) BlockSize() int { return b2BlockSize }

func (d *blake2bHasher) addCounter(n uint64) {
	d.t[0] += n
	if d.t[0] < n {
		d.t[1]++
	}
}

func b2g(v *[16]uint64, a, b, c, e int, x, y uint64) {
	v[a] = v[a] + v[b] + x
	v[e] = bits.RotateLeft64(v[e]^v[a], -32)
	v[c] = v[c] + v[e]
	v[b] = bits.RotateLeft64(v[b]^v[c], -24)
	v[a] = v[a] + v[b] + y
	v[e] = bits.RotateLeft64(v[e]^v[a], -16)
	v[c] = v[c] + v[e]
	v[b] = bits.RotateLeft64(v[b]^v[c], -63)
}

func (d *blake2bHasher) compress(block []byte, last bool) {
	var m [16]uint64
	for i := range m {
		m[i] = binary.LittleEndian.Uint64(block[i*8:])
	}
	var v [16]uint64
	copy(v[:8], d.h[:])
	copy(v[8:], b2IV[:])
	v[12] ^= d.t[0]
	v[13] ^= d.t[1]
	if last {
		v[14] = ^v[14]
	}
	for r := 0; r < 12; r++ {
		s := &b2Sigma[r]
		b2g(&v, 0, 4, 8, 12, m[s[0]], m[s[1]])
		b2g(&v, 1, 5, 9, 13, m[s[2]], m[s[3]])
		b2g(&v, 2, 6, 10, 14, m[s[4]], m[s[5]])
		b2g(&v, 3, 7, 11, 15, m[s[6]], m[s[7]])
		b2g(&v, 0, 5, 10, 15, m[s[8]], m[s[9]])
		b2g(&v, 1, 6, 11, 12, m[s[10]], m[s[11]])
		b2g(&v, 2, 7, 8, 13, m[s[12]], m[s[13]])
		b2g(&v, 3, 4, 9, 14, m[s[14]], m[s[15]])
	}
	for i := 0; i < 8; i++ {
		d.h[i] ^= v[i] ^ v[i+8]
	}
}

func (d *blake2bHasher) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		// The final block must be compressed with the "last" flag, so a full
		// buffer is only flushed once we know more data follows it.
		if d.n == b2BlockSize {
			d.addCounter(b2BlockSize)
			d.compress(d.buf[:], false)
			d.n = 0
		}
		k := copy(d.buf[d.n:], p)
		d.n += k
		p = p[k:]
	}
	return n, nil
}

func (d *blake2bHasher) Sum(b []byte) []byte {
	c := *d
	c.addCounter(uint64(c.n))
	for i := c.n; i < b2BlockSize; i++ {
		c.buf[i] = 0
	}
	c.compress(c.buf[:], true)
	var out [b2Size]byte
	for i, w := range c.h {
		binary.LittleEndian.PutUint64(out[i*8:], w)
	}
	return append(b, out[:]...)
}
