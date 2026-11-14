//go:build live

package main

// Live tests of archives with real tools (board issue #106): a 7z made by the installed 7-Zip through the Pack job,
// read back by the app and by 7-Zip; Find files inside 7z, RAR and ZIP; Feed to listbox of results that all come from
// one archive, then F5 and F8 on that list (the paths the page builds from the list's names).
//
// testdata/live/sample.rar is a 241-byte RAR 5 archive (stored, no compression) written by the archive engine's own
// test writer (internal/archive/sevenrar/writers_test.go, writeRar5) for this test, so it carries no third-party
// content: docs/readme.txt, docs/other.txt and notes.md. Where unrar (or 7-Zip with RAR support) is installed, the test
// first checks that the real tool reads it the same way.

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// liveSevenZip is the 7-Zip the app would use (vfsSevenZip), or the test is skipped (failed when "7z" is required).
func liveSevenZip(t *testing.T) string {
	t.Helper()
	exe := vfsSevenZip()
	if exe == "" {
		liveMissing(t, "7z", "no 7zz, 7z or 7za on PATH")
	}
	return exe
}

// liveRun runs a tool and returns its output; it fails the test when the tool fails.
func liveRun(t *testing.T, exe string, args ...string) string {
	t.Helper()
	out, err := exec.Command(exe, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", filepath.Base(exe), args, err, out)
	}
	return string(out)
}

// liveSltPaths are the "Path = " lines of `7z l -slt` after the archive's own line, sorted.
func liveSltPaths(out string) []string {
	var paths []string
	first := true
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "Path = "); ok {
			if first { // the archive itself
				first = false
				continue
			}
			paths = append(paths, filepath.ToSlash(p))
		}
	}
	sort.Strings(paths)
	return paths
}

// liveFeedOfArchive searches arc for *.txt and *.md files holding "needle" (Find files with "search in archives"; the
// mask keeps out the archive file itself, whose raw bytes hold the text when its members are stored), feeds the results
// to a panel, checks the list holds want (archive members, relative to the archive), and returns the archive it searched
// (a copy of arc in a folder of its own) and the real paths the page would build from its names (base + separator +
// name, frontend/src/features/find.js).
func liveFeedOfArchive(t *testing.T, arc string, want []string) (string, []string) {
	t.Helper()
	// The archive is searched in a folder of its own: next to it lie the files it was packed from and an unpacked
	// copy, which hold the same text and would be found too.
	alone := filepath.Join(t.TempDir(), filepath.Base(arc))
	if b, err := os.ReadFile(arc); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(alone, b, 0o644); err != nil {
		t.Fatal(err)
	}
	arc = alone
	fd := newTestFind(t)
	items := runTestFind(t, fd, FindRequest{SearchFor: "*.txt;*.md", SearchIn: filepath.Dir(arc), Depth: -1, Archives: true,
		TextOn: true, Text: "needle", Encodings: []string{"utf8"}})
	var names []string
	for _, it := range items {
		if !it.Archive || !strings.HasPrefix(it.Path, arc+string(filepath.Separator)) {
			t.Fatalf("result outside %s: %+v", filepath.Base(arc), it)
		}
		names = append(names, it.Name)
	}
	sort.Strings(names)
	var wantNames []string
	for _, w := range want {
		wantNames = append(wantNames, filepath.Base(w))
	}
	sort.Strings(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Fatalf("search inside %s found %v, want %v", filepath.Base(arc), names, wantNames)
	}
	var id int
	fd.mu.Lock()
	for k := range fd.runs {
		id = max(id, k)
	}
	fd.mu.Unlock()
	info, err := fd.Feed(id, filepath.Dir(arc))
	if err != nil {
		t.Fatal(err)
	}
	if info.Base != arc || info.Count != len(want) || info.Note != "" {
		t.Fatalf("Feed of results from one archive: %+v (want its members, relative to %s)", info, arc)
	}
	base, err := fd.FeedBase(info.Path)
	if err != nil || base != arc {
		t.Fatalf("FeedBase %q %v", base, err)
	}
	l, err := findFeedFS{}.List(t.Context(), info.Path)
	if err != nil {
		t.Fatal(err)
	}
	var listed, reals []string
	for _, e := range l.Entries {
		listed = append(listed, filepath.ToSlash(e.Name))
		reals = append(reals, base+string(filepath.Separator)+e.Name)
	}
	sort.Strings(listed)
	w := append([]string(nil), want...)
	sort.Strings(w)
	if !slices.Equal(listed, w) {
		t.Fatalf("the fed list shows %v, want %v", listed, w)
	}
	sort.Strings(reals)
	return arc, reals
}

// liveCopyOutOfFeed is F5 on the fed list: VfsJobs copies the archive members to a local folder.
func liveCopyOutOfFeed(t *testing.T, e *liveEnv, reals []string, want map[string]string) {
	t.Helper()
	out := t.TempDir()
	job := e.vfsJob(VfsRequest{Kind: "copy", Sources: reals, Target: out})
	if len(job.p.Errors) > 0 {
		t.Fatalf("F5 from the fed list: %v", job.p.Errors)
	}
	for name, data := range want {
		if b, err := os.ReadFile(filepath.Join(out, name)); err != nil || string(b) != data {
			t.Errorf("%s copied out of the list: %q %v", name, b, err)
		}
	}
}

// A 7z made by the real 7-Zip through the Pack dialog's job (F-607), read back as a folder, tested, unpacked, checked
// by 7-Zip itself; Find files inside it; Feed to listbox, then F5 (copies out) and F8 (refused: 7z is read-only here).
func TestLive7z(t *testing.T) {
	exe := liveSevenZip(t)
	e := newLiveEnv(t, nil)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	vfsWrite(t, filepath.Join(src, "docs", "readme.txt"), "a needle in the 7z\n")
	vfsWrite(t, filepath.Join(src, "docs", "other.txt"), "nothing here\n")
	vfsWrite(t, filepath.Join(src, "notes.md"), "the needle again\n")
	big := bytes.Repeat([]byte("0123456789abcdef"), 40000)
	vfsWrite(t, filepath.Join(src, "big.bin"), string(big))
	if !slices.ContainsFunc(vfsPackers(), func(p vfsPacker) bool { return p.Name == "7z" && p.Available }) {
		t.Fatal("the Pack dialog does not offer 7z although 7-Zip is installed")
	}
	arc := filepath.Join(dir, "packed.7z")
	job := e.vfsJob(VfsRequest{Kind: "pack", Format: "7z", Level: 5, Target: arc,
		Sources: []string{filepath.Join(src, "docs"), filepath.Join(src, "notes.md"), filepath.Join(src, "big.bin")}})
	if len(job.p.Errors) > 0 {
		t.Fatalf("pack 7z: %v", job.p.Errors)
	}
	want := []string{"big.bin", "docs", "docs/other.txt", "docs/readme.txt", "notes.md"}
	if got := liveSltPaths(liveRun(t, exe, "l", "-slt", arc)); !slices.Equal(got, want) {
		t.Fatalf("7-Zip lists %v, want %v", got, want)
	}
	liveRun(t, exe, "t", arc)

	// the app reads it as a folder
	if got := strings.Join(e.names(arc), ","); got != "big.bin,docs,notes.md" {
		t.Fatalf("the 7z as a folder: %s", got)
	}
	if got := strings.Join(e.names(filepath.Join(arc, "docs")), ","); got != "other.txt,readme.txt" {
		t.Fatalf("docs in the 7z: %s", got)
	}
	if got := e.read(filepath.Join(arc, "big.bin")); !bytes.Equal(got, big) {
		t.Fatal("big.bin read from the 7z differs")
	}
	if info := e.f.PathInfo(filepath.Join(arc, "docs")); info.Kind != "archive" || !info.ReadOnly {
		t.Fatalf("PathInfo %+v", info)
	}
	job = e.vfsJob(VfsRequest{Kind: "test", Sources: []string{arc}})
	if len(job.p.Report) != 1 || !job.p.Report[0].OK || job.p.Report[0].Files != 4 {
		t.Fatalf("Test: %+v %v", job.p.Report, job.p.Errors)
	}
	out := filepath.Join(dir, "unpacked")
	job = e.vfsJob(VfsRequest{Kind: "unpack", Target: out, Sources: []string{arc}})
	if len(job.p.Errors) > 0 {
		t.Fatalf("unpack: %v", job.p.Errors)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "docs", "readme.txt")); string(b) != "a needle in the 7z\n" {
		t.Fatalf("unpacked readme.txt %q", b)
	}

	// adding to the 7z goes through 7-Zip as well
	vfsWrite(t, filepath.Join(dir, "more.txt"), "added later")
	job = e.vfsJob(VfsRequest{Kind: "pack", Format: "7z", Target: arc, Sources: []string{filepath.Join(dir, "more.txt")}})
	if len(job.p.Errors) > 0 {
		t.Fatalf("add to the 7z: %v", job.p.Errors)
	}
	if got := liveSltPaths(liveRun(t, exe, "l", "-slt", arc)); !slices.Contains(got, "more.txt") || len(got) != 6 {
		t.Fatalf("after adding: %v", got)
	}

	// "move to archive": the archive is tested by 7-Zip before the sources go
	mv := filepath.Join(dir, "mv")
	vfsWrite(t, filepath.Join(mv, "m.txt"), "moved into the archive")
	job = e.vfsJob(VfsRequest{Kind: "pack", Format: "7z", Move: true, Target: filepath.Join(dir, "moved.7z"), Sources: []string{filepath.Join(mv, "m.txt")}})
	if _, err := os.Stat(filepath.Join(mv, "m.txt")); len(job.p.Errors) > 0 || err == nil {
		t.Fatalf("move to 7z: %v, source still there: %v", job.p.Errors, err == nil)
	}

	// Find files inside the 7z, Feed to listbox, F5 and F8 on the list
	_, reals := liveFeedOfArchive(t, arc, []string{"docs/readme.txt", "notes.md"})
	liveCopyOutOfFeed(t, e, reals, map[string]string{"readme.txt": "a needle in the 7z\n", "notes.md": "the needle again\n"})
	if _, err := e.js.Start(VfsRequest{Kind: "delete", Sources: reals}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("F8 on members of a 7z: %v, want the read-only refusal", err)
	}
}

// liveRarTool is a program that can list RAR archives: unrar, else 7-Zip (which reads RAR where its RAR codec is
// installed), else "".
func liveRarTool() (exe string, isUnrar bool) {
	if p, err := exec.LookPath("unrar"); err == nil {
		return p, true
	}
	return vfsSevenZip(), false
}

// The committed RAR 5 sample: checked with the real unrar where there is one, searched inside, fed to a panel,
// copied out with F5, and F8 refused (RAR is read-only).
func TestLiveRar(t *testing.T) {
	e := newLiveEnv(t, nil)
	dir := t.TempDir()
	arc := filepath.Join(dir, "sample.rar")
	b, err := os.ReadFile(filepath.Join("testdata", "live", "sample.rar"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(arc, b, 0o644)
	want := []string{"docs", "docs/other.txt", "docs/readme.txt", "notes.md"}
	switch exe, isUnrar := liveRarTool(); {
	case isUnrar:
		out := liveRun(t, exe, "lb", arc)
		var got []string
		for _, l := range strings.Fields(strings.ReplaceAll(out, "\\", "/")) {
			got = append(got, l)
		}
		sort.Strings(got)
		if !slices.Equal(got, want) {
			t.Fatalf("unrar lists %v, want %v", got, want)
		}
		liveRun(t, exe, "t", arc)
	case exe != "":
		if out, err := exec.Command(exe, "l", "-slt", arc).CombinedOutput(); err == nil {
			if got := liveSltPaths(string(out)); !slices.Equal(got, want) {
				t.Fatalf("7-Zip lists %v, want %v", got, want)
			}
		} else if liveRequired("unrar") {
			t.Fatalf("no unrar, and 7-Zip can't read the RAR: %v\n%s", err, out)
		} else {
			t.Logf("7-Zip can't read RAR here (no RAR codec): %v", err)
		}
	default:
		if liveRequired("unrar") {
			t.Fatal("unrar is required (MC_LIVE_REQUIRE) but not installed")
		}
		t.Log("no unrar or 7-Zip: the sample is checked by the app only")
	}
	if got := strings.Join(e.names(filepath.Join(arc, "docs")), ","); got != "other.txt,readme.txt" {
		t.Fatalf("docs in the RAR: %s", got)
	}
	_, reals := liveFeedOfArchive(t, arc, []string{"docs/readme.txt", "notes.md"})
	liveCopyOutOfFeed(t, e, reals, map[string]string{"readme.txt": "a needle inside the rar archive\n", "notes.md": "# notes\n\nthe needle is here too\n"})
	if _, err := e.js.Start(VfsRequest{Kind: "delete", Sources: reals}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("F8 on members of a RAR: %v, want the read-only refusal", err)
	}
}

// A ZIP (made by 7-Zip when it is installed, so a zip from another program): results from one zip fed to a panel,
// F5 copies them out, F8 removes them from the zip, and 7-Zip agrees with what is left.
func TestLiveZipFeedCopyDelete(t *testing.T) {
	e := newLiveEnv(t, nil)
	dir := t.TempDir()
	arc := filepath.Join(dir, "box.zip")
	exe := vfsSevenZip()
	files := map[string]string{"docs/readme.txt": "a needle in the zip\n", "docs/keep.txt": "keep me\n", "notes.md": "needle again\n"}
	if exe != "" {
		src := filepath.Join(dir, "src")
		for n, d := range files {
			vfsWrite(t, filepath.Join(src, filepath.FromSlash(n)), d)
		}
		cmd := exec.Command(exe, "a", "-tzip", "-bd", "-y", arc, "docs", "notes.md")
		cmd.Dir = src
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("7-Zip a -tzip: %v\n%s", err, out)
		}
		os.RemoveAll(src)
	} else {
		if liveRequired("7z") {
			t.Fatal("7z is required (MC_LIVE_REQUIRE) but not installed")
		}
		zf, _ := os.Create(arc)
		zw := zip.NewWriter(zf)
		for n, d := range files {
			w, _ := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Now()})
			io.WriteString(w, d)
		}
		zw.Close()
		zf.Close()
	}
	arc, reals := liveFeedOfArchive(t, arc, []string{"docs/readme.txt", "notes.md"}) // (the copy the list shows)
	liveCopyOutOfFeed(t, e, reals, map[string]string{"readme.txt": files["docs/readme.txt"], "notes.md": files["notes.md"]})
	job := e.vfsJob(VfsRequest{Kind: "delete", Sources: reals})
	if len(job.p.Errors) > 0 {
		t.Fatalf("F8 on the fed list: %v", job.p.Errors)
	}
	if got := strings.Join(e.names(arc), ","); got != "docs" {
		t.Fatalf("the zip after F8 holds %s", got)
	}
	if got := strings.Join(e.names(filepath.Join(arc, "docs")), ","); got != "keep.txt" {
		t.Fatalf("docs after F8 holds %s", got)
	}
	if exe != "" {
		got := liveSltPaths(liveRun(t, exe, "l", "-slt", arc))
		if slices.Contains(got, "notes.md") || slices.Contains(got, "docs/readme.txt") || !slices.Contains(got, "docs/keep.txt") {
			t.Fatalf("7-Zip lists %v after F8", got)
		}
		liveRun(t, exe, "t", arc)
	}
}
