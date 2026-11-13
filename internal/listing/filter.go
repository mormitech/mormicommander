// This file decides which entries a panel shows at all. Total Commander hides hidden and system files unless the user
// turns them on (we bind the toggle to Ctrl+H), can show only programs (Ctrl+F11) or only files matching a user
// filter (Ctrl+F12), keeps an ignore list that can be switched off, and puts a ".." line on top of every folder except
// a root. A user filter applies to files only by default, because hiding folders would make it impossible to navigate
// into them, which is TC's behaviour too. Wildcard matching itself lives in the masks engine; here a filter is just a
// predicate so any matcher can plug in.

package listing

// FilterMode is the panel filter chosen from the Show menu.
type FilterMode int

const (
	ShowAll      FilterMode = iota // cm_SrcAllFiles, Ctrl+F10
	ShowPrograms                   // cm_SrcExecs, Ctrl+F11
	ShowCustom                     // cm_SrcUserSpec, Ctrl+F12: Match decides
)

// FilterOptions controls which items are listed.
type FilterOptions struct {
	ShowHidden bool             // show hidden files and dotfiles (F-090)
	ShowSystem bool             // show Windows system files
	ShowUp     bool             // add ".." on top unless AtRoot (F-102)
	AtRoot     bool             // the folder is a root, so there is no ".."
	Dir        string           // the folder being listed, stored in the ".." item
	Mode       FilterMode       // all / programs / custom
	Match      func(*Item) bool // custom filter for ShowCustom; nil shows everything
	FilterDirs bool             // apply the custom filter to folders too
	Ignore     func(*Item) bool // ignore list: true hides the item
	IgnoreOff  bool             // cm_SwitchIgnoreList: show ignored items anyway
}

// Visible reports whether one item passes the filter. ".." is always visible here; Filter adds or drops it.
func (o *FilterOptions) Visible(it *Item) bool {
	if it.IsUp() {
		return true
	}
	if it.Name == "." {
		return false
	}
	if !o.ShowHidden && it.Attr&AttrHidden != 0 {
		return false
	}
	if !o.ShowSystem && it.Attr&AttrSystem != 0 {
		return false
	}
	if o.Ignore != nil && !o.IgnoreOff && o.Ignore(it) {
		return false
	}
	switch o.Mode {
	case ShowPrograms:
		if it.IsDir() {
			return true
		}
		return it.Attr&AttrExec != 0 || IsWindowsExecName(it.Name)
	case ShowCustom:
		if o.Match == nil {
			return true
		}
		if it.IsDir() && !o.FilterDirs {
			return true
		}
		return o.Match(it)
	}
	return true
}

// Active reports whether the filter hides anything beyond hidden/system files, so the UI can flag it in the path bar
// as TC does with "*.jpg" after the path.
func (o *FilterOptions) Active() bool {
	return o.Mode != ShowAll
}

// Filter returns the visible items, with a ".." entry first when ShowUp is set and the folder is not a root. Any ".."
// already in the input is dropped first, so sources may or may not provide one.
func Filter(items []Item, o FilterOptions) []Item {
	out := make([]Item, 0, len(items)+1)
	if o.ShowUp && !o.AtRoot {
		out = append(out, UpItem(o.Dir))
	}
	for i := range items {
		if items[i].IsUp() {
			continue
		}
		if o.Visible(&items[i]) {
			out = append(out, items[i])
		}
	}
	return out
}
