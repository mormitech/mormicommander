// This file implements column sets: TC's Brief, Full and Comments views and the user's custom column sets
// (Configuration > Custom columns). A column has a title, a content template with [=ns.field.unit] placeholders, a
// width and an alignment; a set is a named list of columns. Templates are parsed once into a compiled form, so
// rendering 100k rows does not re-parse strings, and each compiled column also knows how to produce a sort value, which
// is what makes "sort by custom column" and cm_SrcSortByColN work. Folder rules pick a set automatically for matching
// paths (for example a "Pictures" set with image dimensions for any folder named Pictures).

package listing

import (
	"fmt"
	"path"
	"strings"
)

// Align is a column's text alignment.
type Align int

const (
	AlignLeft Align = iota
	AlignRight
	AlignCenter
)

// Column is one column of a set, as stored in the configuration.
type Column struct {
	Title    string `json:"title"`
	Template string `json:"template"` // e.g. "[=tc.size.k]" or "[=tc.writedate.yyyy] / [=tc.attributestr]"
	Width    int    `json:"width"`    // pixels; 0 = auto
	Align    Align  `json:"align"`
}

// ColumnSet is a named list of columns.
type ColumnSet struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// templatePart is either literal text (field == nil) or a placeholder.
type templatePart struct {
	text  string
	field *Field
	name  string // "ns.name" as written, for error messages
	unit  string
}

// CompiledColumn is a parsed column ready to render.
type CompiledColumn struct {
	Column
	parts []templatePart
	// single is the index of the only placeholder when the template has exactly one, so the column sorts by the
	// field's typed value (sizes as numbers, dates as instants) even with literal text around it ("[=tc.size] bytes").
	single int
}

// CompiledSet is a parsed column set.
type CompiledSet struct {
	Name    string
	Columns []CompiledColumn
}

// ParseTemplate splits a template into literal text and placeholders. A '[' not followed by '=' is literal, so titles
// like "[x]" survive. Unknown fields are an error, so a typo is caught in the editor instead of rendering blanks.
func ParseTemplate(tpl string) ([]templatePart, error) {
	var parts []templatePart
	rest := tpl
	for {
		i := strings.Index(rest, "[=")
		if i < 0 {
			if rest != "" {
				parts = append(parts, templatePart{text: rest})
			}
			return parts, nil
		}
		if i > 0 {
			parts = append(parts, templatePart{text: rest[:i]})
		}
		end := strings.IndexByte(rest[i:], ']')
		if end < 0 {
			return nil, fmt.Errorf("listing: unterminated placeholder in %q", tpl)
		}
		spec := rest[i+2 : i+end]
		rest = rest[i+end+1:]
		// "ns.field.unit": the unit is everything after the second dot, so date pictures may contain dots.
		ns, after, ok := strings.Cut(spec, ".")
		if !ok {
			return nil, fmt.Errorf("listing: placeholder %q needs namespace.field", spec)
		}
		name, unit, _ := strings.Cut(after, ".")
		q := ns + "." + name
		fl, found := LookupField(q)
		if !found {
			return nil, fmt.Errorf("listing: unknown field %q", q)
		}
		f := fl
		parts = append(parts, templatePart{field: &f, name: q, unit: unit})
	}
}

// Compile parses every column of the set.
func (s ColumnSet) Compile() (*CompiledSet, error) {
	cs := &CompiledSet{Name: s.Name}
	for _, c := range s.Columns {
		cc, err := c.Compile()
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", c.Title, err)
		}
		cs.Columns = append(cs.Columns, *cc)
	}
	return cs, nil
}

// MustCompile is Compile for the built-in sets, which are known to be valid.
func (s ColumnSet) MustCompile() *CompiledSet {
	cs, err := s.Compile()
	if err != nil {
		panic(err)
	}
	return cs
}

// Compile parses one column.
func (c Column) Compile() (*CompiledColumn, error) {
	parts, err := ParseTemplate(c.Template)
	if err != nil {
		return nil, err
	}
	cc := &CompiledColumn{Column: c, parts: parts, single: -1}
	nFields, idx := 0, -1
	for i, p := range parts {
		if p.field != nil {
			nFields++
			idx = i
		}
	}
	if nFields == 1 {
		cc.single = idx
	}
	return cc, nil
}

// Render returns the column's text for an item.
func (c *CompiledColumn) Render(it *Item, f *Format) string {
	if len(c.parts) == 1 && c.parts[0].field != nil {
		p := c.parts[0]
		return p.field.Get(it, p.unit, f).Text
	}
	var b strings.Builder
	for _, p := range c.parts {
		if p.field == nil {
			b.WriteString(p.text)
		} else {
			b.WriteString(p.field.Get(it, p.unit, f).Text)
		}
	}
	return b.String()
}

// SortValue returns the value the column sorts by: the typed field value when the template has one placeholder,
// otherwise the rendered text.
func (c *CompiledColumn) SortValue(it *Item, f *Format) Value {
	if c.single >= 0 {
		p := c.parts[c.single]
		return p.field.Get(it, p.unit, f)
	}
	return TextValue(c.Render(it, f))
}

// Fields lists the placeholders the column uses, as "ns.name".
func (c *CompiledColumn) Fields() []string {
	var out []string
	for _, p := range c.parts {
		if p.field != nil {
			out = append(out, p.name)
		}
	}
	return out
}

// Render returns one string per column.
func (s *CompiledSet) Render(it *Item, f *Format) []string {
	out := make([]string, len(s.Columns))
	for i := range s.Columns {
		out[i] = s.Columns[i].Render(it, f)
	}
	return out
}

// Titles returns the header titles.
func (s *CompiledSet) Titles() []string {
	out := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		out[i] = c.Title
	}
	return out
}

// Built-in sets. Full view is TC's default five columns; Brief shows only the name (laid out in several columns by
// BriefLayout); Comments puts the descript.ion comment next to the name; FullSeparateExt is Full with the extension in
// its own column turned off (F-092: names shown whole).
var (
	BriefSet = ColumnSet{Name: "Brief", Columns: []Column{
		{Title: "Name", Template: "[=tc.fullname]"},
	}}
	FullSet = ColumnSet{Name: "Full", Columns: []Column{
		{Title: "Name", Template: "[=tc.name]"},
		{Title: "Ext", Template: "[=tc.ext]", Width: 50},
		{Title: "Size", Template: "[=tc.size]", Width: 90, Align: AlignRight},
		{Title: "Date", Template: "[=tc.writedate]", Width: 130},
		{Title: "Attr", Template: "[=tc.attributestr]", Width: 90},
	}}
	FullNameSet = ColumnSet{Name: "Full (names whole)", Columns: []Column{
		{Title: "Name", Template: "[=tc.fullname]"},
		{Title: "Size", Template: "[=tc.size]", Width: 90, Align: AlignRight},
		{Title: "Date", Template: "[=tc.writedate]", Width: 130},
		{Title: "Attr", Template: "[=tc.attributestr]", Width: 90},
	}}
	CommentsSet = ColumnSet{Name: "Comments", Columns: []Column{
		{Title: "Name", Template: "[=tc.fullname]"},
		{Title: "Comment", Template: "[=tc.comment]"},
	}}
)

// FolderRule switches to a column set when the panel shows a matching folder. Pattern is a path.Match pattern tried
// against the whole path and against the last path element, case-insensitively, so "Pictures" and "*/photos/*" both
// work.
type FolderRule struct {
	Pattern string `json:"pattern"`
	Set     string `json:"set"`
}

// ChooseSet returns the name of the set the first matching rule names, or fallback.
func ChooseSet(dir string, rules []FolderRule, fallback string) string {
	d := strings.ToLower(strings.ReplaceAll(dir, `\`, "/"))
	d = strings.TrimSuffix(d, "/")
	base := path.Base(d)
	for _, r := range rules {
		p := strings.ToLower(strings.ReplaceAll(r.Pattern, `\`, "/"))
		if ok, _ := path.Match(p, d); ok {
			return r.Set
		}
		if ok, _ := path.Match(p, base); ok {
			return r.Set
		}
	}
	return fallback
}
