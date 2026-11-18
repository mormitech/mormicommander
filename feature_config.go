package main

// The Configuration feature, Go side (FEATURES.md section 17): the Options dialog's pages and values, colours, hotkey
// overrides, saving, editing, importing, exporting and restoring the settings file. Everything is kept by
// internal/config in mormicommander.ini, a wincmd.ini-style file (F-443) in the settings folder, next to the program in a
// portable install (F-444), or wherever -I= points (F-445). The page builds the dialog from the field list the engine
// gives (Schema), so a new option in internal/config shows up in the dialog without UI work.
//
// The button bar is in feature_config_bar.go, the Start menu and the start-up parameters in feature_config_start.go.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/config"
	"mormicommander/internal/launch"
)

// Configuration is what the page calls for the Options dialog (window.go.main.Configuration).
type Configuration struct {
	app      *App
	mu       sync.Mutex
	cfg      *config.Config
	openErr  error
	watching bool
}

// configFeature is the one Configuration object, for the other config files of this feature.
var configFeature *Configuration

func init() {
	registerFeature("config", func(a *App) []exposed {
		configFeature = &Configuration{app: a}
		return []exposed{expose(configFeature), expose(&ConfigBar{app: a}), expose(&ConfigStart{app: a})}
	})
	// the settings are opened at start (the page reads them at once anyway), so COMMANDER_PATH and the like are in the
	// environment before any program is started (F-853)
	onStartup(func(a *App) {
		if configFeature != nil {
			if _, err := configFeature.get(); err == nil {
				configFeature.mu.Unlock()
			}
		}
	})
	registerControl("config-info", func(a *App, m map[string]any) (map[string]any, error) {
		info, err := configFeature.Info()
		return map[string]any{"ok": err == nil, "info": info}, err
	})
}

// configLocate finds the settings files: -I= / -F= first, then a portable file next to the program, then the settings
// folder of configdir.go (which also honours MORMICOMMANDER_CONFIG_DIR and the demo's own folder).
func configLocate(o startOpts) (config.Locations, error) {
	lo := config.LocateOptions{IniOverride: o.IniPath, FtpOverride: o.FtpIniPath, UserConfigDir: string(filepath.Separator) + "-"}
	if demoDir != "" || os.Getenv("MORMICOMMANDER_CONFIG_DIR") != "" {
		lo.ExeDir = filepath.Join(os.TempDir(), "mormicommander-no-portable") // a demo or test run never picks a portable file
	}
	loc, err := config.Locate(lo)
	if err != nil {
		return loc, err
	}
	if !loc.Override && !loc.Portable {
		dir := configDir()
		if dir == "" {
			return loc, errors.New("The settings folder could not be found.")
		}
		loc.Dir, loc.Ini = dir, filepath.Join(dir, config.IniName)
		if o.FtpIniPath == "" {
			loc.FtpIni = filepath.Join(dir, config.FtpIniName)
		}
	}
	return loc, nil
}

// exportCommanderEnv puts COMMANDER_PATH, COMMANDER_INI and COMMANDER_EXE in the app's own environment, so every
// program it starts (buttons, the command line, F3/F4 tools, a terminal) sees them, as programs started by Total
// Commander do (F-853). Programs macOS opens through Finder's way (open, a double click on a document) get the
// environment of the system instead, not this one.
func exportCommanderEnv(v config.Vars, setenv func(k, v string) error) {
	for _, k := range []string{config.VarCommanderPath, config.VarCommanderIni, config.VarCommanderExe} {
		if val := v[k]; val != "" {
			_ = setenv(k, val)
		}
	}
}

// get opens the configuration the first time it is needed and returns it locked; call c.mu.Unlock() when done.
func (c *Configuration) get() (*config.Config, error) {
	c.mu.Lock()
	if c.cfg == nil && c.openErr == nil {
		loc, err := configLocate(c.app.opts)
		if err == nil {
			c.cfg, err = config.Open(loc)
		}
		c.openErr = err
		if c.cfg != nil && c.app.wapp != nil { // (the real app only: tests keep their environment)
			exportCommanderEnv(c.cfg.Vars, os.Setenv)
		}
	}
	if c.cfg == nil {
		c.mu.Unlock()
		if c.openErr != nil {
			return nil, fmt.Errorf("The settings file could not be read: %v", c.openErr)
		}
		return nil, errors.New("The settings file could not be read.")
	}
	return c.cfg, nil
}

// configField is one option of the dialog, as the page sees it.
type configField struct {
	Name    string         `json:"name"`
	Page    string         `json:"page"`
	PageID  string         `json:"pageId"`
	Section string         `json:"section"`
	Key     string         `json:"key"`
	Label   string         `json:"label"`
	Type    string         `json:"type"`
	Default string         `json:"default"`
	Min     *int           `json:"min"`
	Max     *int           `json:"max"`
	Options []configOption `json:"options"`
	// NoEffect: kept in the file, but the program does not act on it yet (internal/config/noeffect.go, issue #94).
	NoEffect bool `json:"noEffect"`
}

type configOption struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type configPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Schema lists the pages of the Options dialog and every option on them, in dialog order.
func (c *Configuration) Schema() map[string]any {
	var pages []configPage
	for _, p := range config.Pages() {
		pages = append(pages, configPage{p.ID, p.Title})
	}
	var fields []configField
	for _, f := range config.Fields() {
		cf := configField{Name: f.Name, Page: f.Page, PageID: f.PageID, Section: f.Section, Key: f.Key, Label: f.Label,
			Type: f.Type, Default: f.Default, Min: f.Min, Max: f.Max, NoEffect: f.NoEffect}
		for _, o := range f.Options {
			cf.Options = append(cf.Options, configOption{o.Value, o.Label})
		}
		fields = append(fields, cf)
	}
	return map[string]any{"pages": pages, "fields": fields}
}

// Values returns every option as text ("1"/"0" for yes/no), by "Page.Field" name.
func (c *Configuration) Values() (map[string]string, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	out := map[string]string{}
	for _, f := range config.Fields() {
		v, _ := cfg.Settings.Get(f.Name)
		out[f.Name] = v
	}
	return out, nil
}

// Apply changes the given options ("Page.Field" → text), saves the settings file and returns the names that changed.
func (c *Configuration) Apply(values map[string]string) ([]string, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	next := cfg.Settings
	for name, v := range values {
		if err := next.Set(name, v); err != nil {
			return nil, fmt.Errorf("The option %s could not be set: %v", name, err)
		}
	}
	changed := cfg.Settings.Diff(&next)
	cfg.Settings = next
	if err := cfg.Save(); err != nil {
		return changed, fmt.Errorf("The settings could not be saved: %v", err)
	}
	return changed, nil
}

// Search returns the pages and options that match every word of the text (the search box of the dialog, F-827).
func (c *Configuration) Search(query string) map[string]any {
	fields, pages := config.SearchSettings(query)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return map[string]any{"pages": pages, "fields": names}
}

// SearchWords is Search with a choice of whole words: with wholeWords, "two" finds "Two drive button bars" but not "network"
// (the "Whole words" box under the search box, board issue #109). Search itself stays as it was.
func (c *Configuration) SearchWords(query string, wholeWords bool) map[string]any {
	fields, pages := config.SearchSettingsWords(query, wholeWords)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return map[string]any{"pages": pages, "fields": names}
}

// configScheme is a colour scheme for the page: colours as "#rrggbb", "" for "the theme's colour".
type configScheme struct {
	Name             string              `json:"name"`
	Colors           map[string]string   `json:"colors"`
	InverseCursor    bool                `json:"inverseCursor"`
	InverseSelection bool                `json:"inverseSelection"`
	Filters          []map[string]string `json:"filters"`
}

func configSchemeColors(s *config.ColorScheme) map[string]*config.Color {
	return map[string]*config.Color{"fore": &s.Fore, "back": &s.Back, "back2": &s.Back2, "mark": &s.Mark, "cursor": &s.Cursor,
		"cursorText": &s.CursorText, "inactiveCursor": &s.InactiveCursor, "inactiveMark": &s.InactiveMark, "inactiveBack": &s.InactiveBack}
}

func configSchemeOut(s config.ColorScheme) configScheme {
	out := configScheme{Name: s.Name, Colors: map[string]string{}, InverseCursor: s.InverseCursor, InverseSelection: s.InverseSelection,
		Filters: []map[string]string{}}
	for k, c := range configSchemeColors(&s) {
		if c.IsDefault() {
			out.Colors[k] = ""
		} else {
			out.Colors[k] = c.Hex()
		}
	}
	for _, f := range s.Filters {
		col := ""
		if !f.Color.IsDefault() {
			col = f.Color.Hex()
		}
		out.Filters = append(out.Filters, map[string]string{"mask": f.Mask, "color": col})
	}
	return out
}

func configSchemeIn(in configScheme, name string) (config.ColorScheme, error) {
	s := config.ColorScheme{Name: name, InverseCursor: in.InverseCursor, InverseSelection: in.InverseSelection}
	for k, c := range configSchemeColors(&s) {
		*c = config.DefaultColor
		if v := strings.TrimSpace(in.Colors[k]); v != "" {
			pc, err := config.ParseColor(v)
			if err != nil {
				return s, fmt.Errorf("%q is not a colour.", v)
			}
			*c = pc
		}
	}
	for _, f := range in.Filters {
		mask := strings.TrimSpace(f["mask"])
		if mask == "" {
			continue
		}
		col := config.DefaultColor
		if v := strings.TrimSpace(f["color"]); v != "" {
			pc, err := config.ParseColor(v)
			if err != nil {
				return s, fmt.Errorf("%q is not a colour.", v)
			}
			col = pc
		}
		s.Filters = append(s.Filters, config.ColorFilter{Mask: mask, Color: col})
	}
	return s, nil
}

// Colors returns the light and dark colour schemes and the built-in presets (Colors page, F-417, F-435).
func (c *Configuration) Colors() (map[string]any, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	var builtin []configScheme
	for _, s := range config.BuiltinSchemes() {
		builtin = append(builtin, configSchemeOut(s))
	}
	return map[string]any{"light": configSchemeOut(cfg.Colors), "dark": configSchemeOut(cfg.ColorsDark), "builtin": builtin}, nil
}

// SetColors stores the light and dark colour schemes and saves the settings file.
func (c *Configuration) SetColors(light, dark configScheme) error {
	l, err := configSchemeIn(light, config.SectionColors)
	if err != nil {
		return err
	}
	d, err := configSchemeIn(dark, config.SectionColorsDark)
	if err != nil {
		return err
	}
	cfg, err := c.get()
	if err != nil {
		return err
	}
	defer c.mu.Unlock()
	cfg.Colors, cfg.ColorsDark = l, d
	return cfg.Save()
}

// configComboFromHotkey writes a hotkey the way the page does ("Ctrl+Shift+F5", "Cmd+Backspace").
func configComboFromHotkey(h config.Hotkey) string {
	var parts []string
	for _, m := range []struct {
		on   bool
		name string
	}{{h.Ctrl, "Ctrl"}, {h.Alt, "Alt"}, {h.Shift, "Shift"}, {h.Win, "Cmd"}} {
		if m.on {
			parts = append(parts, m.name)
		}
	}
	key := h.Key
	switch key {
	case "Back":
		key = "Backspace"
	case "Del":
		key = "Delete"
	case "NumEnter":
		key = "Enter"
	}
	return strings.Join(append(parts, key), "+")
}

// Keymap returns the user's hotkey overrides ([Shortcuts], [ShortcutsWin]) as combo → command ("" = key taken away).
func (c *Configuration) Keymap() (map[string]string, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	out := map[string]string{}
	for _, b := range cfg.Keymap.Bindings() {
		out[configComboFromHotkey(b.Key)] = b.Command
	}
	return out, nil
}

// SetKeymap replaces the hotkey overrides (combo → command, "" removes a default key) and saves the settings file.
func (c *Configuration) SetKeymap(m map[string]string) error {
	km := config.NewKeymap()
	for combo, cmd := range m {
		h, err := config.ParseHotkey(combo)
		if err != nil {
			return fmt.Errorf("%q is not a key the settings file can keep.", combo)
		}
		km.Bind(h, cmd)
	}
	cfg, err := c.get()
	if err != nil {
		return err
	}
	defer c.mu.Unlock()
	cfg.Keymap = km
	return cfg.Save()
}

// Info tells where the settings files are and what went wrong while reading them.
func (c *Configuration) Info() (map[string]any, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	_, statErr := os.Stat(cfg.Loc.Ini)
	return map[string]any{"ini": cfg.Loc.Ini, "dir": cfg.Loc.Dir, "ftpIni": cfg.Loc.FtpIni, "portable": cfg.Loc.Portable,
		"override": cfg.Loc.Override, "exists": statErr == nil, "warnings": append([]string{}, cfg.Warnings...),
		"settingsJson": c.app.settings.Path()}, nil
}

// CmdHistory returns the command line history kept in mormicommander.ini ([Command line history], newest first): the
// lines a Total Commander import brought over (F-295). The page keeps its own history in settings.json and merges
// these into it (features/cmdline-import.js).
func (c *Configuration) CmdHistory() ([]string, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	return append([]string{}, cfg.CmdHistory.Items()...), nil
}

// Save writes the settings file now (cm_ConfigSaveSettings, F-440).
func (c *Configuration) Save() error {
	cfg, err := c.get()
	if err != nil {
		return err
	}
	defer c.mu.Unlock()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("The settings could not be saved: %v", err)
	}
	return nil
}

// Reload reads the settings file again, after it was edited by hand (F-826).
func (c *Configuration) Reload() error {
	cfg, err := c.get()
	if err != nil {
		return err
	}
	err = cfg.Reload()
	c.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("The settings file could not be read again: %v", err)
	}
	c.app.emit("config:changed")
	return nil
}

// Export writes the settings, with unsaved changes, to another file (a backup, F-447).
func (c *Configuration) Export(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("Give the name of the backup file.")
	}
	cfg, err := c.get()
	if err != nil {
		return err
	}
	defer c.mu.Unlock()
	if err := cfg.Export(path); err != nil {
		return fmt.Errorf("The settings could not be written to %s: %v", path, err)
	}
	return nil
}

// Restore replaces the settings with those of a backup file and saves them (F-447).
func (c *Configuration) Restore(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("The file %s is not there.", path)
	}
	cfg, err := c.get()
	if err != nil {
		return err
	}
	err = cfg.Restore(path)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("The settings could not be restored from %s: %v", path, err)
	}
	c.app.emit("config:changed")
	return nil
}

// ImportTC brings over the hotlist, hotkeys, colours, columns, button bar, Start menu, user commands and options of a
// Total Commander wincmd.ini (F-446), saves them, and says what was imported.
func (c *Configuration) ImportTC(path string, replaceHotlist bool) (map[string]any, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("The file %s is not there.", path)
	}
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	opt := config.AllImport()
	opt.ReplaceHotlist = replaceHotlist
	// The hotlist lives in folders.ini (feature_tabshotlist.go), not in mormicommander.ini: the engine merges TC's
	// [DirMenu] into the hotlist it is handed, so it is handed the live one and the result goes back there.
	th := thInstance
	saved := cfg.Hotlist.Items
	if th != nil {
		th.mu.Lock()
		cfg.Hotlist.Items = append([]*config.MenuItem(nil), th.hot.Items...)
		th.mu.Unlock()
	}
	rep, err := config.ImportTC(cfg, path, opt)
	var hotErr error
	if th != nil {
		merged := cfg.Hotlist.Items
		cfg.Hotlist.Items = saved
		if err == nil && configImportedHotlist(rep.Imported) {
			th.mu.Lock()
			th.hot.Items = merged
			th.hot.Save(th.file)
			hotErr = th.save()
			th.mu.Unlock()
		}
	}
	if err == nil {
		err = cfg.Save()
	}
	c.mu.Unlock()
	if err == nil {
		err = hotErr
	}
	if err != nil {
		return nil, fmt.Errorf("The file could not be imported: %v", err)
	}
	c.app.emit("config:changed")
	return map[string]any{"imported": append([]string{}, rep.Imported...), "warnings": append([]string{}, rep.Warnings...)}, nil
}

// EditFile opens the settings file in the editor of Configuration > Viewer/Editor (or the system's text editor) and
// reads it again whenever it is saved, for half an hour (cm_ConfigChangeIniFiles, F-826).
func (c *Configuration) EditFile() (string, error) {
	cfg, err := c.get()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(cfg.Loc.Ini); err != nil { // an editor wants a file to open
		cfg.Save()
	}
	path, editor := cfg.Loc.Ini, cfg.Settings.ViewerEditor.Editor
	c.mu.Unlock()
	l := launch.New()
	if err := l.OpenInTool(launch.ParseTool(editor), path, launch.ParamContext{}); err != nil {
		return path, fmt.Errorf("The editor could not be started: %v", err)
	}
	c.watch(path)
	return path, nil
}

func (c *Configuration) watch(path string) {
	c.mu.Lock()
	if c.watching {
		c.mu.Unlock()
		return
	}
	c.watching = true
	c.mu.Unlock()
	stamp := func() time.Time {
		if st, err := os.Stat(path); err == nil {
			return st.ModTime()
		}
		return time.Time{}
	}
	go func() {
		defer func() { c.mu.Lock(); c.watching = false; c.mu.Unlock() }()
		last := stamp()
		for end := time.Now().Add(30 * time.Minute); time.Now().Before(end); {
			time.Sleep(time.Second)
			if t := stamp(); !t.Equal(last) {
				last = t
				if c.Reload() == nil {
					c.app.emit("config:reloaded", path)
				}
			}
		}
	}()
}

// ChooseFile shows the system's file dialog and returns the chosen path ("" when cancelled). save asks for a new name.
func (c *Configuration) ChooseFile(title string, save bool, name string, pattern string) (string, error) {
	kind := "open-file"
	if save {
		kind = "save-file"
	}
	if v, ok := c.app.mockAnswer(kind, title); ok { // control mode: the queued answer, never a system dialog
		return v, nil
	}
	if c.app.ctx == nil {
		return "", errors.New("The file dialog is not available.")
	}
	var filters []fileFilter
	if pattern != "" {
		filters = []fileFilter{{DisplayName: pattern, Pattern: pattern}}
	}
	dir := ""
	if info, err := c.Info(); err == nil {
		dir, _ = info["dir"].(string)
	}
	if save {
		return c.app.saveFileDialog(title, dir, name, filters)
	}
	return c.app.openFileDialog(title, dir, filters)
}

// SavePosition saves the window's place and size now (cm_ConfigSavePos, F-439).
func (c *Configuration) SavePosition() { c.app.SaveWindow() }

// Value returns one raw value of the settings file ("" when it is not there), for options without a dialog field.
func (c *Configuration) Value(section, key string) (string, error) {
	cfg, err := c.get()
	if err != nil {
		return "", err
	}
	defer c.mu.Unlock()
	v, _ := cfg.File.Get(section, key)
	return v, nil
}

// SetValue stores one raw value in the settings file and saves it.
func (c *Configuration) SetValue(section, key, value string) error {
	cfg, err := c.get()
	if err != nil {
		return err
	}
	defer c.mu.Unlock()
	cfg.File.Set(section, key, value)
	return cfg.Save()
}

// configImportedHotlist says whether an import report has a hotlist line (the engine adds one only when TC's
// [DirMenu] had entries).
func configImportedHotlist(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "hotlist:") {
			return true
		}
	}
	return false
}
