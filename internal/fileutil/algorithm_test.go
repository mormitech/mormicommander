// Tests for the algorithm table: known digests of standard inputs, name and
// extension lookups in both directions, and digest-length guessing.

package fileutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownDigests(t *testing.T) {
	tests := []struct {
		alg  Algorithm
		in   string
		want string
	}{
		{CRC32, "123456789", "cbf43926"},
		{MD5, "abc", "900150983cd24fb0d6963f7d28e17f72"},
		{SHA1, "abc", "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{SHA224, "abc", "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7"},
		{SHA256, "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{SHA384, "abc", "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7"},
		{SHA512, "abc", "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
		{SHA3_256, "abc", "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532"},
		{SHA3_512, "abc", "b751850b1a57168a5693cd924b6b096e08f621827444f70d884f5d0240d2712e10e116e9192af3c91a7ec57647e3934057340b4cf408d5a56592f8274eec53f0"},
		{BLAKE2b, "abc", "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"},
		{BLAKE3, "abc", "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85"},
	}
	for _, tt := range tests {
		d, err := HashReader(context.Background(), strings.NewReader(tt.in), []Algorithm{tt.alg}, "", 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		if d[tt.alg] != tt.want {
			t.Errorf("%v(%q) = %s, want %s", tt.alg, tt.in, d[tt.alg], tt.want)
		}
	}
}

func TestParseAlgorithm(t *testing.T) {
	tests := []struct {
		in   string
		want Algorithm
		ok   bool
	}{
		{"crc32", CRC32, true},
		{"CRC", CRC32, true},
		{"sfv", CRC32, true},
		{".sfv", CRC32, true},
		{"md5", MD5, true},
		{"SHA-1", SHA1, true},
		{"sha", SHA1, true},
		{"sha256", SHA256, true},
		{"SHA-512", SHA512, true},
		{"sha3-256", SHA3_256, true},
		{"BLAKE2b-512", BLAKE2b, true},
		{"b2", BLAKE2b, true},
		{"b3", BLAKE3, true},
		{"blake3", BLAKE3, true},
		{"whirlpool", InvalidAlgorithm, false},
		{"", InvalidAlgorithm, false},
	}
	for _, tt := range tests {
		got, err := ParseAlgorithm(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseAlgorithm(%q) = %v, %v; want %v ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
	}
}

func TestAlgorithmForPath(t *testing.T) {
	tests := map[string]Algorithm{
		"a.sfv":            CRC32,
		"A.SFV":            CRC32,
		"a.md5":            MD5,
		"a.sha":            SHA1,
		"a.sha1":           SHA1,
		"dir/x.sha256":     SHA256,
		"x.sha512":         SHA512,
		"x.sha3-256":       SHA3_256,
		"x.blake2":         BLAKE2b,
		"x.b3":             BLAKE3,
		"x.txt":            InvalidAlgorithm,
		"x.crc":            InvalidAlgorithm, // split info, not a checksum list
		"noext":            InvalidAlgorithm,
		"image.iso.md5":    MD5,
		"SHA256SUMS":       InvalidAlgorithm,
		"x.SHA384":         SHA384,
		"release.blake3":   BLAKE3,
		"release.sha224":   SHA224,
		"release.sha3-512": SHA3_512,
	}
	for p, want := range tests {
		if got := AlgorithmForPath(p); got != want {
			t.Errorf("AlgorithmForPath(%q) = %v, want %v", p, got, want)
		}
		if IsChecksumFile(p) != (want != InvalidAlgorithm) {
			t.Errorf("IsChecksumFile(%q) wrong", p)
		}
	}
}

func TestAlgorithmBasics(t *testing.T) {
	for _, a := range Algorithms {
		if !a.Valid() || a.New() == nil || a.Extension() == "" || a.String() == "" {
			t.Errorf("%v: incomplete table entry", a)
		}
		back, err := ParseAlgorithm(a.String())
		if err != nil || back != a {
			t.Errorf("ParseAlgorithm(%q) = %v, %v", a.String(), back, err)
		}
		if AlgorithmForPath("x"+a.Extension()) != a {
			t.Errorf("%v: extension %s does not map back", a, a.Extension())
		}
	}
	if InvalidAlgorithm.Valid() || InvalidAlgorithm.New() != nil {
		t.Error("InvalidAlgorithm must not be usable")
	}
	if got := AlgorithmsForHexLen(64); len(got) != 3 || got[0] != SHA256 {
		t.Errorf("AlgorithmsForHexLen(64) = %v", got)
	}
	if got := AlgorithmsForHexLen(13); got != nil {
		t.Errorf("AlgorithmsForHexLen(13) = %v", got)
	}
}

func TestHashFileMultiAndErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	os.WriteFile(p, []byte("abc"), 0o644)
	var last Progress
	d, err := HashFile(context.Background(), p, []Algorithm{MD5, SHA1, CRC32}, func(pr Progress) { last = pr })
	if err != nil {
		t.Fatal(err)
	}
	if d[MD5] != "900150983cd24fb0d6963f7d28e17f72" || d[SHA1] != "a9993e364706816aba3e25717850c26c9cd0d89d" || d[CRC32] != "352441c2" {
		t.Errorf("digests = %v", d)
	}
	if last.Done != 3 || last.Total != 3 || last.File != p {
		t.Errorf("last progress = %+v", last)
	}
	if _, err := HashFile(context.Background(), dir, []Algorithm{MD5}, nil); err == nil {
		t.Error("hashing a folder should fail")
	}
	if _, err := HashFile(context.Background(), filepath.Join(dir, "nope"), []Algorithm{MD5}, nil); err == nil {
		t.Error("hashing a missing file should fail")
	}
	if _, err := HashFile(context.Background(), p, nil, nil); err == nil {
		t.Error("no algorithm should fail")
	}
	if _, err := HashFile(context.Background(), p, []Algorithm{InvalidAlgorithm}, nil); err == nil {
		t.Error("invalid algorithm should fail")
	}
}
