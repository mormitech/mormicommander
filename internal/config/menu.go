// Numbered menus, the storage format Total Commander uses for the directory
// hotlist ([DirMenu]) and the user Start menu ([user]). Each entry is a group
// of keys sharing a number: menu1=<title>, cmd1=<command>, and further fields
// such as path1, param1 or key1. Structure is encoded in the titles: a title
// starting with "-" opens a submenu ("-Projects"), a title of exactly "-" is a
// separator, and "--" closes the current submenu. Numbers run from 1 without
// gaps; TC stops reading at the first missing number, and so do we. This file
// turns that flat list into a tree and back, so the hotlist and Start menu
// code only deals with nested items. Unbalanced input is repaired: a missing
// "--" closes the submenu at the end, an extra one at the top level is
// ignored.

package config

import (
	"fmt"
	"strconv"
	"strings"
)

// MenuKind says what a menu item is.
type MenuKind int

const (
	MenuCommand   MenuKind = iota // a normal entry that runs a command
	MenuSubmenu                   // opens Children
	MenuSeparator                 // a line between entries
)

// MenuItem is one node of a numbered menu tree.
type MenuItem struct {
	Kind     MenuKind
	Title    string            // without the leading "-" of a submenu
	Fields   map[string]string // lower-case field name (cmd, path, param, key...) -> value
	Children []*MenuItem
}

// Field returns a field value ("" if missing).
func (m *MenuItem) Field(name string) string {
	if m.Fields == nil {
		return ""
	}
	return m.Fields[strings.ToLower(name)]
}

// SetField sets or (with "") removes a field.
func (m *MenuItem) SetField(name, value string) {
	name = strings.ToLower(name)
	if value == "" {
		delete(m.Fields, name)
		return
	}
	if m.Fields == nil {
		m.Fields = map[string]string{}
	}
	m.Fields[name] = value
}

// Clone returns a deep copy.
func (m *MenuItem) Clone() *MenuItem {
	c := &MenuItem{Kind: m.Kind, Title: m.Title}
	if m.Fields != nil {
		c.Fields = map[string]string{}
		for k, v := range m.Fields {
			c.Fields[k] = v
		}
	}
	for _, ch := range m.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return c
}

// ParseNumberedMenu builds a tree from a section's lower-cased map (see
// File.Map). fields lists the per-entry fields besides "menu" to collect.
func ParseNumberedMenu(m map[string]string, fields []string) []*MenuItem {
	var root []*MenuItem
	stack := []*[]*MenuItem{&root}
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		title, ok := m["menu"+n]
		if !ok {
			break
		}
		cur := stack[len(stack)-1]
		switch {
		case title == "--":
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case title == "-":
			*cur = append(*cur, &MenuItem{Kind: MenuSeparator})
		case strings.HasPrefix(title, "-"):
			it := &MenuItem{Kind: MenuSubmenu, Title: title[1:]}
			collectFields(it, m, n, fields)
			*cur = append(*cur, it)
			stack = append(stack, &it.Children)
		default:
			it := &MenuItem{Kind: MenuCommand, Title: title}
			collectFields(it, m, n, fields)
			*cur = append(*cur, it)
		}
	}
	return root
}

func collectFields(it *MenuItem, m map[string]string, n string, fields []string) {
	for _, f := range fields {
		if v, ok := m[strings.ToLower(f)+n]; ok && v != "" {
			it.SetField(f, v)
		}
	}
}

// NumberedMenuKVs flattens a tree back into numbered keys. Field names are
// written in the given order and spelling (TC writes lower case).
func NumberedMenuKVs(items []*MenuItem, fields []string) []KV {
	var kvs []KV
	n := 0
	var walk func([]*MenuItem)
	walk = func(list []*MenuItem) {
		for _, it := range list {
			n++
			num := strconv.Itoa(n)
			switch it.Kind {
			case MenuSeparator:
				kvs = append(kvs, KV{"menu" + num, "-"})
			case MenuSubmenu:
				kvs = append(kvs, KV{"menu" + num, "-" + it.Title})
				kvs = appendFields(kvs, it, num, fields)
				walk(it.Children)
				n++
				kvs = append(kvs, KV{"menu" + strconv.Itoa(n), "--"})
			default:
				title := it.Title
				if strings.HasPrefix(title, "-") {
					// A title starting with "-" would read back as a
					// submenu, separator or submenu end (TC has the same
					// limit), so the first hyphen becomes U+2010, which
					// looks the same.
					title = "‐" + title[1:]
				}
				kvs = append(kvs, KV{"menu" + num, title})
				kvs = appendFields(kvs, it, num, fields)
			}
		}
	}
	walk(items)
	return kvs
}

func appendFields(kvs []KV, it *MenuItem, num string, fields []string) []KV {
	for _, f := range fields {
		if v := it.Field(f); v != "" {
			kvs = append(kvs, KV{strings.ToLower(f) + num, v})
		}
	}
	return kvs
}

// MenuPath addresses an item in a tree: indexes from the top level down.
type MenuPath []int

// String renders a path like "2/0/3".
func (p MenuPath) String() string {
	parts := make([]string, len(p))
	for i, x := range p {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, "/")
}

// ParseMenuPath parses the String form.
func ParseMenuPath(s string) (MenuPath, error) {
	if s == "" {
		return MenuPath{}, nil
	}
	var p MenuPath
	for _, part := range strings.Split(s, "/") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("config: bad menu path %q", s)
		}
		p = append(p, n)
	}
	return p, nil
}

// menuList returns a pointer to the child list addressed by parent (empty =
// top level). The item at parent must be a submenu.
func menuList(root *[]*MenuItem, parent MenuPath) (*[]*MenuItem, error) {
	list := root
	for _, i := range parent {
		if i < 0 || i >= len(*list) {
			return nil, fmt.Errorf("config: menu index %d out of range", i)
		}
		it := (*list)[i]
		if it.Kind != MenuSubmenu {
			return nil, fmt.Errorf("config: menu item %d is not a submenu", i)
		}
		list = &it.Children
	}
	return list, nil
}

// MenuGet returns the item at path.
func MenuGet(root []*MenuItem, p MenuPath) (*MenuItem, error) {
	if len(p) == 0 {
		return nil, fmt.Errorf("config: empty menu path")
	}
	list, err := menuList(&root, p[:len(p)-1])
	if err != nil {
		return nil, err
	}
	i := p[len(p)-1]
	if i < 0 || i >= len(*list) {
		return nil, fmt.Errorf("config: menu index %d out of range", i)
	}
	return (*list)[i], nil
}

// MenuInsert inserts it into the submenu at parent at index (clamped; -1 =
// append) and returns the new item's path.
func MenuInsert(root *[]*MenuItem, parent MenuPath, index int, it *MenuItem) (MenuPath, error) {
	list, err := menuList(root, parent)
	if err != nil {
		return nil, err
	}
	if index < 0 || index > len(*list) {
		index = len(*list)
	}
	*list = append(*list, nil)
	copy((*list)[index+1:], (*list)[index:])
	(*list)[index] = it
	return append(append(MenuPath{}, parent...), index), nil
}

// MenuRemove removes and returns the item at path.
func MenuRemove(root *[]*MenuItem, p MenuPath) (*MenuItem, error) {
	if len(p) == 0 {
		return nil, fmt.Errorf("config: empty menu path")
	}
	list, err := menuList(root, p[:len(p)-1])
	if err != nil {
		return nil, err
	}
	i := p[len(p)-1]
	if i < 0 || i >= len(*list) {
		return nil, fmt.Errorf("config: menu index %d out of range", i)
	}
	it := (*list)[i]
	*list = append((*list)[:i], (*list)[i+1:]...)
	return it, nil
}

// MenuMove moves the item at from into the submenu toParent at index. Moving
// a submenu into itself is refused. Indexes in toParent refer to the tree
// after the item was taken out.
func MenuMove(root *[]*MenuItem, from, toParent MenuPath, index int) (MenuPath, error) {
	if len(toParent) >= len(from) {
		same := true
		for i := range from {
			if from[i] != toParent[i] {
				same = false
				break
			}
		}
		if same {
			return nil, fmt.Errorf("config: can't move a submenu into itself")
		}
	}
	it, err := MenuRemove(root, from)
	if err != nil {
		return nil, err
	}
	// Taking the item out shifts later siblings on the way to toParent.
	adj := append(MenuPath{}, toParent...)
	if len(from) <= len(adj) {
		k := len(from) - 1
		samePrefix := true
		for i := 0; i < k; i++ {
			if from[i] != adj[i] {
				samePrefix = false
				break
			}
		}
		if samePrefix && adj[k] > from[k] {
			adj[k]--
		}
	}
	p, err := MenuInsert(root, adj, index, it)
	if err != nil {
		// Put it back where it was so a failed move changes nothing.
		MenuInsert(root, from[:len(from)-1], from[len(from)-1], it)
		return nil, err
	}
	return p, nil
}

// MenuWalk calls fn for every item, depth first, with its path and the
// titles of its parent submenus.
func MenuWalk(root []*MenuItem, fn func(p MenuPath, parents []string, it *MenuItem)) {
	var walk func(list []*MenuItem, p MenuPath, parents []string)
	walk = func(list []*MenuItem, p MenuPath, parents []string) {
		for i, it := range list {
			ip := append(append(MenuPath{}, p...), i)
			fn(ip, parents, it)
			if it.Kind == MenuSubmenu {
				walk(it.Children, ip, append(append([]string{}, parents...), it.Title))
			}
		}
	}
	walk(root, nil, nil)
}

// Accelerator returns the lower-case letter after the first single "&" in a
// title (TC hotkey letters, F-656), or 0. "&&" is a literal ampersand.
func Accelerator(title string) rune {
	rs := []rune(title)
	for i := 0; i < len(rs)-1; i++ {
		if rs[i] == '&' {
			if rs[i+1] == '&' {
				i++
				continue
			}
			return toLowerRune(rs[i+1])
		}
	}
	return 0
}

// StripAccelerator removes accelerator markers ("&Home" -> "Home",
// "A && B" -> "A & B").
func StripAccelerator(title string) string {
	var b strings.Builder
	rs := []rune(title)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '&' && i+1 < len(rs) {
			i++
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

func toLowerRune(r rune) rune {
	return []rune(strings.ToLower(string(r)))[0]
}

// FindAccelerator returns the index of the first command or submenu in list
// whose accelerator is r, or -1. Without an explicit "&", TC falls back to
// the first letter of the title; so do we.
func FindAccelerator(list []*MenuItem, r rune) int {
	r = toLowerRune(r)
	for i, it := range list {
		if it.Kind != MenuSeparator && Accelerator(it.Title) == r {
			return i
		}
	}
	for i, it := range list {
		if it.Kind == MenuSeparator || Accelerator(it.Title) != 0 {
			continue
		}
		t := []rune(StripAccelerator(strings.TrimSpace(it.Title)))
		if len(t) > 0 && toLowerRune(t[0]) == r {
			return i
		}
	}
	return -1
}
