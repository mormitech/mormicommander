// Config ties all settings together: it finds the settings file (paths.go),
// loads it into the typed models (options, colours, hotlist, Start menu,
// hotkeys, column sets, histories, tabs...), and saves everything back in one
// atomic write. The parsed INI document stays in memory the whole time, so
// whatever the models don't know about, like comments, keys of features we
// don't have yet, or another program's sections, is written back untouched.
// Settings files edited by hand while the app runs can be re-read with
// Reload (F-826), exported to a backup file and restored from one (F-447),
// and saved on demand (cm_ConfigSaveSettings, F-440). Config is not safe for
// concurrent use; the app holds it behind its own mutex.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is the loaded configuration.
type Config struct {
	Loc  Locations
	Vars Vars
	File *File

	Settings   Settings
	Colors     ColorScheme
	ColorsDark ColorScheme
	Hotlist    *Hotlist
	StartMenu  *StartMenu
	Keymap     *Keymap
	Columns    []ColumnSet
	ViewModes  []ViewMode
	Tooltips   []TooltipRule
	Tools      []ExternalTool

	LeftHistory  *History
	RightHistory *History
	CmdHistory   *History
	// Inputs holds the dialog input histories by section name
	// (SectionSearchName, SectionSearchText, SectionSelection, ...).
	Inputs   map[string]*History
	Frequent *Frequent

	Left, Right         PanelState
	LeftTabs, RightTabs TabSet

	// Warnings collects problems found while loading (bad values, bad
	// hotkeys); the UI may show them once.
	Warnings []string
}

// inputSections are the dialog histories kept in Config.Inputs.
var inputSections = []string{SectionSearchName, SectionSearchText, SectionSearchIn,
	SectionSelection, SectionRenameTemplate, SectionMkDirHistory}

// Open loads the configuration at loc. A missing settings file is not an
// error: the defaults are used and the file is created on the first Save.
func Open(loc Locations) (*Config, error) {
	c := &Config{Loc: loc}
	c.Vars = DefaultVars(programDir(loc), loc.Ini)
	f, err := LoadFile(loc.Ini)
	switch {
	case errors.Is(err, os.ErrNotExist):
		f = NewFile()
		if runtime.GOOS == "windows" {
			f.Newline = "\r\n"
		}
	case err != nil:
		return nil, err
	}
	c.load(f)
	return c, nil
}

// programDir is the folder %COMMANDER_PATH% stands for: the program folder
// in portable mode, otherwise the settings folder (where bars, tab files and
// the ignore list live). Using the settings folder keeps the default paths
// writable on every platform.
func programDir(loc Locations) string {
	if loc.Portable {
		return filepath.Dir(loc.Ini)
	}
	return loc.Dir
}

func (c *Config) load(f *File) {
	c.File = f
	c.Warnings = nil
	var w []string
	c.Settings, w = LoadSettings(f)
	c.Warnings = append(c.Warnings, w...)
	c.Colors = LoadColorScheme(f, SectionColors)
	c.ColorsDark = LoadColorScheme(f, SectionColorsDark)
	c.Hotlist = LoadHotlist(f)
	c.StartMenu = LoadStartMenu(f)
	c.Keymap, w = LoadKeymap(f)
	for _, b := range w {
		c.Warnings = append(c.Warnings, "bad hotkey: "+b)
	}
	c.Columns = LoadColumnSets(f)
	c.ViewModes = LoadViewModes(f)
	c.Tooltips = LoadTooltipRules(f)
	c.Tools = LoadExternalTools(f)

	hs := c.Settings.Hotlist
	c.LeftHistory = NewHistory(hs.HistorySize, true)
	c.LeftHistory.Load(f, SectionLeftHistory)
	c.RightHistory = NewHistory(hs.HistorySize, true)
	c.RightHistory.Load(f, SectionRightHistory)
	c.CmdHistory = NewHistory(hs.CmdHistSize, false)
	c.CmdHistory.Load(f, SectionCmdLineHistory)
	c.Inputs = map[string]*History{}
	for _, sec := range inputSections {
		h := NewHistory(DefaultHistorySize, sec == SectionSearchIn || sec == SectionMkDirHistory)
		h.Load(f, sec)
		c.Inputs[sec] = h
	}
	c.Frequent = NewFrequent()
	c.Frequent.Max = max(hs.FrequentSize*3, 50)
	c.Frequent.Load(f, SectionFrequentDirs)

	c.Left = LoadPanelState(f, SectionLeft)
	c.Right = LoadPanelState(f, SectionRight)
	c.LeftTabs = LoadTabSet(f, SectionLeftTabs)
	c.RightTabs = LoadTabSet(f, SectionRightTabs)
}

// Sync writes the models into the in-memory document without saving.
// History-like data is only written when the matching "remember" option is
// on, and removed when it is off, like TC.
func (c *Config) Sync() {
	f := c.File
	c.Settings.Save(f)
	if f.HasSection(SectionColors) || !c.Colors.allDefault() {
		SaveColorScheme(f, SectionColors, c.Colors)
	}
	if f.HasSection(SectionColorsDark) || !c.ColorsDark.allDefault() {
		SaveColorScheme(f, SectionColorsDark, c.ColorsDark)
	}
	if len(c.Hotlist.Items) > 0 || f.HasSection(SectionDirMenu) {
		c.Hotlist.Save(f)
	}
	if len(c.StartMenu.Items) > 0 || f.HasSection(SectionUserMenu) {
		c.StartMenu.Save(f)
	}
	if len(c.Keymap.Bindings()) > 0 || f.HasSection(SectionShortcuts) {
		c.Keymap.Save(f)
	}
	if len(c.Columns) > 0 || f.HasSection(SectionCustomFields) {
		SaveColumnSets(f, c.Columns)
	}
	if len(c.ViewModes) > 0 || f.HasSection(SectionViewModes) {
		SaveViewModes(f, c.ViewModes)
	}
	if len(c.Tooltips) > 0 || f.HasSection(SectionTooltips) {
		SaveTooltipRules(f, c.Tooltips)
	}
	if len(c.Tools) > 0 || f.HasSection(SectionExternalTools) {
		SaveExternalTools(f, c.Tools)
	}
	op := c.Settings.Operation
	syncHist := func(keep bool, h *History, sec string) {
		if keep {
			h.Save(f, sec)
		} else {
			f.DeleteSection(sec)
		}
	}
	syncHist(op.SaveHistory, c.LeftHistory, SectionLeftHistory)
	syncHist(op.SaveHistory, c.RightHistory, SectionRightHistory)
	syncHist(op.SaveCommands, c.CmdHistory, SectionCmdLineHistory)
	for _, sec := range inputSections {
		if h := c.Inputs[sec]; h != nil && (h.Len() > 0 || f.HasSection(sec)) {
			h.Save(f, sec)
		}
	}
	if op.SaveHistory {
		if len(c.Frequent.Top(1)) > 0 || f.HasSection(SectionFrequentDirs) {
			c.Frequent.Save(f, SectionFrequentDirs)
		}
	} else {
		f.DeleteSection(SectionFrequentDirs)
	}
	if op.SavePath {
		SavePanelState(f, SectionLeft, c.Left)
		SavePanelState(f, SectionRight, c.Right)
		if c.Settings.FolderTabs.RestoreTabs {
			SaveTabSet(f, SectionLeftTabs, c.LeftTabs)
			SaveTabSet(f, SectionRightTabs, c.RightTabs)
		}
	}
}

func (s ColorScheme) allDefault() bool {
	for _, k := range colorKeys {
		if !k.get(&s).IsDefault() {
			return false
		}
	}
	return len(s.Filters) == 0 && !s.InverseCursor && !s.InverseSelection
}

// Save syncs the models and writes the settings file atomically.
func (c *Config) Save() error {
	c.Sync()
	return c.File.Save(c.Loc.Ini)
}

// Reload re-reads the settings file from disk, dropping unsaved changes
// (after the user edited the file by hand, F-826).
func (c *Config) Reload() error {
	f, err := LoadFile(c.Loc.Ini)
	if err != nil {
		return err
	}
	c.load(f)
	return nil
}

// Export writes the current configuration (including unsaved changes) to
// another file, e.g. a backup chosen by the user (F-447). The live settings
// file is not touched.
func (c *Config) Export(path string) error {
	c.Sync()
	return c.File.Clone().Save(path)
}

// Restore replaces the configuration with the content of a backup file and
// saves it as the live settings file.
func (c *Config) Restore(path string) error {
	f, err := LoadFile(path)
	if err != nil {
		return err
	}
	c.load(f)
	return f.Save(c.Loc.Ini)
}

// Expand expands commander and environment variables (F-849).
func (c *Config) Expand(s string) string { return c.Vars.Expand(s) }

// Path expands s and makes it absolute relative to the settings folder.
func (c *Config) Path(s string) string {
	s = c.Expand(s)
	if s == "" || filepath.IsAbs(s) || strings.HasPrefix(s, `\\`) {
		return s
	}
	return filepath.Join(c.Loc.Dir, s)
}

// ButtonBarPath returns the main bar file: [Buttonbar] Buttonbar=, or
// default.bar in the settings folder.
func (c *Config) ButtonBarPath() string {
	if p, ok := c.File.Get(SectionButtonbar, "Buttonbar"); ok && p != "" {
		return c.Path(p)
	}
	return filepath.Join(c.Loc.Dir, "default.bar")
}

// SetButtonBarPath makes another .bar file the main bar (F-309); paths
// inside the settings folder are stored with %COMMANDER_PATH%.
func (c *Config) SetButtonBarPath(p string) {
	c.File.Set(SectionButtonbar, "Buttonbar", c.Vars.Contract(p))
}

// VerticalBarPath returns the middle bar file (F-314), or "" if none.
func (c *Config) VerticalBarPath() string {
	if p, ok := c.File.Get(SectionButtonbarVertical, "Buttonbar"); ok && p != "" {
		return c.Path(p)
	}
	return ""
}

// LoadButtonBar reads the main bar, or returns DefaultBar when the file
// doesn't exist yet.
func (c *Config) LoadButtonBar() (*Bar, error) {
	bar, _, err := LoadBarFile(c.ButtonBarPath())
	if errors.Is(err, os.ErrNotExist) {
		return DefaultBar(), nil
	}
	return bar, err
}

// UserCmdPath is the usercmd.ini next to the settings file.
func (c *Config) UserCmdPath() string { return filepath.Join(c.Loc.Dir, UserCmdIniName) }

// LoadUserCommands reads usercmd.ini (missing file = none).
func (c *Config) LoadUserCommands() ([]UserCommand, *File, error) {
	f, err := LoadFile(c.UserCmdPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, NewFile(), nil
	}
	if err != nil {
		return nil, nil, err
	}
	return LoadUserCommands(f), f, nil
}

// LoadIgnoreList reads the ignore file named on the Ignore list page; it
// returns an empty list when the page's option is off.
func (c *Config) LoadIgnoreList() (IgnoreList, error) {
	if !c.Settings.IgnoreList.Enabled {
		return IgnoreList{}, nil
	}
	return LoadIgnoreList(c.Path(c.Settings.IgnoreList.File))
}

// ActiveColors returns the scheme to use: the dark one when dark mode is on
// (or follows a dark system) and it has colours, otherwise the normal one,
// with unset colours filled from the matching built-in preset.
func (c *Config) ActiveColors(systemDark bool) ColorScheme {
	dark := c.Settings.Colors.DarkMode == 1 || (c.Settings.Colors.DarkMode == 2 && systemDark)
	base := BuiltinSchemes()[0]
	s := c.Colors
	if dark {
		base = BuiltinSchemes()[1]
		if !c.ColorsDark.allDefault() {
			s = c.ColorsDark
		} else {
			s = ColorScheme{Filters: c.Colors.Filters}
			for _, k := range colorKeys {
				*k.get(&s) = DefaultColor
			}
		}
	}
	for _, k := range colorKeys {
		if k.get(&s).IsDefault() {
			*k.get(&s) = *k.get(&base)
		}
	}
	if s.Name == "" || s.Name == SectionColors || s.Name == SectionColorsDark {
		s.Name = base.Name
	}
	return s
}

// DefaultBar is the button bar of a fresh install (F-304).
func DefaultBar() *Bar {
	b := func(icon, cmd, tip string) Button { return Button{Icon: "mc:" + icon, Command: cmd, Tooltip: tip} }
	sep := Button{}
	return &Bar{Buttons: []Button{
		b("reread", "cm_RereadSource", "Reread source window"),
		sep,
		b("brief", "cm_SrcShort", "Brief"),
		b("full", "cm_SrcLong", "Full"),
		b("thumbnails", "cm_SrcThumbs", "Thumbnails"),
		b("tree", "cm_SrcTree", "Tree"),
		sep,
		b("invert", "cm_ExchangeSelection", "Invert selection"),
		sep,
		b("back", "cm_GoToPrev", "Back"),
		b("forward", "cm_GoToNext", "Forward"),
		sep,
		b("pack", "cm_PackFiles", "Pack files"),
		b("unpack", "cm_UnpackFiles", "Unpack files"),
		sep,
		b("find", "cm_SearchFor", "Find files"),
		b("view", "cm_List", "View file"),
		b("rename", "cm_MultiRename", "Multi-rename tool"),
		b("sync", "cm_SyncDirs", "Synchronize directories"),
		b("ftp", "cm_FtpConnect", "FTP connect"),
		b("hotlist", "cm_DirectoryHotlist", "Directory hotlist"),
		sep,
		b("edit", "cm_Edit", "Edit file"),
		b("compare", "cm_CompareFilesByContent", "Compare by content"),
	}}
}
