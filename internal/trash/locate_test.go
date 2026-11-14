package trash

// Tests of finding a trashed item and putting it back (board issue #13). The real trash is never read or changed:
// trashDirs points at temporary folders, and a fake command runner plays the system's trash, moving the item into the
// fake trash the way each system does (a number added to a name that is taken, a .trashinfo record, a $R/$I pair).

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// fakeTrash points trashDirs at dir for one test.
func fakeTrash(t *testing.T, dirs ...string) {
	t.Helper()
	old := trashDirs
	trashDirs = func(string) []string { return dirs }
	t.Cleanup(func() { trashDirs = old })
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// macName is the name the macOS Trash gives an item whose name is taken: "x 2.txt", "x 3.txt"...
func macName(dir, name string) string {
	if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 2; ; n++ {
		c := fmt.Sprintf("%s %d%s", stem, n, ext)
		if _, err := os.Lstat(filepath.Join(dir, c)); err != nil {
			return c
		}
	}
}

// moveRunner replaces the command runner with one that renames every path argument into dir under the name
// name(dir, base) gives, the way the system's trash does.
func moveRunner(t *testing.T, dir string, name func(dir, base string) string) {
	t.Helper()
	oldRun, oldLook := runCommand, lookPath
	runCommand = func(cmd *exec.Cmd) ([]byte, error) {
		paths := cmd.Args[1:]
		if v := cmd.Env; len(v) > 0 { // Windows passes the path in the environment
			for _, e := range v {
				if p, ok := strings.CutPrefix(e, "MORMICOMMANDER_PATH="); ok {
					paths = []string{p}
				}
			}
		}
		for _, p := range paths {
			if !filepath.IsAbs(p) {
				continue
			}
			if _, err := os.Lstat(p); err != nil {
				continue
			}
			if err := os.Rename(p, filepath.Join(dir, name(dir, filepath.Base(p)))); err != nil {
				return []byte(err.Error()), err
			}
		}
		return nil, nil
	}
	lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	t.Cleanup(func() { runCommand, lookPath = oldRun, oldLook })
}

// TestLocateAndRestoreAfterMove runs the platform's own Move against a fake trash, with the name taken already, then
// finds the item by its identity and puts it back.
func TestLocateAndRestoreAfterMove(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "Trash")
	os.MkdirAll(bin, 0o700)
	fakeTrash(t, bin)
	moveRunner(t, bin, macName)
	writeFile(t, filepath.Join(bin, "report.txt"), "an older report, deleted last week")

	src := filepath.Join(root, "work", "report.txt")
	writeFile(t, src, "this week's report")
	m, ok := Before(src)
	if !ok || m.Path() != src {
		t.Fatal("Before")
	}
	if err := Move(src); err != nil {
		t.Fatal(err)
	}
	got, ok := Locate(m)
	if !ok || filepath.Base(got) != "report 2.txt" {
		t.Fatalf("Locate = %q %v", got, ok)
	}
	if err := Restore(got, src); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(src); string(b) != "this week's report" {
		t.Fatalf("restored %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(bin, "report.txt")); string(b) != "an older report, deleted last week" {
		t.Error("the other item in the trash was touched")
	}

	// a folder, whose own folder went away in the meantime: it is made again
	dir := filepath.Join(root, "gone", "photos")
	writeFile(t, filepath.Join(dir, "a.jpg"), "jpg")
	m, _ = Before(dir)
	if err := Move(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "gone"))
	got, ok = Locate(m)
	if !ok {
		t.Fatal("folder not found")
	}
	if err := Restore(got, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.jpg")); err != nil {
		t.Fatal("folder contents not back")
	}
}

func TestRestoreNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "Trash")
	fakeTrash(t, bin)
	trashed, original := filepath.Join(bin, "a.txt"), filepath.Join(root, "a.txt")
	writeFile(t, trashed, "old")
	writeFile(t, original, "a new file with the same name")
	if err := Restore(trashed, original); !errors.Is(err, ErrOccupied) {
		t.Fatalf("Restore over a file = %v", err)
	}
	if b, _ := os.ReadFile(original); string(b) != "a new file with the same name" {
		t.Error("overwritten")
	}
	if _, err := os.Lstat(trashed); err != nil {
		t.Error("the trashed item moved")
	}
	// only items of the trash are moved
	other := filepath.Join(root, "elsewhere", "b.txt")
	writeFile(t, other, "x")
	if err := Restore(other, filepath.Join(root, "b.txt")); !errors.Is(err, ErrNotInTrash) {
		t.Fatalf("Restore from outside the trash = %v", err)
	}
	if err := Restore("rel/a", original); err == nil {
		t.Error("relative path accepted")
	}
	// not found: nothing in the trash is the same file
	gone := filepath.Join(root, "c.txt")
	writeFile(t, gone, "c")
	m, _ := Before(gone)
	// the other file is made before the original goes, so the file system can't give it the original's inode again
	// (Linux reuses a freed inode number at once, which would make it the "same" file)
	writeFile(t, filepath.Join(bin, "c.txt"), "c") // same name and content, another file
	os.Remove(gone)
	if p, ok := Locate(m); ok {
		t.Fatalf("Locate found %q", p)
	}
	if _, ok := Locate(Mark{}); ok {
		t.Error("an empty mark was found")
	}
}

// TestXDGTrashInfo plays gio on a volume without a trash of its own: the item is copied to the home trash in a
// temporary XDG_DATA_HOME (so it is another file there) with its .trashinfo record, and is found by that record; the
// restore removes the record too.
func TestXDGTrashInfo(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	files := xdgHomeTrash()
	if files != filepath.Join(data, "Trash", "files") {
		t.Fatalf("home trash %q", files)
	}
	info := filepath.Join(data, "Trash", "info")
	fakeTrash(t, files)

	src := filepath.Join(t.TempDir(), "My Notes", "plan 100%.txt")
	writeFile(t, src, "plan")
	m, _ := Before(src)
	trashInfo := func(name, path string, at time.Time) {
		writeFile(t, filepath.Join(info, name+".trashinfo"), "[Trash Info]\nPath="+
			strings.ReplaceAll(strings.ReplaceAll(filepath.ToSlash(path), "%", "%25"), " ", "%20")+
			"\nDeletionDate="+at.Format("2006-01-02T15:04:05")+"\n")
	}
	// an older delete of the same file is in the trash too
	writeFile(t, filepath.Join(files, "plan 100%.txt"), "an older plan")
	trashInfo("plan 100%.txt", src, time.Now().Add(-time.Hour))
	// gio: a copy under a free name, then the original is removed
	copyTo := filepath.Join(files, "plan 100%.2.txt")
	in, _ := os.Open(src)
	out, _ := os.Create(copyTo)
	io.Copy(out, in)
	in.Close()
	out.Close()
	os.Remove(src)
	trashInfo("plan 100%.2.txt", src, time.Now())

	got, ok := Locate(m)
	if !ok || got != copyTo {
		t.Fatalf("Locate = %q %v", got, ok)
	}
	if err := Restore(got, src); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(src); string(b) != "plan" {
		t.Fatalf("restored %q", b)
	}
	if _, err := os.Lstat(filepath.Join(info, "plan 100%.2.txt.trashinfo")); err == nil {
		t.Error("the .trashinfo record was left behind")
	}
	if _, err := os.Lstat(filepath.Join(info, "plan 100%.txt.trashinfo")); err != nil {
		t.Error("the record of the older delete was removed")
	}
}

func TestParseTrashInfo(t *testing.T) {
	p, at, err := parseTrashInfo([]byte("[Trash Info]\nPath=foo/b%C3%A1r.txt\nDeletionDate=2026-11-14T10:20:30\n"), "/mnt/disk")
	if err != nil || p != filepath.FromSlash("/mnt/disk/foo/bár.txt") || at.Format(time.DateTime) != "2026-11-14 10:20:30" {
		t.Fatalf("relative: %q %v %v", p, at, err)
	}
	for _, bad := range []string{"", "[Other]\nPath=/a\nDeletionDate=2026-11-14T10:20:30", "[Trash Info]\nPath=/a\nDeletionDate=soon"} {
		if _, _, err := parseTrashInfo([]byte(bad), "/"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	f := filepath.FromSlash
	if xdgTopDir(f("/mnt/disk/.Trash/1000/files")) != f("/mnt/disk") || xdgTopDir(f("/mnt/disk/.Trash-1000/files")) != f("/mnt/disk") {
		t.Error("xdgTopDir")
	}
}

// recycleRecord builds a "$I" file of the given version.
func recycleRecord(version int, path string, at time.Time) []byte {
	u := append(utf16.Encode([]rune(path)), 0)
	b := make([]byte, 24)
	binary.LittleEndian.PutUint64(b[0:], uint64(version))
	binary.LittleEndian.PutUint64(b[8:], 1234)
	binary.LittleEndian.PutUint64(b[16:], uint64(at.UnixNano()/100)+116444736000000000)
	if version == 2 {
		b = binary.LittleEndian.AppendUint32(b, uint32(len(u)))
	} else {
		u = append(u, make([]uint16, 260-len(u))...)
	}
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

func TestParseRecycleInfo(t *testing.T) {
	at := time.Date(2026, 11, 14, 9, 8, 7, 0, time.UTC)
	for _, v := range []int{1, 2} {
		p, got, err := parseRecycleInfo(recycleRecord(v, `C:\Users\Ann\Desktop\árvíztűrő.txt`, at))
		if err != nil || p != `C:\Users\Ann\Desktop\árvíztűrő.txt` || !got.Equal(at) {
			t.Errorf("version %d: %q %v %v", v, p, got, err)
		}
	}
	short := recycleRecord(2, `C:\a`, at)
	for _, bad := range [][]byte{nil, short[:20], short[:30], append([]byte{9}, short[1:]...)} {
		if _, _, err := parseRecycleInfo(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

// TestRecycleBinRecord plays the Recycle Bin: the item becomes "$R..." with a "$I..." record next to it. Here it is
// found by the record (the test runs on every system, where the old path is a plain one) and the restore removes the
// record.
func TestRecycleBinRecord(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "$Recycle.Bin", "S-1-5-21-1")
	fakeTrash(t, bin)
	src := filepath.Join(root, "docs", "letter.docx")
	writeFile(t, src, "dear")
	m, _ := Before(src)
	writeFile(t, filepath.Join(bin, "$RAB12CD.docx"), "dear")
	os.Remove(src)
	os.WriteFile(filepath.Join(bin, "$IAB12CD.docx"), recycleRecord(2, src, time.Now()), 0o644)
	got, ok := Locate(m)
	if !ok || filepath.Base(got) != "$RAB12CD.docx" {
		t.Fatalf("Locate = %q %v", got, ok)
	}
	if err := Restore(got, src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(bin, "$IAB12CD.docx")); err == nil {
		t.Error("the $I record was left behind")
	}
	if b, _ := os.ReadFile(src); string(b) != "dear" {
		t.Error("not restored")
	}
}

func TestNamePrefixAndRecord(t *testing.T) {
	for in, want := range map[string]string{"report.tar.gz": "report", ".bashrc": ".bashrc", "noext": "noext"} {
		if got := namePrefix(in); got != want {
			t.Errorf("namePrefix(%q) = %q", in, got)
		}
	}
	if got := recordOf(filepath.FromSlash("/h/.local/share/Trash/files/a.txt")); got != filepath.FromSlash("/h/.local/share/Trash/info/a.txt.trashinfo") {
		t.Errorf("xdg record %q", got)
	}
	if got := recordOf(filepath.FromSlash("/C/$Recycle.Bin/S-1/$RX1.txt")); got != filepath.FromSlash("/C/$Recycle.Bin/S-1/$IX1.txt") {
		t.Errorf("recycle record %q", got)
	}
	if recordOf(filepath.FromSlash("/Users/a/.Trash/x.txt")) != "" {
		t.Error("the macOS Trash has no record file")
	}
}
