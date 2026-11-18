package main

// The folder size treemap's memory between sessions (board #24, F-872). The treemap itself is the page's
// (frontend/src/features/treemap.js); it counts with Tools.Space like Space and Alt+Shift+Enter. With Options >
// Operation > "Folder size treemap remembers folder sizes between sessions" (Operation.TreemapRemember, off by
// default) it hands the sizes it counted to Treemap.Remember, which keeps them in foldersizes.bin in the settings
// folder (dirsize.Store: gzip-compressed gob, saved atomically), and asks Treemap.Remembered for them when it opens a
// folder, to draw those blocks at once, marked as remembered, while it counts again. A remembered size is never put in
// the size column (F-183 shows only real counts). With the option off nothing is kept, and the file, which holds
// folder names, is removed the next time the treemap asks.

import (
	"os"
	"path/filepath"
	"sync"

	"mormicommander/internal/dirsize"
)

// Treemap is what the page calls for the treemap's remembered sizes (window.go.main.Treemap).
type Treemap struct {
	app *App

	mu    sync.Mutex
	store *dirsize.Store
	file  string
}

// treemapMaxAsk bounds how many folders one call may ask about or remember (a panel shows at most a few thousand).
const treemapMaxAsk = 100000

func init() {
	t := &Treemap{}
	registerFeature("treemap", func(a *App) []exposed { t.app = a; return []exposed{expose(t)} })
	onShutdown(func(*App) { t.save() })
}

// treemapRemember says whether the option to remember sizes between sessions is on (a variable for the tests).
var treemapRemember = func() bool {
	s, ok := currentSettings()
	return ok && s.Operation.TreemapRemember
}

// treemapFile is the file the sizes are kept in, "" when there is no settings folder.
func treemapFile() string {
	d := configDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "foldersizes.bin")
}

// open returns the store of the current settings folder, loading it the first time.
func (t *Treemap) open() *dirsize.Store {
	f := treemapFile()
	if f == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.store == nil || t.file != f {
		t.store, t.file = dirsize.OpenStore(f), f
	}
	return t.store
}

func (t *Treemap) save() {
	t.mu.Lock()
	s := t.store
	t.mu.Unlock()
	if s != nil {
		s.Save()
	}
}

// Remembered returns the remembered sizes of the given folders that have one, keyed by the path as given. With the
// option off it returns none and forgets everything kept before.
func (t *Treemap) Remembered(paths []string) map[string]dirsize.Remembered {
	out := map[string]dirsize.Remembered{}
	if !treemapRemember() {
		t.Forget()
		return out
	}
	s := t.open()
	if s == nil {
		return out
	}
	if len(paths) > treemapMaxAsk {
		paths = paths[:treemapMaxAsk]
	}
	for _, p := range paths {
		if r, ok := s.Get(p); ok {
			out[p] = r
		}
	}
	return out
}

// Remember keeps the counted sizes (folder path → bytes) when the option is on, and saves them; it says how many it
// kept.
func (t *Treemap) Remember(sizes map[string]int64) (int, error) {
	if !treemapRemember() || len(sizes) == 0 || len(sizes) > treemapMaxAsk {
		return 0, nil
	}
	s := t.open()
	if s == nil {
		return 0, nil
	}
	n := 0
	for p, b := range sizes {
		if filepath.IsAbs(p) && b >= 0 {
			s.Put(dirsize.Remembered{Path: p, Bytes: b})
			n++
		}
	}
	return n, s.Save()
}

// Forget removes every remembered size and the file.
func (t *Treemap) Forget() error {
	t.mu.Lock()
	s := t.store
	t.store, t.file = nil, ""
	t.mu.Unlock()
	if s != nil {
		s.Clear()
	}
	f := treemapFile()
	if f == "" {
		return nil
	}
	if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
