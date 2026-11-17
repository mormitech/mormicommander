package main

// Options > Find files > Use the system index (board issue #94): a name search asks the system's file index first
// (Spotlight's mdfind on macOS, locate on Linux; Windows Search has no command to ask, so there the search walks the
// folders as always) for the files whose names match, and then walks only the folders on the way to them. Every other
// part of the search (depth, hidden files, dates, sizes, the text inside) still applies to what the index found. Files
// the index has not seen yet (made a moment ago, or in folders it leaves out) are not found this way.

import (
	"context"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mormicommander/internal/find"
)

// findIndexFS walks only toward the paths the index named.
type findIndexFS struct {
	inner find.FS
	keep  map[string]bool // the paths found and every folder above them, cleaned
}

func (x *findIndexFS) Lstat(p string) (fs.FileInfo, error)  { return x.inner.Lstat(p) }
func (x *findIndexFS) Stat(p string) (fs.FileInfo, error)   { return x.inner.Stat(p) }
func (x *findIndexFS) Open(p string) (io.ReadCloser, error) { return x.inner.Open(p) }
func (x *findIndexFS) Join(elem ...string) string           { return x.inner.Join(elem...) }
func (x *findIndexFS) Base(p string) string                 { return x.inner.Base(p) }

func (x *findIndexFS) ReadDir(p string) ([]fs.DirEntry, error) {
	all, err := x.inner.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, e := range all {
		if x.keep[filepath.Clean(x.inner.Join(p, e.Name()))] {
			out = append(out, e)
		}
	}
	return out, nil
}

// findIndexPaths asks the system index for the files under roots whose names match the masks of searchFor ("*.txt
// *.md|old*": the part after "|" is left to the search itself). ok is false when there is no index to ask, the search
// is not a plain name search, or the index did not answer: the search then walks as always.
func findIndexPaths(roots []string, searchFor string) (paths []string, ok bool) {
	masks := strings.Fields(strings.ReplaceAll(strings.SplitN(searchFor, "|", 2)[0], ";", " "))
	if len(masks) == 0 || len(roots) == 0 {
		return nil, false
	}
	for _, m := range masks {
		if strings.ContainsAny(m, `"'\/`) {
			return nil, false
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		for _, root := range roots {
			for _, m := range masks {
				out, err := exec.CommandContext(ctx, "mdfind", "-onlyin", root, "kMDItemFSName == \""+m+"\"c").Output()
				if err != nil {
					return nil, false
				}
				paths = append(paths, findLines(out)...)
			}
		}
		return paths, true
	case "linux":
		if _, err := exec.LookPath("locate"); err != nil {
			return nil, false
		}
		for _, m := range masks {
			out, err := exec.CommandContext(ctx, "locate", "-i", "-b", "--existing", m).Output()
			if err != nil && len(out) == 0 {
				continue // (locate exits 1 when nothing matched)
			}
			for _, p := range findLines(out) {
				for _, root := range roots {
					if pathWithin(p, root) {
						paths = append(paths, p)
						break
					}
				}
			}
		}
		return paths, true
	}
	return nil, false
}

func findLines(b []byte) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// findUseIndex puts the index in front of a local name search when Options > Find files > Use the system index is on.
func findUseIndex(o *find.Options) {
	s, ok := currentSettings()
	if !ok || !s.FindFiles.UseIndex || o.FS != nil || len(o.Within) > 0 || o.NameRegex || o.SearchArchives || len(o.SearchIn) == 0 {
		return
	}
	paths, ok := findIndexPaths(o.SearchIn, o.SearchFor)
	if !ok {
		return
	}
	keep := map[string]bool{}
	for _, p := range paths {
		p = filepath.Clean(p)
		for !keep[p] {
			keep[p] = true
			up := filepath.Dir(p)
			if up == p {
				break
			}
			p = up
		}
	}
	o.FS = &findIndexFS{inner: find.OSFS{}, keep: keep}
}
