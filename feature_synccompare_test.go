package main

// Tests of the thin page layers of Synchronize folders, Compare by content and Compare folders (feature_syncdirs.go,
// feature_cmpfiles.go, feature_dircmp.go): everything runs in temporary folders and without a window, so events go
// nowhere and the tests wait for the background work through the session state.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mormicommander/internal/compare"
	"mormicommander/internal/syncdirs"
)

func scWrite(t *testing.T, p, s string, mt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func scWait(t *testing.T, s *SyncDirs, id int) {
	t.Helper()
	for i := 0; i < 500; i++ {
		ss, _ := s.get(id)
		ss.mu.Lock()
		busy := ss.busy
		ss.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the background work did not finish")
}

func TestSyncDirsCompareAndSync(t *testing.T) {
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", t.TempDir())
	l, r := t.TempDir(), t.TempDir()
	old, now := time.Now().Add(-time.Hour), time.Now()
	scWrite(t, filepath.Join(l, "a.txt"), "new", now)
	scWrite(t, filepath.Join(r, "a.txt"), "old", old)
	scWrite(t, filepath.Join(l, "sub", "b.txt"), "b", now)
	scWrite(t, filepath.Join(r, "c.txt"), "c", now)

	s := &SyncDirs{app: &App{}, sess: map[int]*syncSession{}}
	id := s.Open()
	defer s.Close(id)
	if err := s.Compare(id, SyncCompareRequest{Left: l, Right: r, Options: syncdirs.DefaultOptions()}); err != nil {
		t.Fatal(err)
	}
	scWait(t, s, id)
	v, err := s.Rows(id, syncdirs.ShowAll())
	if err != nil || !v.Compared {
		t.Fatalf("rows: %v %+v", err, v)
	}
	if v.Summary.LeftNewer != 1 || v.Summary.RightOnly != 1 || v.Summary.LeftOnly < 1 {
		t.Fatalf("summary %+v", v.Summary)
	}
	if n, err := s.SetAction(id, []string{"c.txt"}, syncdirs.NoAction, false); err != nil || n != 1 {
		t.Fatalf("set action: %d %v", n, err)
	}
	p, err := s.Plan(id, syncdirs.AllOps())
	if err != nil || p.FilesLR != 2 || p.FilesRL != 0 {
		t.Fatalf("plan %+v %v", p, err)
	}
	if err := s.Synchronize(id, SyncExecRequest{Filter: syncdirs.AllOps()}); err != nil {
		t.Fatal(err)
	}
	scWait(t, s, id)
	if b, _ := os.ReadFile(filepath.Join(r, "a.txt")); string(b) != "new" {
		t.Fatalf("a.txt on the right is %q", b)
	}
	if _, err := os.Stat(filepath.Join(r, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r, "c.txt")); err != nil {
		t.Fatal("c.txt had no action but is gone")
	}
	if err := s.SaveProfile(syncdirs.Profile{Name: "x", Left: l, Right: r, Options: syncdirs.DefaultOptions(), Show: syncdirs.DefaultShow()}); err != nil {
		t.Fatal(err)
	}
	if ps, err := s.Profiles(); err != nil || len(ps) != 1 || ps[0].Name != "x" {
		t.Fatalf("profiles %v %v", ps, err)
	}
	if err := s.Compare(id, SyncCompareRequest{Left: filepath.Join(l, "nope"), Right: r}); err == nil {
		t.Fatal("a missing folder was accepted")
	}
}

func TestCompareFilesEditAndSave(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a.txt"), filepath.Join(d, "b.txt")
	scWrite(t, a, "one\ntwo\nthree\n", time.Now())
	scWrite(t, b, "one\nTWO\nthree\nfour\n", time.Now())
	c := &CompareFiles{app: &App{}, sess: map[int]*cmpSession{}}
	v, err := c.Open(a, b, CmpOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.Binary || v.Identical || len(v.Blocks) == 0 {
		t.Fatalf("view %+v", v)
	}
	if v2, _ := c.SetOptions(v.ID, CmpOptions{IgnoreCase: true}); v2 == nil || len(v2.Blocks) != 1 {
		t.Fatalf("ignore case: %+v", v2)
	}
	v, err = c.CopyAll(v.ID, 0)
	if err != nil || !v.Identical || !v.Modified[1] {
		t.Fatalf("copy all: %v %+v", err, v)
	}
	if _, err := c.Save(v.ID, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(b); string(got) != "one\ntwo\nthree\n" {
		t.Fatalf("saved %q", got)
	}
	v, err = c.SetLine(v.ID, 0, 1, "zwei")
	if err != nil || v.Rows[1].A != "zwei" {
		t.Fatalf("set line: %v %+v", err, v.Rows)
	}
	scWrite(t, b, "x\x00y", time.Now())
	if v, err = c.Reopen(v.ID, a, b); err != nil || !v.Binary {
		t.Fatalf("binary: %v %+v", err, v)
	}
	if bs, err := c.Bytes(v.ID, 0, 4); err != nil || string(bs.B) != "x\x00y" {
		t.Fatalf("bytes %v %v", bs, err)
	}
}

func TestUTF16Spans(t *testing.T) {
	got := utf16Spans("aő😀b", []compare.Span{{Start: 0, End: 1}, {Start: 1, End: 7, Changed: true}, {Start: 7, End: 8}})
	if len(got) != 1 || got[0] != [2]int{1, 4} {
		t.Fatalf("spans %v", got)
	}
}

func TestCompareDirsListingsAndDisk(t *testing.T) {
	l, r := t.TempDir(), t.TempDir()
	now := time.Now()
	scWrite(t, filepath.Join(l, "same.txt"), "s", now)
	scWrite(t, filepath.Join(r, "same.txt"), "s", now)
	scWrite(t, filepath.Join(l, "only.txt"), "o", now)
	scWrite(t, filepath.Join(r, "newer.txt"), "n", now)
	scWrite(t, filepath.Join(l, "newer.txt"), "n", now.Add(-time.Hour))
	c := &CompareDirs{app: &App{}}
	res, err := c.Compare(DirCmpRequest{Left: l, Right: r, Tolerance: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OnDisk || len(res.LeftSelect) != 1 || res.LeftSelect[0] != "only.txt" || len(res.RightSelect) != 1 || res.RightSelect[0] != "newer.txt" {
		t.Fatalf("disk: %+v", res)
	}
	ms := now.UnixMilli()
	res, err = c.Compare(DirCmpRequest{Left: "ftp://x/a", Right: "ftp://x/b",
		LeftEntries:  []DirCmpEntry{{Name: "a", Size: 1, MTime: ms}, {Name: "b", Size: 1, MTime: ms}},
		RightEntries: []DirCmpEntry{{Name: "a", Size: 1, MTime: ms}}})
	if err != nil || res.OnDisk || len(res.LeftSelect) != 1 || res.LeftSelect[0] != "b" || len(res.LeftEqual) != 1 {
		t.Fatalf("listings: %v %+v", err, res)
	}
}

// TestCompareDirsOneSidedFolders checks the folder modes Shift+F2 can run with (board issue #9): plain compare leaves a
// folder that exists on one side only alone, "missing" (the Mark menu's "mark one-sided folders", or plain Shift+F2
// with compareDirs.oneSidedFolders on) selects it, on disk and for listings of other panels.
func TestCompareDirsOneSidedFolders(t *testing.T) {
	l, r := t.TempDir(), t.TempDir()
	now := time.Now()
	scWrite(t, filepath.Join(l, "lonely", "x.txt"), "x", now)
	scWrite(t, filepath.Join(l, "both", "y.txt"), "y", now)
	scWrite(t, filepath.Join(r, "both", "y.txt"), "y", now)
	scWrite(t, filepath.Join(r, "rightdir", "z.txt"), "z", now)
	c := &CompareDirs{app: &App{}}
	has := func(s []string, n string) bool {
		for _, x := range s {
			if x == n {
				return true
			}
		}
		return false
	}
	res, err := c.Compare(DirCmpRequest{Left: l, Right: r, Tolerance: 2})
	if err != nil {
		t.Fatal(err)
	}
	if has(res.LeftSelect, "lonely") || has(res.RightSelect, "rightdir") {
		t.Fatalf("plain compare selected folders: %+v", res)
	}
	res, err = c.Compare(DirCmpRequest{Left: l, Right: r, Tolerance: 2, Subdirs: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if !has(res.LeftSelect, "lonely") || !has(res.RightSelect, "rightdir") || has(res.LeftSelect, "both") || has(res.RightSelect, "both") {
		t.Fatalf("missing mode: %+v", res)
	}
	ms := now.UnixMilli()
	res, err = c.Compare(DirCmpRequest{Left: "ftp://x/a", Right: "ftp://x/b", Subdirs: "missing",
		LeftEntries:  []DirCmpEntry{{Name: "d", Dir: true, MTime: ms}, {Name: "both", Dir: true, MTime: ms}},
		RightEntries: []DirCmpEntry{{Name: "both", Dir: true, MTime: ms}}})
	if err != nil || !has(res.LeftSelect, "d") || has(res.LeftSelect, "both") {
		t.Fatalf("listings: %v %+v", err, res)
	}
}
