// Folder tabs, panel state, saved tab sets and window position. Total
// Commander remembers the tabs of each panel in [lefttabs] and [righttabs]
// with keys "<n>_path", "<n>_caption" (a custom title), "<n>_options" (sort
// and view flags, kept as is) and activetab=<n>; we add "<n>_view" and
// "<n>_columnset" (the tab's own view, issue #128), which TC ignores; the state of the visible
// folder of each panel (path, sort column, view) lives in [left] and
// [right]. A tab set can also be saved to a .tab file whose [activetabs] and
// [inactivetabs] sections hold the tabs of the active and the other panel;
// the hotlist opens such a file with "cd name.tab" (F-664). Favourite tab sets
// (F-434) are named tab pairs we keep in "[FavoriteTab <name>]" sections.
// The window position is stored per screen resolution, in a section named
// like "[1920x1080 (8x16)]", so moving between a laptop screen and a monitor
// gives each its own window size (F-439); we write TC's exact section form.

package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Section names.
const (
	SectionLeft         = "left"
	SectionRight        = "right"
	SectionLeftTabs     = "lefttabs"
	SectionRightTabs    = "righttabs"
	SectionActiveTabs   = "activetabs"
	SectionInactiveTabs = "inactivetabs"
	favoriteTabPrefix   = "FavoriteTab "
)

// LockMode is the lock state of a tab.
type LockMode int

const (
	TabUnlocked       LockMode = iota
	TabLocked                  // folder can't be changed
	TabLockedChangeOK          // locked, but changing folder opens a new tab
)

// Tab is one folder tab.
type Tab struct {
	Path      string
	Caption   string // custom title ("" = folder name)
	Options   string // TC option flags, kept as is
	Lock      LockMode
	View      string // the tab's view ("brief", "full", "comments", "custom", "thumbs"; "" = the panel's)
	ColumnSet string // custom column set name when View is "custom"
}

// TabSet is the tabs of one panel.
type TabSet struct {
	Tabs   []Tab
	Active int
}

// ActiveTab returns the active tab (zero Tab if none).
func (t TabSet) ActiveTab() Tab {
	if t.Active >= 0 && t.Active < len(t.Tabs) {
		return t.Tabs[t.Active]
	}
	return Tab{}
}

func loadTabSet(m map[string]string, prefix string) TabSet {
	var ts TabSet
	prefix = strings.ToLower(prefix)
	for i := 0; ; i++ {
		n := prefix + strconv.Itoa(i) + "_"
		p, ok := m[n+"path"]
		if !ok {
			break
		}
		lock, _ := strconv.Atoi(m[n+"locked"])
		if lock < 0 || lock > 2 {
			lock = 0
		}
		ts.Tabs = append(ts.Tabs, Tab{Path: p, Caption: m[n+"caption"], Options: m[n+"options"], Lock: LockMode(lock),
			View: m[n+"view"], ColumnSet: m[n+"columnset"]})
	}
	ts.Active, _ = strconv.Atoi(m[prefix+"activetab"])
	if ts.Active < 0 || ts.Active >= len(ts.Tabs) {
		ts.Active = 0
	}
	return ts
}

func (t TabSet) kvs(prefix string) []KV {
	var kvs []KV
	for i, tab := range t.Tabs {
		n := prefix + strconv.Itoa(i) + "_"
		kvs = append(kvs, KV{n + "path", tab.Path})
		if tab.Caption != "" {
			kvs = append(kvs, KV{n + "caption", tab.Caption})
		}
		if tab.Options != "" {
			kvs = append(kvs, KV{n + "options", tab.Options})
		}
		if tab.Lock != TabUnlocked {
			kvs = append(kvs, KV{n + "locked", strconv.Itoa(int(tab.Lock))})
		}
		if tab.View != "" {
			kvs = append(kvs, KV{n + "view", tab.View})
		}
		if tab.ColumnSet != "" {
			kvs = append(kvs, KV{n + "columnset", tab.ColumnSet})
		}
	}
	return append(kvs, KV{prefix + "activetab", strconv.Itoa(t.Active)})
}

// LoadTabSet reads a tab section ([lefttabs], [righttabs], [activetabs]...).
func LoadTabSet(f *File, section string) TabSet { return loadTabSet(f.Map(section), "") }

// SaveTabSet writes a tab section.
func SaveTabSet(f *File, section string, t TabSet) { f.ReplaceSection(section, t.kvs("")) }

// TabFile is a saved .tab file.
type TabFile struct {
	Active   TabSet // tabs of the panel that was active
	Inactive TabSet // tabs of the other panel (may be empty)
}

// LoadTabFile reads a .tab file.
func LoadTabFile(path string) (TabFile, error) {
	f, err := LoadFile(path)
	if err != nil {
		return TabFile{}, err
	}
	return TabFile{Active: LoadTabSet(f, SectionActiveTabs), Inactive: LoadTabSet(f, SectionInactiveTabs)}, nil
}

// SaveTabFile writes a .tab file atomically.
func SaveTabFile(path string, t TabFile) error {
	f := NewFile()
	SaveTabSet(f, SectionActiveTabs, t.Active)
	if len(t.Inactive.Tabs) > 0 {
		SaveTabSet(f, SectionInactiveTabs, t.Inactive)
	}
	return f.Save(path)
}

// FavoriteTabs is a named pair of tab sets.
type FavoriteTabs struct {
	Name  string
	Left  TabSet
	Right TabSet
}

// LoadFavoriteTabs lists the saved favourite tab sets, sorted by name.
func LoadFavoriteTabs(f *File) []FavoriteTabs {
	var out []FavoriteTabs
	for _, sec := range f.Sections() {
		if len(sec) <= len(favoriteTabPrefix) || !strings.EqualFold(sec[:len(favoriteTabPrefix)], favoriteTabPrefix) {
			continue
		}
		m := f.Map(sec)
		out = append(out, FavoriteTabs{Name: sec[len(favoriteTabPrefix):], Left: loadTabSet(m, "L"), Right: loadTabSet(m, "R")})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// SaveFavoriteTabs stores (or replaces) one favourite set.
func SaveFavoriteTabs(f *File, fav FavoriteTabs) {
	f.ReplaceSection(favoriteTabPrefix+fav.Name, append(fav.Left.kvs("L"), fav.Right.kvs("R")...))
}

// DeleteFavoriteTabs removes a favourite set.
func DeleteFavoriteTabs(f *File, name string) bool { return f.DeleteSection(favoriteTabPrefix + name) }

// PanelState is the visible state of one panel ([left]/[right]).
type PanelState struct {
	Path       string
	View       string // "brief", "full", "thumbnails", "tree", "comments" or "custom"
	ColumnSet  string // custom column set name when View is "custom"
	SortColumn int    // TC sortorder: 0 name, 1 ext, 2 size, 3 date, ...
	SortDesc   bool   // TC negative
}

// LoadPanelState reads [left] or [right].
func LoadPanelState(f *File, section string) PanelState {
	return PanelState{
		Path:       f.GetDefault(section, "path", ""),
		View:       f.GetDefault(section, "View", "full"),
		ColumnSet:  f.GetDefault(section, "ColumnSet", ""),
		SortColumn: f.GetInt(section, "sortorder", 0),
		SortDesc:   f.GetBool(section, "negative", false),
	}
}

// SavePanelState writes [left] or [right], keeping other keys.
func SavePanelState(f *File, section string, p PanelState) {
	f.Set(section, "path", p.Path)
	f.Set(section, "View", p.View)
	if p.ColumnSet != "" || f.Has(section, "ColumnSet") {
		f.Set(section, "ColumnSet", p.ColumnSet)
	}
	f.SetInt(section, "sortorder", p.SortColumn)
	f.SetBool(section, "negative", p.SortDesc)
}

// WindowPos is the main window geometry for one screen size.
type WindowPos struct {
	X, Y, W, H int
	Divider    int // splitter position in per mille of the width (500 = middle)
	Maximized  bool
}

// ScreenSection returns TC's section name for a screen size.
func ScreenSection(screenW, screenH int) string {
	return fmt.Sprintf("%dx%d (8x16)", screenW, screenH)
}

// LoadWindowPos finds the saved position for a screen size. Any section
// named "<w>x<h>" with an optional suffix matches.
func LoadWindowPos(f *File, screenW, screenH int) (WindowPos, bool) {
	want := fmt.Sprintf("%dx%d", screenW, screenH)
	for _, sec := range f.Sections() {
		if sec != want && !strings.HasPrefix(sec, want+" ") {
			continue
		}
		if !f.Has(sec, "dx") {
			continue
		}
		return WindowPos{
			X: f.GetInt(sec, "x", 0), Y: f.GetInt(sec, "y", 0),
			W: f.GetInt(sec, "dx", 0), H: f.GetInt(sec, "dy", 0),
			Divider:   f.GetInt(sec, "Divider", 500),
			Maximized: f.GetBool(sec, "maximized", false),
		}, true
	}
	return WindowPos{}, false
}

// SaveWindowPos stores the position for a screen size (cm_ConfigSavePos).
func SaveWindowPos(f *File, screenW, screenH int, p WindowPos) {
	sec := ScreenSection(screenW, screenH)
	want := fmt.Sprintf("%dx%d", screenW, screenH)
	for _, s := range f.Sections() {
		if s == want || strings.HasPrefix(s, want+" ") {
			sec = s
			break
		}
	}
	f.SetInt(sec, "x", p.X)
	f.SetInt(sec, "y", p.Y)
	f.SetInt(sec, "dx", p.W)
	f.SetInt(sec, "dy", p.H)
	f.SetInt(sec, "Divider", p.Divider)
	f.SetBool(sec, "maximized", p.Maximized)
}
