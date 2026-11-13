// This file reads and writes descript.ion, the per-folder comment file Total Commander (and 4DOS before it) uses for
// the Comments view and the tc.comment field. Each line is a file name followed by a space and the comment; names with
// spaces are quoted. A multi-line comment is stored on one line with "\n" for each break and the two bytes 0x04 0xC2
// at the end, which is TC's marker. Older files are in the ANSI code page, newer ones in UTF-8 (often with a BOM): we
// read both, treating invalid UTF-8 as Windows-1252/Latin-1, and write UTF-8, with a BOM only when the text needs one.
// Entries for files that no longer exist are kept, because TC keeps them too and another tool may still want them.

package listing

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// CommentFileName is the name of the comment file in each folder.
const CommentFileName = "descript.ion"

const multiLineMark = "\x04\xC2"

type commentEntry struct {
	name, text string
}

// Comments is the parsed content of one descript.ion. Names are matched case-insensitively, as on Windows.
type Comments struct {
	entries []commentEntry
	index   map[string]int
}

// NewComments returns an empty comment table.
func NewComments() *Comments { return &Comments{index: map[string]int{}} }

// ParseComments parses descript.ion bytes.
func ParseComments(data []byte) *Comments {
	c := NewComments()
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	for _, raw := range bytes.Split(data, []byte("\n")) {
		raw = bytes.TrimRight(raw, "\r")
		multi := false
		for _, mark := range [][]byte{[]byte(multiLineMark), []byte("\x04\xC3\x82"), []byte("\x04")} {
			// TC writes 0x04 0xC2; a tool that re-encoded the file as UTF-8 turns 0xC2 into "Â" (C3 82), and some
			// editors drop the second byte. All three mark a multi-line comment.
			if bytes.HasSuffix(raw, mark) {
				multi = true
				raw = raw[:len(raw)-len(mark)]
				break
			}
		}
		line := decodeCommentLine(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		var name, text string
		if strings.HasPrefix(line, `"`) {
			end := strings.IndexByte(line[1:], '"')
			if end < 0 {
				continue
			}
			name = line[1 : 1+end]
			text = strings.TrimLeft(line[end+2:], " \t")
		} else {
			i := strings.IndexAny(line, " \t")
			if i < 0 {
				name = line
			} else {
				name, text = line[:i], strings.TrimLeft(line[i+1:], " \t")
			}
		}
		if multi {
			text = strings.ReplaceAll(text, `\n`, "\n")
		}
		c.Set(name, text)
	}
	return c
}

// decodeCommentLine returns the line as a Go string, mapping bytes of non-UTF-8 lines to Latin-1 runes.
func decodeCommentLine(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// LoadComments reads dir/descript.ion; a missing file gives an empty table.
func LoadComments(dir string) (*Comments, error) {
	data, err := os.ReadFile(filepath.Join(dir, CommentFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return NewComments(), nil
	}
	if err != nil {
		return nil, err
	}
	return ParseComments(data), nil
}

func key(name string) string { return strings.ToLower(name) }

// Get returns the comment of name, or "".
func (c *Comments) Get(name string) string {
	if i, ok := c.index[key(name)]; ok {
		return c.entries[i].text
	}
	return ""
}

// Len is the number of comments.
func (c *Comments) Len() int { return len(c.index) }

// Set stores a comment; an empty text removes the entry.
func (c *Comments) Set(name, text string) {
	k := key(name)
	i, ok := c.index[k]
	if text == "" {
		if ok {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			c.reindex()
		}
		return
	}
	if ok {
		c.entries[i] = commentEntry{name: name, text: text}
		return
	}
	c.index[k] = len(c.entries)
	c.entries = append(c.entries, commentEntry{name: name, text: text})
}

// Rename moves a comment with its file, as TC does when renaming or moving within a folder.
func (c *Comments) Rename(oldName, newName string) {
	t := c.Get(oldName)
	if t == "" {
		return
	}
	c.Set(oldName, "")
	c.Set(newName, t)
}

func (c *Comments) reindex() {
	c.index = make(map[string]int, len(c.entries))
	for i, e := range c.entries {
		c.index[key(e.name)] = i
	}
}

// Bytes serialises the table in descript.ion format with CRLF line ends.
func (c *Comments) Bytes() []byte {
	var b strings.Builder
	needBOM := false
	for _, e := range c.entries {
		name := e.name
		if strings.ContainsAny(name, " \t") {
			name = `"` + name + `"`
		}
		text := e.text
		multi := strings.Contains(text, "\n")
		if multi {
			text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", `\n`)
		}
		line := name + " " + text
		for _, r := range line {
			if r >= 0x80 {
				needBOM = true
				break
			}
		}
		b.WriteString(line)
		if multi {
			b.WriteString(multiLineMark)
		}
		b.WriteString("\r\n")
	}
	out := []byte(b.String())
	if needBOM {
		out = append([]byte("\xEF\xBB\xBF"), out...)
	}
	return out
}

// Save writes dir/descript.ion atomically, or removes it when there are no comments left.
func (c *Comments) Save(dir string) error {
	p := filepath.Join(dir, CommentFileName)
	if c.Len() == 0 {
		err := os.Remove(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	tmp, err := os.CreateTemp(dir, ".descript-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(c.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// ApplyComments fills Item.Comment from the table.
func ApplyComments(items []Item, c *Comments) {
	if c == nil {
		return
	}
	for i := range items {
		if !items[i].IsUp() {
			items[i].Comment = c.Get(items[i].Name)
		}
	}
}
