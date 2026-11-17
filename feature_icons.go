package main

// System file icons for the file lists (board issue #94, F-095: Options > Icons > Show icons "All, including file
// icons"). The page asks for the icon of a file type (by extension) or of one file or folder (a program, a folder with
// an icon of its own) and gets a PNG as a data: URL, made by the system: Finder's icons on macOS
// (feature_icons_darwin.go), Explorer's on Windows (feature_icons_windows.go). Linux has no single place to ask, so
// there the page keeps its own icons (feature_icons_other.go answers nothing). Answers are kept, so a type is asked
// once per size.

import (
	"encoding/base64"
	"strings"
	"sync"
)

// Icons is what the page calls for system icons (window.go.main.Icons).
type Icons struct {
	mu    sync.Mutex
	cache map[string]string
}

func init() {
	registerFeature("icons", func(a *App) []exposed { return []exposed{expose(&Icons{})} })
}

// Supported says whether this system gives file icons.
func (ic *Icons) Supported() bool { return sysIconsSupported }

func iconPx(size int) int {
	if size < 16 {
		return 16
	}
	if size > 64 {
		return 64
	}
	return size
}

// ForTypes answers the icon of each extension ("txt", "" for a file without one, "/" for a plain folder) at size
// pixels (drawn at twice that for sharp screens), as data: URLs; an extension the system has no icon for is left out.
func (ic *Icons) ForTypes(exts []string, size int) map[string]string {
	out := map[string]string{}
	px := iconPx(size) * 2
	for _, e := range exts {
		e = strings.ToLower(strings.TrimPrefix(e, "."))
		if u := ic.get("t:"+e, px, func() []byte { return sysIconForType(e, px) }); u != "" {
			out[e] = u
		}
	}
	return out
}

// ForPaths answers the icon of each file or folder itself (programs, folders and files with icons of their own), as
// ForTypes does.
func (ic *Icons) ForPaths(paths []string, size int) map[string]string {
	out := map[string]string{}
	px := iconPx(size) * 2
	for _, p := range paths {
		if strings.Contains(p, "://") {
			continue // (a server or an archive: nothing the system can draw)
		}
		if u := ic.get("p:"+p, px, func() []byte { return sysIconForPath(p, px) }); u != "" {
			out[p] = u
		}
	}
	return out
}

func (ic *Icons) get(key string, px int, make func() []byte) string {
	k := key + "@" + itoa(px)
	ic.mu.Lock()
	if ic.cache == nil {
		ic.cache = map[string]string{}
	}
	u, ok := ic.cache[k]
	ic.mu.Unlock()
	if ok {
		return u
	}
	if png := make(); len(png) > 0 {
		u = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	ic.mu.Lock()
	if len(ic.cache) > 4000 {
		ic.cache = map[string]string{} // (a long session with many programs: start over rather than grow without end)
	}
	ic.cache[k] = u
	ic.mu.Unlock()
	return u
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
