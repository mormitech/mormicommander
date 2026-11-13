package main

// The button bar, Go side (FEATURES.md section 11): reading and writing .bar files (the main bar, the vertical middle
// bar, sub-bars, other bars to switch to), expanding a button's parameters (%P %N %S %L ... and the "?" prompt) into a
// program to start, the clipboard format of a single button, buttons made from dropped files, and the user's em_
// commands in usercmd.ini. The engines are internal/config (bars, user commands), internal/params (placeholders) and
// internal/launch (starting programs).

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mormicommander/internal/config"
	"mormicommander/internal/launch"
	"mormicommander/internal/params"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ConfigBar is what the page calls for the button bars (window.go.main.ConfigBar).
type ConfigBar struct {
	app      *App
	mu       sync.Mutex
	launches map[string]*params.Launch // prepared buttons waiting for the "?" prompt or for Run
}

// configButton is one button as the page sees it.
type configButton struct {
	Icon    string `json:"icon"`
	Cmd     string `json:"cmd"`
	Params  string `json:"params"`
	Path    string `json:"path"`
	Tooltip string `json:"tooltip"`
	Iconic  int    `json:"iconic"`
	Sep     bool   `json:"sep"`
	SubBar  bool   `json:"subBar"`
}

func configButtonOut(b config.Button) configButton {
	return configButton{Icon: b.Icon, Cmd: b.Command, Params: b.Params, Path: b.Path, Tooltip: b.Tooltip, Iconic: b.Iconic,
		Sep: b.IsSeparator(), SubBar: b.IsSubBar()}
}

func configButtonIn(b configButton) config.Button {
	if b.Sep {
		return config.Button{}
	}
	return config.Button{Icon: strings.TrimSpace(b.Icon), Command: strings.TrimSpace(b.Cmd), Params: b.Params, Path: strings.TrimSpace(b.Path),
		Tooltip: strings.TrimSpace(b.Tooltip), Iconic: b.Iconic}
}

func configBarOut(path string, bar *config.Bar, exists bool) map[string]any {
	buttons := []configButton{}
	for _, b := range bar.Buttons {
		buttons = append(buttons, configButtonOut(b))
	}
	return map[string]any{"path": path, "buttons": buttons, "exists": exists}
}

// barPath is the file of the main ("main") or the vertical ("vertical") bar.
func configBarPath(cfg *config.Config, which string) string {
	if which == "vertical" {
		if p := cfg.VerticalBarPath(); p != "" {
			return p
		}
		return filepath.Join(cfg.Loc.Dir, "vertical.bar")
	}
	return cfg.ButtonBarPath()
}

// Bar returns the main bar ("main") or the vertical middle bar ("vertical"). A main bar that was never saved is the
// default bar of a fresh install (F-304).
func (b *ConfigBar) Bar(which string) (map[string]any, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return nil, err
	}
	path := configBarPath(cfg, which)
	configFeature.mu.Unlock()
	bar, _, err := config.LoadBarFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist) && which == "vertical":
		return configBarOut(path, &config.Bar{}, false), nil
	case errors.Is(err, os.ErrNotExist):
		return configBarOut(path, config.DefaultBar(), false), nil
	case err != nil:
		return nil, fmt.Errorf("The button bar %s could not be read: %v", path, err)
	}
	return configBarOut(path, bar, true), nil
}

// LoadBar reads any .bar file (a sub-bar, F-308, or a bar to switch to, F-309). Variables such as %COMMANDER_PATH% are
// expanded and a relative name is taken from the settings folder.
func (b *ConfigBar) LoadBar(path string) (map[string]any, error) {
	full, err := configExpandPath(path)
	if err != nil {
		return nil, err
	}
	bar, _, err := config.LoadBarFile(full)
	if err != nil {
		return nil, fmt.Errorf("The button bar %s could not be read.", full)
	}
	return configBarOut(full, bar, true), nil
}

func configExpandPath(p string) (string, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return "", err
	}
	defer configFeature.mu.Unlock()
	head, _ := config.SplitCommand(strings.TrimSpace(p))
	if head == "" {
		head = strings.TrimSpace(p)
	}
	return cfg.Path(strings.Trim(head, `"`)), nil
}

// SaveBar writes a bar to path (the current file when empty) and makes it the main or vertical bar.
func (b *ConfigBar) SaveBar(which, path string, buttons []configButton) (string, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return "", err
	}
	defer configFeature.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		path = configBarPath(cfg, which)
	} else {
		path = cfg.Path(strings.TrimSpace(path))
	}
	bar := &config.Bar{}
	for _, x := range buttons {
		bar.Buttons = append(bar.Buttons, configButtonIn(x))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("The folder of %s could not be made: %v", path, err)
	}
	if err := bar.SaveBarFile(path); err != nil {
		return "", fmt.Errorf("The button bar could not be saved: %v", err)
	}
	configSetBarPath(cfg, which, path)
	return path, cfg.Save()
}

func configSetBarPath(cfg *config.Config, which, path string) {
	if which == "vertical" {
		cfg.File.Set(config.SectionButtonbarVertical, "Buttonbar", cfg.Vars.Contract(path))
	} else {
		cfg.SetButtonBarPath(path)
	}
}

// UseBar makes another .bar file the main or vertical bar (F-309).
func (b *ConfigBar) UseBar(which, path string) error {
	full, err := configExpandPath(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err != nil {
		return fmt.Errorf("The button bar %s is not there.", full)
	}
	cfg, err := configFeature.get()
	if err != nil {
		return err
	}
	defer configFeature.mu.Unlock()
	configSetBarPath(cfg, which, full)
	return cfg.Save()
}

// ListBars lists the .bar files in the settings folder.
func (b *ConfigBar) ListBars() ([]string, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return nil, err
	}
	dir := cfg.Loc.Dir
	configFeature.mu.Unlock()
	m, _ := filepath.Glob(filepath.Join(dir, "*.bar"))
	sort.Strings(m)
	if m == nil {
		m = []string{}
	}
	return m, nil
}

// Options returns the look of the bars: icon size (16, 24 or 32), flat buttons, wrapping onto more lines (F-312).
func (b *ConfigBar) Options() (map[string]any, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return nil, err
	}
	defer configFeature.mu.Unlock()
	size := cfg.File.GetInt(config.SectionButtonbar, "Buttonheight", 20)
	icon := 16
	switch {
	case size >= 36:
		icon = 32
	case size >= 28:
		icon = 24
	}
	if v, ok := cfg.File.Get(config.SectionButtonbar, "IconSize"); ok {
		if n, err := strconv.Atoi(v); err == nil && (n == 16 || n == 24 || n == 32) {
			icon = n
		}
	}
	flat := cfg.File.GetBool(config.SectionButtonbar, "FlatIcons", true)
	wrap := cfg.File.GetBool(config.SectionButtonbar, "WrapButtons", false)
	return map[string]any{"iconSize": icon, "flat": flat, "wrap": wrap}, nil
}

// SetOptions stores the look of the bars and saves the settings file.
func (b *ConfigBar) SetOptions(iconSize int, flat, wrap bool) error {
	if iconSize != 24 && iconSize != 32 {
		iconSize = 16
	}
	cfg, err := configFeature.get()
	if err != nil {
		return err
	}
	defer configFeature.mu.Unlock()
	bit := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	cfg.File.Set(config.SectionButtonbar, "IconSize", strconv.Itoa(iconSize))
	cfg.File.Set(config.SectionButtonbar, "Buttonheight", strconv.Itoa(iconSize+8))
	cfg.File.Set(config.SectionButtonbar, "FlatIcons", bit(flat))
	cfg.File.Set(config.SectionButtonbar, "WrapButtons", bit(wrap))
	return cfg.Save()
}

// IconData returns an image file named as a button's icon as a data: URL, so the page can show it ("" when the icon is
// not an image file the page can show, such as "x.exe,2").
func (b *ConfigBar) IconData(icon string) string {
	if icon == "" || strings.HasPrefix(icon, "mc:") {
		return ""
	}
	path := icon
	if i := strings.LastIndex(path, ","); i > 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(path[i+1:])); err == nil {
			path = path[:i]
		}
	}
	full, err := configExpandPath(path)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(full))
	kind := mime.TypeByExtension(ext)
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".bmp":
	default:
		return ""
	}
	if ext == ".svg" {
		kind = "image/svg+xml"
	}
	if ext == ".ico" {
		kind = "image/x-icon"
	}
	st, err := os.Stat(full)
	if err != nil || st.IsDir() || st.Size() > 512<<10 {
		return ""
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// configPanel is one panel's state for the placeholders.
type configPanel struct {
	Path     string   `json:"path"`
	Cursor   string   `json:"cursor"`
	CursorIs bool     `json:"cursorDir"`
	Selected []string `json:"selected"`
	SelDirs  []bool   `json:"selectedDirs"`
}

func (p configPanel) params() params.Panel {
	out := params.Panel{Path: p.Path, Cursor: params.Entry{Name: p.Cursor, Dir: p.CursorIs}}
	for i, n := range p.Selected {
		out.Selected = append(out.Selected, params.Entry{Name: n, Dir: i < len(p.SelDirs) && p.SelDirs[i]})
	}
	return out
}

// configLaunch is a prepared button: what to run, or the questions to ask first.
type configLaunch struct {
	ID       string              `json:"id"`
	Internal bool                `json:"internal"`
	Program  string              `json:"program"`
	Line     string              `json:"line"`
	Dir      string              `json:"dir"`
	Prompt   bool                `json:"prompt"`
	Canceled bool                `json:"canceled"`
	Ask      []map[string]string `json:"ask"`
}

// Prepare expands a button (or a Start menu entry) for the panels: the placeholders of the parameters and the start
// folder (F-310, F-814). answers holds the user's answers to %[question;default] (by question); when one is missing the
// questions come back in Ask and nothing is prepared. dropped are files dropped on the button (F-307). An internal
// command comes back with Internal set; a program is kept under ID for Run (or Discard).
func (b *ConfigBar) Prepare(btn configButton, source, target configPanel, sourceIsRight bool, answers map[string]string, dropped []string) (configLaunch, error) {
	var missing []map[string]string
	seen := map[string]bool{}
	ctx := params.Context{Source: source.params(), Target: target.params(), SourceIsRight: sourceIsRight, Dropped: dropped,
		Ask: func(caption, def string) (string, bool) {
			if v, ok := answers[caption]; ok {
				return v, true
			}
			if !seen[caption] {
				seen[caption] = true
				missing = append(missing, map[string]string{"caption": caption, "default": def})
			}
			return def, true
		}}
	if cfg, err := configFeature.get(); err == nil {
		ctx.Env.Vars = map[string]string{}
		for k, v := range cfg.Vars {
			ctx.Env.Vars[k] = v
		}
		configFeature.mu.Unlock()
		btn.Path = cfg.Vars.Expand(btn.Path)
	}
	l, err := params.PrepareButton(params.Button{Command: btn.Cmd, Params: btn.Params, StartPath: btn.Path, Iconic: btn.Iconic}, ctx)
	if err != nil {
		l.Cleanup()
		return configLaunch{}, fmt.Errorf("The parameters could not be filled in: %v", err)
	}
	if len(missing) > 0 {
		l.Cleanup()
		return configLaunch{Ask: missing}, nil
	}
	out := configLaunch{Internal: l.Internal, Program: l.Program, Line: l.Line, Dir: l.Dir, Prompt: l.Prompt, Canceled: l.Canceled}
	if l.Canceled || l.Internal {
		l.Cleanup()
		return out, nil
	}
	var id [8]byte
	rand.Read(id[:])
	out.ID = hex.EncodeToString(id[:])
	b.mu.Lock()
	if b.launches == nil {
		b.launches = map[string]*params.Launch{}
	}
	b.launches[out.ID] = &l
	b.mu.Unlock()
	return out, nil
}

func (b *ConfigBar) take(id string) *params.Launch {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.launches[id]
	delete(b.launches, id)
	return l
}

// Run starts a prepared program; line replaces the parameters when the user edited them in the "?" prompt.
func (b *ConfigBar) Run(id string, line string, edited bool) error {
	l := b.take(id)
	if l == nil {
		return errors.New("The button is no longer ready; click it again.")
	}
	if edited {
		if err := l.SetLine(line); err != nil {
			l.Cleanup()
			return fmt.Errorf("The parameters could not be read: %v", err)
		}
	}
	show := launch.ShowNormal
	switch l.Show {
	case params.ShowMinimized:
		show = launch.ShowMinimized
	case params.ShowMaximized:
		show = launch.ShowMaximized
	}
	err := launch.New().Run(launch.RunSpec{Program: l.Program, Params: l.Line, Dir: l.Dir, Show: show, Admin: l.Elevate, Cleanup: l.ListFiles})
	if err != nil {
		return fmt.Errorf("%s could not be started: %v", l.Program, err)
	}
	return nil
}

// Discard forgets a prepared program (the "?" prompt was cancelled) and deletes its list files.
func (b *ConfigBar) Discard(id string) {
	if l := b.take(id); l != nil {
		l.Cleanup()
	}
}

// CopyButton puts a button on the clipboard in Total Commander's format (F-313).
func (b *ConfigBar) CopyButton(btn configButton) error {
	if b.app.ctx == nil {
		return errors.New("The clipboard is not available.")
	}
	return wruntime.ClipboardSetText(b.app.ctx, configButtonIn(btn).ClipboardText())
}

// PasteButton reads a button from the clipboard (copied here or in Total Commander).
func (b *ConfigBar) PasteButton() (configButton, error) {
	if b.app.ctx == nil {
		return configButton{}, errors.New("The clipboard is not available.")
	}
	text, err := wruntime.ClipboardGetText(b.app.ctx)
	if err != nil {
		return configButton{}, err
	}
	return b.ParseButton(text)
}

// ParseButton reads a button from text in the clipboard format.
func (b *ConfigBar) ParseButton(text string) (configButton, error) {
	btn, err := config.ParseButtonClipboard(text)
	if err != nil {
		return configButton{}, errors.New("The clipboard does not hold a button.")
	}
	return configButtonOut(btn), nil
}

// ButtonText is a button in the clipboard format.
func (b *ConfigBar) ButtonText(btn configButton) string { return configButtonIn(btn).ClipboardText() }

// ButtonForFile is the button that dropping a program or file on the bar adds (F-306).
func (b *ConfigBar) ButtonForFile(path string) configButton {
	return configButtonOut(config.ButtonForFile(path))
}

// configUserCmd is an em_ command of usercmd.ini.
type configUserCmd struct {
	Name   string `json:"name"`
	Cmd    string `json:"cmd"`
	Params string `json:"params"`
	Path   string `json:"path"`
	Menu   string `json:"menu"`
	Icon   string `json:"icon"`
	Iconic int    `json:"iconic"`
}

// UserCommands lists the em_ commands of usercmd.ini (F-311).
func (b *ConfigBar) UserCommands() ([]configUserCmd, error) {
	cfg, err := configFeature.get()
	if err != nil {
		return nil, err
	}
	defer configFeature.mu.Unlock()
	list, _, err := cfg.LoadUserCommands()
	if err != nil {
		return nil, fmt.Errorf("usercmd.ini could not be read: %v", err)
	}
	out := []configUserCmd{}
	for _, u := range list {
		out = append(out, configUserCmd{u.Name, u.Command, u.Params, u.Path, u.Menu, u.Icon, u.Iconic})
	}
	return out, nil
}

// SaveUserCommands replaces the em_ commands of usercmd.ini (keeping its comments and other sections' order).
func (b *ConfigBar) SaveUserCommands(cmds []configUserCmd) error {
	cfg, err := configFeature.get()
	if err != nil {
		return err
	}
	defer configFeature.mu.Unlock()
	old, f, err := cfg.LoadUserCommands()
	if err != nil {
		return fmt.Errorf("usercmd.ini could not be read: %v", err)
	}
	keep := map[string]bool{}
	for _, u := range cmds {
		if !config.ValidUserCommandName(u.Name) {
			return fmt.Errorf("%q is not a user command name: it starts with em_ and has letters, digits and _ only.", u.Name)
		}
		keep[strings.ToLower(u.Name)] = true
		if err := config.SaveUserCommand(f, config.UserCommand{Name: u.Name, Command: u.Cmd, Params: u.Params, Path: u.Path, Menu: u.Menu, Icon: u.Icon, Iconic: u.Iconic}); err != nil {
			return err
		}
	}
	for _, u := range old {
		if !keep[strings.ToLower(u.Name)] {
			config.DeleteUserCommand(f, u.Name)
		}
	}
	if err := os.MkdirAll(filepath.Dir(cfg.UserCmdPath()), 0o755); err != nil {
		return err
	}
	return f.Save(cfg.UserCmdPath())
}
