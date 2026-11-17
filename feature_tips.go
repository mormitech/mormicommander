package main

// The facts the file list tooltips and the ignore list need from the Go side (board issue #94: Options > Tooltips and
// Options > Ignore list). frontend/src/features/config/listopts.js is the page's side.

import (
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
