// Import of a real Total Commander wincmd.ini (F-446, F-663). People moving
// from TC bring years of hotlist entries, hotkeys, colours, column sets,
// buttons and Start menu items, and retyping them is the main reason not to
// switch. ImportTC reads a copy of their wincmd.ini (in whatever encoding TC
// wrote it), and merges the chosen parts into our configuration: the hotlist
// goes into a "Total Commander" submenu (or replaces ours), hotkeys become
// overrides, colours and file type colours replace ours, column sets and
// Start menu entries are added, and options that share TC's key names are
// copied. The main button bar file named in the ini is located (its
// %COMMANDER_PATH% is the TC folder, and Windows paths that don't exist here
// are looked up by file name next to the ini), copied into our settings
// folder and made our main bar. A usercmd.ini next to the wincmd.ini brings
// its em_ commands along. The copied bar files and usercmd.ini are written to
// our settings folder right away, but the main settings file is only changed
// in memory: the caller saves it (or throws the import away), and the report
// lists what was imported and what could not be.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ImportOptions chooses what to import. The zero value imports nothing;
// use AllImport for everything.
type ImportOptions struct {
	Hotlist, Keymap, Colors, Columns, ButtonBar, StartMenu, Histories, Settings, UserCommands bool
	// ReplaceHotlist replaces our hotlist instead of adding a submenu.
	ReplaceHotlist bool
	// TCDir is TC's program folder for %COMMANDER_PATH%; default: the
	// folder of the ini file.
	TCDir string
	// Codepage for ini files without BOM that are not UTF-8 (EncCP1252 or
	// EncCP1250); default EncCP1252.
	Codepage Encoding
}

// AllImport imports everything, merging the hotlist into a submenu.
func AllImport() ImportOptions {
	return ImportOptions{Hotlist: true, Keymap: true, Colors: true, Columns: true, ButtonBar: true,
		StartMenu: true, Histories: true, Settings: true, UserCommands: true}
}

// ImportReport describes an import.
type ImportReport struct {
	Imported []string // human-readable lines like "hotlist: 12 entries"
	Warnings []string
}

func (r *ImportReport) add(format string, a ...any) {
	r.Imported = append(r.Imported, fmt.Sprintf(format, a...))
}
func (r *ImportReport) warn(format string, a ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
}

// ImportTC merges a TC wincmd.ini into c (in memory; call c.Save after).
func ImportTC(c *Config, iniPath string, opt ImportOptions) (ImportReport, error) {
	var rep ImportReport
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return rep, err
	}
	cp := opt.Codepage
	if cp != EncCP1250 {
		cp = EncCP1252
	}
	tc := Parse(data, cp)
	tcDir := opt.TCDir
	if tcDir == "" {
		tcDir = filepath.Dir(iniPath)
	}
	// TC's own variables point at the TC folder while reading its files.
	tcVars := Vars{VarCommanderPath: tcDir, VarCommanderIni: iniPath}

	if opt.Hotlist {
		h := LoadHotlist(tc)
		n := countCommands(h.Items)
		if n > 0 {
			if opt.ReplaceHotlist {
				c.Hotlist.Items = h.Items
			} else {
				sub := NewHotlistSubmenu("Total Commander")
				sub.Children = h.Items
				c.Hotlist.Items = append(c.Hotlist.Items, sub)
			}
			rep.add("hotlist: %d entries", n)
			warnForeignPaths(&rep, "hotlist", h.Items)
		}
	}

	if opt.Keymap {
		km, bad := LoadKeymap(tc)
		for _, b := range km.Bindings() {
			c.Keymap.Bind(b.Key, b.Command)
		}
		if n := len(km.Bindings()); n > 0 {
			rep.add("hotkeys: %d", n)
		}
		for _, b := range bad {
			rep.warn("hotkey not understood: %s", b)
		}
	}

	if opt.Colors && tc.HasSection(SectionColors) {
		s := LoadColorScheme(tc, SectionColors)
		s.Name = SectionColors
		c.Colors = s
		rep.add("colours: %d file type colours", len(s.Filters))
		if tc.HasSection(SectionColorsDark) {
			c.ColorsDark = LoadColorScheme(tc, SectionColorsDark)
			rep.add("dark mode colours")
		}
	}

	if opt.Columns {
		added := 0
		for _, cs := range LoadColumnSets(tc) {
			if hasColumnSet(c.Columns, cs.Name) {
				rep.warn("column set %q already exists, skipped", cs.Name)
				continue
			}
			c.Columns = append(c.Columns, cs)
			added++
		}
		if added > 0 {
			rep.add("column sets: %d", added)
		}
	}

	if opt.StartMenu {
		sm := LoadStartMenu(tc)
		if n := countCommands(sm.Items); n > 0 {
			c.StartMenu.Items = append(c.StartMenu.Items, sm.Items...)
			rep.add("start menu: %d entries", n)
		}
	}

	if opt.Histories {
		merge := func(dst *History, sec string) int {
			src := NewHistory(dst.Max, dst.IsPath)
			src.Load(tc, sec)
			items := src.Items()
			for i := len(items) - 1; i >= 0; i-- {
				dst.Add(items[i])
			}
			return len(items)
		}
		n := merge(c.LeftHistory, SectionLeftHistory) + merge(c.RightHistory, SectionRightHistory)
		n += merge(c.CmdHistory, SectionCmdLineHistory)
		for _, sec := range inputSections {
			n += merge(c.Inputs[sec], sec)
		}
		if n > 0 {
			rep.add("history entries: %d", n)
		}
	}

	if opt.Settings {
		n := 0
		for _, fi := range Fields() {
			v, ok := tc.Get(fi.Section, fi.Key)
			if !ok {
				continue
			}
			if fi.Type == "string" && looksLikeForeignPath(v) {
				rep.warn("option %s not imported: %s is a Windows path", fi.Name, v)
				continue
			}
			if err := c.Settings.Set(fi.Name, v); err != nil {
				rep.warn("%v", err)
				continue
			}
			n++
		}
		if n > 0 {
			rep.add("options: %d", n)
		}
	}

	if opt.ButtonBar {
		importBar(c, tc, tcVars, tcDir, &rep)
	}

	if opt.UserCommands {
		importUserCommands(c, tcDir, filepath.Dir(iniPath), &rep)
	}
	return rep, nil
}

func countCommands(items []*MenuItem) int {
	n := 0
	MenuWalk(items, func(_ MenuPath, _ []string, it *MenuItem) {
		if it.Kind == MenuCommand {
			n++
		}
	})
	return n
}

func hasColumnSet(sets []ColumnSet, name string) bool {
	for _, s := range sets {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

// looksLikeForeignPath reports a Windows drive path on a non-Windows system.
func looksLikeForeignPath(p string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	p = strings.TrimSpace(p)
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
}

func warnForeignPaths(rep *ImportReport, what string, items []*MenuItem) {
	n := 0
	MenuWalk(items, func(_ MenuPath, _ []string, it *MenuItem) {
		if it.Kind != MenuCommand {
			return
		}
		a := ParseHotlistCommand(it.Field("cmd"))
		if looksLikeForeignPath(a.Path) || looksLikeForeignPath(it.Field("path")) {
			n++
		}
	})
	if n > 0 {
		rep.warn("%s: %d entries point to Windows drives; edit them to local folders", what, n)
	}
}

// findTCFile resolves a file named in TC's ini: expand TC variables, use it
// if it exists, else look for a file with the same name (any case) in the
// given folders.
func findTCFile(name string, v Vars, dirs ...string) (string, bool) {
	p := v.Expand(name)
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
		return p, true
	}
	base := p
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), base) {
				return filepath.Join(d, e.Name()), true
			}
		}
	}
	return "", false
}

func importBar(c *Config, tc *File, v Vars, tcDir string, rep *ImportReport) {
	name := tc.GetDefault(SectionButtonbar, "Buttonbar", "%COMMANDER_PATH%\\DEFAULT.BAR")
	src, ok := findTCFile(name, v, tcDir)
	if !ok {
		rep.warn("button bar %s not found next to the ini file", name)
		return
	}
	bar, _, err := LoadBarFile(src)
	if err != nil {
		rep.warn("button bar %s: %v", src, err)
		return
	}
	// Sub-bar files referenced by buttons come along too.
	copied := map[string]string{}
	var copyBar func(b *Bar, depth int)
	copyBar = func(b *Bar, depth int) {
		for i, btn := range b.Buttons {
			if !btn.IsSubBar() || depth > 5 {
				continue
			}
			head, rest := SplitCommand(btn.Command)
			sp, ok := findTCFile(head, v, tcDir)
			if !ok {
				rep.warn("sub-bar %s not found", head)
				continue
			}
			dst, done := copied[sp]
			if !done {
				sub, _, err := LoadBarFile(sp)
				if err != nil {
					rep.warn("sub-bar %s: %v", sp, err)
					continue
				}
				dst = filepath.Join(c.Loc.Dir, "tc_"+strings.ToLower(filepath.Base(sp)))
				copied[sp] = dst
				copyBar(sub, depth+1)
				if err := sub.SaveBarFile(dst); err != nil {
					rep.warn("saving %s: %v", dst, err)
					continue
				}
			}
			b.Buttons[i].Command = strings.TrimSpace(QuoteArg(c.Vars.Contract(dst)) + " " + rest)
		}
	}
	copyBar(bar, 0)
	dst := filepath.Join(c.Loc.Dir, "tc_"+strings.ToLower(filepath.Base(src)))
	if err := bar.SaveBarFile(dst); err != nil {
		rep.warn("saving %s: %v", dst, err)
		return
	}
	c.SetButtonBarPath(dst)
	rep.add("button bar: %d buttons (%s)", len(bar.Buttons), filepath.Base(dst))
	for _, k := range []string{"Buttonheight", "SmallIcons", "SmallIconSize", "FlatIcons", "XPstyle"} {
		if val, ok := tc.Get(SectionButtonbar, k); ok {
			c.File.Set(SectionButtonbar, k, val)
		}
	}
}

func importUserCommands(c *Config, tcDir, iniDir string, rep *ImportReport) {
	src, ok := findTCFile(UserCmdIniName, Vars{}, iniDir, tcDir)
	if !ok {
		return
	}
	tf, err := LoadFile(src)
	if err != nil {
		rep.warn("%s: %v", src, err)
		return
	}
	cmds := LoadUserCommands(tf)
	if len(cmds) == 0 {
		return
	}
	existing, f, err := c.LoadUserCommands()
	if err != nil {
		rep.warn("%s: %v", c.UserCmdPath(), err)
		return
	}
	added := 0
	for _, u := range cmds {
		if _, dup := FindUserCommand(existing, u.Name); dup {
			rep.warn("user command %s already exists, skipped", u.Name)
			continue
		}
		if err := SaveUserCommand(f, u); err == nil {
			added++
		}
	}
	if added == 0 {
		return
	}
	if err := f.Save(c.UserCmdPath()); err != nil {
		rep.warn("saving %s: %v", c.UserCmdPath(), err)
		return
	}
	rep.add("user commands: %d", added)
}
