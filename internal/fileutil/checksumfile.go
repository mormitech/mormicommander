// Checksum files come in three common text layouts, and a verifier has to
// read all of them because users download them from everywhere:
//
//   - GNU (md5sum, sha256sum, Total Commander): "digest *name" or
//     "digest  name", one file per line;
//   - BSD (macOS md5/shasum --tag, b3sum --tag): "SHA256 (name) = digest";
//   - SFV (CRC32, used for release packs): "name DIGEST", with ";" comments.
//
// This file formats lines in those layouts and parses them back, recognising
// each line on its own so mixed or hand-edited files still work. It also
// copes with a UTF-8 byte order mark, Windows line endings, the GNU escape
// convention for names containing newlines, and the "digest only" files that
// many download sites publish next to an ISO image.

package fileutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LineStyle chooses the layout of a checksum line.
type LineStyle int

const (
	// StyleGNU is "digest *name", the md5sum/sha256sum layout TC writes.
	StyleGNU LineStyle = iota
	// StyleBSD is "ALG (name) = digest", as written by "shasum --tag".
	StyleBSD
	// StyleSFV is "name DIGEST", the Simple File Verification layout for CRC32.
	StyleSFV
)

// FormatLine returns one checksum line without the line ending. binary adds
// the "*" marker GNU tools use for binary mode, which TC always writes.
// Names containing a newline or carriage return are written with the GNU
// escape convention (a leading backslash on the line, and \\, \n, \r inside
// the name); other names, including Windows paths with backslashes, are
// written as they are so TC-style files stay readable by TC.
func FormatLine(style LineStyle, alg Algorithm, name, digest string, binary bool) string {
	prefix := ""
	if strings.ContainsAny(name, "\n\r") {
		prefix = `\`
		name = escapeName(name)
	}
	switch style {
	case StyleBSD:
		return fmt.Sprintf("%s%s (%s) = %s", prefix, alg.String(), name, digest)
	case StyleSFV:
		return fmt.Sprintf("%s%s %s", prefix, name, strings.ToUpper(digest))
	default:
		mark := " "
		if binary {
			mark = "*"
		}
		return prefix + digest + " " + mark + name
	}
}

func escapeName(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`)
	return r.Replace(s)
}

func unescapeName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ChecksumEntry is one parsed line of a checksum file.
type ChecksumEntry struct {
	Line   int    // 1-based line number in the checksum file
	Name   string // file name as written (relative or absolute, any separator)
	Digest string // lowercase hex digest
	Binary bool   // the GNU "*" marker was present
	// Algorithm is the algorithm the line is for. When the file gives no
	// clear answer (a 64-digit digest in a file without a telling extension)
	// Candidates lists every algorithm with that digest length and the
	// verifier accepts a match with any of them.
	Algorithm  Algorithm
	Candidates []Algorithm
}

// LineError describes a line that could not be understood.
type LineError struct {
	Line int
	Text string
}

// ChecksumList is a parsed checksum file.
type ChecksumList struct {
	Path      string
	Algorithm Algorithm // algorithm implied by the extension, if any
	Entries   []ChecksumEntry
	BadLines  []LineError
}

var (
	reBSD     = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*) ?\((.*)\) ?= ?([0-9A-Fa-f]+)$`)
	reGNU     = regexp.MustCompile(`^([0-9A-Fa-f]+)(?: [ *]|\t\*?| \*?)(.+)$`)
	reSFV     = regexp.MustCompile(`^(.+?)[ \t]+([0-9A-Fa-f]{8})$`)
	reHexOnly = regexp.MustCompile(`^([0-9A-Fa-f]+)$`)
)

// ParseChecksumFile reads and parses a checksum file. The extension gives the
// algorithm hint; see ParseChecksums.
func ParseChecksumFile(path string) (*ChecksumList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	l, err := ParseChecksums(f, AlgorithmForPath(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if err != nil {
		return nil, err
	}
	l.Path = path
	return l, nil
}

// ParseChecksums parses checksum lines from r. hint is the algorithm implied
// by the file name (InvalidAlgorithm when unknown). impliedName is used for a
// "digest only" line, where the file name is not written in the line; it is
// normally the checksum file's own name without its extension
// ("image.iso.sha256" -> "image.iso").
func ParseChecksums(r io.Reader, hint Algorithm, impliedName string) (*ChecksumList, error) {
	l := &ChecksumList{Algorithm: hint}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Bytes()
		if n == 1 {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		}
		text := strings.TrimRight(string(line), "\r")
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || trimmed[0] == ';' || trimmed[0] == '#' {
			continue
		}
		if e, ok := parseLine(text, hint, impliedName); ok {
			e.Line = n
			l.Entries = append(l.Entries, e)
		} else {
			l.BadLines = append(l.BadLines, LineError{Line: n, Text: text})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return l, nil
}

// parseLine recognises a single line. A leading backslash means the name is
// GNU-escaped.
func parseLine(text string, hint Algorithm, impliedName string) (ChecksumEntry, bool) {
	escaped := false
	body := strings.TrimLeft(text, " \t")
	if strings.HasPrefix(body, `\`) {
		escaped = true
		body = body[1:]
	}
	finish := func(e ChecksumEntry) (ChecksumEntry, bool) {
		if escaped {
			e.Name = unescapeName(e.Name)
		}
		e.Digest = strings.ToLower(e.Digest)
		if e.Algorithm == InvalidAlgorithm {
			if hint.Valid() && hint.HexLen() == len(e.Digest) {
				e.Algorithm = hint
			} else {
				c := AlgorithmsForHexLen(len(e.Digest))
				if len(c) == 0 {
					return e, false
				}
				e.Algorithm = c[0]
				if len(c) > 1 {
					e.Candidates = c
				}
			}
		} else if e.Algorithm.HexLen() != len(e.Digest) {
			return e, false
		}
		if e.Name == "" {
			return e, false
		}
		return e, true
	}

	// BSD layout carries its own tag, so it is unambiguous when the tag is
	// one we know.
	if m := reBSD.FindStringSubmatch(body); m != nil {
		if a := algorithmForTag(m[1]); a != InvalidAlgorithm {
			return finish(ChecksumEntry{Name: m[2], Digest: m[3], Algorithm: a})
		}
	}
	tryGNU := func() (ChecksumEntry, bool) {
		m := reGNU.FindStringSubmatch(body)
		if m == nil || len(AlgorithmsForHexLen(len(m[1]))) == 0 {
			return ChecksumEntry{}, false
		}
		bin := strings.Contains(body[len(m[1]):len(body)-len(m[2])], "*")
		return finish(ChecksumEntry{Name: m[2], Digest: m[1], Binary: bin})
	}
	trySFV := func() (ChecksumEntry, bool) {
		m := reSFV.FindStringSubmatch(body)
		if m == nil {
			return ChecksumEntry{}, false
		}
		return finish(ChecksumEntry{Name: strings.TrimSpace(m[1]), Digest: m[2], Algorithm: CRC32})
	}
	if hint == CRC32 {
		if e, ok := trySFV(); ok {
			return e, true
		}
		if e, ok := tryGNU(); ok {
			return e, true
		}
	} else {
		if e, ok := tryGNU(); ok {
			return e, true
		}
		if e, ok := trySFV(); ok && hint == InvalidAlgorithm {
			return e, true
		}
	}
	// A bare digest names the file implied by the checksum file's name.
	if m := reHexOnly.FindStringSubmatch(strings.TrimSpace(body)); m != nil && impliedName != "" {
		return finish(ChecksumEntry{Name: impliedName, Digest: m[1]})
	}
	return ChecksumEntry{}, false
}
