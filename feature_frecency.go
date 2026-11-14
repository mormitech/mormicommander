package main

// The frecency jump (F-867, board issue #27): one list of the folders used most often and most recently, which the page
// (frontend/src/features/frecency.js) shows as a popup that narrows while the user types part of a folder name. The
// list mixes the app's own frequently used folders (the visit counts of feature_tabshotlist.go, F-661) with zoxide's
// database when zoxide is installed. zoxide's file is only read, each time the list is asked for (and kept in memory
// while it has not changed); nothing from it is written anywhere, so the data stays where zoxide keeps it.
//
// MORMICOMMANDER_ZOXIDE_DB, when set, is the zoxide database to read instead ("none" reads none), so tests and QA
// runs never see the user's real folders; a demo run reads none either.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/frecency"
)

// Frecency is what the page calls for the frecency jump (window.go.main.Frecency).
type Frecency struct {
	app *App

	mu       sync.Mutex
	dbPath   string // zoxide's file the cache below is of
	dbMod    time.Time
	dbSize   int64
	dbData   []frecency.Entry
	dbFailed string // why the file could not be read, "" when it could
}

// FrecencyList is the ranked list and where zoxide's part came from.
type FrecencyList struct {
	Items    []frecency.Item `json:"items"`
	Zoxide   string          `json:"zoxide"`   // the zoxide database read, "" when none was
	Problem  string          `json:"problem"`  // why zoxide's database could not be read, "" when it could or was not asked for
	Imported int             `json:"imported"` // folders that came from zoxide's database
}

// frecencyMaxFile bounds the size of a zoxide database that is read at all (a real one is a few hundred kilobytes).
const frecencyMaxFile = 64 << 20

func init() {
	registerFeature("frecency", func(a *App) []exposed { return []exposed{expose(&Frecency{app: a})} })
}

// zoxidePath says which zoxide database to read here, "" for none.
func zoxidePath() string {
	if p, ok := os.LookupEnv("MORMICOMMANDER_ZOXIDE_DB"); ok {
		if p == "none" {
			return ""
		}
		return p
	}
	if demoDir != "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	return frecency.ZoxideDBPath(runtime.GOOS, os.Getenv, home)
}

// zoxide returns zoxide's entries, reading the file again only when it changed. A missing file (zoxide not installed)
// is no problem and gives no entries; a file that cannot be read gives the reason.
func (f *Frecency) zoxide(path string) ([]frecency.Entry, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := os.Stat(path)
	if err != nil {
		f.dbPath, f.dbData, f.dbFailed = "", nil, ""
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ""
		}
		return nil, fmt.Sprintf("The zoxide database %s could not be read: %v", path, err)
	}
	if path == f.dbPath && st.ModTime().Equal(f.dbMod) && st.Size() == f.dbSize {
		return f.dbData, f.dbFailed
	}
	f.dbPath, f.dbMod, f.dbSize, f.dbData, f.dbFailed = path, st.ModTime(), st.Size(), nil, ""
	switch {
	case st.IsDir():
		f.dbFailed = fmt.Sprintf("The zoxide database %s is a folder, not a file.", path)
	case st.Size() > frecencyMaxFile:
		f.dbFailed = fmt.Sprintf("The zoxide database %s is too big to read.", path)
	default:
		b, err := os.ReadFile(path)
		if err != nil {
			f.dbFailed = fmt.Sprintf("The zoxide database %s could not be read: %v", path, err)
			break
		}
		if f.dbData, err = frecency.ParseZoxide(b); err != nil {
			f.dbFailed = fmt.Sprintf("%v (%s)", err, path)
		}
	}
	return f.dbData, f.dbFailed
}

// frecencyKey says when two spellings are the same folder: without trailing separators, and without regard to case
// on Windows and macOS, whose file systems ignore case by default.
func frecencyKey(p string) string {
	if len(p) > 1 {
		p = strings.TrimRight(p, `/\`)
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// List returns the folders ranked by how often and how recently they were used, best first, at most max of them
// (max <= 0 means 500). With zoxide true, zoxide's folders are mixed in when its database is there.
func (f *Frecency) List(zoxide bool, max int) FrecencyList {
	var own map[string]float64
	if th := thInstance; th != nil {
		th.mu.Lock()
		own = th.freq.Scores()
		th.mu.Unlock()
	}
	out := FrecencyList{}
	var zox []frecency.Entry
	if zoxide {
		if path := zoxidePath(); path != "" {
			zox, out.Problem = f.zoxide(path)
			if out.Problem == "" && zox != nil {
				out.Zoxide = path
			}
		}
	}
	out.Items = frecency.Merge(own, zox, time.Now(), frecencyKey)
	if max <= 0 {
		max = 500
	}
	if len(out.Items) > max {
		out.Items = out.Items[:max]
	}
	for _, it := range out.Items {
		if it.Source != "app" {
			out.Imported++
		}
	}
	if out.Items == nil {
		out.Items = []frecency.Item{}
	}
	return out
}
