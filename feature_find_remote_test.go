package main

// Find files on a server folder (F-545): a URL in Search in goes through the panels' file systems (findVFS). The
// server here is an in-memory file system registered as "findmem://", so the test needs no network; the FTP and SFTP
// adapters themselves are tested against local test servers in internal/vfs.

import (
	"context"
	"io/fs"
	"path"
	"reflect"
	"strings"
	"testing"

	"mormicommander/internal/vfs"
)

// findMemFS is a tiny server: full URL paths ("findmem://h/dir/name") to entries; a path ending in "/" is a folder.
type findMemFS struct{ files map[string]int64 }

func (m findMemFS) List(_ context.Context, p string) (vfs.Listing, error) {
	p = m.Clean(p)
	l := vfs.Listing{Path: p, Parent: m.Parent(p)}
	seen := map[string]bool{}
	for k, size := range m.files {
		if !strings.HasPrefix(k, p+"/") {
			continue
		}
		rest := strings.TrimPrefix(k, p+"/")
		name, _, sub := strings.Cut(rest, "/")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		e := vfs.Entry{Name: name, Size: size, MTime: 1700000000000, Mode: 0o644}
		if sub {
			e.Flags, e.Size, e.Mode = vfs.FlagDir, 0, uint32(fs.ModeDir|0o755)
		}
		l.Entries = append(l.Entries, e)
	}
	return l, nil
}

func (m findMemFS) Stat(p string) (vfs.Entry, error) {
	p = m.Clean(p)
	if size, ok := m.files[p]; ok {
		return vfs.Entry{Name: path.Base(p), Size: size, MTime: 1700000000000, Mode: 0o644}, nil
	}
	for k := range m.files {
		if strings.HasPrefix(k, p+"/") {
			return vfs.Entry{Name: path.Base(p), Flags: vfs.FlagDir, MTime: 1700000000000, Mode: uint32(fs.ModeDir | 0o755)}, nil
		}
	}
	return vfs.Entry{}, fs.ErrNotExist
}

func (findMemFS) Clean(p string) string { return strings.TrimRight(p, "/") }
func (findMemFS) Parent(p string) string {
	if i := strings.LastIndex(p, "/"); i > len("findmem://") {
		return p[:i]
	}
	return ""
}
func (findMemFS) Root(string) string           { return "findmem://h" }
func (findMemFS) Join(dir, name string) string { return strings.TrimRight(dir, "/") + "/" + name }

func init() {
	vfs.Register("findmem", findMemFS{files: map[string]int64{
		"findmem://h/pub/readme.txt":     10,
		"findmem://h/pub/old.txt":        5,
		"findmem://h/pub/big.iso":        5 << 20,
		"findmem://h/pub/vendor/lib.txt": 1,
		"findmem://h/pub/sub/notes.txt":  3,
	}})
}

func TestFindOnServerFolder(t *testing.T) {
	f := newTestFind(t)
	got := findNames(runTestFind(t, f, FindRequest{SearchFor: "*.txt|old* vendor/", SearchIn: "findmem://h/pub", Depth: -1}))
	if want := []string{"notes.txt", "readme.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("server search: %v, want %v", got, want)
	}
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "*", SearchIn: "findmem://h/pub", Depth: 0, Kind: "files", SizeOn: true, SizeOp: ">", SizeValue: 1, SizeUnit: "MB"}))
	if want := []string{"big.iso"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("server size filter: %v, want %v", got, want)
	}
	// a saved search as a filter on a server folder (F-537)
	if err := f.Save("Texts", FindRequest{SearchFor: "*.txt|old*", SearchIn: "x"}, false); err != nil {
		t.Fatal(err)
	}
	idx, err := f.TemplateMatch("Texts", "findmem://h/pub", []string{"readme.txt", "old.txt", "big.iso", "sub"})
	if err != nil || !reflect.DeepEqual(idx, []int{0}) {
		t.Fatalf("TemplateMatch on a server: %v %v", idx, err)
	}
}
