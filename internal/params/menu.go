// The start menu (TC's "user menu") is stored as a flat numbered list in
// the [user] section of the ini file: menu1, cmd1, param1, path1, key1 and
// iconic1, then menu2 and so on. Submenus are encoded inside that list by
// the title alone: a title starting with "-" opens a submenu with the rest
// as its name, a title of exactly "--" closes the current submenu, and a
// lone "-" is a separator. BuildMenu turns the flat list into a tree for
// the UI and FlattenMenu turns it back, so the change-start-menu dialog can
// edit a normal tree while the file stays compatible with TC's format.
package params

import (
	"fmt"
	"strings"
)

// MenuEntry is one numbered line of the flat [user] list.
type MenuEntry struct {
	Title     string // menuN
	Command   string // cmdN
	Params    string // paramN
	StartPath string // pathN
	Key       string // keyN, e.g. "C+A+F1"
	Iconic    int    // iconicN
}

// MenuItem is one node of the start-menu tree.
type MenuItem struct {
	Title     string
	Separator bool
	Children  []MenuItem // non-nil for a submenu
	Button    Button
	Key       string
}

// IsSubmenu reports whether the item opens a submenu.
func (m MenuItem) IsSubmenu() bool { return m.Children != nil }

// BuildMenu converts the flat list into a tree. Unclosed submenus are
// closed at the end; a stray "--" at top level is an error.
func BuildMenu(flat []MenuEntry) ([]MenuItem, error) {
	type frame struct {
		item  MenuItem
		items []MenuItem
	}
	stack := []frame{{}}
	for i, e := range flat {
		top := &stack[len(stack)-1]
		switch {
		case e.Title == "--":
			if len(stack) == 1 {
				return nil, fmt.Errorf("params: menu entry %d closes a submenu that was never opened", i+1)
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			f.item.Children = append([]MenuItem{}, f.items...)
			p := &stack[len(stack)-1]
			p.items = append(p.items, f.item)
		case e.Title == "-":
			top.items = append(top.items, MenuItem{Separator: true})
		case strings.HasPrefix(e.Title, "-"):
			stack = append(stack, frame{item: MenuItem{Title: e.Title[1:], Key: e.Key}})
		default:
			top.items = append(top.items, MenuItem{Title: e.Title, Key: e.Key, Button: Button{
				Command: e.Command, Params: e.Params, StartPath: e.StartPath, Iconic: e.Iconic}})
		}
	}
	for len(stack) > 1 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		f.item.Children = append([]MenuItem{}, f.items...)
		p := &stack[len(stack)-1]
		p.items = append(p.items, f.item)
	}
	return stack[0].items, nil
}

// FlattenMenu converts a tree back to the flat list.
func FlattenMenu(items []MenuItem) []MenuEntry {
	var out []MenuEntry
	for _, it := range items {
		switch {
		case it.Separator:
			out = append(out, MenuEntry{Title: "-"})
		case it.IsSubmenu():
			out = append(out, MenuEntry{Title: "-" + it.Title, Key: it.Key})
			out = append(out, FlattenMenu(it.Children)...)
			out = append(out, MenuEntry{Title: "--"})
		default:
			out = append(out, MenuEntry{Title: it.Title, Key: it.Key, Command: it.Button.Command,
				Params: it.Button.Params, StartPath: it.Button.StartPath, Iconic: it.Button.Iconic})
		}
	}
	return out
}
