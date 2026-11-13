// These tests cover unpacking: the whole archive and a selection, file masks,
// flattened paths, BaseDir, every overwrite mode and conflict answer, the
// traversal protections (".." names, absolute names, writing through
// existing links, links pointing outside), RAR hard links, progress, cancel,
// and that a failed or cancelled file never replaces an existing one.

package sevenrar

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// tree returns "path=content" lines (folders as "path/") for everything
// below dir, sorted, so a whole extraction result can be compared at once.
func tree(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			lines = append(lines, rel+"->"+filepath.ToSlash(target))
		case d.IsDir():
			lines = append(lines, rel+"/")
		default:
			b, _ := os.ReadFile(p)
			if len(b) > 24 {
				b = append(b[:24:24], "..."...)
			}
			lines = append(lines, rel+"="+string(b))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func openT(t *testing.T, p string) *Archive {
	t.Helper()
	a, err := Open(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestExtractAll(t *testing.T) {
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			a := openT(t, makeArchive(t, format, sampleFiles()))
			dest := filepath.Join(t.TempDir(), "out") // created by Extract
			var last Progress
			res, err := a.Extract(context.Background(), dest, ExtractOptions{Progress: func(p Progress) { last = p }})
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Join([]string{
				"docs/",
				"docs/empty.txt=",
				"docs/readme.txt=hello from the archive\n",
				"src/",
				"src/main/",
				"src/main/app.go=package main\npackage mai...",
				"top.bin=\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03...",
				"árvíztűrő tükörfúrógép.txt=unicode ok",
			}, "\n")
			if got := tree(t, dest); got != want {
				t.Errorf("tree:\n%s\nwant:\n%s", got, want)
			}
			if res.Files != 5 || res.Dirs != 1 || len(res.Skipped) != 0 {
				t.Errorf("result = %+v", res)
			}
			total := int64(23 + 650 + 10 + 5000)
			if res.Bytes != total || last.Bytes != total || last.TotalBytes != total || last.Files != 5 || last.TotalFiles != 5 {
				t.Errorf("bytes = %d, last progress = %+v", res.Bytes, last)
			}
			st, err := os.Stat(filepath.Join(dest, "src", "main", "app.go"))
			if err != nil {
				t.Fatal(err)
			}
			if !st.ModTime().Equal(testTime) {
				t.Errorf("mtime = %v, want %v", st.ModTime(), testTime)
			}
			if runtime.GOOS != "windows" && format != "rar4" && st.Mode().Perm() != 0o755 {
				t.Errorf("mode = %v", st.Mode())
			}
			if dst, _ := os.Stat(filepath.Join(dest, "docs")); !dst.ModTime().Equal(testTime) {
				t.Errorf("folder mtime = %v", dst.ModTime())
			}
			// No temporary files are left behind.
			if strings.Contains(tree(t, dest), ".mormi-") {
				t.Error("temporary file left")
			}
		})
	}
}

func TestExtractSelection(t *testing.T) {
	cases := []struct {
		name string
		opts ExtractOptions
		want string
	}{
		{"one file", ExtractOptions{Paths: []string{"docs/readme.txt"}},
			"docs/\ndocs/readme.txt=hello from the archive\n"},
		{"folder", ExtractOptions{Paths: []string{"src"}},
			"src/\nsrc/main/\nsrc/main/app.go=package main\npackage mai..."},
		{"from inside a folder", ExtractOptions{Paths: []string{"docs/readme.txt"}, BaseDir: "docs"},
			"readme.txt=hello from the archive\n"},
		{"subfolder from inside", ExtractOptions{Paths: []string{"src/main"}, BaseDir: "src"},
			"main/\nmain/app.go=package main\npackage mai..."},
		{"mask", ExtractOptions{Mask: "*.txt | empty*"},
			"docs/\ndocs/readme.txt=hello from the archive\n\nárvíztűrő tükörfúrógép.txt=unicode ok"},
		{"mask and selection", ExtractOptions{Paths: []string{"docs", "top.bin"}, Mask: "*.bin;*.go"},
			"top.bin=\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03\xfa\x00\x01\x02\x03..."},
		{"flatten", ExtractOptions{FlattenPaths: true, Mask: "*.txt *.go"},
			"app.go=package main\npackage mai...\nempty.txt=\nreadme.txt=hello from the archive\n\nárvíztűrő tükörfúrógép.txt=unicode ok"},
		{"leading slash in selection", ExtractOptions{Paths: []string{"/docs/readme.txt"}},
			"docs/\ndocs/readme.txt=hello from the archive\n"},
	}
	for _, format := range formats {
		a := openT(t, makeArchive(t, format, sampleFiles()))
		for _, c := range cases {
			t.Run(format+"/"+c.name, func(t *testing.T) {
				dest := t.TempDir()
				if _, err := a.Extract(context.Background(), dest, c.opts); err != nil {
					t.Fatal(err)
				}
				if got := tree(t, dest); got != c.want {
					t.Errorf("tree:\n%q\nwant:\n%q", got, c.want)
				}
			})
		}
	}
}

func TestExtractNeverEscapes(t *testing.T) {
	files := []tFile{
		{name: "../evil.txt", data: []byte("evil")},
		{name: "a/../../evil2.txt", data: []byte("evil")},
		{name: "/abs/file.txt", data: []byte("abs")},
		{name: `C:\win\file.txt`, data: []byte("drive")},
		{name: "ok.txt", data: []byte("ok")},
	}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			dest := filepath.Join(root, "inner", "dest")
			a := openT(t, makeArchive(t, format, files))
			res, err := a.Extract(context.Background(), dest, ExtractOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := "abs/\nabs/file.txt=abs\nok.txt=ok\nwin/\nwin/file.txt=drive"
			if got := tree(t, dest); got != want {
				t.Errorf("dest tree:\n%s\nwant:\n%s", got, want)
			}
			if got := tree(t, root); strings.Contains(got, "evil") {
				t.Errorf("escaped:\n%s", got)
			}
			if len(res.Skipped) != 2 {
				t.Errorf("skipped = %+v", res.Skipped)
			}
		})
	}
}

func TestExtractDoesNotWriteThroughExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	dest := filepath.Join(root, "dest")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Fatal(err)
	}
	a := openT(t, makeArchive(t, "7z", []tFile{{name: "link/pwned.txt", data: []byte("x")}, {name: "fine.txt", data: []byte("y")}}))
	res, err := a.Extract(context.Background(), dest, ExtractOptions{Overwrite: OverwriteAll})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); err == nil {
		t.Fatal("file written through symlink")
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "symbolic link") {
		t.Errorf("skipped = %+v", res.Skipped)
	}
	if res.Files != 1 {
		t.Errorf("files = %d", res.Files)
	}
}

func TestExtractExistingSymlinkTargetIsReplacedNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	victim := filepath.Join(root, "victim.txt")
	writeFile(t, victim, []byte("keep me"))
	dest := filepath.Join(root, "dest")
	os.MkdirAll(dest, 0o755)
	if err := os.Symlink(victim, filepath.Join(dest, "f.txt")); err != nil {
		t.Fatal(err)
	}
	a := openT(t, makeArchive(t, "rar5", []tFile{{name: "f.txt", data: []byte("new")}}))
	if _, err := a.Extract(context.Background(), dest, ExtractOptions{Overwrite: OverwriteAll}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep me" {
		t.Errorf("victim changed to %q", b)
	}
	if got := tree(t, dest); got != "f.txt=new" {
		t.Errorf("tree = %q", got)
	}
}

func TestExtractSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	files := []tFile{
		{name: "dir/real.txt", data: []byte("real")},
		{name: "dir/good", data: []byte("real.txt"), mode: fs.ModeSymlink},
		{name: "dir/up", data: []byte("../dir/real.txt"), mode: fs.ModeSymlink},
		{name: "abs", data: []byte("/etc/passwd"), mode: fs.ModeSymlink},
		{name: "escape", data: []byte("../outside"), mode: fs.ModeSymlink},
		{name: "dir/sneaky", data: []byte("x/../../.."), mode: fs.ModeSymlink},
	}
	for _, format := range []string{"7z", "rar5"} {
		t.Run(format, func(t *testing.T) {
			a := openT(t, makeArchive(t, format, files))
			var links int
			for _, e := range a.Entries() {
				if e.Symlink {
					links++
				}
			}
			if links != 5 {
				t.Errorf("symlink entries = %d", links)
			}

			dest := t.TempDir()
			res, err := a.Extract(context.Background(), dest, ExtractOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := "dir/\ndir/good->real.txt\ndir/real.txt=real\ndir/up->../dir/real.txt"
			if got := tree(t, dest); got != want {
				t.Errorf("tree:\n%s\nwant:\n%s", got, want)
			}
			if res.Links != 2 || len(res.Skipped) != 3 {
				t.Errorf("result = %+v", res)
			}

			dest = t.TempDir()
			res, err = a.Extract(context.Background(), dest, ExtractOptions{Symlinks: SymlinksSkip})
			if err != nil || res.Links != 0 || tree(t, dest) != "dir/\ndir/real.txt=real" {
				t.Errorf("skip policy: %+v %v\n%s", res, err, tree(t, dest))
			}

			dest = t.TempDir()
			_, err = a.Extract(context.Background(), dest, ExtractOptions{Symlinks: SymlinksAsFiles, Paths: []string{"abs"}})
			if err != nil || tree(t, dest) != "abs=/etc/passwd" {
				t.Errorf("as files: %v\n%s", err, tree(t, dest))
			}
		})
	}
}

func TestExtractSymlinkThroughForeignLinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	dest := filepath.Join(root, "dest")
	os.MkdirAll(dest, 0o755)
	os.Symlink(root, filepath.Join(dest, "out")) // pre-existing, not ours
	a := openT(t, makeArchive(t, "rar5", []tFile{{name: "l", data: []byte("out/secret"), mode: fs.ModeSymlink}}))
	res, err := a.Extract(context.Background(), dest, ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Links != 0 || len(res.Skipped) != 1 {
		t.Errorf("result = %+v", res)
	}
}

func TestExtractRarHardLinks(t *testing.T) {
	files := []tFile{
		{name: "orig.txt", data: []byte("shared data")},
		{name: "hard.txt", link: 4, target: "orig.txt"},
		{name: "copy.txt", link: 5, target: "orig.txt"},
		{name: "dangling.txt", link: 4, target: "missing.txt"},
	}
	a := openT(t, makeArchive(t, "rar5", files))
	dest := t.TempDir()
	res, err := a.Extract(context.Background(), dest, ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "copy.txt=shared data\nhard.txt=shared data\norig.txt=shared data"
	if got := tree(t, dest); got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != "dangling.txt" {
		t.Errorf("skipped = %+v", res.Skipped)
	}
	o, _ := os.Stat(filepath.Join(dest, "orig.txt"))
	h, _ := os.Stat(filepath.Join(dest, "hard.txt"))
	c, _ := os.Stat(filepath.Join(dest, "copy.txt"))
	if !os.SameFile(o, h) {
		t.Error("hard.txt is not a hard link")
	}
	if os.SameFile(o, c) {
		t.Error("copy.txt should be a copy")
	}
}

func TestExtractConflicts(t *testing.T) {
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []tFile{{name: "a.txt", data: []byte("archive")}}
	p := makeArchive(t, "7z", files)

	setup := func(t *testing.T, mtime time.Time) string {
		dest := t.TempDir()
		f := filepath.Join(dest, "a.txt")
		writeFile(t, f, []byte("existing"))
		os.Chtimes(f, mtime, mtime)
		return dest
	}
	cases := []struct {
		name     string
		mtime    time.Time
		opts     ExtractOptions
		want     string
		skipped  int
		wantErr  error
		conflict bool
	}{
		{name: "default asks nobody: skip", mtime: old, want: "a.txt=existing", skipped: 1},
		{name: "overwrite all", mtime: old, opts: ExtractOptions{Overwrite: OverwriteAll}, want: "a.txt=archive"},
		{name: "skip all", mtime: old, opts: ExtractOptions{Overwrite: OverwriteSkip}, want: "a.txt=existing", skipped: 1},
		{name: "older: existing is older", mtime: old, opts: ExtractOptions{Overwrite: OverwriteOlder}, want: "a.txt=archive"},
		{name: "older: existing is newer", mtime: newer, opts: ExtractOptions{Overwrite: OverwriteOlder}, want: "a.txt=existing", skipped: 1},
		{name: "rename new", mtime: old, opts: ExtractOptions{Overwrite: OverwriteRename}, want: "a (2).txt=archive\na.txt=existing"},
		{name: "rename existing", mtime: old, opts: ExtractOptions{Overwrite: OverwriteRenameExisting}, want: "a (2).txt=existing\na.txt=archive"},
		{name: "ask: overwrite", mtime: old, conflict: true, opts: ExtractOptions{OnConflict: func(c Conflict) ConflictDecision {
			return ConflictDecision{Action: ActionOverwrite}
		}}, want: "a.txt=archive"},
		{name: "ask: rename to given name", mtime: old, conflict: true, opts: ExtractOptions{OnConflict: func(c Conflict) ConflictDecision {
			return ConflictDecision{Action: ActionRename, NewName: "b.txt"}
		}}, want: "a.txt=existing\nb.txt=archive"},
		{name: "ask: rename to unsafe name", mtime: old, conflict: true, opts: ExtractOptions{OnConflict: func(c Conflict) ConflictDecision {
			return ConflictDecision{Action: ActionRename, NewName: "../b.txt"}
		}}, want: "a.txt=existing", wantErr: ErrUnsafePath},
		{name: "ask: cancel", mtime: old, conflict: true, opts: ExtractOptions{OnConflict: func(c Conflict) ConflictDecision {
			return ConflictDecision{Action: ActionCancel}
		}}, want: "a.txt=existing", wantErr: context.Canceled},
	}
	a := openT(t, p)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest := setup(t, c.mtime)
			asked := false
			if c.opts.OnConflict != nil {
				inner := c.opts.OnConflict
				c.opts.OnConflict = func(cf Conflict) ConflictDecision {
					asked = true
					if cf.ExistingSize != 8 || cf.Entry.Path != "a.txt" || filepath.Base(cf.Target) != "a.txt" {
						t.Errorf("conflict = %+v", cf)
					}
					return inner(cf)
				}
			}
			res, err := a.Extract(context.Background(), dest, c.opts)
			if c.wantErr != nil && !errors.Is(err, c.wantErr) || c.wantErr == nil && err != nil {
				t.Errorf("err = %v, want %v", err, c.wantErr)
			}
			if got := tree(t, dest); got != c.want {
				t.Errorf("tree:\n%s\nwant:\n%s", got, c.want)
			}
			if len(res.Skipped) != c.skipped {
				t.Errorf("skipped = %+v", res.Skipped)
			}
			if asked != c.conflict {
				t.Errorf("asked = %v", asked)
			}
		})
	}
}

func TestExtractConflictApplyToAll(t *testing.T) {
	files := []tFile{{name: "a.txt", data: []byte("A")}, {name: "b.txt", data: []byte("B")}, {name: "c.txt", data: []byte("C")}}
	a := openT(t, makeArchive(t, "rar5", files))
	dest := t.TempDir()
	for _, f := range files {
		writeFile(t, filepath.Join(dest, f.name), []byte("old"))
	}
	asked := 0
	_, err := a.Extract(context.Background(), dest, ExtractOptions{OnConflict: func(Conflict) ConflictDecision {
		asked++
		return ConflictDecision{Action: ActionOverwrite, ApplyToAll: true}
	}})
	if err != nil || asked != 1 {
		t.Fatalf("err = %v, asked = %d", err, asked)
	}
	if got := tree(t, dest); got != "a.txt=A\nb.txt=B\nc.txt=C" {
		t.Errorf("tree = %q", got)
	}
}

func TestExtractDuplicateEntriesLastWins(t *testing.T) {
	files := []tFile{{name: "dup.txt", data: []byte("first")}, {name: "dup.txt", data: []byte("second")}}
	a := openT(t, makeArchive(t, "rar5", files))
	dest := t.TempDir()
	if _, err := a.Extract(context.Background(), dest, ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, dest); got != "dup.txt=second" {
		t.Errorf("tree = %q", got)
	}
	if got := string(readAll(t, a, "dup.txt")); got != "second" {
		t.Errorf("Open = %q", got)
	}
}

func TestExtractFileOverFolderFails(t *testing.T) {
	a := openT(t, makeArchive(t, "7z", []tFile{{name: "x", data: []byte("file")}, {name: "y.txt", data: []byte("y")}}))
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "x"), 0o755)
	res, err := a.Extract(context.Background(), dest, ExtractOptions{Overwrite: OverwriteAll})
	if err == nil || len(res.Errors) != 1 || res.Files != 1 {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestExtractCRCErrorKeepsExistingFile(t *testing.T) {
	for _, keep := range []bool{false, true} {
		p := makeArchive(t, "rar5", []tFile{{name: "f.txt", data: []byte("some precious data")}, {name: "g.txt", data: []byte("fine")}})
		b, _ := os.ReadFile(p)
		b[bytes.Index(b, []byte("precious"))] = 'P'
		writeFile(t, p, b)
		a := openT(t, p)
		dest := t.TempDir()
		writeFile(t, filepath.Join(dest, "f.txt"), []byte("existing"))
		res, err := a.Extract(context.Background(), dest, ExtractOptions{Overwrite: OverwriteAll, KeepBroken: keep})
		if !errors.Is(err, ErrCRC) || len(res.Errors) != 1 || res.Errors[0].Path != "f.txt" {
			t.Fatalf("keep=%v: res = %+v, err = %v", keep, res, err)
		}
		want := "f.txt=existing\ng.txt=fine"
		if keep {
			want = "f.txt=some Precious data\ng.txt=fine"
		}
		if got := tree(t, dest); got != want {
			t.Errorf("keep=%v: tree = %q, want %q", keep, got, want)
		}
	}
}

func TestExtractOnErrorAbort(t *testing.T) {
	p := makeArchive(t, "7z", []tFile{{name: "f.txt", data: []byte("some precious data")}, {name: "g.txt", data: []byte("fine")}})
	b, _ := os.ReadFile(p)
	b[bytes.Index(b, []byte("precious"))] = 'P'
	writeFile(t, p, b)
	a := openT(t, p)
	dest := t.TempDir()
	calls := 0
	res, err := a.Extract(context.Background(), dest, ExtractOptions{OnError: func(e *EntryError) bool { calls++; return false }})
	if !errors.Is(err, ErrCRC) || calls != 1 || res.Files != 0 {
		t.Errorf("res = %+v, err = %v, calls = %d", res, err, calls)
	}
	if got := tree(t, dest); got != "" {
		t.Errorf("tree = %q", got)
	}
}

func TestExtractCancel(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 3<<20)
	for _, format := range []string{"7z", "rar5"} {
		t.Run(format, func(t *testing.T) {
			a := openT(t, makeArchive(t, format, []tFile{{name: "big.bin", data: big}, {name: "after.txt", data: []byte("a")}}))
			dest := t.TempDir()
			writeFile(t, filepath.Join(dest, "big.bin"), []byte("existing"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			res, err := a.Extract(ctx, dest, ExtractOptions{Overwrite: OverwriteAll, Progress: func(p Progress) {
				if p.Bytes > 0 {
					cancel()
				}
			}})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v", err)
			}
			if res.Files != 0 {
				t.Errorf("files = %d", res.Files)
			}
			if got := tree(t, dest); got != "big.bin=existing" {
				t.Errorf("tree = %q", got)
			}

			ctx, cancel2 := context.WithCancel(context.Background())
			cancel2()
			if _, err := a.Test(ctx, nil); !errors.Is(err, context.Canceled) {
				t.Errorf("Test err = %v", err)
			}
		})
	}
}

func TestExtractEncryptedWithPasswordFunc(t *testing.T) {
	files := sampleFiles()
	p := writeRar5(t, filepath.Join(t.TempDir(), "enc"), files, rarOpts{password: "pw"})[0]
	var reqs []PasswordRequest
	a, err := Open(p, Options{PasswordFunc: func(r PasswordRequest) (string, bool) {
		reqs = append(reqs, r)
		if r.Attempt == 1 {
			return "nope", true
		}
		return "pw", true
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	dest := t.TempDir()
	res, err := a.Extract(context.Background(), dest, ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 5 || len(reqs) != 2 || reqs[0].Entry != "docs/readme.txt" || reqs[1].Attempt != 2 {
		t.Errorf("res = %+v, reqs = %+v", res, reqs)
	}
	if !strings.Contains(tree(t, dest), "docs/readme.txt=hello from the archive") {
		t.Error("content wrong")
	}
	// The password is remembered for later operations.
	if _, err := a.Test(context.Background(), nil); err != nil || len(reqs) != 2 {
		t.Errorf("Test: %v, reqs = %d", err, len(reqs))
	}

	b, err := Open(p, Options{PasswordFunc: func(PasswordRequest) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.Extract(context.Background(), t.TempDir(), ExtractOptions{}); !errors.Is(err, ErrPasswordCancelled) {
		t.Errorf("cancelled: %v", err)
	}
	// Extracting only unencrypted entries (folders) needs no password.
	if _, err := b.Extract(context.Background(), t.TempDir(), ExtractOptions{Paths: []string{"docs"}, Mask: "*.none"}); err != nil {
		t.Errorf("no encrypted entries selected: %v", err)
	}
}

func TestFreeName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), nil)
	writeFile(t, filepath.Join(dir, "a (2).txt"), nil)
	writeFile(t, filepath.Join(dir, ".hidden"), nil)
	cases := map[string]string{
		"a.txt":   "a (3).txt",
		"b":       "b (2)",
		".hidden": ".hidden (2)",
	}
	for in, want := range cases {
		if got := filepath.Base(freeName(filepath.Join(dir, in))); got != want {
			t.Errorf("freeName(%s) = %s, want %s", in, got, want)
		}
	}
}
