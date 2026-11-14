package main

// Branch view (F-086 cm_DirBranch, Ctrl+B; F-087 cm_DirBranchSel, Ctrl+Shift+B): a panel shows every file of a folder
// and all its subfolders as one flat list, each named by its path relative to the folder ("sub/deeper/a.txt"), with
// the real file's size, date and attributes. It is a folder of its own for the panels, like Feed to listbox
// (feature_find_feed.go), so the panel code needs nothing new: the page maps each name back to the real file by
// joining it to the folder (frontend/src/features/branch.js adds a Panel path hook), and every file command (view,
// edit, copy, move, delete, Multi-rename) works on the real files.
//
// Two kinds of paths:
//
//   - branch://<folder> (branch:///Users/me/photos, branch://C:\photos): every file under the folder. The path says it
//     all, so it needs no state, survives a restart in a tab, and a branch path with a name appended is the real
//     file's path with the scheme in front.
//   - branchsel://N: only the selected files and the files of the selected folders of a folder (F-087). The selection
//     is kept here while the app runs, like the result lists of Feed to listbox.
//
// The ".." of both goes back to the folder itself (TC: Ctrl+B again, or "..", leaves the branch view). Listing the
// path walks the tree again (internal/branch), so Ctrl+R and the reread after a file job show the tree as it is now.
// The walk runs inside FS.List, so it is off the page's thread, a newer listing or Esc (FS.CancelList) stops it, and
// its file count can be watched through Branch.Progress while it runs. Big trees are cut at branchLimit files.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"mormicommander/internal/branch"
	"mormicommander/internal/vfs"
)

const (
	branchScheme    = "branch"
	branchSelScheme = "branchsel"
	// branchLimit is the most files a branch view lists: enough for any project or photo tree, small enough that a
	// branch view of a whole disk still answers (the page says when the list was cut).
	branchLimit = 200000
)

func init() {
	vfs.Register(branchScheme, branchFS{})
	vfs.Register(branchSelScheme, branchFS{})
	registerFeature("branch", func(a *App) []exposed { return []exposed{expose(&Branch{app: a})} })
}

// branchPath says whether p is a branch view.
func branchPath(p string) bool {
	p = strings.TrimSpace(p)
	return strings.HasPrefix(p, branchScheme+"://") || strings.HasPrefix(p, branchSelScheme+"://")
}

// branchSel is the selection a branchsel:// view lists.
type branchSel struct {
	base  string   // the folder
	roots []string // the selected names in it
}

// branchWalk is a listing in progress.
type branchWalk struct {
	path  string
	files atomic.Int64
}

// BranchInfo is how the last listing of a branch view went.
type BranchInfo struct {
	Path      string `json:"path"`
	Base      string `json:"base"`      // the folder the names are relative to
	Files     int    `json:"files"`     // files listed
	Truncated bool   `json:"truncated"` // the limit cut the list
	Limit     int    `json:"limit"`
	Skipped   int    `json:"skipped"` // subfolders that could not be read
}

// BranchProgress is a listing still running: the branch view and the files found so far.
type BranchProgress struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
}

type branchStore struct {
	mu    sync.Mutex
	seq   int
	sels  map[string]*branchSel
	walks map[*branchWalk]bool
	last  map[string]BranchInfo
}

var branches = &branchStore{sels: map[string]*branchSel{}, walks: map[*branchWalk]bool{}, last: map[string]BranchInfo{}}

// branchSplit cuts a branch path into its kind and the rest: for branch:// the real folder (or a real path under it),
// for branchsel:// the view's root (branchsel://N) and the name under it.
func branchSplit(p string) (sel bool, rest, root, name string) {
	p = strings.TrimSpace(p)
	if s, ok := strings.CutPrefix(p, branchSelScheme+"://"); ok {
		s = strings.TrimRight(s, `/\`)
		id, n := s, ""
		if i := strings.IndexAny(s, `/\`); i >= 0 {
			id, n = s[:i], s[i+1:]
		}
		return true, s, branchSelScheme + "://" + id, n
	}
	return false, strings.TrimPrefix(p, branchScheme+"://"), "", ""
}

// branchReal turns a branch path into the real path it stands for: the folder for the view itself, the real file for
// a name in it. ok is false for paths that are not branch views, and for a selection that is gone.
func branchReal(p string) (string, bool) {
	if !branchPath(p) {
		return p, false
	}
	sel, rest, root, name := branchSplit(p)
	if !sel {
		if rest == "" {
			return "", false
		}
		return vfs.Local{}.Clean(rest), true
	}
	s := branches.sel(root)
	if s == nil {
		return "", false
	}
	if name == "" {
		return s.base, true
	}
	return filepath.Join(s.base, filepath.FromSlash(name)), true
}

func (s *branchStore) sel(root string) *branchSel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sels[root]
}

// Branch is what the page calls for the branch view (window.go.main.Branch).
type Branch struct{ app *App }

// Select makes the branch view of some files and folders of a folder (F-087) and returns its panel path.
func (b *Branch) Select(base string, names []string) (string, error) {
	if b.app.mock != nil {
		return "", errors.New("This is the demo: its files are not real, so there is no branch view.")
	}
	if strings.Contains(base, "://") {
		return "", errors.New("The branch view works only for folders on this computer.")
	}
	if len(names) == 0 {
		return "", errors.New("Select the folders to list first.")
	}
	base = vfs.Local{}.Clean(base)
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("The folder %s is not there.", base)
	}
	roots := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		n = filepath.Clean(filepath.FromSlash(n))
		if n == "." || n == ".." || filepath.IsAbs(n) || strings.HasPrefix(n, ".."+string(filepath.Separator)) || seen[n] {
			continue
		}
		seen[n] = true
		roots = append(roots, n)
	}
	if len(roots) == 0 {
		return "", errors.New("Select the folders to list first.")
	}
	branches.mu.Lock()
	branches.seq++
	root := fmt.Sprintf("%s://%d", branchSelScheme, branches.seq)
	branches.sels[root] = &branchSel{base: base, roots: roots}
	branches.mu.Unlock()
	return root, nil
}

// Progress lists the branch views being read now, with the files found so far.
func (b *Branch) Progress() []BranchProgress {
	branches.mu.Lock()
	defer branches.mu.Unlock()
	out := []BranchProgress{}
	for w := range branches.walks {
		out = append(out, BranchProgress{Path: w.path, Files: int(w.files.Load())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Info tells how the last listing of a branch view went (how many files, whether the limit cut it).
func (b *Branch) Info(path string) BranchInfo {
	c := branchFS{}.Clean(path)
	branches.mu.Lock()
	defer branches.mu.Unlock()
	if i, ok := branches.last[c]; ok {
		return i
	}
	base, _ := branchReal(c)
	return BranchInfo{Path: c, Base: base, Limit: branchLimit}
}

// branchFS is the vfs.FS of branch views.
type branchFS struct{}

var errBranchGone = errors.New("this branch view is gone (the branch view of selected folders is not kept after the app ends); select the folders and press Ctrl+Shift+B again")

func (branchFS) List(ctx context.Context, p string) (vfs.Listing, error) {
	c := branchFS{}.Clean(p)
	sel, _, root, name := branchSplit(c)
	var base string
	var roots []string
	if sel {
		s := branches.sel(root)
		if s == nil {
			return vfs.Listing{}, errBranchGone
		}
		if name != "" {
			return vfs.Listing{}, fs.ErrNotExist
		}
		base, roots = s.base, s.roots
	} else {
		real, ok := branchReal(c)
		if !ok {
			return vfs.Listing{}, fs.ErrNotExist
		}
		base = real
	}
	w := &branchWalk{path: c}
	branches.mu.Lock()
	branches.walks[w] = true
	branches.mu.Unlock()
	defer func() {
		branches.mu.Lock()
		delete(branches.walks, w)
		branches.mu.Unlock()
	}()
	r, err := branch.Walk(ctx, base, roots, branchLimit, func(n int) { w.files.Store(int64(n)) })
	if err != nil {
		return vfs.Listing{}, err
	}
	branches.mu.Lock()
	branches.last[c] = BranchInfo{Path: c, Base: base, Files: len(r.Entries), Truncated: r.Truncated, Limit: branchLimit, Skipped: r.Skipped}
	branches.mu.Unlock()
	return vfs.Listing{Path: c, Parent: base, Entries: r.Entries}, nil
}

func (branchFS) Stat(p string) (vfs.Entry, error) {
	sel, _, root, name := branchSplit(p)
	if sel && branches.sel(root) == nil {
		return vfs.Entry{}, errBranchGone
	}
	real, ok := branchReal(p)
	if !ok {
		return vfs.Entry{}, fs.ErrNotExist
	}
	e, err := vfs.Local{}.Stat(real)
	if sel && name == "" {
		e.Name = "Branch view"
	}
	return e, err
}

func (branchFS) Clean(p string) string {
	sel, rest, root, name := branchSplit(p)
	if sel {
		if name == "" {
			return root
		}
		return root + "/" + filepath.ToSlash(name)
	}
	if rest == "" {
		return branchScheme + "://"
	}
	return branchScheme + "://" + vfs.Local{}.Clean(rest)
}

// Parent of a branch view is the folder it lists: ".." leaves the branch view.
func (branchFS) Parent(p string) string {
	sel, _, root, _ := branchSplit(p)
	if sel {
		if s := branches.sel(root); s != nil {
			return s.base
		}
		return ""
	}
	real, _ := branchReal(p)
	return real
}

func (branchFS) Root(p string) string {
	real, ok := branchReal(p)
	if !ok {
		return ""
	}
	return vfs.Local{}.Root(real)
}

// Join puts a name of the list under the view, which gives the real file.
func (branchFS) Join(dir, name string) string {
	real, ok := branchReal(dir)
	if !ok {
		return strings.TrimRight(dir, `/\`) + "/" + name
	}
	return filepath.Join(real, filepath.FromSlash(name))
}
