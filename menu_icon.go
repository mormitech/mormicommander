package main

// Icons of menu entries (the Start menu's own entries, Change start menu > Icon, F-450): the user names an image file
// (PNG, JPEG, GIF, BMP or WebP; %COMMANDER_PATH% and relative paths as for button icons) and the native menu shows it
// at 16x16 before the label. The file is read only when the menu is built, scaled once and kept while it is unchanged.
// A file that is missing, too big or not an image just gives no icon.

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const menuIconSize = 16

type menuIconEntry struct {
	mod  time.Time
	size int64
	png  []byte
}

var menuIcons struct {
	mu sync.Mutex
	m  map[string]menuIconEntry
}

// menuIcon is the 16x16 PNG of an icon file named in the settings, or nil.
func menuIcon(icon string) []byte {
	icon = strings.TrimSpace(icon)
	if icon == "" || strings.HasPrefix(icon, "mc:") {
		return nil
	}
	full := icon
	if configFeature != nil { // not set up in tests
		var err error
		if full, err = configExpandPath(icon); err != nil {
			return nil
		}
	}
	if !filepath.IsAbs(full) {
		return nil
	}
	return menuIconFile(full)
}

// menuIconFile reads, scales and caches one image file.
func menuIconFile(full string) []byte {
	switch strings.ToLower(filepath.Ext(full)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
	default:
		return nil
	}
	st, err := os.Stat(full)
	if err != nil || st.IsDir() || st.Size() > 4<<20 {
		return nil
	}
	menuIcons.mu.Lock()
	defer menuIcons.mu.Unlock()
	if e, ok := menuIcons.m[full]; ok && e.mod.Equal(st.ModTime()) && e.size == st.Size() {
		return e.png
	}
	out := menuIconPNG(full)
	if menuIcons.m == nil {
		menuIcons.m = map[string]menuIconEntry{}
	}
	menuIcons.m[full] = menuIconEntry{mod: st.ModTime(), size: st.Size(), png: out}
	return out
}

// menuIconPNG decodes the file and scales it into a 16x16 square, keeping its proportions.
func menuIconPNG(full string) []byte {
	f, err := os.Open(full)
	if err != nil {
		return nil
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > 64<<20 {
		return nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	w, h := menuIconSize, menuIconSize
	if b.Dx() > b.Dy() {
		h = max(1, menuIconSize*b.Dy()/b.Dx())
	} else if b.Dy() > b.Dx() {
		w = max(1, menuIconSize*b.Dx()/b.Dy())
	}
	dst := image.NewRGBA(image.Rect(0, 0, menuIconSize, menuIconSize))
	x0, y0 := (menuIconSize-w)/2, (menuIconSize-h)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x0, y0, x0+w, y0+h), src, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil
	}
	return buf.Bytes()
}
