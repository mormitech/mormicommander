// This file turns the flat list of records stored in an archive into the
// folder tree the panel shows. Archives store full paths ("a/b/c.txt") and
// often omit records for the folders themselves, may use backslashes when
// made on Windows, and malicious ones carry absolute paths or ".." elements to
// escape the target folder when unpacked ("zip slip"). CleanPath normalizes a
// stored name into a safe relative slash path, and Index adds the implied
// folders and answers ReadDir/Stat quickly, so each format only has to supply
// its records.
package archive

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// CleanPath normalizes a name stored in an archive. Backslashes become
// slashes, drive letters, UNC prefixes and leading slashes are removed, "."
// elements are dropped and ".." elements are resolved without ever going above
// the root. insecure reports whether the name tried to escape (absolute path
// or ".." that climbed above its own start), so the UI can warn about it. The
// result has no leading or trailing slash; the root is "".
func CleanPath(raw string) (clean string, insecure bool) {
	p := strings.ReplaceAll(raw, "\\", "/")
	// Drive letter ("C:" or "C:/...") makes the path absolute on Windows.
	if len(p) >= 2 && p[1] == ':' && isASCIILetter(p[0]) {
		p = p[2:]
		insecure = true
	}
	if strings.HasPrefix(p, "/") {
		insecure = true
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			insecure = true
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/"), insecure
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// Index is the folder tree built from an archive's records. Formats embed or
// hold one and forward Entries, ReadDir and Stat to it.
type Index struct {
	entries  []Entry
	byPath   map[string]int   // path -> index of the last entry with that path
	children map[string][]int // folder path -> indexes of direct children
}

// NewIndex builds the tree. Each record must have ID, RawName and the data
// fields filled in; Path, Name and Insecure are computed here from RawName
// (when Path is empty) so every format cleans names the same way. Records
// whose name cleans to the root are dropped. Missing parent folders are added
// as implied folders with ID -1.
func NewIndex(records []Entry) *Index {
	ix := &Index{
		byPath:   make(map[string]int, len(records)),
		children: make(map[string][]int),
	}
	for _, r := range records {
		if r.Path == "" {
			r.Path, r.Insecure = CleanPath(r.RawName)
		}
		if r.Path == "" {
			continue
		}
		r.Name = path.Base(r.Path)
		if r.IsDir {
			r.Mode |= fs.ModeDir
			r.DOSAttrs |= AttrDir
		}
		ix.add(r)
	}
	// Add implied parents for every entry. Iterating by index lets us append
	// while looping; implied folders get their own parents added in turn.
	for i := 0; i < len(ix.entries); i++ {
		ix.ensureDir(path.Dir(ix.entries[i].Path), ix.entries[i].ModTime)
	}
	for k := range ix.children {
		kids := ix.children[k]
		sort.SliceStable(kids, func(a, b int) bool {
			return ix.entries[kids[a]].Name < ix.entries[kids[b]].Name
		})
	}
	return ix
}

func (ix *Index) add(e Entry) {
	i := len(ix.entries)
	ix.entries = append(ix.entries, e)
	if prev, ok := ix.byPath[e.Path]; ok && ix.entries[prev].IsDir && e.IsDir {
		// A second record for the same folder: keep one child slot. The
		// later record wins for Stat, matching zip tools.
		ix.byPath[e.Path] = i
		kids := ix.children[parentOf(e.Path)]
		for k, v := range kids {
			if v == prev {
				kids[k] = i
			}
		}
		return
	}
	ix.byPath[e.Path] = i
	parent := parentOf(e.Path)
	ix.children[parent] = append(ix.children[parent], i)
}

func (ix *Index) ensureDir(dir string, mt time.Time) {
	if dir == "." || dir == "" {
		return
	}
	if i, ok := ix.byPath[dir]; ok && ix.entries[i].IsDir {
		return
	}
	ix.add(Entry{
		ID: -1, Path: dir, Name: path.Base(dir), RawName: dir,
		IsDir: true, Implied: true, Mode: fs.ModeDir | 0o755, DOSAttrs: AttrDir,
		ModTime: mt,
	})
}

func parentOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	return d
}

// Entries returns all entries, records first, implied folders last.
func (ix *Index) Entries() []Entry { return ix.entries }

// Stat returns the entry at name. The root ("" or "/") is a synthetic folder.
func (ix *Index) Stat(name string) (Entry, error) {
	name, _ = CleanPath(name)
	if name == "" {
		return Entry{ID: -1, IsDir: true, Implied: true, Mode: fs.ModeDir | 0o755, DOSAttrs: AttrDir}, nil
	}
	i, ok := ix.byPath[name]
	if !ok {
		return Entry{}, fmt.Errorf("%w: %s", fs.ErrNotExist, name)
	}
	return ix.entries[i], nil
}

// ReadDir returns the direct children of dir sorted by name.
func (ix *Index) ReadDir(dir string) ([]Entry, error) {
	dir, _ = CleanPath(dir)
	if dir != "" {
		e, err := ix.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !e.IsDir {
			return nil, fmt.Errorf("archive: %s is not a folder", dir)
		}
	}
	kids := ix.children[dir]
	out := make([]Entry, len(kids))
	for k, i := range kids {
		out[k] = ix.entries[i]
	}
	return out, nil
}

// Under returns the entry at p (if any) and every entry below it, in archive
// order. With p == "" it returns everything. This is what "select a folder and
// press F5" needs.
func (ix *Index) Under(p string) []Entry {
	p, _ = CleanPath(p)
	var out []Entry
	for _, e := range ix.entries {
		if IsUnder(e.Path, p) {
			out = append(out, e)
		}
	}
	return out
}

// IsUnder reports whether entry path p equals dir or lies below it. An empty
// dir contains everything.
func IsUnder(p, dir string) bool {
	if dir == "" {
		return true
	}
	return p == dir || strings.HasPrefix(p, dir+"/")
}
