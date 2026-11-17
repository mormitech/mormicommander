package main

// The facts the file list tooltips and the ignore list need from the Go side (board issue #94: Options > Tooltips and
// Options > Ignore list). frontend/src/features/config/listopts.js is the page's side.

import (
	"fmt"
	"image"
	"os"
	"strings"

	"mormicommander/internal/config"
)

// ImageSize is the picture size of an image file, for the tooltip of an image in the file list (Tooltips > Use
// per-type tooltip fields). Zero when the file is not a picture Go can read the size of.
func (v *Views) ImageSize(path string) map[string]int {
	f, err := os.Open(path)
	if err != nil {
		return map[string]int{"w": 0, "h": 0}
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return map[string]int{"w": 0, "h": 0}
	}
	return map[string]int{"w": cfg.Width, "h": cfg.Height}
}

// IgnoreList is the ignore file of Options > Ignore list: where it is and its masks, one per line, without blank lines
// and comments (";" or "#"). A missing file gives no masks. The page decides whether the list is used
// (IgnoreList.Enabled, cm_SwitchIgnoreList), so the file is read whether the option is on or not.
func (c *Configuration) IgnoreList() (map[string]any, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	path := cfg.Path(cfg.Settings.IgnoreList.File)
	c.mu.Unlock()
	masks := []string{}
	if data, err := os.ReadFile(path); err == nil {
		text, _ := config.Decode(data, config.EncCP1252)
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == ';' || line[0] == '#' {
				continue
			}
			masks = append(masks, line)
		}
	}
	return map[string]any{"file": path, "masks": masks}, nil
}

// currentSettings is a copy of the Options dialog's values for Go features that follow an option (false when the
// settings file can't be read: the caller keeps its default then).
func currentSettings() (config.Settings, bool) {
	if configFeature == nil {
		return config.Settings{}, false
	}
	cfg, err := configFeature.get()
	if err != nil {
		return config.Settings{}, false
	}
	defer configFeature.mu.Unlock()
	return cfg.Settings, true
}

// configMenuItem is one entry of a custom main menu file for the page: a submenu (items), a line (sep) or a command.
type configMenuItem struct {
	Label string           `json:"label,omitempty"`
	Cmd   string           `json:"cmd,omitempty"`
	Sep   bool             `json:"sep,omitempty"`
	Items []configMenuItem `json:"items,omitempty"`
}

// MainMenu is the menu bar of Options > Misc > Custom main menu file (a Total Commander .mnu file): its menus with
// their entries, "&" taken out of the titles. Empty when no file is set; an error when the file can't be read.
func (c *Configuration) MainMenu() ([]configMenuItem, error) {
	cfg, err := c.get()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(cfg.Settings.Misc.MainMenuFile)
	path := ""
	if name != "" {
		path = cfg.Path(name)
	}
	c.mu.Unlock()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("The main menu file %s can't be read: %v", path, err)
	}
	text, _ := config.Decode(data, config.EncCP1252)
	items, err := config.ParseMenuFile(text)
	if err != nil {
		return nil, fmt.Errorf("The main menu file %s: %s", path, strings.TrimPrefix(err.Error(), "config: "))
	}
	var conv func([]*config.MenuItem) []configMenuItem
	conv = func(in []*config.MenuItem) []configMenuItem {
		out := []configMenuItem{}
		for _, it := range in {
			switch it.Kind {
			case config.MenuSeparator:
				out = append(out, configMenuItem{Sep: true})
			case config.MenuSubmenu:
				out = append(out, configMenuItem{Label: strings.ReplaceAll(it.Title, "&", ""), Items: conv(it.Children)})
			default:
				out = append(out, configMenuItem{Label: strings.ReplaceAll(it.Title, "&", ""), Cmd: it.Field("cmd")})
			}
		}
		return out
	}
	return conv(items), nil
}
