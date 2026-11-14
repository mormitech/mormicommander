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

	"mormicommander/internal/config"
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
		if n, end := quotedName(line); end > 0 {
			name = n
			text = strings.TrimLeft(line[end+1:], " \t")
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
		if strings.ContainsAny(name, " \t") || strings.HasPrefix(name, `"`) {
			name = `"` + escapeQuotes(name) + `"`
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

// Save writes dir/descript.ion atomically (keeping its permission bits), or removes it when there are no comments left.
func (c *Comments) Save(dir string) error {
	p := filepath.Join(dir, CommentFileName)
	if c.Len() == 0 {
		err := os.Remove(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	// config.WriteFileAtomic keeps the existing file's permission bits (and
	// follows a symlink), so other users of a shared folder can still read
	// the comments after a save.
	if err := config.WriteFileAtomic(p, c.Bytes(), 0o644); err != nil {
		return err
	}
	// Total Commander keeps descript.ion hidden on Windows so it does not
	// clutter every folder; the atomic write replaces the file with a fresh
	// one, so the hidden attribute is set again after each save. Elsewhere
	// this does nothing (the name has no leading dot to hide it).
	hideCommentFile(p)
	return nil
}

// quotedName reads the quoted name at the start of line and returns it with the index of its closing quote, or end
// -1 when the line does not start with a quoted name. Inside the quotes a doubled quote ("") stands for one literal
// quote: Bytes writes it that way for a quote that would otherwise look like the end of the name (one followed by a
// space, a tab, another quote or the end of the name), so a Unix name such as `a" b` survives a save and a load.
// The closing quote is the first single '"' followed by a space, a tab or the end of the line. When no such quote
// is found this way, the older rule of closingQuote is tried, so files written before quotes were doubled still read
// back as they always did.
func quotedName(line string) (string, int) {
	if !strings.HasPrefix(line, `"`) {
		return "", -1
	}
	var b strings.Builder
	for i := 1; i < len(line); i++ {
		c := line[i]
		if c != '"' {
			b.WriteByte(c)
			continue
		}
		if i+1 < len(line) && line[i+1] == '"' {
			b.WriteByte('"')
			i++
			continue
		}
		if i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t' {
			return b.String(), i
		}
		b.WriteByte('"')
	}
	if end := closingQuote(line); end > 0 {
		return line[1:end], end
	}
	return "", -1
}

// escapeQuotes doubles each quote in a name that quotedName would otherwise read as the closing quote or as half of
// an escaped pair: one followed by a space, a tab, another quote or the end of the name. Other quotes stay single,
// which keeps the file unchanged for every name that already read back correctly.
func escapeQuotes(name string) string {
	if !strings.Contains(name, `"`) {
		return name
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		b.WriteByte(c)
		if c == '"' && (i+1 == len(name) || name[i+1] == ' ' || name[i+1] == '\t' || name[i+1] == '"') {
			b.WriteByte('"')
		}
	}
	return b.String()
}

// closingQuote returns the index of the quote that ends a quoted name at the
// start of line, or -1 when the line does not start with one. The closing
// quote is the first '"' followed by a space, a tab or the end of the line, so
// a Unix name that itself starts with or contains '"' reads back whole; a line
// starting with '"' that has no such closing quote is read as an unquoted name
// instead of being dropped. This was the only rule before quotes were doubled
// and is kept as the fallback for such older files.
func closingQuote(line string) int {
	if !strings.HasPrefix(line, `"`) {
		return -1
	}
	for i := 1; i < len(line); i++ {
		if line[i] == '"' && (i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t') {
			return i
		}
	}
	return -1
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
