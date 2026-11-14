package main

// Tests of Synchronize folders with an archive or a server as a side (F-555, feature_syncdirs_vfs.go). Everything runs
// in temporary folders: real zip files made here, the 7z sample of the archive engine, and the SFTP and WebDAV test
// servers of the connection tests on 127.0.0.1. Each test compares, synchronizes in both directions and compares again,
// which must then show every file as equal.

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/syncdirs"
)

// scZipFile is one entry of scZip: its content and date.
type scZipFile struct {
	data string
	mt   time.Time
}

// scZip writes a zip file at p with the given entries (a name ending in "/" is a folder).
func scZip(t *testing.T, p string, files map[string]scZipFile) {
	t.Helper()
	out, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for n, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Modified: f.mt, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.data))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
}

// scApp is a SyncDirs with the panels' router, as in the app (the demo has none).
func scApp(t *testing.T) (*SyncDirs, *FS) {
	t.Helper()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", t.TempDir())
	f, _ := vfsTestApp(t)
	return &SyncDirs{app: f.app, sess: map[int]*syncSession{}}, f
}

// scCompare compares and returns all rows; it fails the test on an error.
func scCompare(t *testing.T, s *SyncDirs, id int, l, r string, opts syncdirs.Options) *SyncView {
	t.Helper()
	if err := s.Compare(id, SyncCompareRequest{Left: l, Right: r, Options: opts}); err != nil {
		t.Fatalf("compare %s with %s: %v", l, r, err)
	}
	scWait(t, s, id)
	v, err := s.Rows(id, syncdirs.ShowAll())
	if err != nil || !v.Compared {
		t.Fatalf("rows: %v %+v", err, v)
	}
	if len(v.Errors) > 0 {
		t.Fatalf("compare errors: %v", v.Errors)
	}
	return v
}

// scSync synchronizes everything (no questions, deletes permanent unless trash) and returns the end report.
func scSync(t *testing.T, s *SyncDirs, id int, trash bool) map[string]any {
	t.Helper()
	ss, _ := s.get(id)
	var done map[string]any
	ss.mu.Lock()
	l, r := ss.lside.raw, ss.rside.raw
	ss.mu.Unlock()
	if err := s.Synchronize(id, SyncExecRequest{Filter: syncdirs.AllOps(), Trash: trash, Left: l, Right: r}); err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	scWait(t, s, id)
	ss.mu.Lock()
	done = ss.lastDone
	ss.mu.Unlock()
	if e, _ := done["error"].(string); e != "" {
		t.Fatalf("synchronize: %s", e)
	}
	if errs, _ := done["errors"].([]string); len(errs) > 0 {
		t.Fatalf("synchronize: %v", errs)
	}
	return done
}

// scRow finds a row by path.
func scRow(t *testing.T, v *SyncView, p string) *syncdirs.Item {
	t.Helper()
	for _, it := range v.Rows {
		if it.Path == p {
			return it
		}
	}
	t.Fatalf("no row %s", p)
	return nil
}

func scAllEqual(t *testing.T, v *SyncView) {
	t.Helper()
	if !v.Summary.AllEqual() {
		var bad []string
		for _, it := range v.Rows {
			if it.Category != syncdirs.Equal {
				bad = append(bad, it.Path+"="+it.Category.String())
			}
		}
		t.Fatalf("not all equal after the sync: %+v %v", v.Summary, bad)
	}
}

// A local folder against a zip file and against a folder inside it: compare, copy both ways, delete inside the zip,
// compare again.
func TestSyncDirsWithZip(t *testing.T) {
	s, _ := scApp(t)
	dir := t.TempDir()
	l := filepath.Join(dir, "L")
	old, now := time.Now().Add(-48*time.Hour).Truncate(time.Second), time.Now().Add(-time.Hour).Truncate(time.Second)
	scWrite(t, filepath.Join(l, "same.txt"), "same", old)
	scWrite(t, filepath.Join(l, "lnew.txt"), "new left", now)
	scWrite(t, filepath.Join(l, "sub", "deep", "lonly.txt"), "only left", now)
	scWrite(t, filepath.Join(l, "rnew.txt"), "old left", old)
	zp := filepath.Join(dir, "backup.zip")
	scZip(t, zp, map[string]scZipFile{
		"same.txt":  {"same", old},
		"lnew.txt":  {"old", old},
		"rnew.txt":  {"newer in the zip", now},
		"ronly.txt": {"only right", now},
		"gone.txt":  {"to delete", now},
	})

	id := s.Open()
	defer s.Close(id)
	v := scCompare(t, s, id, l, zp, syncdirs.DefaultOptions())
	if v.RightSide == nil || v.RightSide.Kind != "archive" || v.RightSide.ReadOnly || !v.RightSide.Times || v.RightSide.Trash || v.LeftSide.Kind != "local" {
		t.Fatalf("sides: %+v %+v", v.LeftSide, v.RightSide)
	}
	if scRow(t, v, "same.txt").Category != syncdirs.Equal || scRow(t, v, "lnew.txt").Category != syncdirs.LeftNewer ||
		scRow(t, v, "rnew.txt").Category != syncdirs.RightNewer || scRow(t, v, "ronly.txt").Category != syncdirs.RightOnly ||
		scRow(t, v, "sub/deep/lonly.txt").Category != syncdirs.LeftOnly {
		t.Fatalf("rows: %+v", v.Summary)
	}
	if n, err := s.SetAction(id, []string{"gone.txt"}, syncdirs.DeleteRight, false); err != nil || n != 1 {
		t.Fatalf("delete in the zip: %d %v", n, err)
	}
	scSync(t, s, id, true) // "Delete to the trash" on: the zip has no trash, so the entry is removed from it
	if b, _ := os.ReadFile(filepath.Join(l, "rnew.txt")); string(b) != "newer in the zip" {
		t.Fatalf("rnew.txt from the zip is %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(l, "ronly.txt")); string(b) != "only right" {
		t.Fatalf("ronly.txt from the zip is %q", b)
	}
	names := strings.Join(vfsZipNames(t, zp), ",")
	if strings.Contains(names, "gone.txt") || !strings.Contains(names, "sub/deep/lonly.txt") || !strings.Contains(names, "lnew.txt") {
		t.Fatalf("zip holds %s", names)
	}
	if left, _ := os.ReadDir(dir); len(left) != 2 {
		t.Fatalf("temporary files left next to the zip: %v", left)
	}
	v = scCompare(t, s, id, l, zp, syncdirs.DefaultOptions())
	scAllEqual(t, v)
	// By content reads the zip entries.
	v = scCompare(t, s, id, l, zp, syncdirs.Options{Subdirs: true, ByContent: true, ToleranceSeconds: 2})
	scAllEqual(t, v)
	if !scRow(t, v, "lnew.txt").ContentCompared {
		t.Fatal("by content did not read the zip")
	}

	// A folder inside the zip as the left side, a new local folder as the right side (zip → local).
	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	v = scCompare(t, s, id, filepath.Join(zp, "sub"), out, syncdirs.DefaultOptions())
	if v.LeftSide.Kind != "archive" || v.Summary.LeftOnly == 0 {
		t.Fatalf("zip/sub against an empty folder: %+v %+v", v.LeftSide, v.Summary)
	}
	scSync(t, s, id, false)
	if b, _ := os.ReadFile(filepath.Join(out, "deep", "lonly.txt")); string(b) != "only left" {
		t.Fatalf("unpacked file is %q", b)
	}
	scAllEqual(t, scCompare(t, s, id, filepath.Join(zp, "sub"), out, syncdirs.DefaultOptions()))
	if err := s.Synchronize(id, SyncExecRequest{Filter: syncdirs.AllOps(), Left: zp, Right: out}); err == nil || !strings.Contains(err.Error(), "changed since the last compare") {
		t.Fatalf("a sync with other folders in the fields: %v", err)
	}
}

// vfsZipNames lists the entries of a zip file.
func vfsZipNames(t *testing.T, p string) []string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

// A 7z archive can only be read: it is a source of copies, nothing is ever copied into it or deleted in it, and the
// page is told why.
func TestSyncDirsReadOnlyArchive(t *testing.T) {
	s, _ := scApp(t)
	b, err := os.ReadFile(filepath.Join("internal", "archive", "sevenrar", "testdata", "7z", "lzma2.7z"))
	if err != nil {
		t.Skip("7z sample not available:", err)
	}
	dir := t.TempDir()
	sz := filepath.Join(dir, "s.7z")
	os.WriteFile(sz, b, 0o644)
	sum := vfsSum(t, sz)
	l := filepath.Join(dir, "L")
	scWrite(t, filepath.Join(l, "extra.txt"), "only local", time.Now())

	id := s.Open()
	defer s.Close(id)
	v := scCompare(t, s, id, l, sz, syncdirs.DefaultOptions())
	if !v.RightSide.ReadOnly || !strings.Contains(v.RightSide.Why, "read-only") {
		t.Fatalf("right side: %+v", v.RightSide)
	}
	if it := scRow(t, v, "extra.txt"); it.Action != syncdirs.NoAction {
		t.Fatalf("a copy into the 7z was proposed: %v", it.Action)
	}
	if _, err := s.SetAction(id, []string{"extra.txt"}, syncdirs.CopyLeftToRight, false); err == nil {
		t.Fatal("a copy into the 7z was accepted")
	}
	var fromRight []string
	for _, it := range v.Rows {
		if it.Action == syncdirs.CopyRightToLeft && !it.IsDir {
			fromRight = append(fromRight, it.Path)
		}
		if it.Action == syncdirs.CopyLeftToRight || it.Action == syncdirs.DeleteRight || it.Action == syncdirs.DeleteBoth {
			t.Fatalf("%s writes to the 7z: %v", it.Path, it.Action)
		}
	}
	if len(fromRight) == 0 {
		t.Fatal("nothing is copied out of the 7z")
	}
	p, err := s.Plan(id, syncdirs.AllOps())
	if err != nil || p.NeedsWriteRight || p.FilesLR != 0 {
		t.Fatalf("plan: %+v %v", p, err)
	}
	scSync(t, s, id, false)
	for _, f := range fromRight {
		if _, err := os.Stat(filepath.Join(l, filepath.FromSlash(f))); err != nil {
			t.Fatalf("%s was not unpacked: %v", f, err)
		}
	}
	if vfsSum(t, sz) != sum {
		t.Fatal("the 7z changed")
	}
	v = scCompare(t, s, id, l, sz, syncdirs.DefaultOptions())
	if v.Summary.LeftOnly != 1 || v.Summary.RightOnly+v.Summary.LeftNewer+v.Summary.RightNewer+v.Summary.Unequal != 0 {
		t.Fatalf("after unpacking the 7z: %+v", v.Summary)
	}
}

// A local folder against an SFTP folder: the URL is typed in the field (the host key and the password are asked as
// in the path bar), copies go up and down, a delete on the server is permanent (there is no trash), dates are kept.
func TestSyncDirsWithSFTP(t *testing.T) {
	netSSHIsolate(t)
	s, _ := scApp(t)
	srv := newNetSSHServer(t, false)
	answers := map[string]string{"hostkey": "save", "password": netSSHPass}
	n, _ := netSSHNet(t, func(kind, _ string) (string, bool) { v, ok := answers[kind]; return v, ok })
	old := syncConnect
	syncConnect = func(raw string) (string, error) { c, err := n.Connect(raw); return c.Path, err }
	t.Cleanup(func() { syncConnect = old })

	oldT, now := time.Now().Add(-48*time.Hour).Truncate(time.Second), time.Now().Add(-time.Hour).Truncate(time.Second)
	l := t.TempDir()
	scWrite(t, filepath.Join(l, "same.txt"), "same", oldT)
	scWrite(t, filepath.Join(l, "up.txt"), "newer here", now)
	scWrite(t, filepath.Join(l, "dir", "new.txt"), "new folder", now)
	scWrite(t, filepath.Join(srv.Root, "same.txt"), "same", oldT)
	scWrite(t, filepath.Join(srv.Root, "up.txt"), "older there", oldT)
	scWrite(t, filepath.Join(srv.Root, "down.txt"), "only on the server", now)
	scWrite(t, filepath.Join(srv.Root, "gone.txt"), "delete me", now)

	id := s.Open()
	defer s.Close(id)
	v := scCompare(t, s, id, l, srv.URL, syncdirs.DefaultOptions())
	if v.RightSide.Kind != "sftp" || v.RightSide.ReadOnly || !v.RightSide.Times || v.RightSide.Trash || strings.Contains(v.Right, netSSHPass) {
		t.Fatalf("right side %+v %s", v.RightSide, v.Right)
	}
	if scRow(t, v, "same.txt").Category != syncdirs.Equal || scRow(t, v, "up.txt").Category != syncdirs.LeftNewer || scRow(t, v, "down.txt").Category != syncdirs.RightOnly {
		t.Fatalf("summary %+v", v.Summary)
	}
	if _, err := s.SetAction(id, []string{"gone.txt"}, syncdirs.DeleteRight, false); err != nil {
		t.Fatal(err)
	}
	scSync(t, s, id, true)
	if b, _ := os.ReadFile(filepath.Join(srv.Root, "up.txt")); string(b) != "newer here" {
		t.Fatalf("up.txt on the server is %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(srv.Root, "dir", "new.txt")); string(b) != "new folder" {
		t.Fatalf("dir/new.txt on the server is %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(l, "down.txt")); string(b) != "only on the server" {
		t.Fatalf("down.txt here is %q", b)
	}
	if _, err := os.Stat(filepath.Join(srv.Root, "gone.txt")); err == nil {
		t.Fatal("gone.txt is still on the server")
	}
	if ents, _ := os.ReadDir(srv.Root); len(ents) != 4 {
		t.Fatalf("server folder holds %d entries (temporary files left?)", len(ents))
	}
	if fi, _ := os.Stat(filepath.Join(srv.Root, "up.txt")); !fi.ModTime().Equal(now) {
		t.Fatalf("up.txt on the server has the date %v, want %v", fi.ModTime(), now)
	}
	scAllEqual(t, scCompare(t, s, id, l, srv.URL, syncdirs.DefaultOptions()))
	scAllEqual(t, scCompare(t, s, id, l, srv.URL, syncdirs.Options{Subdirs: true, ByContent: true}))
}

// A WebDAV share can't keep file dates: the sync works, says so in its warnings, and a compare by content shows the
// copied files as equal.
func TestSyncDirsWithWebDAVWarnsAboutDates(t *testing.T) {
	s, _ := scApp(t)
	host, dir := davServer(t, "bob", "s3cret")
	n, _ := testNet(t, map[string]string{"password": "s3cret"})
	old := syncConnect
	syncConnect = func(raw string) (string, error) { c, err := n.Connect(raw); return c.Path, err }
	t.Cleanup(func() { syncConnect = old })
	os.MkdirAll(filepath.Join(dir, "share"), 0o755)
	l := t.TempDir()
	scWrite(t, filepath.Join(l, "a.txt"), "alpha", time.Now().Add(-time.Hour))
	scWrite(t, filepath.Join(l, "f", "b.txt"), "beta", time.Now().Add(-time.Hour))
	scWrite(t, filepath.Join(dir, "share", "a.txt"), "old", time.Now().Add(-48*time.Hour))
	url := "webdav://bob@" + host + "/share"

	id := s.Open()
	defer s.Close(id)
	v := scCompare(t, s, id, l, url, syncdirs.DefaultOptions())
	if v.RightSide.Kind != "other" || v.RightSide.Times || v.RightSide.ReadOnly {
		t.Fatalf("right side %+v", v.RightSide)
	}
	warnings, _ := scSync(t, s, id, false)["warnings"].([]string)
	if b, _ := os.ReadFile(filepath.Join(dir, "share", "a.txt")); string(b) != "alpha" {
		t.Fatalf("a.txt on the share is %q", b)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "can't set file dates") {
		t.Fatalf("warnings %v", warnings)
	}
	scAllEqual(t, scCompare(t, s, id, l, url, syncdirs.Options{Subdirs: true, ByContent: true, IgnoreDate: true}))
}

// The field checks: a text file is not a folder, a missing path inside a non-archive is missing, and a local pair
// is still two plain local folders (nothing changes for them).
func TestSyncDirsSideChecks(t *testing.T) {
	s, _ := scApp(t)
	dir := t.TempDir()
	vfsWrite(t, filepath.Join(dir, "note.txt"), "x")
	if _, err := s.syncOpenSide("left", filepath.Join(dir, "note.txt")); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Fatalf("a text file: %v", err)
	}
	if _, err := s.syncOpenSide("left", filepath.Join(dir, "nope", "x")); err == nil || !strings.Contains(err.Error(), "not there") {
		t.Fatalf("a missing folder: %v", err)
	}
	if _, err := s.syncOpenSide("right", "find://results"); err == nil || !strings.Contains(err.Error(), "can't be synchronized") {
		t.Fatalf("search results: %v", err)
	}
	sd, err := s.syncOpenSide("left", dir)
	if err != nil || sd.vfs != nil || sd.info.Kind != "local" || !sd.info.Trash {
		t.Fatalf("a local folder: %+v %v", sd, err)
	}
	if _, ok := sd.fs.(*syncdirs.OSFS); !ok {
		t.Fatalf("a local folder is not the engine's own local side: %T", sd.fs)
	}
}
