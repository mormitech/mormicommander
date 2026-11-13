// This file sorts a panel. Total Commander's rules are more particular than "sort by a field": ".." always stays on
// top, folders come before files, folders can be kept in name order whatever the file sort is ("sort folders by name
// always"), a folder without a calculated size sorts by name in size order, the date and size sorts start newest and
// largest first, clicking the same column again reverses it, and Ctrl+click adds secondary keys. SortSpec stores the
// keys and these switches, Toggle implements the click behaviour, and Sort applies it all with a stable sort so
// unsorted (file system) order is the final tie breaker.

package listing

import (
	"sort"
)

// SortBy names a sort field.
type SortBy int

const (
	ByName     SortBy = iota // name (cm_SrcByName, Ctrl+F3)
	ByExt                    // extension, then name (cm_SrcByExt, Ctrl+F4)
	ByDate                   // modification time (cm_SrcByDateTime, Ctrl+F5)
	BySize                   // size (cm_SrcBySize, Ctrl+F6)
	ByUnsorted               // file system order (cm_SrcUnsorted, Ctrl+F7)
	ByAttr                   // attribute string
	ByColumn                 // column N of the current column set (cm_SrcSortByColN, header click on a custom column)
)

// String returns the TC-style name of the sort field.
func (b SortBy) String() string {
	switch b {
	case ByName:
		return "name"
	case ByExt:
		return "ext"
	case ByDate:
		return "date"
	case BySize:
		return "size"
	case ByUnsorted:
		return "unsorted"
	case ByAttr:
		return "attr"
	case ByColumn:
		return "column"
	}
	return "?"
}

// DefaultDesc reports the direction a fresh sort on this field starts with: newest and largest first, like TC.
func (b SortBy) DefaultDesc() bool { return b == ByDate || b == BySize }

// SortKey is one sort criterion.
type SortKey struct {
	By     SortBy `json:"by"`
	Column int    `json:"column,omitempty"` // 0-based column index for ByColumn
	Desc   bool   `json:"desc,omitempty"`   // reversed relative to ascending
}

// SortSpec is a panel's complete sort state; each tab keeps its own (F-117), which is why it is plain data.
type SortSpec struct {
	Keys       []SortKey `json:"keys"`                 // primary first; empty means by name ascending
	DirsByName bool      `json:"dirsByName,omitempty"` // folders always sorted by name (F-098, cm_SwitchDirSort)
	MixDirs    bool      `json:"mixDirs,omitempty"`    // do not keep folders above files
}

// DefaultSort is TC's start-up sort: by name, ascending.
var DefaultSort = SortSpec{Keys: []SortKey{{By: ByName}}}

// Primary returns the main key, by name when there is none.
func (s SortSpec) Primary() SortKey {
	if len(s.Keys) == 0 {
		return SortKey{By: ByName}
	}
	return s.Keys[0]
}

func sameKey(a, b SortKey) bool { return a.By == b.By && (a.By != ByColumn || a.Column == b.Column) }

// Toggle applies a sort command or a header click. Choosing the current primary key reverses it; choosing another
// field makes it the only key in its default direction. With add (Ctrl+click on a header) the field is appended as a
// secondary key, or reversed if it is already one of the keys.
func (s SortSpec) Toggle(by SortBy, column int, add bool) SortSpec {
	k := SortKey{By: by, Column: column, Desc: by.DefaultDesc()}
	out := s
	out.Keys = append([]SortKey(nil), s.Keys...)
	if len(out.Keys) == 0 {
		out.Keys = []SortKey{{By: ByName}}
	}
	if add {
		for i := range out.Keys {
			if sameKey(out.Keys[i], k) {
				out.Keys[i].Desc = !out.Keys[i].Desc
				return out
			}
		}
		out.Keys = append(out.Keys, k)
		return out
	}
	if sameKey(out.Keys[0], k) {
		out.Keys = []SortKey{{By: by, Column: column, Desc: !out.Keys[0].Desc}}
		return out
	}
	out.Keys = []SortKey{k}
	return out
}

// Reverse flips the primary key (cm_SrcNegOrder).
func (s SortSpec) Reverse() SortSpec {
	out := s
	out.Keys = append([]SortKey(nil), s.Keys...)
	if len(out.Keys) == 0 {
		out.Keys = []SortKey{{By: ByName}}
	}
	out.Keys[0].Desc = !out.Keys[0].Desc
	return out
}

// SortOptions carries what the sorter needs besides the spec.
type SortOptions struct {
	Collator *Collator    // nil means case-insensitive code point order
	Format   *Format      // used to render attribute strings and custom columns; nil means defaults
	Set      *CompiledSet // the panel's column set, needed for ByColumn keys
}

// Sort orders items in place according to spec.
func Sort(items []Item, spec SortSpec, opt SortOptions) {
	idx := Order(items, spec, opt)
	tmp := make([]Item, len(items))
	for i, j := range idx {
		tmp[i] = items[j]
	}
	copy(items, tmp)
}

// Order returns the sorted order as indexes into items, leaving items untouched. A panel that keeps the raw listing
// and re-sorts on every sort command uses this to avoid copying large structs.
func Order(items []Item, spec SortSpec, opt SortOptions) []int {
	col := opt.Collator
	if col == nil {
		col = NewCollator(Collation{})
	}
	f := opt.Format
	if f == nil {
		f = &Format{}
	}
	keys := spec.Keys
	if len(keys) == 0 {
		keys = DefaultSort.Keys
	}

	// Precompute values that are costly to derive (attribute strings, rendered custom columns) once per item and key.
	cached := make([][]Value, len(keys))
	for ki, k := range keys {
		switch k.By {
		case ByAttr:
			vs := make([]Value, len(items))
			for i := range items {
				vs[i] = TextValue(AttrString(&items[i], f.AttrStyle))
			}
			cached[ki] = vs
		case ByColumn:
			if opt.Set == nil || k.Column < 0 || k.Column >= len(opt.Set.Columns) {
				continue
			}
			c := &opt.Set.Columns[k.Column]
			vs := make([]Value, len(items))
			for i := range items {
				vs[i] = c.SortValue(&items[i], f)
			}
			cached[ki] = vs
		}
	}
	exts := make([]string, len(items))
	for i := range items {
		_, exts[i] = SplitExtFor(&items[i], f.DirExt)
	}

	byName := func(i, j int) int {
		if c := col.Compare(items[i].Name, items[j].Name); c != 0 {
			return c
		}
		return runeCompare(items[i].Name, items[j].Name, true)
	}

	cmpKey := func(ki int, k SortKey, i, j int) int {
		a, b := &items[i], &items[j]
		switch k.By {
		case ByName:
			return byName(i, j)
		case ByExt:
			if c := col.Compare(exts[i], exts[j]); c != 0 {
				return c
			}
			return byName(i, j)
		case ByDate:
			return cmpTime(a.ModTime.UnixNano(), b.ModTime.UnixNano())
		case BySize:
			return cmpInt(sizeKey(a), sizeKey(b))
		case ByUnsorted:
			return cmpInt(int64(a.Order), int64(b.Order))
		case ByAttr, ByColumn:
			if cached[ki] == nil {
				return 0
			}
			return compareValues(cached[ki][i], cached[ki][j], col)
		}
		return 0
	}

	primaryName := keys[0].By == ByName
	primaryDesc := keys[0].Desc

	less := func(i, j int) bool {
		a, b := &items[i], &items[j]
		if a.IsUp() != b.IsUp() {
			return a.IsUp()
		}
		da, db := a.IsDir(), b.IsDir()
		if !spec.MixDirs && da != db {
			return da
		}
		if da && db && spec.DirsByName && !spec.MixDirs {
			c := byName(i, j)
			if primaryName && primaryDesc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
			return a.Order < b.Order
		}
		for ki, k := range keys {
			c := cmpKey(ki, k, i, j)
			if c == 0 {
				continue
			}
			// Empty custom-column values stay at the bottom in both directions.
			if k.By == ByColumn && cached[ki] != nil {
				ea, eb := cached[ki][i].Empty, cached[ki][j].Empty
				if ea != eb {
					return eb
				}
			}
			if k.Desc {
				c = -c
			}
			return c < 0
		}
		// Ties: by name in the primary direction when sorting by name or extension, else ascending; then raw order.
		c := byName(i, j)
		if (keys[0].By == ByName || keys[0].By == ByExt) && primaryDesc {
			c = -c
		}
		if c != 0 {
			return c < 0
		}
		return a.Order < b.Order
	}

	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool { return less(idx[x], idx[y]) })
	return idx
}

// sizeKey is the value a size sort uses: the file size, the computed folder size, or -1 for a folder that has not
// been calculated, so such folders group together and fall back to name order.
func sizeKey(it *Item) int64 {
	if it.IsDir() {
		return it.DirSize
	}
	return it.Size
}

func cmpTime(a, b int64) int { return cmpInt(a, b) }

// compareValues orders two field values of the same column.
func compareValues(a, b Value, col *Collator) int {
	if a.Empty != b.Empty {
		if a.Empty {
			return 1
		}
		return -1
	}
	switch {
	case a.Kind == KindNumber && b.Kind == KindNumber:
		return cmpInt(a.Num, b.Num)
	case a.Kind == KindTime && b.Kind == KindTime:
		return cmpInt(a.Time.UnixNano(), b.Time.UnixNano())
	}
	return col.Compare(a.Text, b.Text)
}
