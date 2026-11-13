// The user Start menu (section 18 of FEATURES.md). In Total Commander the
// "Start" menu in the main menu bar holds the user's own entries: programs to
// run, internal cm_* commands, em_* user commands, with submenus and
// separators and an optional hotkey per entry (F-448, F-449). It is stored in
// the [user] section of wincmd.ini as a numbered menu (menu.go) with the
// fields cmdN, paramN (parameters, with the %-placeholders of params.go),
// pathN (start folder) and keyN (hotkey in TC notation, see keymap.go). The
// Change start menu dialog (F-450) edits the tree through the MenuInsert/
// MenuRemove/MenuMove helpers.

package config

import "strings"

// SectionUserMenu is TC's Start menu section.
const SectionUserMenu = "user"

var startMenuFields = []string{"cmd", "param", "path", "key"}

// StartMenu is the user's Start menu.
type StartMenu struct {
	Items []*MenuItem
}

// LoadStartMenu reads the [user] section.
func LoadStartMenu(f *File) *StartMenu {
	return &StartMenu{Items: ParseNumberedMenu(f.Map(SectionUserMenu), startMenuFields)}
}

// Save writes the menu back.
func (s *StartMenu) Save(f *File) {
	f.ReplaceSection(SectionUserMenu, NumberedMenuKVs(s.Items, startMenuFields))
}

// NewStartItem makes a Start menu entry.
func NewStartItem(title, cmd, params, startPath, hotkey string) *MenuItem {
	it := &MenuItem{Kind: MenuCommand, Title: title}
	it.SetField("cmd", cmd)
	it.SetField("param", params)
	it.SetField("path", startPath)
	it.SetField("key", hotkey)
	return it
}

// StartHotkey is an entry reachable by its own hotkey.
type StartHotkey struct {
	Key  Hotkey
	Path MenuPath
	Item *MenuItem
}

// Hotkeys lists the entries that have a parseable hotkey; invalid keys are
// returned separately as "path: key" strings.
func (s *StartMenu) Hotkeys() ([]StartHotkey, []string) {
	var out []StartHotkey
	var bad []string
	MenuWalk(s.Items, func(p MenuPath, _ []string, it *MenuItem) {
		k := strings.TrimSpace(it.Field("key"))
		if k == "" || it.Kind != MenuCommand {
			return
		}
		h, err := ParseHotkey(k)
		if err != nil {
			bad = append(bad, p.String()+": "+k)
			return
		}
		out = append(out, StartHotkey{Key: h, Path: p, Item: it})
	})
	return out, bad
}

// Action decodes an entry's command like a hotlist command; Args come from
// the param field when the command itself has none.
func (s *StartMenu) Action(it *MenuItem, v Vars) Action {
	a := ParseHotlistCommand(it.Field("cmd"))
	if p := it.Field("param"); p != "" {
		if a.Args != "" {
			a.Args += " " + p
		} else {
			a.Args = p
		}
	}
	a.Path = v.Expand(a.Path)
	if a.Kind == ActionProgram {
		a.Command = v.Expand(a.Command)
		a.Target = v.Expand(it.Field("path")) // start folder
	}
	return a
}
