// Hashing single files is the building block for everything else in the
// checksum feature, and it is also used directly by three smaller commands:
// the Properties dialog that shows a file's checksums (F-691), "calculate the
// checksum of the selected files and copy it to the clipboard" (F-693), and
// TC's trick of comparing one file with a checksum the user pasted in. All of
// them read each file only once, even when several algorithms are wanted,
// because the file data is fed to every hash at the same time.

package fileutil

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// Digests maps an algorithm to the lowercase hex digest of one file.
type Digests map[Algorithm]string

// HashReader hashes everything r delivers with all the given algorithms in a
// single pass. name is only used for progress reports.
func HashReader(ctx context.Context, r io.Reader, algs []Algorithm, name string, size int64, progress ProgressFunc) (Digests, error) {
	t := newTracker(ctx, progress, size)
	return t.hash(r, algs, name, size)
}

func (t *tracker) hash(r io.Reader, algs []Algorithm, name string, size int64) (Digests, error) {
	if len(algs) == 0 {
		return nil, fmt.Errorf("no checksum type chosen")
	}
	hs := make([]hash.Hash, len(algs))
	ws := make([]io.Writer, len(algs))
	for i, a := range algs {
		if !a.Valid() {
			return nil, fmt.Errorf("unknown checksum type %v", a)
		}
		hs[i] = a.New()
		ws[i] = hs[i]
	}
	if _, err := t.copyN(io.MultiWriter(ws...), r, -1, name, 0, size); err != nil {
		return nil, err
	}
	out := make(Digests, len(algs))
	for i, a := range algs {
		out[a] = hex.EncodeToString(hs[i].Sum(nil))
	}
	return out, nil
}

// HashFile returns the hex digests of one file for the given algorithms.
func HashFile(ctx context.Context, path string, algs []Algorithm, progress ProgressFunc) (Digests, error) {
	size, err := fileSize(path)
	if err != nil {
		return nil, err
	}
	t := newTracker(ctx, progress, size)
	return t.hashFile(path, algs, size)
}

func (t *tracker) hashFile(path string, algs []Algorithm, size int64) (Digests, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return t.hash(f, algs, path, size)
}

// FileDigest is one file's result in a multi-file hashing run.
type FileDigest struct {
	Path   string
	Digest string
	Err    error
}

// HashFiles hashes several files with one algorithm, for example to copy
// their checksums to the clipboard. A file that cannot be read gets its error
// in the result instead of stopping the whole run; only a cancel stops early.
func HashFiles(ctx context.Context, paths []string, alg Algorithm, progress ProgressFunc) ([]FileDigest, error) {
	var total int64
	for _, p := range paths {
		if s, err := fileSize(p); err == nil {
			total += s
		}
	}
	t := newTracker(ctx, progress, total)
	out := make([]FileDigest, 0, len(paths))
	for _, p := range paths {
		if err := t.check(); err != nil {
			return out, err
		}
		size, err := fileSize(p)
		if err != nil {
			out = append(out, FileDigest{Path: p, Err: err})
			continue
		}
		d, err := t.hashFile(p, []Algorithm{alg}, size)
		if err != nil && t.check() != nil {
			return out, t.check()
		}
		out = append(out, FileDigest{Path: p, Digest: d[alg], Err: err})
	}
	return out, nil
}

// ClipboardText formats hashing results the way the "checksum to clipboard"
// command puts them on the clipboard: just the digest for a single file, or
// one "digest *name" line per file (the md5sum/sha256sum layout) for several,
// so the text can be pasted straight into a checksum file. Names are written
// as given; pass base names or paths relative to the panel as wanted.
// Files that failed are left out.
func ClipboardText(results []FileDigest, alg Algorithm, uppercase bool) string {
	var ok []FileDigest
	for _, r := range results {
		if r.Err == nil {
			ok = append(ok, r)
		}
	}
	conv := func(s string) string {
		if uppercase {
			return strings.ToUpper(s)
		}
		return s
	}
	if len(ok) == 1 {
		return conv(ok[0].Digest)
	}
	var b strings.Builder
	style := StyleGNU
	if alg == CRC32 {
		style = StyleSFV
	}
	for _, r := range ok {
		b.WriteString(FormatLine(style, alg, r.Path, conv(r.Digest), true))
		b.WriteString(newlineFor(defaultUnixFormat))
	}
	return b.String()
}

// MatchChecksum compares a file with a digest the user typed or pasted. The
// algorithm is picked from the digest length; when several algorithms share
// a length (SHA256, SHA3-256 and BLAKE3 all have 64 digits) the file is hashed
// with all of them in one pass and whichever matches is returned. When none
// matches, alg is the most likely candidate and ok is false.
func MatchChecksum(ctx context.Context, path, digest string, progress ProgressFunc) (alg Algorithm, ok bool, err error) {
	want := strings.ToLower(strings.TrimSpace(digest))
	if !isHex(want) {
		return InvalidAlgorithm, false, fmt.Errorf("%q is not a hexadecimal checksum", digest)
	}
	cands := AlgorithmsForHexLen(len(want))
	if len(cands) == 0 {
		return InvalidAlgorithm, false, fmt.Errorf("a checksum with %d digits does not match any known type", len(want))
	}
	d, err := HashFile(ctx, path, cands, progress)
	if err != nil {
		return InvalidAlgorithm, false, err
	}
	for _, a := range cands {
		if d[a] == want {
			return a, true, nil
		}
	}
	return cands[0], false, nil
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}
