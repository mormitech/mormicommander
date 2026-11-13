package main

import (
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The menu bar is the native one on every system, but its contents come from the page: the page knows the commands
// (frontend/src/core/menus.js lays out Total Commander's menus Files, Mark, Commands, Net, Show, Configuration, Start,
// Help with their cm_* commands) and sends the whole tree with SetMenu whenever it changes (a command is added, a check
// mark flips). Choosing an item sends the "menu" event with the cm_* name back to the page, which runs it exactly as the
// key or the button would. So a new feature never edits this file.
//
// Items carry no keyboard shortcuts: the page owns the keyboard (the user can change every key), and a shortcut in the
// menu would take the key away from it. The exceptions are the system's own: Cmd+Q, Cmd+H and the Edit menu on macOS
// (Cut, Copy, Paste in text fields).

// MenuItem is one entry of the menu tree the page sends.
type MenuItem struct {
	Label    string     `json:"label"`
	Cmd      string     `json:"cmd"`      // the cm_* command it runs
	Disabled bool       `json:"disabled"` // greyed out (a command that is not there yet, or can't run now)
	Checked  *bool      `json:"checked"`  // nil: no check mark at all; else the state of a toggle
	Sep      bool       `json:"sep"`      // a separator line
	Items    []MenuItem `json:"items"`    // a submenu
}

// SetMenu replaces the menu bar with the given tree (top-level items are the menus).
func (a *App) SetMenu(spec []MenuItem) {
	if a.ctx == nil || a.mode != "" {
		return
	}
	a.mu.Lock()
	a.menuBar = baseMenu(a, spec)
	m := a.menuBar
	a.mu.Unlock()
	wruntime.MenuSetApplicationMenu(a.ctx, m)
}

// baseMenu builds the menu bar: on macOS the application menu, Edit, the page's menus, Window and Help (the Help title
// ends in a zero-width space, or macOS adds its own Search field to it); elsewhere just the page's menus.
func baseMenu(a *App, spec []MenuItem) *menu.Menu {
	m := menu.NewMenu()
	var help *MenuItem
	var rest []MenuItem
	for i := range spec {
		if spec[i].Label == "Help" {
			help = &spec[i]
		} else {
			rest = append(rest, spec[i])
		}
	}
	if runtime.GOOS == "darwin" {
		app := m.AddSubmenu("mormiCommander")
		app.AddText("About mormiCommander", nil, func(*menu.CallbackData) { a.ShowAbout() })
		app.AddSeparator()
		app.AddText("Hide mormiCommander", keys.CmdOrCtrl("h"), func(*menu.CallbackData) { wruntime.Hide(a.ctx) })
		app.AddSeparator()
		app.AddText("Quit mormiCommander", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { a.emit("menu", "cm_Exit") })
		m.Append(menu.EditMenu())
		for _, it := range rest {
			addItems(a, m.AddSubmenu(it.Label), it.Items)
		}
		m.Append(menu.WindowMenu())
		if help != nil {
			addItems(a, m.AddSubmenu("Help​"), help.Items)
		}
		return m
	}
	for _, it := range rest {
		addItems(a, m.AddSubmenu(it.Label), it.Items)
	}
	if help != nil {
		addItems(a, m.AddSubmenu("Help"), help.Items)
	} else {
		m.AddSubmenu("Help").AddText("About mormiCommander", nil, func(*menu.CallbackData) { a.ShowAbout() })
	}
	return m
}

func addItems(a *App, m *menu.Menu, items []MenuItem) {
	for _, it := range items {
		switch {
		case it.Sep:
			m.AddSeparator()
		case len(it.Items) > 0:
			addItems(a, m.AddSubmenu(it.Label), it.Items)
		default:
			cmd := it.Cmd
			cb := func(*menu.CallbackData) { a.emit("menu", cmd) }
			var mi *menu.MenuItem
			if it.Checked != nil {
				mi = m.AddCheckbox(it.Label, *it.Checked, nil, cb)
			} else {
				mi = m.AddText(it.Label, nil, cb)
			}
			mi.Disabled = it.Disabled
		}
	}
}
