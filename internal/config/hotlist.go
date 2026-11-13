// The directory hotlist (Ctrl+D, section 31 of FEATURES.md). It is a tree of
// favourite folders with submenus and separators, stored in TC's [DirMenu]
// section as a numbered menu (menu.go): menuN is the title with an optional
// "&" hotkey letter, cmdN the command, and pathN an optional folder for the
// other panel, which TC calls the target path (F-653). The command is usually
// "cd <folder>", but may be any internal cm_* command, a user em_* command or
// a program to start (F-655). A "cd" to a .tab file opens a saved set of
// folder tabs (F-664), and a "cd" to "folder\*.txt" opens the folder with a
// filter. Folders may use %COMMANDER_PATH%, environment variables, $HOME and ~
// (F-849); Action expands them for the current platform. Filter implements
// type-to-search in the popup (F-658) and FindAccelerator the &letter keys.

package config

import (
	"path/filepath"
	"strings"
)

// SectionDirMenu is TC's hotlist section.
const SectionDirMenu = "DirMenu"

var hotlistFields = []string{"cmd", "path"}

// Hotlist is the directory hotlist tree. Items are MenuItems whose "cmd"
// field holds the command and "path" field the target folder.
type Hotlist struct {
	Items []*MenuItem
}

// LoadHotlist reads [DirMenu].
func LoadHotlist(f *File) *Hotlist {
	return &Hotlist{Items: ParseNumberedMenu(f.Map(SectionDirMenu), hotlistFields)}
}

// Save writes the hotlist back to [DirMenu], replacing the old entries.
func (h *Hotlist) Save(f *File) {
	f.ReplaceSection(SectionDirMenu, NumberedMenuKVs(h.Items, hotlistFields))
}

// NewHotlistEntry makes a "cd" entry; target may be "".
func NewHotlistEntry(title, folder, target string) *MenuItem {
	it := &MenuItem{Kind: MenuCommand, Title: title}
	it.SetField("cmd", "cd "+QuoteArg(folder))
	it.SetField("path", target)
	return it
}

// NewHotlistSubmenu makes an empty submenu.
func NewHotlistSubmenu(title string) *MenuItem {
	return &MenuItem{Kind: MenuSubmenu, Title: title}
}

// NewSeparator makes a separator line.
func NewSeparator() *MenuItem { return &MenuItem{Kind: MenuSeparator} }

// AddFolder appends a "cd" entry for folder at the top level (Ctrl+D, A:
// "Add current folder", F-652) and returns its path. The title defaults to
// the folder's last element. target is the other panel's folder for F-653
// (pass "" for a plain entry).
func (h *Hotlist) AddFolder(title, folder, target string) MenuPath {
	if title == "" {
		title = filepath.Base(strings.TrimRight(folder, `/\`))
		if title == "." || title == "" || title == "/" || title == `\` {
			title = folder
		}
	}
	p, _ := MenuInsert(&h.Items, nil, -1, NewHotlistEntry(title, folder, target))
	return p
}

// Contains reports whether some "cd" entry already opens folder.
func (h *Hotlist) Contains(folder string) bool {
	found := false
	MenuWalk(h.Items, func(_ MenuPath, _ []string, it *MenuItem) {
		if it.Kind == MenuCommand {
			if a := ParseHotlistCommand(it.Field("cmd")); a.Kind == ActionChangeDir && pathsEqual(a.Path, folder) {
				found = true
			}
		}
	})
	return found
}

// Insert, Remove and Move edit the tree (see MenuInsert/MenuRemove/MenuMove).
func (h *Hotlist) Insert(parent MenuPath, index int, it *MenuItem) (MenuPath, error) {
	return MenuInsert(&h.Items, parent, index, it)
}

// Remove deletes an entry (a submenu with everything in it).
func (h *Hotlist) Remove(p MenuPath) (*MenuItem, error) { return MenuRemove(&h.Items, p) }

// Move moves an entry to another place.
func (h *Hotlist) Move(from, toParent MenuPath, index int) (MenuPath, error) {
	return MenuMove(&h.Items, from, toParent, index)
}

// Get returns the entry at a path.
func (h *Hotlist) Get(p MenuPath) (*MenuItem, error) { return MenuGet(h.Items, p) }

// HotlistMatch is one result of Filter.
type HotlistMatch struct {
	Path       MenuPath
	Breadcrumb string // parent submenus joined with " > "
	Item       *MenuItem
}

// Filter returns the command entries whose title (without "&"), command or
// target contains every word of query, ignoring case (F-658). An empty query
// returns all command entries.
func (h *Hotlist) Filter(query string) []HotlistMatch {
	words := strings.Fields(strings.ToLower(query))
	var out []HotlistMatch
	MenuWalk(h.Items, func(p MenuPath, parents []string, it *MenuItem) {
		if it.Kind != MenuCommand {
			return
		}
		hay := strings.ToLower(StripAccelerator(it.Title) + " " + it.Field("cmd") + " " + it.Field("path"))
		for _, w := range words {
			if !strings.Contains(hay, w) {
				return
			}
		}
		crumbs := make([]string, len(parents))
		for i, c := range parents {
			crumbs[i] = StripAccelerator(c)
		}
		out = append(out, HotlistMatch{Path: p, Breadcrumb: strings.Join(crumbs, " > "), Item: it})
	})
	return out
}

// ActionKind says what running a hotlist (or Start menu) command does.
type ActionKind int

const (
	ActionNone      ActionKind = iota
	ActionChangeDir            // go to Path (optionally Filter), Target in the other panel
	ActionTabSet               // load a saved tab set from Path (.tab file)
	ActionInternal             // run Command (cm_*) with Args
	ActionUser                 // run user command Command (em_*) with Args
	ActionProgram              // start program Command with Args
)

// Action is a decoded command line.
type Action struct {
	Kind    ActionKind
	Path    string // folder or .tab file (ActionChangeDir, ActionTabSet)
	Filter  string // file mask after the folder ("c:\x\*.txt" -> "*.txt")
	Target  string // other panel folder (hotlist pathN)
	Command string // cm_/em_ name or program
	Args    string // remaining arguments, unparsed
}

// ParseHotlistCommand decodes a TC command such as "cd c:\data",
// "cm_SrcThumbs", "em_backup now" or "notepad.exe %P%N". Variables are not
// expanded; see Expand.
func ParseHotlistCommand(cmd string) Action {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return Action{}
	}
	head, rest := SplitCommand(cmd)
	low := strings.ToLower(head)
	switch {
	case low == "cd":
		p := UnquoteArg(strings.TrimSpace(rest))
		if strings.HasSuffix(strings.ToLower(p), ".tab") {
			return Action{Kind: ActionTabSet, Path: p}
		}
		dir, filter := splitFilter(p)
		return Action{Kind: ActionChangeDir, Path: dir, Filter: filter}
	case strings.HasPrefix(low, "cm_"):
		return Action{Kind: ActionInternal, Command: head, Args: rest}
	case strings.HasPrefix(low, "em_"):
		return Action{Kind: ActionUser, Command: head, Args: rest}
	}
	return Action{Kind: ActionProgram, Command: head, Args: rest}
}

// splitFilter separates a trailing wildcard mask from a folder.
func splitFilter(p string) (string, string) {
	i := strings.LastIndexAny(p, `/\`)
	last := p[i+1:]
	if strings.ContainsAny(last, "*?") {
		dir := p[:i+1]
		if len(dir) > 1 {
			dir = strings.TrimRight(dir, `/\`)
			if strings.HasSuffix(dir, ":") { // keep "c:\"
				dir += `\`
			}
		}
		return dir, last
	}
	return p, ""
}

// Action decodes an entry's command and target path and expands variables.
func (h *Hotlist) Action(it *MenuItem, v Vars) Action {
	a := ParseHotlistCommand(it.Field("cmd"))
	a.Path = v.Expand(a.Path)
	a.Target = v.Expand(it.Field("path"))
	if a.Kind == ActionProgram {
		a.Command = v.Expand(a.Command)
	}
	return a
}

// SplitCommand splits a command line into the first word (quotes removed)
// and the rest (trimmed, unchanged).
func SplitCommand(s string) (string, string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if j := strings.Index(s[1:], `"`); j >= 0 {
			return s[1 : j+1], strings.TrimSpace(s[j+2:])
		}
		return s[1:], ""
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// QuoteArg puts double quotes around s if it contains a space.
func QuoteArg(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, `"`) {
		return `"` + s + `"`
	}
	return s
}

// UnquoteArg removes one pair of surrounding double quotes.
func UnquoteArg(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
