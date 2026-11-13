// This file holds the in-memory form of one side of a text comparison: the
// file split into lines, each line remembering its own line ending, together
// with the detected encoding and whether the file started with a byte order
// mark. Keeping the per-line endings means a file with mixed CRLF and LF
// endings (common after a file travelled between Windows and macOS) is saved
// back exactly as it was, except for the lines the user actually edited,
// which get the file's dominant ending. Saving goes through a temporary file
// in the same folder and a rename, so a crash or a full disk never leaves a
// half-written file behind, and the original permission bits are kept. A
// symbolic link is followed first, so the file it points to is updated and the
// link stays a link. A file with several hard links is rewritten in place
// instead, because a rename would detach this name from the other names and
// leave them holding the old text.
package compare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Line is one line of a document. EOL is the line terminator as found in the
// file ("\n", "\r\n" or "\r"); it is empty only for a last line that has no
// terminator.
type Line struct {
	Text string
	EOL  string
}

// Document is one side of a comparison.
type Document struct {
	Lines    []Line
	Encoding Encoding
	BOM      bool
	// Path is the file the document was loaded from (may be empty).
	Path string
}

// ParseDocument decodes data in encoding e (EncAuto = detect, using o) and
// splits it into lines. The detection result is returned for the UI (it
// tells, among other things, whether the data looked binary).
func ParseDocument(data []byte, e Encoding, o DetectOptions) (*Document, Detection, error) {
	det := Detect(data, o)
	if e != EncAuto {
		det.Encoding = e
		bom := e.BOM()
		det.BOM = bom != nil && len(data) >= len(bom) && string(data[:len(bom)]) == string(bom)
	}
	text, err := Decode(data, det.Encoding)
	if err != nil {
		return nil, det, err
	}
	return &Document{Lines: SplitLines(text), Encoding: det.Encoding, BOM: det.BOM}, det, nil
}

// NewDocument builds a UTF-8 document from text.
func NewDocument(text string) *Document {
	return &Document{Lines: SplitLines(text), Encoding: EncUTF8}
}

// LoadFile reads and parses a file.
func LoadFile(path string, e Encoding, o DetectOptions) (*Document, Detection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Detection{}, err
	}
	d, det, err := ParseDocument(data, e, o)
	if d != nil {
		d.Path = path
	}
	return d, det, err
}

// SplitLines splits text at "\r\n", "\n" and "\r", keeping each terminator
// with its line. A trailing terminator does not create an extra empty line.
func SplitLines(text string) []Line {
	var lines []Line
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			lines = append(lines, Line{Text: text[start:i], EOL: "\n"})
			start = i + 1
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				lines = append(lines, Line{Text: text[start:i], EOL: "\r\n"})
				i++
			} else {
				lines = append(lines, Line{Text: text[start:i], EOL: "\r"})
			}
			start = i + 1
		}
	}
	if start < len(text) {
		lines = append(lines, Line{Text: text[start:]})
	}
	return lines
}

// Texts returns the line texts without terminators, the input DiffLines
// expects.
func (d *Document) Texts() []string {
	out := make([]string, len(d.Lines))
	for i, l := range d.Lines {
		out[i] = l.Text
	}
	return out
}

// DefaultEOL returns the most frequent line terminator. It is used for lines
// the user adds or edits. "\n" is returned when the document has no
// terminators at all and wins ties.
func (d *Document) DefaultEOL() string {
	var lf, crlf, cr int
	for _, l := range d.Lines {
		switch l.EOL {
		case "\n":
			lf++
		case "\r\n":
			crlf++
		case "\r":
			cr++
		}
	}
	switch {
	case crlf > lf && crlf >= cr:
		return "\r\n"
	case cr > lf && cr > crlf:
		return "\r"
	}
	return "\n"
}

// String joins the lines with their terminators.
func (d *Document) String() string {
	var b strings.Builder
	for _, l := range d.Lines {
		b.WriteString(l.Text)
		b.WriteString(l.EOL)
	}
	return b.String()
}

// Bytes encodes the document for saving, in its encoding and with its BOM.
func (d *Document) Bytes() ([]byte, error) {
	return Encode(d.String(), d.Encoding, d.BOM)
}

// SaveFile writes the document to path atomically. The permission bits of an
// existing file are preserved; new files get 0644.
func (d *Document) SaveFile(path string) error {
	data, err := d.Bytes()
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}

// WriteFileAtomic writes data to a temporary file next to path and renames it
// over path. Symbolic links are resolved first, so the link's target is
// written; a file with more than one hard link is rewritten in place so all
// its names keep sharing the new content.
func WriteFileAtomic(path string, data []byte) error {
	path, err := resolveLinks(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("compare: %s is not a regular file", path)
		}
		mode = st.Mode().Perm()
		if linkCount(path, st) > 1 {
			return writeInPlace(path, data)
		}
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".mc-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// maxLinkHops limits how many symbolic links resolveLinks follows, like the
// operating system's own ELOOP limit.
const maxLinkHops = 40

// resolveLinks follows symbolic links at path until it reaches a name that is
// not a link (an existing file, or a missing name a dangling link points to).
func resolveLinks(path string) (string, error) {
	for i := 0; i < maxLinkHops; i++ {
		st, err := os.Lstat(path)
		if err != nil || st.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return "", fmt.Errorf("compare: %s: too many levels of symbolic links", path)
}

// writeInPlace truncates and rewrites an existing file, keeping its identity
// (inode), which is what hard-linked names share.
func writeInPlace(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
