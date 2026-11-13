package vfs

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Mock is a fictional file system for screenshots and demos (--demo): a home folder of a made-up user "demo", a few
// volumes, and one folder with 100,000 files to show that big folders scroll smoothly. Nothing on the real disk is read
// or changed. Paths use "/" on every platform. Everything is generated from fixed seeds, so screenshots look the same
// on every run.
type Mock struct {
	dirs  map[string][]Entry
	links map[string]string // link path → target
}

// MockBigFolder is the folder with MockBigCount files (for virtual-scrolling checks).
const (
	MockHome      = "/Users/demo"
	MockBigFolder = "/Users/demo/Projects/Big folder"
	MockBigCount  = 100000
)

// mockNow is the fixed "today" of the demo, so dates in screenshots don't change.
var mockNow = time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)

// NewMock builds the fictional tree.
func NewMock() *Mock {
	m := &Mock{dirs: map[string][]Entry{}, links: map[string]string{}}
	dir := func(p string, names ...string) {
		m.ensure(p)
		for _, n := range names {
			if strings.HasSuffix(n, "/") {
				m.addDir(p, strings.TrimSuffix(n, "/"))
			} else {
				m.addFile(p, n)
			}
		}
	}
	dir("/", "Applications/", "Library/", "System/", "Users/", "Volumes/", "tmp/", ".file")
	dir("/Applications", "Calculator.app/", "Calendar.app/", "mormiCommander.app/", "mormiDiskFree.app/", "mormiView.app/", "Notes.app/", "Safari.app/")
	dir("/Library", "Fonts/", "Preferences/")
	dir("/System", "Library/")
	dir("/Users", "demo/", "Shared/")
	dir("/tmp")
	dir("/Volumes", "Backup/", "USB stick/")
	dir("/Volumes/Backup", "Archive 2024/", "Archive 2025/", "backup.log")
	dir("/Volumes/Backup/Archive 2024", "Photos 2024.zip", "Documents 2024.tar.gz")
	dir("/Volumes/Backup/Archive 2025", "Photos 2025.zip", "Documents 2025.tar.gz")
	dir("/Volumes/USB stick", "Slides.pdf", "Video.mp4", "Readme.txt")
	h := MockHome
	dir(h, "Desktop/", "Documents/", "Downloads/", "Music/", "Pictures/", "Projects/", ".config/", ".bashrc", ".zshrc", ".gitconfig", "notes.txt", "todo.md")
	dir(h+"/.config", "git/", "settings.json")
	dir(h+"/Desktop", "Screenshot 2026-10-12.png", "Shopping list.txt", "Meeting notes.md")
	dir(h+"/Documents", "Letters/", "Recipes/", "Taxes/", "Budget 2026.xlsx", "Car insurance.pdf", "CV - Anna Kovacs.docx", "Garden plan.png", "Notes.txt", "Passwords - not here.txt", "Reading list.md")
	dir(h+"/Documents/Letters", "To the landlord.docx", "Thank you note.docx")
	dir(h+"/Documents/Recipes", "Goulash.md", "Lecso.md", "Pancakes.md", "Plum dumplings.md")
	dir(h+"/Documents/Taxes", "2024.pdf", "2025.pdf", "Receipts.zip")
	dir(h+"/Downloads", "go1.27.darwin-arm64.pkg", "Holiday photos.zip", "Invoice 0042.pdf", "manual.pdf", "song.mp3", "setup.exe")
	dir(h+"/Music", "Albums/", "Playlists/", "Favourite.m3u")
	dir(h+"/Music/Albums", "Blue Train/", "Kind of Blue/")
	dir(h+"/Music/Albums/Blue Train", "01 Blue Train.flac", "02 Moment's Notice.flac", "03 Locomotion.flac")
	dir(h+"/Music/Albums/Kind of Blue", "01 So What.flac", "02 Freddie Freeloader.flac", "03 Blue in Green.flac")
	dir(h+"/Music/Playlists", "Morning.m3u", "Work.m3u")
	dir(h+"/Pictures", "Family/", "Holiday 2026/", "Wallpapers/", "avatar.png")
	dir(h+"/Pictures/Family", "Birthday.jpg", "Christmas.jpg", "Grandma.jpg")
	dir(h+"/Pictures/Wallpapers", "Lake Balaton.jpg", "Mountains.jpg", "Night sky.jpg")
	m.ensure(h + "/Pictures/Holiday 2026")
	for i := 1; i <= 48; i++ {
		m.addFile(h+"/Pictures/Holiday 2026", fmt.Sprintf("IMG_%04d.jpg", 4200+i))
	}
	dir(h+"/Projects", "Big folder/", "mormi-notes/", "website/", "ideas.txt")
	dir(h+"/Projects/mormi-notes", "cmd/", "README.md", "go.mod", "go.sum", "main.go", "main_test.go", "Makefile", "build.sh")
	dir(h+"/Projects/mormi-notes/cmd", "notes/")
	dir(h+"/Projects/mormi-notes/cmd/notes", "main.go")
	dir(h+"/Projects/website", "images/", "index.html", "style.css", "app.js", "favicon.ico")
	dir(h+"/Projects/website/images", "logo.svg", "hero.jpg")
	m.ensure(MockBigFolder) // filled on demand, see List
	m.addLink(h, "Latest photos", h+"/Pictures/Holiday 2026")
	m.addLink(h+"/Desktop", "Old project", h+"/Projects/gone") // a broken link
	// the build script and a few others are executable
	m.setFlag(h+"/Projects/mormi-notes", "build.sh", FlagExec)
	m.setFlag("/", ".file", FlagHidden)
	return m
}

func (m *Mock) ensure(p string) {
	if _, ok := m.dirs[p]; !ok {
		m.dirs[p] = []Entry{}
	}
}

// seed is a stable number for a path, from which its size and date are made.
func seed(p string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(p))
	return h.Sum64()
}

func mockTime(p string) int64 {
	s := seed(p)
	return mockNow.Add(-time.Duration(s%(400*24)) * time.Hour).Add(-time.Duration(s>>20%60) * time.Minute).UnixMilli()
}

// mockSize makes a believable size from the extension and the path.
func mockSize(p string) int64 {
	s := int64(seed(p) % 1000)
	_, ext := SplitExt(path.Base(p))
	switch strings.ToLower(ext) {
	case "jpg", "png":
		return 1_200_000 + s*4_100
	case "flac":
		return 28_000_000 + s*31_000
	case "mp3":
		return 4_000_000 + s*7_000
	case "mp4":
		return 640_000_000 + s*1_000_000
	case "zip", "gz", "pkg":
		return 80_000_000 + s*900_000
	case "pdf":
		return 180_000 + s*2_300
	case "docx", "xlsx":
		return 24_000 + s*90
	case "exe":
		return 12_000_000 + s*10_000
	}
	return 300 + s*13
}

func (m *Mock) addDir(dir, name string) {
	p := path.Join(dir, name)
	m.dirs[dir] = append(m.dirs[dir], Entry{Name: name, MTime: mockTime(p), Flags: FlagDir | hiddenFlag(name), Mode: uint32(os.ModeDir | 0o755)})
	m.ensure(p)
}

func (m *Mock) addFile(dir, name string) {
	p := path.Join(dir, name)
	m.dirs[dir] = append(m.dirs[dir], Entry{Name: name, Size: mockSize(p), MTime: mockTime(p), Flags: hiddenFlag(name), Mode: 0o644})
}

func (m *Mock) addLink(dir, name, target string) {
	p := path.Join(dir, name)
	m.links[p] = target
	e := Entry{Name: name, MTime: mockTime(p), Flags: FlagLink, Mode: uint32(os.ModeSymlink | 0o755)}
	if _, ok := m.dirs[target]; ok {
		e.Flags |= FlagDir
	} else {
		e.Flags |= FlagBroken
	}
	m.dirs[dir] = append(m.dirs[dir], e)
}

func (m *Mock) setFlag(dir, name string, flag int) {
	for i := range m.dirs[dir] {
		if m.dirs[dir][i].Name == name {
			m.dirs[dir][i].Flags |= flag
			if flag == FlagExec {
				m.dirs[dir][i].Mode = 0o755
			}
		}
	}
}

func hiddenFlag(name string) int {
	if strings.HasPrefix(name, ".") {
		return FlagHidden
	}
	return 0
}

// resolve follows mock links in p, so "/Users/demo/Latest photos" lists the holiday folder.
func (m *Mock) resolve(p string) string {
	for i := 0; i < 8; i++ {
		t, ok := m.links[p]
		if !ok {
			// a link can also be a parent of p
			changed := false
			for l, tgt := range m.links {
				if strings.HasPrefix(p, l+"/") {
					p = tgt + p[len(l):]
					changed = true
					break
				}
			}
			if !changed {
				return p
			}
			continue
		}
		p = t
	}
	return p
}

// List returns the fictional folder p.
func (m *Mock) List(ctx context.Context, p string) (Listing, error) {
	p = m.Clean(p)
	real := m.resolve(p)
	if real == MockBigFolder {
		es := make([]Entry, MockBigCount)
		for i := range es {
			name := fmt.Sprintf("file%06d.dat", i+1)
			es[i] = Entry{Name: name, Size: int64(seed(name)%5_000_000) + 100, MTime: mockNow.Add(-time.Duration(i) * time.Minute).UnixMilli(), Mode: 0o644}
		}
		return Listing{Path: p, Parent: slashParent(p), Entries: es}, nil
	}
	es, ok := m.dirs[real]
	if !ok {
		if _, err := m.Stat(p); err == nil {
			return Listing{}, ErrNotFolder
		}
		return Listing{}, &os.PathError{Op: "open", Path: p, Err: os.ErrNotExist}
	}
	out := append([]Entry(nil), es...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return Listing{Path: p, Parent: slashParent(p), Entries: out}, nil
}

// Stat describes one fictional path.
func (m *Mock) Stat(p string) (Entry, error) {
	p = m.Clean(p)
	if p == "/" {
		return Entry{Name: "/", Flags: FlagDir, Mode: uint32(os.ModeDir | 0o755)}, nil
	}
	dir, name := path.Split(p)
	dir = m.resolve(m.Clean(dir))
	if dir == MockBigFolder && strings.HasPrefix(name, "file") {
		return Entry{Name: name, Size: int64(seed(name)%5_000_000) + 100, Mode: 0o644}, nil
	}
	for _, e := range m.dirs[dir] {
		if e.Name == name {
			return e, nil
		}
	}
	return Entry{}, &os.PathError{Op: "stat", Path: p, Err: os.ErrNotExist}
}

// LinkTarget is where a mock link points ("" for a path that is no link).
func (m *Mock) LinkTarget(p string) string { return m.links[m.Clean(p)] }

// Clean makes p a clean absolute "/" path. "~" is the demo home folder.
func (m *Mock) Clean(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = MockHome + p[1:]
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// Parent is the folder above p.
func (m *Mock) Parent(p string) string { return slashParent(m.Clean(p)) }

// Root is the volume root of p: "/Volumes/X" for the mock volumes, "/" for the rest.
func (m *Mock) Root(p string) string {
	p = m.Clean(p)
	if rest, ok := strings.CutPrefix(p, "/Volumes/"); ok && rest != "" {
		name, _, _ := strings.Cut(rest, "/")
		return "/Volumes/" + name
	}
	return "/"
}

// Join puts name under dir.
func (m *Mock) Join(dir, name string) string { return path.Join(dir, name) }
