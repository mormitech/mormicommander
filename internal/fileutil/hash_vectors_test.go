// These tests pin the two hash functions implemented by hand in this package
// (BLAKE2b-512 and BLAKE3-256) to published test vectors, and check that every
// algorithm writes in pieces gives the same digest as one big write, so the
// streaming code paths used for large files cannot drift from the one-shot
// result.

package fileutil

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func pattern251(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestBLAKE3Vectors(t *testing.T) {
	// From the official BLAKE3 test_vectors.json (input is i % 251).
	tests := []struct {
		n    int
		want string
	}{
		{0, "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262"},
		{1, "2d3adedff11b61f14c886e35afa036736dcd87a74d27b5c1510225d0f592e213"},
		{1024, "42214739f095a406f3fc83deb889744ac00df831c10daa55189b5d121c855af7"},
		{1025, "d00278ae47eb27b34faecf67b4fe263f82d5412916c1ffd97c8cb7fb814b8444"},
		{2048, "e776b6028c7cd22a4d0ba182a8bf62205d2ef576467e838ed6f2529b85fba24a"},
		{102400, "bc3e3d41a1146b069abffad3c0d44860cf664390afce4d9661f7902e7943e085"},
	}
	for _, tt := range tests {
		h := newBLAKE3()
		h.Write(pattern251(tt.n))
		if got := hex.EncodeToString(h.Sum(nil)); got != tt.want {
			t.Errorf("BLAKE3 len %d = %s, want %s", tt.n, got, tt.want)
		}
	}
	h := newBLAKE3()
	h.Write([]byte("abc"))
	if got := hex.EncodeToString(h.Sum(nil)); got != "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85" {
		t.Errorf("BLAKE3(abc) = %s", got)
	}
}

func TestBLAKE2bVectors(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "786a02f742015903c6c6fd852552d272912f4740e15847618a86e217f71f5419d25e1031afee585313896444934eb04b903a685b1448b755d56f701afe9be2ce"},
		{"abc", "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"},
	}
	for _, tt := range tests {
		h := newBLAKE2b()
		h.Write([]byte(tt.in))
		if got := hex.EncodeToString(h.Sum(nil)); got != tt.want {
			t.Errorf("BLAKE2b(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestBLAKE2bLongVectors(t *testing.T) {
	// Values produced by Python's hashlib.blake2b over the i % 251 pattern,
	// covering exact block multiples where the "last block" rule matters.
	tests := []struct {
		n    int
		want string
	}{
		{128, blake2bRef128},
		{129, blake2bRef129},
		{1000, blake2bRef1000},
	}
	for _, tt := range tests {
		h := newBLAKE2b()
		h.Write(pattern251(tt.n))
		if got := hex.EncodeToString(h.Sum(nil)); got != tt.want {
			t.Errorf("BLAKE2b len %d = %s, want %s", tt.n, got, tt.want)
		}
	}
}

func TestStreamingMatchesOneShot(t *testing.T) {
	data := pattern251(70000)
	for _, a := range Algorithms {
		one := a.New()
		one.Write(data)
		want := one.Sum(nil)

		for _, step := range []int{1, 7, 64, 127, 1000, 1024, 4096} {
			h := a.New()
			for i := 0; i < len(data); i += step {
				end := min(i+step, len(data))
				h.Write(data[i:end])
			}
			// Sum must not disturb the state: calling it twice is identical.
			s1 := h.Sum(nil)
			s2 := h.Sum(nil)
			if !bytes.Equal(s1, want) || !bytes.Equal(s2, want) {
				t.Errorf("%v step %d: streaming digest differs", a, step)
			}
		}
		h := a.New()
		h.Write([]byte("junk"))
		h.Reset()
		h.Write(data)
		if !bytes.Equal(h.Sum(nil), want) {
			t.Errorf("%v: Reset does not restore the initial state", a)
		}
		if len(want)*2 != a.HexLen() {
			t.Errorf("%v: digest is %d hex digits, HexLen says %d", a, len(want)*2, a.HexLen())
		}
	}
}
