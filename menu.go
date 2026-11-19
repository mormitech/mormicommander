package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
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
//
// With Wails v3 the menu bar is the application's on macOS and the main window's own on Windows and Linux, so SetMenu
// puts the new tree in both places.

// MenuItem is one entry of the menu tree the page sends.
type MenuItem struct {
	Label    string     `json:"label"`
	Cmd      string     `json:"cmd"`      // the cm_* command it runs
	Disabled bool       `json:"disabled"` // greyed out (a command that is not there yet, or can't run now)
	Checked  *bool      `json:"checked"`  // nil: no check mark at all; else the state of a toggle
	Sep      bool       `json:"sep"`      // a separator line
	Items    []MenuItem `json:"items"`    // a submenu
	Icon     string     `json:"icon"`     // an image file drawn before the label (Start menu entries, F-450); "" = none
	Key      string     `json:"key"`      // a top-level menu's English name when its label is translated (#113), else ""
}

// SetMenuWords gives the translations of the menu words the program itself writes (issue #113): the macOS application
// menu, the system's Edit and Window menus, Help. The page sends them with the language file (English → translation;
// empty for English) and then the menu tree again, so the next SetMenu uses them.
func (a *App) SetMenuWords(words map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(words) == 0 {
		a.menuWords = nil
		return
	}
	a.menuWords = map[string]string{}
	for k, v := range words {
		if k != "" && v != "" {
			a.menuWords[k] = v
		}
	}
}

// menuWord is the translation of one of the program's own menu words, or the word itself.
func (a *App) menuWord(s string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if w, ok := a.menuWords[s]; ok {
		return w
	}
	return s
}

// translateRoleMenu writes the labels of a system role menu (Edit, Window) and its items in the interface language;
// the items keep their roles, so they work as before. Without translations it changes nothing.
func (a *App) translateRoleMenu(it *application.MenuItem) {
	if it == nil {
		return
	}
	if w := a.menuWord(it.Label()); w != it.Label() {
		it.SetLabel(w)
	}
	if sub := it.GetSubmenu(); sub != nil {
		for _, x := range menuItems(sub) {
			a.translateRoleMenu(x)
		}
	}
}

// SetMenu replaces the menu bar with the given tree (top-level items are the menus).
func (a *App) SetMenu(spec []MenuItem) {
	if a.wapp == nil {
		return
	}
	m := baseMenu(a, spec)
	a.mu.Lock()
	a.menuBar = m
	a.mu.Unlock()
	a.applyMenu(m)
}

// applyMenu shows the menu bar: the application's menu on macOS (set on the main thread, as AppKit wants), the main
// window's menu elsewhere.
func (a *App) applyMenu(m *application.Menu) {
	if runtime.GOOS == "darwin" {
		application.InvokeSync(func() { a.wapp.Menu.Set(m) })
		return
	}
	a.wapp.Menu.Set(m)
	if w := a.mainWindow(); w != nil {
		w.SetMenu(m)
	}
}

// SetMenuBar shows or hides the main window's menu bar (Options > Layout > Show main menu bar). On macOS the menu bar
// belongs to the system and always stays; there it does nothing.
func (a *App) SetMenuBar(show bool) {
	if runtime.GOOS == "darwin" || a.wapp == nil {
		return
	}
	if w := a.mainWindow(); w != nil {
		if show {
			w.ShowMenuBar()
		} else {
			w.HideMenuBar()
		}
	}
}

// menuItems is the items of a menu, for the control mode's "menus" and the tests.
func menuItems(m *application.Menu) []*application.MenuItem {
	var out []*application.MenuItem
	for i := 0; m != nil; i++ {
		it := m.ItemAt(i)
		if it == nil {
			break
		}
		out = append(out, it)
	}
	return out
}

// baseMenu builds the menu bar: on macOS the application menu, Edit, the page's menus, Window and Help (the Help title
// ends in a zero-width space, or macOS adds its own Search field to it); elsewhere just the page's menus.
func baseMenu(a *App, spec []MenuItem) *application.Menu {
	m := application.NewMenu()
	var help *MenuItem
	var rest []MenuItem
	for i := range spec {
		if spec[i].Label == "Help" || spec[i].Key == "Help" {
			help = &spec[i]
		} else {
			rest = append(rest, spec[i])
		}
	}
	if runtime.GOOS == "darwin" {
		app := m.AddSubmenu("mormiCommander")
		app.Add(a.menuWord("About mormiCommander")).OnClick(func(*application.Context) { a.ShowAbout() })
		app.AddSeparator()
		app.Add(a.menuWord("Hide mormiCommander")).SetAccelerator("CmdOrCtrl+h").OnClick(func(*application.Context) {
			if a.wapp != nil {
				application.InvokeSync(a.wapp.Hide)
			}
		})
		app.AddSeparator()
		app.Add(a.menuWord("Quit mormiCommander")).SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) { a.emit("menu", "cm_Exit") })
		m.AddRole(application.EditMenu)
		a.translateRoleMenu(m.ItemAt(1))
		for _, it := range rest {
			addItems(a, m.AddSubmenu(it.Label), it.Items)
		}
		m.AddRole(application.WindowMenu)
		a.translateRoleMenu(m.ItemAt(len(rest) + 2))
		if help != nil {
			addItems(a, m.AddSubmenu(helpLabel(help)+"\u200b"), help.Items)
		}
		return m
	}
	for _, it := range rest {
		addItems(a, m.AddSubmenu(it.Label), it.Items)
	}
	if help != nil {
		addItems(a, m.AddSubmenu(helpLabel(help)), help.Items)
	} else {
		m.AddSubmenu(a.menuWord("Help")).Add(a.menuWord("About mormiCommander")).OnClick(func(*application.Context) { a.ShowAbout() })
	}
	return m
}

// helpLabel is the Help menu's label as the page sent it (translated with a language file).
func helpLabel(help *MenuItem) string {
	if help.Label != "" {
		return help.Label
	}
	return "Help"
}

func addItems(a *App, m *application.Menu, items []MenuItem) {
	for _, it := range items {
		switch {
		case it.Sep:
			m.AddSeparator()
		case len(it.Items) > 0:
			addItems(a, m.AddSubmenu(it.Label), it.Items)
		default:
			cmd := it.Cmd
			var mi *application.MenuItem
			if it.Checked != nil {
				mi = m.AddCheckbox(it.Label, *it.Checked)
			} else {
				mi = m.Add(it.Label)
			}
			mi.SetEnabled(!it.Disabled).OnClick(func(*application.Context) { a.emit("menu", cmd) })
			if it.Icon != "" {
				if png := menuIcon(it.Icon); png != nil {
					mi.SetBitmap(png)
				}
			}
		}
	}
}
