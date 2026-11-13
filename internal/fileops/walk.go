package fileops

// Before a job touches anything it lists what it will work on, so the progress dialog can show totals and the free
// space can be checked. The walk is the engine's own (not filepath.WalkDir) because it must do three things the
// standard walk doesn't: follow links to folders on request while catching loops (a link pointing to one of its own
// parent folders), apply TC's "Only files of this type" filter (keeping only the folders that hold a matching file),
// and go through the long-path wrappers on Windows. Folders always come before their contents, so walking the list
// backwards is depth first, which is the order deletes need.

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// item is one thing a job works on: a folder, a file or a link, with where it goes.
type item struct {
	src, dst string
	info     fs.FileInfo // from Lstat; from Stat when a followed link stands for what it points to
	top      int         // index of the source it belongs to
	topSrc   string      // that source
	same     bool        // a move on the same volume: files are renamed, not copied
}

// walkOpts says how a walk treats links and names.
type walkOpts struct {
	follow bool // follow links (copy with LinksFollow)
	skipLn bool // leave links out (copy with LinksSkip)
	filter bool // apply the job's Filter
}

// walk lists src (whose Lstat is info) and everything under it, with their places under dst.
func (j *job) walk(top int, src, dst string, info fs.FileInfo, o walkOpts) []item {
	var out []item
	var visit func(src, dst string, info fs.FileInfo, parents []fs.FileInfo, root bool)
	visit = func(src, dst string, info fs.FileInfo, parents []fs.FileInfo, root bool) {
		if j.ctx.Err() != nil {
			return
		}
		if isLink(info) {
			if o.skipLn {
				return
			}
			if o.follow {
				fi, err := stat(src)
				if err != nil {
					j.fail("Can't follow the link %s: %s.", src, plainErr(err))
					return
				}
				info = fi
			}
		}
		name := filepath.Base(src)
		if !info.IsDir() || isLink(info) {
			if o.filter && !j.filter.MatchFile(name) {
				return
			}
			out = append(out, item{src: src, dst: dst, info: info, top: top})
			return
		}
		if o.filter && !root && !j.filter.MatchDir(name) {
			return
		}
		for _, p := range parents {
			if sameFile(p, info) {
				j.fail("Skipped %s: the link leads back into a folder that contains it.", src)
				return
			}
		}
		out = append(out, item{src: src, dst: dst, info: info, top: top})
		entries, err := readDir(src)
		if err != nil {
			j.fail("Can't read %s: %s.", src, plainErr(err))
			return
		}
		parents = append(parents, info)
		for _, e := range entries {
			child := filepath.Join(src, e.Name())
			ci, err := e.Info()
			if err != nil {
				if ci, err = lstat(child); err != nil {
					j.fail("Can't read %s: %s.", child, plainErr(err))
					continue
				}
			}
			cdst := ""
			if dst != "" {
				cdst = filepath.Join(dst, e.Name())
			}
			visit(child, cdst, ci, parents, false)
		}
	}
	visit(src, dst, info, nil, true)
	for i := range out {
		out[i].topSrc = src
	}
	if o.filter && len(j.filter.Include) > 0 { // an exclude-only filter keeps empty folders (F-460)
		out = keepNeededDirs(out, src)
	}
	return out
}

// keepNeededDirs drops the folders that hold no file the filter took, so a filtered copy doesn't leave a tree of
// empty folders behind. It is used only for filters with Include patterns ("*.txt"): a filter that merely excludes
// some names ("|*.bak") still copies the rest of the tree as it is, empty folders included.
func keepNeededDirs(items []item, top string) []item {
	need := map[string]bool{}
	for _, it := range items {
		if it.info.IsDir() {
			continue
		}
		for d := filepath.Dir(it.src); ; d = filepath.Dir(d) {
			if need[d] {
				break
			}
			need[d] = true
			if d == top || !strings.HasPrefix(d, top) || filepath.Dir(d) == d {
				break
			}
		}
	}
	out := items[:0]
	for _, it := range items {
		if !it.info.IsDir() || need[it.src] {
			out = append(out, it)
		}
	}
	return out
}
