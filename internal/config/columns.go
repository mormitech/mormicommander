// Custom column sets (section 32 of FEATURES.md). A column set is a named
// list of extra columns shown after the file name in Full view: each column
// has a header, a width, an alignment and a content template that mixes
// literal text with field placeholders such as "[=tc.size.K] kB" (F-666,
// F-667), and several fields may share one column. Total Commander stores the
// sets in [CustomFields] as numbered keys: HeadersN and ContentsN hold the
// columns separated by a literal "\n", WidthsN the widths separated by commas
// (the first one is the name column, a negative width means right-aligned),
// OptionsN is kept as is, and the set's name is TitleN. We also accept the
// older single "Titles=a|b" key when importing. AutoSwitchN (ours) is a mask
// of folders for which the set is chosen automatically (F-668). The built-in
// field list (F-665, F-674) lives here too so the column editor and the panel
// renderer share one definition.

package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SectionCustomFields is TC's column set section.
const SectionCustomFields = "CustomFields"

// Alignment of a column.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignRight
	AlignCenter
)

// Column is one custom column.
type Column struct {
	Header  string
	Content string // template like "[=tc.size] bytes"
	Width   int
	Align   Alignment
}

// ColumnSet is a named group of columns.
type ColumnSet struct {
	Name       string
	NameWidth  int // width of the always-present name column (0 = auto)
	Columns    []Column
	Options    string // TC OptionsN, kept unchanged
	AutoSwitch string // folder mask for automatic selection (ours)
}

// MatchFolder reports whether the set should be chosen automatically for a
// folder (mask matched against the folder name and full path).
func (cs ColumnSet) MatchFolder(path string) bool {
	if strings.TrimSpace(cs.AutoSwitch) == "" {
		return false
	}
	name := strings.TrimRight(path, `/\`)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return ParseMask(cs.AutoSwitch).Match(name, path, true)
}

// AutoColumnSet returns the index of the first set whose AutoSwitch mask
// matches folder, or -1.
func AutoColumnSet(sets []ColumnSet, folder string) int {
	for i, s := range sets {
		if s.MatchFolder(folder) {
			return i
		}
	}
	return -1
}

func splitBackslashN(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, `\n`)
}

// LoadColumnSets reads [CustomFields].
func LoadColumnSets(f *File) []ColumnSet {
	m := f.Map(SectionCustomFields)
	var oldTitles []string
	if t, ok := m["titles"]; ok {
		oldTitles = strings.Split(t, "|")
	}
	var out []ColumnSet
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		headers, hasH := m["headers"+n]
		contents, hasC := m["contents"+n]
		if !hasH && !hasC {
			break
		}
		cs := ColumnSet{Options: m["options"+n], AutoSwitch: m["autoswitch"+n]}
		cs.Name = m["title"+n]
		if cs.Name == "" {
			cs.Name = m["titles"+n]
		}
		if cs.Name == "" && i <= len(oldTitles) {
			cs.Name = oldTitles[i-1]
		}
		if cs.Name == "" {
			cs.Name = fmt.Sprintf("Columns %d", i)
		}
		hs, cn := splitBackslashN(headers), splitBackslashN(contents)
		count := max(len(hs), len(cn))
		var widths []int
		for _, w := range strings.Split(m["widths"+n], ",") {
			if v, err := strconv.Atoi(strings.TrimSpace(w)); err == nil {
				widths = append(widths, v)
			}
		}
		if len(widths) == count+1 {
			cs.NameWidth = abs(widths[0])
			widths = widths[1:]
		}
		for j := 0; j < count; j++ {
			c := Column{}
			if j < len(hs) {
				c.Header = hs[j]
			}
			if j < len(cn) {
				c.Content = cn[j]
			}
			if j < len(widths) {
				c.Width = abs(widths[j])
				if widths[j] < 0 {
					c.Align = AlignRight
				}
			}
			cs.Columns = append(cs.Columns, c)
		}
		out = append(out, cs)
	}
	applyAlign(f, out)
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SaveColumnSets writes all sets to [CustomFields], replacing old ones.
// Center alignment has no TC encoding; it is stored as left in WidthsN and
// remembered in AlignN (ours) so we read it back.
func SaveColumnSets(f *File, sets []ColumnSet) {
	var kvs []KV
	for i, cs := range sets {
		n := strconv.Itoa(i + 1)
		var hs, cn, ws, al []string
		ws = append(ws, strconv.Itoa(cs.NameWidth))
		centered := false
		for _, c := range cs.Columns {
			hs = append(hs, c.Header)
			cn = append(cn, c.Content)
			w := c.Width
			if c.Align == AlignRight {
				w = -w
			}
			ws = append(ws, strconv.Itoa(w))
			al = append(al, strconv.Itoa(int(c.Align)))
			if c.Align == AlignCenter {
				centered = true
			}
		}
		kvs = append(kvs,
			KV{"Title" + n, cs.Name},
			KV{"Widths" + n, strings.Join(ws, ",")},
			KV{"Headers" + n, strings.Join(hs, `\n`)},
			KV{"Contents" + n, strings.Join(cn, `\n`)},
		)
		if cs.Options != "" {
			kvs = append(kvs, KV{"Options" + n, cs.Options})
		}
		if cs.AutoSwitch != "" {
			kvs = append(kvs, KV{"AutoSwitch" + n, cs.AutoSwitch})
		}
		if centered {
			kvs = append(kvs, KV{"Align" + n, strings.Join(al, ",")})
		}
	}
	f.ReplaceSection(SectionCustomFields, kvs)
}

// applyAlign reads our AlignN extension back.
func applyAlign(f *File, sets []ColumnSet) {
	m := f.Map(SectionCustomFields)
	for i := range sets {
		v, ok := m["align"+strconv.Itoa(i+1)]
		if !ok {
			continue
		}
		for j, a := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(a)); err == nil && j < len(sets[i].Columns) && n >= 0 && n <= 2 {
				sets[i].Columns[j].Align = Alignment(n)
			}
		}
	}
}

// Segment is a piece of a column template: literal text or a field.
type Segment struct {
	Literal string // used when Field == ""
	Plugin  string // "tc" for built-in fields
	Field   string // field name, lower case
	Unit    string // optional unit or format ("K", "M", "D.M.Y")
}

// ParseTemplate splits "[=tc.size.K] kB" into segments. "[[" is a literal
// "[". Unclosed placeholders are kept as literal text.
func ParseTemplate(t string) []Segment {
	var out []Segment
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			out = append(out, Segment{Literal: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(t); i++ {
		if strings.HasPrefix(t[i:], "[[") {
			lit.WriteByte('[')
			i++
			continue
		}
		if strings.HasPrefix(t[i:], "[=") {
			if j := strings.IndexByte(t[i:], ']'); j > 0 {
				spec := t[i+2 : i+j]
				parts := strings.SplitN(spec, ".", 3)
				if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
					flush()
					s := Segment{Plugin: strings.ToLower(parts[0]), Field: strings.ToLower(parts[1])}
					if len(parts) == 3 {
						s.Unit = parts[2]
					}
					out = append(out, s)
					i += j
					continue
				}
			}
		}
		lit.WriteByte(t[i])
	}
	flush()
	return out
}

// FormatTemplate renders segments back to template text.
func FormatTemplate(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Field == "" {
			b.WriteString(strings.ReplaceAll(s.Literal, "[", "[["))
			continue
		}
		b.WriteString("[=" + s.Plugin + "." + s.Field)
		if s.Unit != "" {
			b.WriteString("." + s.Unit)
		}
		b.WriteString("]")
	}
	return b.String()
}

// RenderTemplate fills a template; value returns the text for a field
// segment (the caller knows the file).
func RenderTemplate(t string, value func(Segment) string) string {
	var b strings.Builder
	for _, s := range ParseTemplate(t) {
		if s.Field == "" {
			b.WriteString(s.Literal)
		} else {
			b.WriteString(value(s))
		}
	}
	return b.String()
}

// FieldDef describes a field offered in the column editor.
type FieldDef struct {
	Plugin string
	Name   string
	Title  string   // default column header
	Units  []string // allowed units/formats; empty = none
	Align  Alignment
	Kind   string // "text", "number", "size", "date"
}

// BuiltinFields lists the fields every panel can show.
func BuiltinFields() []FieldDef {
	size := []string{"bytes", "K", "M", "G", "T", "F"}
	date := []string{"D.M.Y h:m:s", "Y-M-D h:m", "Y-M-D", "h:m:s"}
	return []FieldDef{
		{"tc", "name", "Name", nil, AlignLeft, "text"},
		{"tc", "ext", "Ext", nil, AlignLeft, "text"},
		{"tc", "size", "Size", size, AlignRight, "size"},
		{"tc", "writedate", "Date", date, AlignLeft, "date"},
		{"tc", "creationdate", "Created", date, AlignLeft, "date"},
		{"tc", "accessdate", "Accessed", date, AlignLeft, "date"},
		{"tc", "attributestr", "Attr", nil, AlignLeft, "text"},
		{"tc", "path", "Path", nil, AlignLeft, "text"},
		{"tc", "fullname", "Full name", nil, AlignLeft, "text"},
		{"tc", "comment", "Comment", nil, AlignLeft, "text"},
		{"tc", "filetype", "Type", nil, AlignLeft, "text"},
		{"tc", "permissions", "Permissions", nil, AlignLeft, "text"},
		{"tc", "owner", "Owner", nil, AlignLeft, "text"},
		{"tc", "group", "Group", nil, AlignLeft, "text"},
		{"tc", "linktarget", "Link target", nil, AlignLeft, "text"},
		{"tc", "mime", "MIME type", nil, AlignLeft, "text"},
		{"tc", "imagewidth", "Width", nil, AlignRight, "number"},
		{"tc", "imageheight", "Height", nil, AlignRight, "number"},
		{"tc", "exifdate", "Taken", date, AlignLeft, "date"},
		{"tc", "artist", "Artist", nil, AlignLeft, "text"},
		{"tc", "title", "Title", nil, AlignLeft, "text"},
		{"tc", "album", "Album", nil, AlignLeft, "text"},
		{"tc", "duration", "Duration", nil, AlignRight, "number"},
	}
}

// LookupField finds a built-in field definition.
func LookupField(plugin, name string) (FieldDef, bool) {
	for _, d := range BuiltinFields() {
		if strings.EqualFold(d.Plugin, plugin) && strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return FieldDef{}, false
}
