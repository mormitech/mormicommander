package main

// Compare folders (Shift+F2, cm_CompareDirs and cm_CompareDirsWithSubdirs, FEATURES.md F-133, F-134, F-352) for the
// page: a thin layer over internal/dircompare. The answer is the names to select in each panel and the names that
// compared equal (for "hide equal files"). Folders on this computer are compared on disk (by content and recursively
// when asked); for other panels (archives, FTP) the page sends the two listings it shows and they are compared by name,
// date and size.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/dircompare"
)

// CompareDirs is what the page calls for Compare folders (window.go.main.CompareDirs).
type CompareDirs struct {
	app    *App
	mu     sync.Mutex
	cancel context.CancelFunc
}

func init() {
	registerFeature("dircmp", func(a *App) []exposed { return []exposed{expose(&CompareDirs{app: a})} })
}

// DirCmpRequest is what Shift+F2 sends: both panels' folders and the options.
type DirCmpRequest struct {
	Left          string        `json:"left"`
	Right         string        `json:"right"`
	Subdirs       string        `json:"subdirs"` // "" ignore folders, "missing" select one-sided folders, "recursive"
	ByContent     bool          `json:"byContent"`
	IgnoreDate    bool          `json:"ignoreDate"`
	IgnoreHour    bool          `json:"ignoreHour"`
	Tolerance     int           `json:"tolerance"` // seconds
	IncludeHidden bool          `json:"includeHidden"`
	LeftEntries   []DirCmpEntry `json:"leftEntries"` // the listings, for panels that are not folders on this computer
	RightEntries  []DirCmpEntry `json:"rightEntries"`
}

// DirCmpEntry is one entry of a panel listing.
type DirCmpEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	MTime  int64  `json:"mtime"` // milliseconds since 1970
	Dir    bool   `json:"dir"`
	Hidden bool   `json:"hidden"`
}

// DirCmpResult is what the page selects and reports.
type DirCmpResult struct {
	LeftSelect  []string       `json:"leftSelect"`
	RightSelect []string       `json:"rightSelect"`
	LeftEqual   []string       `json:"leftEqual"`
	RightEqual  []string       `json:"rightEqual"`
	Counts      map[string]int `json:"counts"`
	Errors      []string       `json:"errors"`
	OnDisk      bool           `json:"onDisk"`
}

func dirCmpLocal(p string) bool {
	if p == "" || strings.Contains(p, "://") {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Compare compares the two folders and returns what to select in each panel.
func (c *CompareDirs) Compare(r DirCmpRequest) (*DirCmpResult, error) {
	opt := dircompare.Options{
		TimeTolerance:    time.Duration(r.Tolerance) * time.Second,
		IgnoreHourOffset: r.IgnoreHour,
		IgnoreDate:       r.IgnoreDate,
		ByContent:        r.ByContent,
		IncludeHidden:    r.IncludeHidden,
	}
	switch r.Subdirs {
	case "missing":
		opt.Subdirs = dircompare.SubdirsMissing
	case "recursive":
		opt.Subdirs = dircompare.SubdirsRecursive
	}
	out := &DirCmpResult{LeftSelect: []string{}, RightSelect: []string{}, LeftEqual: []string{}, RightEqual: []string{}, Counts: map[string]int{}, Errors: []string{}}
	if c.app.mock == nil && dirCmpLocal(r.Left) && dirCmpLocal(r.Right) {
		if filepath.Clean(r.Left) == filepath.Clean(r.Right) {
			return out, nil // the same folder: everything is equal
		}
		ctx, cancel := context.WithCancel(context.Background())
		c.mu.Lock()
		if c.cancel != nil {
			c.cancel()
		}
		c.cancel = cancel
		c.mu.Unlock()
		defer func() {
			cancel()
			c.mu.Lock()
			c.cancel = nil
			c.mu.Unlock()
		}()
		res, err := dircompare.Compare(ctx, dircompare.OS{}, r.Left, dircompare.OS{}, r.Right, opt)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, errors.New("The compare was stopped.")
			}
			return nil, err
		}
		out.OnDisk = true
		out.LeftSelect, out.RightSelect, out.LeftEqual, out.RightEqual = dirCmpNZ(res.LeftSelect), dirCmpNZ(res.RightSelect), dirCmpNZ(res.LeftEqual), dirCmpNZ(res.RightEqual)
		for st, n := range res.Counts {
			out.Counts[st.String()] = n
		}
		for _, e := range res.Errors {
			out.Errors = append(out.Errors, e.Error())
		}
		return out, nil
	}
	// Not folders on this computer: compare the listings the panels show, by name, date and size.
	opt.ByContent = false
	if opt.Subdirs == dircompare.SubdirsRecursive {
		opt.Subdirs = dircompare.SubdirsMissing
	}
	conv := func(es []DirCmpEntry) []dircompare.Entry {
		out := make([]dircompare.Entry, 0, len(es))
		for _, e := range es {
			out = append(out, dircompare.Entry{Name: e.Name, Size: e.Size, ModTime: time.UnixMilli(e.MTime), IsDir: e.Dir, Hidden: e.Hidden})
		}
		return out
	}
	for _, it := range dircompare.CompareEntries(conv(r.LeftEntries), conv(r.RightEntries), opt) {
		out.Counts[it.Status.String()]++
		ln, rn := it.Name, it.Name
		if it.Left != nil {
			ln = it.Left.Name
		}
		if it.Right != nil {
			rn = it.Right.Name
		}
		if it.Status.MarksLeft() && it.Left != nil {
			out.LeftSelect = append(out.LeftSelect, ln)
		}
		if it.Status.MarksRight() && it.Right != nil {
			out.RightSelect = append(out.RightSelect, rn)
		}
		if it.Status == dircompare.Equal {
			if it.Left != nil {
				out.LeftEqual = append(out.LeftEqual, ln)
			}
			if it.Right != nil {
				out.RightEqual = append(out.RightEqual, rn)
			}
		}
	}
	return out, nil
}

func dirCmpNZ(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Cancel stops a compare in progress (Esc while it reads big files by content).
func (c *CompareDirs) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
}
