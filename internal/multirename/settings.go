// This file defines the settings of the Multi-Rename Tool and the Item type
// that describes one file in the list. Settings holds every field of the
// dialog that a preset should remember: the two masks, search and replace
// with its option boxes, the case drop-down, the counter fields and our
// extra switches. DefaultSettings gives the state of a freshly opened dialog
// and is also what the "Reset" button (F-518) restores. A Renamer is the
// compiled form of a Settings value; compiling once and reusing it keeps the
// live preview fast while the user types.

package multirename

import (
	"path/filepath"
	"strings"
	"time"
)

// Item is one entry of the rename list.
type Item struct {
	Dir     string            `json:"dir"`  // folder that contains the item
	Name    string            `json:"name"` // current name including extension
	IsDir   bool              `json:"isDir"`
	Size    int64             `json:"size"`
	ModTime time.Time         `json:"modTime"`
	Fields  map[string]string `json:"fields,omitempty"` // extra [=field] values, e.g. EXIF
}

// Path returns the item's full path.
func (it Item) Path() string { return filepath.Join(it.Dir, it.Name) }

// Settings is the full state of the dialog's input fields.
type Settings struct {
	NameMask string `json:"nameMask"`
	ExtMask  string `json:"extMask"`

	Search        string `json:"search"`
	Replace       string `json:"replace"`
	CaseSensitive bool   `json:"caseSensitive"` // the "^" box
	RegEx         bool   `json:"regex"`
	OnlyFirst     bool   `json:"onlyFirst"`  // replace only the first match
	IncludeExt    bool   `json:"includeExt"` // the "[E]" box: search the extension too

	Case CaseMode `json:"case"`

	CounterStart          int  `json:"counterStart"`
	CounterStep           int  `json:"counterStep"`
	CounterDigits         int  `json:"counterDigits"` // AutoDigits for automatic width
	CounterResetPerFolder bool `json:"counterResetPerFolder"`

	RemoveAccents bool `json:"removeAccents"`
	// SplitDirExt treats the part after the last dot of a folder name as its
	// extension. Off by default, because folder names such as "v1.2" or
	// "My.Project" rarely have a real extension and renaming them with a
	// "[N]_x" mask should not lose the tail.
	SplitDirExt bool `json:"splitDirExt"`

	// DateFormat and TimeFormat are Go time layouts for [d] and [t]. Total
	// Commander uses the system locale and replaces ':' with '.', which is
	// what the defaults imitate in a locale-independent way.
	DateFormat string `json:"dateFormat,omitempty"`
	TimeFormat string `json:"timeFormat,omitempty"`
}

// Default layouts for [d] and [t].
const (
	DefaultDateFormat = "2006-01-02"
	DefaultTimeFormat = "15.04.05"
)

// DefaultSettings returns the settings of a freshly opened dialog.
func DefaultSettings() Settings {
	return Settings{
		NameMask:      "[N]",
		ExtMask:       "[E]",
		CounterStart:  1,
		CounterStep:   1,
		CounterDigits: 1,
	}
}

// Renamer is a compiled Settings value.
type Renamer struct {
	settings Settings
	name     *Mask
	ext      *Mask
	repl     *replacer
	source   FieldSource // extra [=field] values; nil = only built-in fields
}

// SetFieldSource sets where [=field] values that are not built in come from
// (EXIF dates, image sizes, audio tags); nil turns that off again.
func (r *Renamer) SetFieldSource(f FieldSource) { r.source = f }

// SettingsError reports which input field is invalid, so the dialog can
// highlight it.
type SettingsError struct {
	Field string // "nameMask", "extMask" or "search"
	Err   error
}

func (e *SettingsError) Error() string { return e.Field + ": " + e.Err.Error() }
func (e *SettingsError) Unwrap() error { return e.Err }

// Compile validates the settings and prepares them for rendering.
func Compile(s Settings) (*Renamer, error) {
	nm, err := CompileMask(s.NameMask)
	if err != nil {
		return nil, &SettingsError{Field: "nameMask", Err: err}
	}
	em, err := CompileMask(s.ExtMask)
	if err != nil {
		return nil, &SettingsError{Field: "extMask", Err: err}
	}
	rp, err := newReplacer(s)
	if err != nil {
		return nil, &SettingsError{Field: "search", Err: err}
	}
	return &Renamer{settings: s, name: nm, ext: em, repl: rp}, nil
}

// Settings returns the settings the Renamer was compiled from.
func (r *Renamer) Settings() Settings { return r.settings }

// renderCtx carries everything a placeholder may need for one item.
type renderCtx struct {
	item     *Item
	settings *Settings
	name     string // old name without extension
	ext      string // old extension without the dot
	parent   string
	grand    string
	index    int // counter index within the counter group
	total    int // size of the counter group, for automatic width
	position int // position in the whole list
	listLen  int
	source   FieldSource
}

func (c *renderCtx) dateFormat() string {
	if c.settings.DateFormat != "" {
		return c.settings.DateFormat
	}
	return DefaultDateFormat
}

func (c *renderCtx) timeFormat() string {
	if c.settings.TimeFormat != "" {
		return c.settings.TimeFormat
	}
	return DefaultTimeFormat
}

// SplitExt splits a file name into name and extension at the last dot. A
// leading dot does not start an extension, so ".bashrc" is all name, which
// is what Unix users expect; a trailing dot gives an empty extension.
func SplitExt(name string) (base, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// joinExt is the inverse of SplitExt: an empty extension adds no dot.
func joinExt(base, ext string) string {
	if ext == "" {
		return base
	}
	return base + "." + ext
}

// counterGroups computes, for every item, its counter index and the size of
// its group. Without "reset per folder" the whole list is one group.
func counterGroups(items []Item, perFolder bool) (idx, total []int) {
	idx = make([]int, len(items))
	total = make([]int, len(items))
	if !perFolder {
		for i := range items {
			idx[i], total[i] = i, len(items)
		}
		return
	}
	seen := map[string]int{}
	for i, it := range items {
		k := filepath.Clean(it.Dir)
		idx[i] = seen[k]
		seen[k]++
	}
	for i, it := range items {
		total[i] = seen[filepath.Clean(it.Dir)]
	}
	return
}

// NewNames computes the new names for items in list order without checking
// for conflicts. Warnings per item are returned alongside (nil when none).
func (r *Renamer) NewNames(items []Item) ([]string, [][]string) {
	idx, total := counterGroups(items, r.settings.CounterResetPerFolder)
	names := make([]string, len(items))
	warns := make([][]string, len(items))
	for i := range items {
		names[i], warns[i] = r.newName(&items[i], idx[i], total[i], i, len(items))
	}
	return names, warns
}

// newName runs the whole pipeline for one item: masks, then search and
// replace, then the case drop-down, then accent removal. This is the order
// Total Commander uses (search and replace sees the masked name, case is
// applied last) with our accent step at the very end so it also cleans up
// replacement text.
func (r *Renamer) newName(it *Item, index, total, pos, n int) (string, []string) {
	s := &r.settings
	base, ext := it.Name, ""
	if !it.IsDir || s.SplitDirExt {
		base, ext = SplitExt(it.Name)
	}
	parent := filepath.Base(filepath.Clean(it.Dir))
	grand := filepath.Base(filepath.Dir(filepath.Clean(it.Dir)))
	if parent == "." || parent == string(filepath.Separator) {
		parent = ""
	}
	if grand == "." || grand == string(filepath.Separator) {
		grand = ""
	}
	ctx := &renderCtx{
		item: it, settings: s, name: base, ext: ext, parent: parent, grand: grand,
		index: index, total: total, position: pos, listLen: n, source: r.source,
	}
	newBase, w1 := r.name.render(ctx)
	newExt, w2 := r.ext.render(ctx)
	warns := append(w1, w2...)
	var full string
	if r.repl != nil {
		if s.IncludeExt {
			full = r.repl.apply(joinExt(newBase, newExt))
		} else {
			full = joinExt(r.repl.apply(newBase), newExt)
		}
	} else {
		full = joinExt(newBase, newExt)
	}
	full = ApplyCase(full, s.Case)
	if s.RemoveAccents {
		full = RemoveAccents(full)
	}
	return full, warns
}
