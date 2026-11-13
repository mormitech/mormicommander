// This file lists the checksum algorithms the Create/Verify checksum dialogs
// offer and everything that depends on the choice: the display name, the
// file extension Total Commander and the command-line tools use (.sfv, .md5,
// .sha, .sha256, ...), the tag BSD-style files put in front of a line, and the
// length of the hex digest. Verifying a checksum file has to work backwards
// from those clues, so the lookup helpers live here too. TC itself offers
// CRC32, MD5, SHA1, SHA256 and SHA512; SHA224, SHA384, SHA3, BLAKE2b and
// BLAKE3 are our additions so files made by sha224sum, b2sum or b3sum can be
// checked as well.

package fileutil

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"fmt"
	"hash"
	"hash/crc32"
	"path/filepath"
	"strings"
)

// Algorithm identifies a checksum algorithm.
type Algorithm int

// The supported algorithms. The zero value is deliberately invalid so a
// forgotten field in an options struct is caught instead of silently meaning
// CRC32.
const (
	InvalidAlgorithm Algorithm = iota
	CRC32
	MD5
	SHA1
	SHA224
	SHA256
	SHA384
	SHA512
	SHA3_256
	SHA3_512
	BLAKE2b
	BLAKE3
)

// Algorithms lists every valid algorithm in the order the dialog shows them.
var Algorithms = []Algorithm{CRC32, MD5, SHA1, SHA224, SHA256, SHA384, SHA512, SHA3_256, SHA3_512, BLAKE2b, BLAKE3}

type algInfo struct {
	name    string   // display name and BSD tag
	ext     string   // extension used when creating a checksum file
	aliases []string // other extensions recognised when verifying
	tags    []string // other BSD tags recognised when verifying
	hexLen  int
}

var algInfos = map[Algorithm]algInfo{
	CRC32:    {name: "CRC32", ext: ".sfv", aliases: []string{".crc32"}, hexLen: 8},
	MD5:      {name: "MD5", ext: ".md5", hexLen: 32},
	SHA1:     {name: "SHA1", ext: ".sha", aliases: []string{".sha1"}, tags: []string{"SHA-1"}, hexLen: 40},
	SHA224:   {name: "SHA224", ext: ".sha224", tags: []string{"SHA-224"}, hexLen: 56},
	SHA256:   {name: "SHA256", ext: ".sha256", tags: []string{"SHA-256"}, hexLen: 64},
	SHA384:   {name: "SHA384", ext: ".sha384", tags: []string{"SHA-384"}, hexLen: 96},
	SHA512:   {name: "SHA512", ext: ".sha512", tags: []string{"SHA-512"}, hexLen: 128},
	SHA3_256: {name: "SHA3-256", ext: ".sha3-256", aliases: []string{".sha3"}, tags: []string{"SHA3_256"}, hexLen: 64},
	SHA3_512: {name: "SHA3-512", ext: ".sha3-512", tags: []string{"SHA3_512"}, hexLen: 128},
	BLAKE2b:  {name: "BLAKE2b", ext: ".blake2", aliases: []string{".b2", ".blake2b", ".b2sum"}, tags: []string{"BLAKE2b-512", "BLAKE2"}, hexLen: 128},
	BLAKE3:   {name: "BLAKE3", ext: ".blake3", aliases: []string{".b3", ".b3sum"}, hexLen: 64},
}

// String returns the display name, e.g. "SHA256".
func (a Algorithm) String() string {
	if in, ok := algInfos[a]; ok {
		return in.name
	}
	return fmt.Sprintf("Algorithm(%d)", int(a))
}

// Valid reports whether a is one of the supported algorithms.
func (a Algorithm) Valid() bool {
	_, ok := algInfos[a]
	return ok
}

// Extension returns the file extension (with the dot) for a checksum file of
// this algorithm, e.g. ".sfv" for CRC32 or ".sha" for SHA1 as TC writes it.
func (a Algorithm) Extension() string { return algInfos[a].ext }

// HexLen returns the number of hex digits in a digest of this algorithm.
func (a Algorithm) HexLen() int { return algInfos[a].hexLen }

// New returns a fresh hash for the algorithm. CRC32 is the IEEE polynomial
// used by SFV files, ZIP and TC's .crc split info files.
func (a Algorithm) New() hash.Hash {
	switch a {
	case CRC32:
		return crc32.NewIEEE()
	case MD5:
		return md5.New()
	case SHA1:
		return sha1.New()
	case SHA224:
		return sha256.New224()
	case SHA256:
		return sha256.New()
	case SHA384:
		return sha512.New384()
	case SHA512:
		return sha512.New()
	case SHA3_256:
		return sha3.New256()
	case SHA3_512:
		return sha3.New512()
	case BLAKE2b:
		return newBLAKE2b()
	case BLAKE3:
		return newBLAKE3()
	}
	return nil
}

// ParseAlgorithm turns a user-typed or BSD-tag name such as "sha256",
// "SHA-256", "crc", "sfv" or "b3" into an Algorithm. It ignores case.
func ParseAlgorithm(name string) (Algorithm, error) {
	n := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(name, ".")))
	switch n {
	case "CRC", "SFV":
		return CRC32, nil
	}
	for _, a := range Algorithms {
		in := algInfos[a]
		if n == strings.ToUpper(in.name) || "."+strings.ToLower(n) == in.ext {
			return a, nil
		}
		for _, t := range in.tags {
			if n == strings.ToUpper(t) {
				return a, nil
			}
		}
		for _, x := range in.aliases {
			if "."+strings.ToLower(n) == x {
				return a, nil
			}
		}
	}
	return InvalidAlgorithm, fmt.Errorf("unknown checksum type %q", name)
}

// AlgorithmForPath guesses the algorithm from a checksum file's extension.
// It returns InvalidAlgorithm when the extension is not a checksum one, in
// which case the verifier falls back to the digest length.
func AlgorithmForPath(path string) Algorithm {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return InvalidAlgorithm
	}
	for _, a := range Algorithms {
		in := algInfos[a]
		if ext == in.ext {
			return a
		}
		for _, x := range in.aliases {
			if ext == x {
				return a
			}
		}
	}
	return InvalidAlgorithm
}

// IsChecksumFile reports whether the path has one of the checksum extensions
// this package understands. The UI uses it to decide whether "Verify
// checksums" applies to the selection.
func IsChecksumFile(path string) bool { return AlgorithmForPath(path) != InvalidAlgorithm }

// AlgorithmsForHexLen lists the algorithms whose digest has n hex digits, in
// the order a guess should try them (the most common one first).
func AlgorithmsForHexLen(n int) []Algorithm {
	var out []Algorithm
	for _, a := range Algorithms {
		if algInfos[a].hexLen == n {
			out = append(out, a)
		}
	}
	return out
}

// algorithmForTag maps a BSD tag ("SHA256", "BLAKE2b", "SHA3-256") to an
// Algorithm.
func algorithmForTag(tag string) Algorithm {
	t := strings.ToUpper(tag)
	for _, a := range Algorithms {
		in := algInfos[a]
		if t == strings.ToUpper(in.name) {
			return a
		}
		for _, x := range in.tags {
			if t == strings.ToUpper(x) {
				return a
			}
		}
	}
	return InvalidAlgorithm
}
