// A wincmd.ini-style INI reader and writer that keeps the file the way the
// user wrote it. Total Commander users edit wincmd.ini by hand, leave comments
// in it, and expect sections to stay where they put them, so this is not a
// "parse into a map, dump the map" INI library: every line of the file is
// kept, untouched lines are written back byte for byte, a changed value is
// rewritten in place, and a new key goes to the end of its section (before the
// blank lines and comments that separate it from the next one). Section and
// key names are compared without regard to case, exactly like the Windows
// GetPrivateProfileString API that Total Commander uses, and when a section or
// key appears twice the first one wins. The line ending style (CRLF or LF) and
// the text encoding of the original file are remembered and reused on save.

package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type lineKind int

const (
	lineBlank lineKind = iota
	lineComment
	lineKV
	lineOther // text without '=' that isn't a comment; kept as is
)

type iniLine struct {
	kind  lineKind
	raw   string // original text; used when the line was not changed
	key   string
	value string
	dirty bool
}

func (l *iniLine) text() string {
	if l.kind == lineKV && l.dirty {
		return l.key + "=" + l.value
	}
	return l.raw
}

type iniSection struct {
	name   string // "" for the preamble before the first header
	header string // original header line, reused when the name is unchanged
	lines  []*iniLine
}

// KV is one key/value pair of a section, in file order.
type KV struct {
	Key   string
	Value string
}

// File is a parsed INI document. The zero value is not usable; use NewFile,
// Parse or LoadFile.
type File struct {
	sections []*iniSection // sections[0] is always the preamble
	// Encoding is used when the file is saved; Parse sets it from the input.
	Encoding Encoding
	// Newline is "\r\n" or "\n"; Parse sets it from the first line ending.
	Newline string
	finalNL bool
}

// NewFile returns an empty document that will be saved as UTF-8 with the
// platform-neutral "\n" line ending (Windows users get "\r\n" through
// NewFileFor or by loading an existing file).
func NewFile() *File {
	return &File{sections: []*iniSection{{}}, Encoding: EncUTF8, Newline: "\n", finalNL: true}
}

// ParseString parses INI text.
func ParseString(text string) *File {
	f := NewFile()
	if i := strings.Index(text, "\n"); i > 0 && text[i-1] == '\r' {
		f.Newline = "\r\n"
	}
	if text == "" {
		return f
	}
	f.finalNL = strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	cur := f.sections[0]
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		t := strings.TrimSpace(raw)
		switch {
		case t == "":
			cur.lines = append(cur.lines, &iniLine{kind: lineBlank, raw: raw})
		case t[0] == ';' || t[0] == '#':
			cur.lines = append(cur.lines, &iniLine{kind: lineComment, raw: raw})
		case t[0] == '[' && strings.Contains(t, "]"):
			name := t[1:strings.Index(t, "]")]
			cur = &iniSection{name: strings.TrimSpace(name), header: raw}
			f.sections = append(f.sections, cur)
		default:
			if i := strings.Index(raw, "="); i >= 0 {
				cur.lines = append(cur.lines, &iniLine{
					kind:  lineKV,
					raw:   raw,
					key:   strings.TrimSpace(raw[:i]),
					value: strings.TrimSpace(raw[i+1:]),
				})
			} else {
				cur.lines = append(cur.lines, &iniLine{kind: lineOther, raw: raw})
			}
		}
	}
	return f
}

// Parse decodes raw bytes (see Decode for the encoding rules; fallback is the
// code page for non-UTF-8 files without a byte order mark) and parses them.
func Parse(data []byte, fallback Encoding) *File {
	text, enc := Decode(data, fallback)
	f := ParseString(text)
	f.Encoding = enc
	return f
}

// LoadFile reads and parses a file. A missing file is reported with an error
// that satisfies errors.Is(err, os.ErrNotExist).
func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, EncCP1252), nil
}

// String renders the document as text (without encoding it).
func (f *File) String() string {
	var b strings.Builder
	first := true
	emit := func(s string) {
		if !first {
			b.WriteString(f.Newline)
		}
		first = false
		b.WriteString(s)
	}
	for i, s := range f.sections {
		if i > 0 {
			h := s.header
			if headerName(h) != s.name {
				h = "[" + s.name + "]"
			}
			emit(h)
		}
		for _, l := range s.lines {
			emit(l.text())
		}
	}
	if f.finalNL && !first {
		b.WriteString(f.Newline)
	}
	return b.String()
}

func headerName(h string) string {
	t := strings.TrimSpace(h)
	if !strings.HasPrefix(t, "[") || !strings.Contains(t, "]") {
		return ""
	}
	return strings.TrimSpace(t[1:strings.Index(t, "]")])
}

// Bytes renders and encodes the document. If the file's code page can't hold
// a character the document switches to UTF-16LE, which is what Total
// Commander does too, and Encoding is updated.
func (f *File) Bytes() []byte {
	text := f.String()
	data, err := Encode(text, f.Encoding)
	if errors.Is(err, ErrUnencodable) {
		f.Encoding = EncUTF16LE
		data, _ = Encode(text, f.Encoding)
	}
	return data
}

// Save writes the document atomically (see WriteFileAtomic).
func (f *File) Save(path string) error {
	return WriteFileAtomic(path, f.Bytes(), 0o644)
}

func (f *File) findSections(name string) []*iniSection {
	var out []*iniSection
	for _, s := range f.sections[1:] {
		if strings.EqualFold(s.name, name) {
			out = append(out, s)
		}
	}
	return out
}

func (f *File) find(section, key string) *iniLine {
	for _, s := range f.findSections(section) {
		for _, l := range s.lines {
			if l.kind == lineKV && strings.EqualFold(l.key, key) {
				return l
			}
		}
	}
	return nil
}

// HasSection reports whether the section exists (even if empty).
func (f *File) HasSection(section string) bool { return len(f.findSections(section)) > 0 }

// Has reports whether the key exists in the section.
func (f *File) Has(section, key string) bool { return f.find(section, key) != nil }

// Get returns the value of a key and whether it exists.
func (f *File) Get(section, key string) (string, bool) {
	if l := f.find(section, key); l != nil {
		return l.value, true
	}
	return "", false
}

// GetDefault returns the value or def when the key is missing.
func (f *File) GetDefault(section, key, def string) string {
	if v, ok := f.Get(section, key); ok {
		return v
	}
	return def
}

// GetInt returns an integer value or def when it is missing or not a number.
// Like GetPrivateProfileInt, leading digits are used ("12abc" = 12).
func (f *File) GetInt(section, key string, def int) int {
	v, ok := f.Get(section, key)
	if !ok {
		return def
	}
	n, ok := parseLeadingInt(v)
	if !ok {
		return def
	}
	return n
}

// GetBool returns a boolean value: 1/true/yes/on are true, 0/false/no/off
// are false, anything else gives def.
func (f *File) GetBool(section, key string, def bool) bool {
	v, ok := f.Get(section, key)
	if !ok {
		return def
	}
	b, ok := parseBool(v)
	if !ok {
		return def
	}
	return b
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off", "":
		return false, true
	}
	if n, ok := parseLeadingInt(v); ok {
		return n != 0, true
	}
	return false, false
}

func parseLeadingInt(v string) (int, bool) {
	v = strings.TrimSpace(v)
	end := 0
	if end < len(v) && (v[end] == '-' || v[end] == '+') {
		end++
	}
	for end < len(v) && v[end] >= '0' && v[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(v[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Set stores a value. An existing key is changed in place (its original
// spelling of the key name is kept); a new key is appended to the first
// section of that name, which is created at the end of the file if needed.
func (f *File) Set(section, key, value string) {
	if l := f.find(section, key); l != nil {
		if l.value != value {
			l.value = value
			l.dirty = true
		}
		return
	}
	s := f.ensureSection(section)
	insertKV(s, &iniLine{kind: lineKV, key: key, value: value, dirty: true})
}

// insertKV adds a line after the last key/value line of the section, so the
// blank lines and comments that end a section stay in front of the next one.
func insertKV(s *iniSection, l *iniLine) {
	pos := 0
	for i, x := range s.lines {
		if x.kind == lineKV || x.kind == lineOther {
			pos = i + 1
		}
	}
	if pos == 0 {
		// Keep a leading comment block of the section in front of the keys.
		for pos < len(s.lines) && s.lines[pos].kind == lineComment {
			pos++
		}
	}
	s.lines = append(s.lines, nil)
	copy(s.lines[pos+1:], s.lines[pos:])
	s.lines[pos] = l
}

func (f *File) ensureSection(name string) *iniSection {
	if ss := f.findSections(name); len(ss) > 0 {
		return ss[0]
	}
	// Separate the new section from the previous content by a blank line.
	last := f.sections[len(f.sections)-1]
	if len(f.sections) > 1 || len(last.lines) > 0 {
		if n := len(last.lines); n == 0 || last.lines[n-1].kind != lineBlank {
			last.lines = append(last.lines, &iniLine{kind: lineBlank})
		}
	}
	s := &iniSection{name: name}
	f.sections = append(f.sections, s)
	return s
}

// EnsureSection creates an empty section if it doesn't exist yet.
func (f *File) EnsureSection(name string) { f.ensureSection(name) }

// SetInt stores an integer value.
func (f *File) SetInt(section, key string, v int) { f.Set(section, key, strconv.Itoa(v)) }

// SetBool stores a boolean as 1 or 0, the way Total Commander does.
func (f *File) SetBool(section, key string, v bool) {
	if v {
		f.Set(section, key, "1")
	} else {
		f.Set(section, key, "0")
	}
}

// Delete removes every occurrence of the key from the section and reports
// whether anything was removed.
func (f *File) Delete(section, key string) bool {
	removed := false
	for _, s := range f.findSections(section) {
		kept := s.lines[:0]
		for _, l := range s.lines {
			if l.kind == lineKV && strings.EqualFold(l.key, key) {
				removed = true
				continue
			}
			kept = append(kept, l)
		}
		s.lines = kept
	}
	return removed
}

// Sections lists section names in file order (duplicates listed once).
func (f *File) Sections() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range f.sections[1:] {
		k := strings.ToLower(s.name)
		if !seen[k] {
			seen[k] = true
			out = append(out, s.name)
		}
	}
	return out
}

// Entries returns the key/value pairs of a section in file order. With
// duplicated sections or keys, only the first occurrence of a key is listed.
func (f *File) Entries(section string) []KV {
	var out []KV
	seen := map[string]bool{}
	for _, s := range f.findSections(section) {
		for _, l := range s.lines {
			if l.kind != lineKV {
				continue
			}
			k := strings.ToLower(l.key)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, KV{l.key, l.value})
		}
	}
	return out
}

// Keys returns the key names of a section in file order.
func (f *File) Keys(section string) []string {
	var out []string
	for _, kv := range f.Entries(section) {
		out = append(out, kv.Key)
	}
	return out
}

// Map returns a section as a map with lower-cased keys, for quick lookups of
// numbered entries such as menu1/cmd1.
func (f *File) Map(section string) map[string]string {
	m := map[string]string{}
	for _, kv := range f.Entries(section) {
		m[strings.ToLower(kv.Key)] = kv.Value
	}
	return m
}

// DeleteSection removes every section with that name, including the lines
// inside it, and reports whether one existed.
func (f *File) DeleteSection(section string) bool {
	kept := f.sections[:1]
	removed := false
	for _, s := range f.sections[1:] {
		if strings.EqualFold(s.name, section) {
			removed = true
			continue
		}
		kept = append(kept, s)
	}
	f.sections = kept
	return removed
}

// RenameSection changes a section's name (all duplicates too).
func (f *File) RenameSection(from, to string) bool {
	ss := f.findSections(from)
	for _, s := range ss {
		s.name = to
	}
	return len(ss) > 0
}

// ReplaceSection swaps all key/value lines of a section for kvs while keeping
// the section's position and its comments; new lines go where the first old
// key was. List-like sections (histories, hotlist, buttons) are saved this way
// so a rewrite doesn't leave stale numbered keys behind. Duplicate sections
// after the first lose their keys.
func (f *File) ReplaceSection(section string, kvs []KV) {
	s := f.ensureSection(section)
	for _, d := range f.findSections(section)[1:] {
		kept := d.lines[:0]
		for _, l := range d.lines {
			if l.kind != lineKV {
				kept = append(kept, l)
			}
		}
		d.lines = kept
	}
	pos := -1
	kept := make([]*iniLine, 0, len(s.lines)+len(kvs))
	for _, l := range s.lines {
		if l.kind == lineKV {
			if pos < 0 {
				pos = len(kept)
			}
			continue
		}
		kept = append(kept, l)
	}
	newLines := make([]*iniLine, len(kvs))
	for i, kv := range kvs {
		newLines[i] = &iniLine{kind: lineKV, key: kv.Key, value: kv.Value, dirty: true}
	}
	if pos < 0 {
		// No keys before: put them after the leading comments, before the
		// trailing blank lines.
		pos = 0
		for pos < len(kept) && kept[pos].kind == lineComment {
			pos++
		}
		end := len(kept)
		for end > pos && kept[end-1].kind == lineBlank {
			end--
		}
		if end > pos {
			pos = end
		}
	}
	s.lines = append(kept[:pos:pos], append(newLines, kept[pos:]...)...)
}

// Clone returns a deep copy of the document.
func (f *File) Clone() *File {
	c := &File{Encoding: f.Encoding, Newline: f.Newline, finalNL: f.finalNL}
	for _, s := range f.sections {
		ns := &iniSection{name: s.name, header: s.header}
		for _, l := range s.lines {
			cp := *l
			ns.lines = append(ns.lines, &cp)
		}
		c.sections = append(c.sections, ns)
	}
	return c
}
