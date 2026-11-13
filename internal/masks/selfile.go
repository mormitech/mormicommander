// This file saves and loads a selection as a text file or clipboard text (cm_SaveSelectionToFile,
// cm_LoadSelectionFromFile, cm_LoadSelectionFromClip). The format is the one Total Commander writes and every other
// tool can produce: one name per line, either bare names or full paths. When loading, a full path selects the entry
// only if its folder is the panel's folder, a bare name selects the entry of that name, and a line with wildcards
// selects everything it matches, so a hand-written list like "*.jpg" also works. Files written on Windows may be
// UTF-16 or have a UTF-8 byte order mark and CRLF line ends; all of those are accepted, because the file may come from
// TC itself, Notepad or a script.

package masks

import (
	"bufio"
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// WriteSelection writes names (one per line, with the platform's line ending). With full set, each name is joined to
// dir to give a full path, as TC does when the selection file is meant for other programs.
func WriteSelection(w io.Writer, dir string, names []string, full bool) error {
	bw := bufio.NewWriter(w)
	for _, n := range names {
		if full {
			n = filepath.Join(dir, n)
		}
		if _, err := bw.WriteString(n + lineEnding); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadSelection reads a selection file or clipboard text and returns its non-empty lines.
func ReadSelection(r io.Reader) ([]string, error) {
	text, err := readText(r)
	if err != nil {
		return nil, err
	}
	return SplitLines(text), nil
}

// SplitLines splits text at any line ending and drops blank lines and surrounding spaces. Spaces inside a line are
// kept, and so are leading spaces after trimming only when quoted: a line written as "  name " in double quotes keeps
// them, because file names can legally start or end with a space.
func SplitLines(text string) []string {
	var out []string
	for _, ln := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		t := strings.TrimSpace(ln)
		if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
			t = t[1 : len(t)-1]
		}
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// readText reads all of r and decodes UTF-16 (with byte order mark) or UTF-8 (with or without one).
func readText(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:]), nil
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return decodeUTF16(b[2:], false), nil
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return decodeUTF16(b[2:], true), nil
	}
	return string(b), nil
}

// decodeUTF16 turns UTF-16 bytes into a string; a trailing odd byte is dropped.
func decodeUTF16(b []byte, bigEndian bool) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		if bigEndian {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			u[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	return string(utf16.Decode(u))
}

// ApplyList marks the entries named by lines (from ReadSelection or SplitLines) in the panel folder dir. With replace
// set, the old selection is dropped first (TC's "load selection"); otherwise the lines add to it. It returns how many
// marks were added.
func (s *Selection) ApplyList(lines []string, dir string, replace bool) int {
	if replace {
		s.Clear()
	}
	cleanDir := filepath.Clean(dir)
	var names []string
	n := 0
	for _, ln := range lines {
		name := ln
		if strings.ContainsAny(ln, `/\`) {
			p := strings.TrimRight(ln, `/\`)
			if p == "" {
				continue
			}
			// Accept both separators so a list written on Windows works on a Samba share seen from macOS, as long as
			// the folder part matches.
			p = filepath.FromSlash(strings.ReplaceAll(p, `\`, "/"))
			if !filepath.IsAbs(p) && dir != "" {
				p = filepath.Join(cleanDir, p)
			}
			if dir == "" || !sameName(filepath.Dir(p), cleanDir, s.cs) {
				continue
			}
			name = filepath.Base(p)
		}
		if m := CompileMask(name, s.cs); m.HasWildcards() {
			l := &List{Include: []*Mask{m}, cs: s.cs}
			n += s.SelectGroup(l, KindBoth)
			continue
		}
		names = append(names, name)
	}
	return n + s.SelectNames(names, true)
}
