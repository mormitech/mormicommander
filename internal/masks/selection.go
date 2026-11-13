// This file is the selection engine behind a file panel's marking commands: toggling one entry (Ins/Space), ranges
// (Shift+cursor), select and unselect by mask (Num + / Num -, with and without folders), select/unselect all
// (Ctrl+Num + / Ctrl+Num -), invert (Num *, Shift+Num *), everything with the same extension or name as the entry
// under the cursor (Alt+Num + / Alt+Num -), files only or folders only, names selected in the other panel, restore
// the selection an operation cleared (Num /), and a memory slot for cm_SaveSelection / cm_LoadSelection. It keeps
// the selection by name rather than by row number, because the panel re-reads and re-sorts its folder all the time
// and Total Commander keeps marks across a refresh. The ".." entry can never be selected, as in TC. Selection holds
// no lock; the panel owns one Selection per tab and calls it from one goroutine.

package masks

import (
	"time"

	"golang.org/x/text/unicode/norm"
)

// Entry is one row of a panel as the selection engine sees it. Only Name and IsDir are needed for marking; Size and
// ModTime feed the status-bar totals, directory compare and clipboard details; Attr and Path are optional extras for
// the clipboard formats and content compare.
type Entry struct {
	Name    string    // name in the folder, never a path
	IsDir   bool      // a folder (or a link to one)
	Size    int64     // bytes; for folders the calculated size, or -1 when not calculated
	ModTime time.Time // last modification
	Attr    string    // attributes as the panel shows them, e.g. "-a--" or "rwxr-xr-x" (optional)
	Path    string    // full or root-relative path (optional; used by content compare and full-name copies)
}

// Kind picks which entries a command acts on.
type Kind uint8

const (
	KindFiles   Kind = iota // files only
	KindFolders             // folders only
	KindBoth                // files and folders
)

// accepts says whether an entry of this type is covered by k.
func (k Kind) accepts(isDir bool) bool {
	switch k {
	case KindFiles:
		return !isDir
	case KindFolders:
		return isDir
	}
	return true
}

// SelectionOptions are the user settings that shape the marking commands.
type SelectionOptions struct {
	Case CaseRule
	// FoldersToo mirrors TC's "Select folders too" option for Num +, Num -, Num * and Ctrl+Num +: when set, those
	// keys act on folders as well, and their Shift variants act on files only (and the other way round).
	FoldersToo bool
}

// Stats is what the status bar shows for the selection.
type Stats struct {
	Files, Folders int
	Bytes          int64 // files plus the folders whose size has been calculated
	FoldersSized   int   // how many selected folders have a known size
}

// Selection is the set of marked entries of one panel.
type Selection struct {
	opt        SelectionOptions
	cs         bool
	entries    []Entry
	sel        []bool
	remembered []string // for Restore (Num /)
	slot       []string // for SaveSlot / LoadSlot
}

// NewSelection creates an empty selection over entries.
func NewSelection(entries []Entry, opt SelectionOptions) *Selection {
	s := &Selection{opt: opt, cs: opt.Case.Sensitive()}
	s.SetEntries(entries)
	return s
}

// Options returns the settings in use.
func (s *Selection) Options() SelectionOptions { return s.opt }

// SetOptions changes the settings, e.g. after the user edits the configuration.
func (s *Selection) SetOptions(opt SelectionOptions) {
	s.opt, s.cs = opt, opt.Case.Sensitive()
}

// SetEntries replaces the listing (after a refresh, sort or folder change) and keeps the marks of names that are
// still there. Call Clear first when the panel moves to another folder.
func (s *Selection) SetEntries(entries []Entry) {
	keep := map[string]bool{}
	for i, on := range s.sel {
		if on {
			keep[s.entries[i].Name] = true
		}
	}
	s.entries = entries
	s.sel = make([]bool, len(entries))
	for i, e := range entries {
		s.sel[i] = keep[e.Name]
	}
}

// Entries returns the current listing (not a copy; do not modify it).
func (s *Selection) Entries() []Entry { return s.entries }

// Len returns the number of entries.
func (s *Selection) Len() int { return len(s.entries) }

// selectable says whether row i may carry a mark: ".." never does.
func (s *Selection) selectable(i int) bool {
	return i >= 0 && i < len(s.entries) && s.entries[i].Name != ".."
}

// set changes one mark and reports whether it changed.
func (s *Selection) set(i int, on bool) bool {
	if !s.selectable(i) || s.sel[i] == on {
		return false
	}
	s.sel[i] = on
	return true
}

// IsSelected says whether row i is marked.
func (s *Selection) IsSelected(i int) bool { return i >= 0 && i < len(s.sel) && s.sel[i] }

// Set marks or unmarks row i.
func (s *Selection) Set(i int, on bool) { s.set(i, on) }

// Toggle flips the mark of row i (Ins, Space, Ctrl+click) and returns the new state.
func (s *Selection) Toggle(i int) bool {
	if !s.selectable(i) {
		return false
	}
	s.sel[i] = !s.sel[i]
	return s.sel[i]
}

// SetRange marks or unmarks rows from..to inclusive, in either order (Shift+cursor, Shift+click, drag-select).
func (s *Selection) SetRange(from, to int, on bool) int {
	if from > to {
		from, to = to, from
	}
	n := 0
	for i := from; i <= to; i++ {
		if s.set(i, on) {
			n++
		}
	}
	return n
}

// Selected returns the marked row numbers in listing order.
func (s *Selection) Selected() []int {
	var out []int
	for i, on := range s.sel {
		if on {
			out = append(out, i)
		}
	}
	return out
}

// SelectedNames returns the marked names in listing order.
func (s *Selection) SelectedNames() []string {
	var out []string
	for i, on := range s.sel {
		if on {
			out = append(out, s.entries[i].Name)
		}
	}
	return out
}

// SelectedEntries returns the marked entries in listing order.
func (s *Selection) SelectedEntries() []Entry {
	var out []Entry
	for i, on := range s.sel {
		if on {
			out = append(out, s.entries[i])
		}
	}
	return out
}

// Count returns the number of marked entries.
func (s *Selection) Count() int {
	n := 0
	for _, on := range s.sel {
		if on {
			n++
		}
	}
	return n
}

// Stats returns counts and total size of the marked entries for the status bar.
func (s *Selection) Stats() Stats {
	var st Stats
	for i, on := range s.sel {
		if !on {
			continue
		}
		e := s.entries[i]
		if e.IsDir {
			st.Folders++
			if e.Size >= 0 {
				st.Bytes += e.Size
				st.FoldersSized++
			}
			continue
		}
		st.Files++
		st.Bytes += e.Size
	}
	return st
}

// GroupKind returns what Num +, Num -, Num * and Ctrl+Num + act on under the current options. shift is true for the
// Shift variants (cm_SelectBoth, cm_UnselectFiles, cm_ExchangeSelBoth, cm_SelectAllBoth), which swap the folder rule.
func (s *Selection) GroupKind(shift bool) Kind {
	if s.opt.FoldersToo != shift {
		return KindBoth
	}
	return KindFiles
}

// apply sets on for every row of kind k that pred accepts and returns how many marks changed.
func (s *Selection) apply(k Kind, on bool, pred func(e Entry) bool) int {
	n := 0
	for i, e := range s.entries {
		if k.accepts(e.IsDir) && (pred == nil || pred(e)) && s.set(i, on) {
			n++
		}
	}
	return n
}

// SelectGroup marks every entry of kind k that matches l (Num +, cm_SelectFiles; with KindBoth cm_SelectBoth).
func (s *Selection) SelectGroup(l *List, k Kind) int {
	return s.apply(k, true, func(e Entry) bool { return l.Match(e.Name, e.IsDir) })
}

// UnselectGroup unmarks every entry of kind k that matches l (Num -, cm_UnselectFiles).
func (s *Selection) UnselectGroup(l *List, k Kind) int {
	return s.apply(k, false, func(e Entry) bool { return l.Match(e.Name, e.IsDir) })
}

// SelectAll marks every entry of kind k (Ctrl+A, cm_SelectAll, cm_SelectAllBoth, cm_SelectAllFiles,
// cm_SelectAllFolders).
func (s *Selection) SelectAll(k Kind) int { return s.apply(k, true, nil) }

// ClearAll unmarks every entry of kind k (cm_ClearAll, cm_ClearFiles, cm_ClearFolders). When anything was marked,
// the old selection is remembered first so Restore (Num /) can bring it back.
func (s *Selection) ClearAll(k Kind) int {
	if s.Count() > 0 {
		s.Remember()
	}
	return s.apply(k, false, nil)
}

// Clear drops every mark without remembering them, for a folder change.
func (s *Selection) Clear() {
	for i := range s.sel {
		s.sel[i] = false
	}
}

// Invert flips the mark of every entry of kind k (Num *, cm_ExchangeSelection with KindFiles, cm_ExchangeSelBoth with
// KindBoth, cm_ExchangeSelFiles / cm_ExchangeSelFolders). Entries of other kinds keep their marks.
func (s *Selection) Invert(k Kind) int {
	n := 0
	for i, e := range s.entries {
		if k.accepts(e.IsDir) && s.selectable(i) {
			s.sel[i] = !s.sel[i]
			n++
		}
	}
	return n
}

// SelectSameExt marks (on) or unmarks every entry of kind k whose extension equals that of row cur (Alt+Num +,
// cm_SelectCurrentExtension; Alt+Num -, cm_UnselectCurrentExtension). A file without extension selects all files
// without extension. It returns how many marks changed; nothing happens on "..".
func (s *Selection) SelectSameExt(cur int, on bool, k Kind) int {
	if !s.selectable(cur) {
		return 0
	}
	_, ext := SplitExt(s.entries[cur].Name)
	return s.apply(k, on, func(e Entry) bool {
		_, x := SplitExt(e.Name)
		return sameName(x, ext, s.cs)
	})
}

// SelectSameName marks or unmarks every entry of kind k whose base name (without extension) equals that of row cur
// (cm_SelectCurrentName / cm_UnselectCurrentName).
func (s *Selection) SelectSameName(cur int, on bool, k Kind) int {
	if !s.selectable(cur) {
		return 0
	}
	base, _ := SplitExt(s.entries[cur].Name)
	return s.apply(k, on, func(e Entry) bool {
		b, _ := SplitExt(e.Name)
		return sameName(b, base, s.cs)
	})
}

// SelectSameNameExt marks or unmarks every entry of kind k whose full name equals that of row cur under the case
// rule (cm_SelectCurrentNameExt / cm_UnselectCurrentNameExt). On a case-sensitive system this also catches
// "Readme.txt" next to "README.TXT" when the rule is set to case-insensitive.
func (s *Selection) SelectSameNameExt(cur int, on bool, k Kind) int {
	if !s.selectable(cur) {
		return 0
	}
	name := s.entries[cur].Name
	return s.apply(k, on, func(e Entry) bool { return sameName(e.Name, name, s.cs) })
}

// SelectNames marks or unmarks entries whose names appear in names, compared under the case rule. It backs
// "select the same names as in the other panel" (cm_SelectFromOtherPanel) and Restore/LoadSlot.
func (s *Selection) SelectNames(names []string, on bool) int {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[s.key(n)] = true
	}
	return s.apply(KindBoth, on, func(e Entry) bool { return set[s.key(e.Name)] })
}

// key maps a name to the form used for set lookups under the case rule.
func (s *Selection) key(name string) string {
	name = norm.NFC.String(name)
	if s.cs {
		return name
	}
	return foldString(name)
}

// SetMarks replaces the whole selection with marks (one per row), as directory compare does. Rows beyond the slice
// are unmarked.
func (s *Selection) SetMarks(marks []bool) {
	for i := range s.sel {
		s.sel[i] = i < len(marks) && marks[i] && s.selectable(i)
	}
}

// Remember stores the current selection for Restore. The panel calls it before an operation (copy, move, delete…)
// clears the marks, so Num / (cm_RestoreSelection) can bring them back afterwards.
func (s *Selection) Remember() { s.remembered = s.SelectedNames() }

// Restore marks again every remembered name that is still in the listing (Num /) and returns how many marks were
// added. Existing marks stay.
func (s *Selection) Restore() int { return s.SelectNames(s.remembered, true) }

// SaveSlot stores the selection in the panel's memory slot (cm_SaveSelection).
func (s *Selection) SaveSlot() { s.slot = s.SelectedNames() }

// LoadSlot replaces the selection with the names in the memory slot that are still present (cm_LoadSelection).
func (s *Selection) LoadSlot() int {
	s.Clear()
	return s.SelectNames(s.slot, true)
}

// Slot returns the names in the memory slot, so the panel can carry it to the other panel or another folder.
func (s *Selection) Slot() []string { return append([]string(nil), s.slot...) }
