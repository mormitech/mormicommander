package main

// Feed to listbox (F-541): the results of a search shown in a panel as a folder of their own, find://results-N. The
// names are the paths relative to the folder all results share (sub/b.txt), the size, date and attributes are the real
// files'. The page maps each name back to the real file (frontend/src/features/find.js adds a Panel path hook), so
// copy, move, delete, view and every other file command works on the real files. Listing the folder again stats every
// file, so files that were deleted or moved drop out (Ctrl+R).
//
// Archives (spec area 5, 6.5 rule 6): when every result is inside one archive, the archive's members are fed; when
// results come from several archives or mix archives and plain files, the archive files themselves are fed instead.
// The ".." of the list goes back to the folder the panel showed before. Result lists live while the app runs.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"mormicommander/internal/vfs"
)

const findFeedScheme = "find"

func init() { vfs.Register(findFeedScheme, findFeedFS{}) }

// findFeedPath says whether p is a fed result list.
func findFeedPath(p string) bool {
	return strings.HasPrefix(strings.TrimSpace(p), findFeedScheme+"://")
}

type findFeedItem struct {
	name   string // as the panel shows it
	real   string // the real path
	member bool   // inside an archive: size, date and folder flag come from the search
	size   int64
	mtime  int64
	dir    bool
}

type findFeed struct {
	root  string // find://results-N
	base  string // the folder the names are relative to ("" when names are full paths)
	back  string // the panel's folder before (the list's "..")
	items []findFeedItem
}

type findFeedStore struct {
	mu    sync.Mutex
	seq   int
	feeds map[string]*findFeed
	def   vfs.FS // the app's file system (the disk, or the demo's)
}

var findFeeds = &findFeedStore{feeds: map[string]*findFeed{}}

func (s *findFeedStore) fsys(p string) vfs.FS {
	def := s.def
	if def == nil {
		def = vfs.Local{}
	}
	return vfs.For(p, def)
}

// split cuts a path of a result list into the list's root and the name under it.
func findFeedSplit(p string) (root, name string) {
	p = strings.TrimRight(p, "/")
	rest := strings.TrimPrefix(p, findFeedScheme+"://")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return findFeedScheme + "://" + rest[:i], rest[i+1:]
	}
	return p, ""
}

func (s *findFeedStore) get(p string) *findFeed {
	root, _ := findFeedSplit(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.feeds[root]
}

// paths are the real files of a list (search in results, F-544).
func (s *findFeedStore) paths(p string) ([]string, bool) {
	f := s.get(p)
	if f == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(f.items))
	for i, it := range f.items {
		out[i] = it.real
	}
	return out, true
}

// FindFeedInfo is a new result list: its panel path and the folder its names are relative to.
type FindFeedInfo struct {
	Path  string `json:"path"`
	Base  string `json:"base"`
	Count int    `json:"count"`
	Note  string `json:"note,omitempty"` // e.g. "Archives are listed instead of the files in them."
}

// Feed makes a result list of a search for a panel (back: the folder the panel shows now, for "..").
func (f *Find) Feed(id int, back string) (FindFeedInfo, error) {
	f.mu.Lock()
	run := f.runs[id]
	f.mu.Unlock()
	if run == nil {
		return FindFeedInfo{}, errors.New("These results are gone. Search again.")
	}
	run.mu.Lock()
	results := append(run.results[:0:0], run.results...)
	run.mu.Unlock()
	if len(results) == 0 {
		return FindFeedInfo{}, errors.New("There are no results to show in the panel.")
	}
	feed := &findFeed{back: back}
	info := FindFeedInfo{}
	archives := map[string]bool{}
	plain := 0
	for _, r := range results {
		if r.Archive != "" {
			archives[r.Archive] = true
		} else {
			plain++
		}
	}
	if len(archives) == 1 && plain == 0 {
		// all in one archive: its members, relative to the archive
		for a := range archives {
			feed.base = a
		}
		for _, r := range results {
			name := r.Inner
			if !strings.Contains(r.Path, "://") {
				name = filepath.FromSlash(name)
			}
			feed.items = append(feed.items, findFeedItem{name: name, real: r.Path, member: true, size: r.Size, mtime: r.ModTime.UnixMilli(), dir: r.IsDir})
		}
	} else {
		seen := map[string]bool{}
		var reals []string
		for _, r := range results {
			p := r.Path
			if r.Archive != "" {
				p = r.Archive
				info.Note = "Archives are listed instead of the files found in them."
			}
			if !seen[p] {
				seen[p] = true
				reals = append(reals, p)
			}
		}
		feed.base = findCommonDir(reals)
		for _, p := range reals {
			feed.items = append(feed.items, findFeedItem{name: findRelName(feed.base, p), real: p})
		}
	}
	findFeeds.mu.Lock()
	findFeeds.seq++
	feed.root = fmt.Sprintf("%s://results-%d", findFeedScheme, findFeeds.seq)
	findFeeds.feeds[feed.root] = feed
	findFeeds.mu.Unlock()
	info.Path, info.Base, info.Count = feed.root, feed.base, len(feed.items)
	return info, nil
}

// FeedBase is the folder the names of a result list are relative to (for the page's path hook).
func (f *Find) FeedBase(p string) (string, error) {
	feed := findFeeds.get(p)
	if feed == nil {
		return "", errFindFeedGone
	}
	return feed.base, nil
}

// findCommonDir is the deepest folder that holds every path ("" when there is none, as for two Windows drives or
// remote and local files together).
func findCommonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	for _, p := range paths {
		if strings.Contains(p, "://") {
			return ""
		}
	}
	base := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		for !findUnder(p, base) {
			up := filepath.Dir(base)
			if up == base {
				return ""
			}
			base = up
		}
	}
	return base
}

func findUnder(p, dir string) bool {
	if strings.HasSuffix(dir, string(filepath.Separator)) {
		return strings.HasPrefix(p, dir) && len(p) > len(dir)
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}

func findRelName(base, p string) string {
	if base == "" {
		return p
	}
	if strings.HasSuffix(base, string(filepath.Separator)) {
		return strings.TrimPrefix(p, base)
	}
	return strings.TrimPrefix(p, base+string(filepath.Separator))
}

// findFeedFS is the vfs.FS of result lists.
type findFeedFS struct{}

var errFindFeedGone = errors.New("these search results are gone (result lists are not kept after the app ends); search again")

func (findFeedFS) List(ctx context.Context, p string) (vfs.Listing, error) {
	root, name := findFeedSplit(p)
	feed := findFeeds.get(root)
	if feed == nil {
		return vfs.Listing{}, errFindFeedGone
	}
	if name != "" {
		return vfs.Listing{}, fs.ErrNotExist
	}
	findFeeds.mu.Lock()
	items := append([]findFeedItem(nil), feed.items...)
	back := feed.back
	findFeeds.mu.Unlock()
	keep := items[:0]
	entries := make([]vfs.Entry, 0, len(items))
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return vfs.Listing{}, err
		}
		var e vfs.Entry
		if it.member {
			e = vfs.Entry{Size: it.size, MTime: it.mtime}
			if it.dir {
				e.Flags = vfs.FlagDir
			}
		} else {
			st, err := findFeeds.fsys(it.real).Stat(it.real)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue // deleted or moved away: it leaves the list
				}
				st = vfs.Entry{}
			}
			e = st
		}
		e.Name = it.name
		entries = append(entries, e)
		keep = append(keep, it)
	}
	findFeeds.mu.Lock()
	feed.items = keep
	findFeeds.mu.Unlock()
	return vfs.Listing{Path: root, Parent: back, Entries: entries}, nil
}

// real finds the real path of a name in a list.
func findFeedReal(p string) (string, *findFeedItem, bool) {
	root, name := findFeedSplit(p)
	feed := findFeeds.get(root)
	if feed == nil || name == "" {
		return "", nil, false
	}
	findFeeds.mu.Lock()
	defer findFeeds.mu.Unlock()
	for i := range feed.items {
		if feed.items[i].name == name || filepath.ToSlash(feed.items[i].name) == name {
			it := feed.items[i]
			return it.real, &it, true
		}
	}
	return "", nil, false
}

func (findFeedFS) Stat(p string) (vfs.Entry, error) {
	root, name := findFeedSplit(p)
	if findFeeds.get(root) == nil {
		return vfs.Entry{}, errFindFeedGone
	}
	if name == "" {
		return vfs.Entry{Name: "Search results", Flags: vfs.FlagDir}, nil
	}
	real, it, ok := findFeedReal(p)
	if !ok {
		return vfs.Entry{}, fs.ErrNotExist
	}
	if it.member {
		e := vfs.Entry{Name: it.name, Size: it.size, MTime: it.mtime}
		if it.dir {
			e.Flags = vfs.FlagDir
		}
		return e, nil
	}
	e, err := findFeeds.fsys(real).Stat(real)
	e.Name = it.name
	return e, err
}

func (findFeedFS) Clean(p string) string {
	root, name := findFeedSplit(strings.TrimSpace(p))
	if name == "" {
		return root
	}
	return root + "/" + name
}

func (findFeedFS) Parent(p string) string {
	root, name := findFeedSplit(p)
	if name != "" {
		return root
	}
	if feed := findFeeds.get(root); feed != nil {
		return feed.back
	}
	return ""
}

func (findFeedFS) Root(p string) string { root, _ := findFeedSplit(p); return root }

func (findFeedFS) Join(dir, name string) string {
	root, sub := findFeedSplit(dir)
	if sub == "" {
		if real, _, ok := findFeedReal(root + "/" + name); ok {
			return real
		}
	}
	return strings.TrimRight(dir, "/") + "/" + name
}
