// Tests for browsing and editing archives through the router: zip, the tar
// family, read-only 7z, nested archives, cache reloading, and copying files
// between archives and the disk. Every archive is built inside t.TempDir()
// with the engines themselves.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/tarfmt"
	"mormicommander/internal/archive/ziparc"
)

// makeTree writes files (slash paths, "dir/" for empty folders) below root.
func makeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// topLevel returns the paths of the entries directly inside dir.
func topLevel(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, de := range des {
		out = append(out, filepath.Join(dir, de.Name()))
	}
	return out
}

// makeZip packs files into a new zip at dst.
func makeZip(t *testing.T, dst string, files map[string]string) {
	t.Helper()
	src := t.TempDir()
	makeTree(t, src, files)
	var ch archive.Changes
	for _, p := range topLevel(t, src) {
		ch.Add = append(ch.Add, archive.AddItem{Source: p})
	}
	if _, err := (ziparc.Packer{}).Update(context.Background(), dst, ch, archive.PackOptions{}); err != nil {
		t.Fatal(err)
	}
}

// makeTar packs files into a new tar-family archive at dst (format from name).
func makeTar(t *testing.T, dst string, files map[string]string) {
	t.Helper()
	src := t.TempDir()
	makeTree(t, src, files)
	if err := tarfmt.Create(context.Background(), dst, topLevel(t, src), tarfmt.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func names(l Listing) []string {
	var out []string
	for _, e := range l.Entries {
		n := e.Name
		if e.Flags&FlagDir != 0 {
			n += "/"
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func readAll(t *testing.T, r *Router, p string) string {
	t.Helper()
	rc, err := r.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("open %s: %v", p, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func writeVia(t *testing.T, r *Router, p, content string) {
	t.Helper()
	w, err := r.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("create %s: %v", p, err)
	}
	if _, err := io.WriteString(w, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close %s: %v", p, err)
	}
}

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	r := NewRouter()
	r.Archives.TempDir = t.TempDir()
	t.Cleanup(func() { r.Close() })
	return r
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s: got %q, want %q", what, got, want)
	}
}

var sampleFiles = map[string]string{
	"docs/a.txt":     "alpha",
	"docs/sub/b.txt": "bravo",
	"docs/empty/":    "",
	"top.bin":        "0123456789",
	".hidden":        "h",
}

func TestArchiveZipBrowse(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "data.zip")
	makeZip(t, zp, sampleFiles)
	r := newTestRouter(t)
	ctx := context.Background()

	l, err := r.List(ctx, zp)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "root", names(l), []string{".hidden", "docs/", "top.bin"})
	if l.Parent != dir {
		t.Errorf("parent of archive root = %q, want %q", l.Parent, dir)
	}
	for _, e := range l.Entries {
		if e.Name == ".hidden" && e.Flags&FlagHidden == 0 {
			t.Error(".hidden not marked hidden")
		}
		if e.Name == "top.bin" && e.Size != 10 {
			t.Errorf("top.bin size %d", e.Size)
		}
		if e.Name == "top.bin" && e.MTime == 0 {
			t.Error("top.bin has no time")
		}
	}
	docs := filepath.Join(zp, "docs")
	l, err = r.List(ctx, docs)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "docs", names(l), []string{"a.txt", "empty/", "sub/"})
	if l.Path != docs || l.Parent != zp {
		t.Errorf("listing path/parent = %q/%q", l.Path, l.Parent)
	}

	tests := []struct {
		path string
		kind Kind
	}{
		{dir, KindLocal},
		{zp, KindLocal}, // the archive file itself is a file on disk
		{docs, KindArchive},
		{filepath.Join(docs, "sub", "b.txt"), KindArchive},
	}
	for _, tt := range tests {
		if k := r.Kind(tt.path); k != tt.kind {
			t.Errorf("Kind(%s) = %v, want %v", tt.path, k, tt.kind)
		}
	}

	if got := readAll(t, r, filepath.Join(docs, "sub", "b.txt")); got != "bravo" {
		t.Errorf("content %q", got)
	}
	e, err := r.Stat(filepath.Join(docs, "a.txt"))
	if err != nil || e.Size != 5 || e.Name != "a.txt" || e.Flags&FlagDir != 0 {
		t.Errorf("stat a.txt = %+v, %v", e, err)
	}
	if e, err := r.Stat(zp); err != nil || e.Flags&FlagDir != 0 {
		t.Errorf("stat archive file = %+v, %v (want a file)", e, err)
	}
	if e, err := r.Archives.Stat(zp); err != nil || e.Flags&FlagDir == 0 || e.Name != "data.zip" {
		t.Errorf("archive root stat = %+v, %v", e, err)
	}
	if got := r.Root(filepath.Join(docs, "sub")); got != zp {
		t.Errorf("Root = %q", got)
	}
	if got := r.Parent(filepath.Join(docs, "sub")); got != docs {
		t.Errorf("Parent = %q", got)
	}
	if got := r.Join(docs, "x"); got != filepath.Join(docs, "x") {
		t.Errorf("Join = %q", got)
	}

	if _, err := r.List(ctx, filepath.Join(zp, "nope")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing folder: %v", err)
	}
	if _, err := r.List(ctx, filepath.Join(zp, "top.bin")); !errors.Is(err, ErrNotFolder) {
		t.Errorf("list a file: %v", err)
	}
	if _, err := r.Open(ctx, docs); !errors.Is(err, ErrIsFolder) {
		t.Errorf("open a folder: %v", err)
	}
	if _, err := r.List(ctx, filepath.Join(dir, "plain.txt", "x")); err == nil {
		t.Error("path below a missing file listed")
	}
	plain := filepath.Join(dir, "plain.txt")
	os.WriteFile(plain, []byte("not an archive"), 0o644)
	if _, err := r.List(ctx, plain); !errors.Is(err, ErrNotFolder) {
		t.Errorf("list plain file: %v", err)
	}
	if _, err := r.List(ctx, filepath.Join(plain, "x")); !errors.Is(err, ErrNotFolder) && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("list below plain file: %v", err)
	}

	info, err := r.Archives.Info(docs)
	if err != nil || info.Format != "zip" || info.Root != zp || info.Inner != "docs" || info.ReadOnly || info.Nested {
		t.Errorf("info = %+v, %v", info, err)
	}
	if c := r.Caps(docs); !c.Read || !c.Write || !c.Archive || c.Remote || c.Chmod {
		t.Errorf("caps = %+v", c)
	}
}

func TestArchiveZipWrite(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "w.zip")
	makeZip(t, zp, sampleFiles)
	r := newTestRouter(t)
	ctx := context.Background()
	in := func(p string) string { return filepath.Join(zp, filepath.FromSlash(p)) }

	// List first so the archive is cached; writes must drop the cache.
	if _, err := r.List(ctx, in("docs")); err != nil {
		t.Fatal(err)
	}
	if err := r.Mkdir(ctx, in("new/deep")); err != nil {
		t.Fatal(err)
	}
	if err := r.Mkdir(ctx, in("docs")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("mkdir existing: %v", err)
	}
	writeVia(t, r, in("new/deep/c.txt"), "charlie")
	if got := readAll(t, r, in("new/deep/c.txt")); got != "charlie" {
		t.Errorf("created content %q", got)
	}
	writeVia(t, r, in("top.bin"), "replaced")
	if got := readAll(t, r, in("top.bin")); got != "replaced" {
		t.Errorf("replaced content %q", got)
	}

	local := filepath.Join(dir, "up.txt")
	os.WriteFile(local, []byte("uploaded"), 0o644)
	var prog int64
	n, err := r.Upload(ctx, local, in("docs/renamed-on-upload.txt"), TransferOptions{Progress: func(d, _ int64) { prog = d }})
	if err != nil || n != 8 || prog != 8 {
		t.Fatalf("upload: %d %v (progress %d)", n, err, prog)
	}
	if err := r.Rename(ctx, in("docs/a.txt"), in("docs/sub/a2.txt")); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(ctx, in("docs/sub/b.txt"), in("docs/sub/a2.txt")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename onto existing: %v", err)
	}
	if err := r.Rename(ctx, in("docs"), in("docs/sub/x")); err == nil {
		t.Error("moved a folder into itself")
	}
	if err := r.Rename(ctx, in("docs/sub/b.txt"), filepath.Join(dir, "out.txt")); !errors.Is(err, ErrCrossFS) {
		t.Errorf("rename out of archive: %v", err)
	}
	if err := r.Remove(ctx, in("docs/sub")); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("remove non-empty: %v", err)
	}
	if err := r.Remove(ctx, in("docs/empty")); err != nil {
		t.Errorf("remove empty folder: %v", err)
	}
	if err := r.RemoveAll(ctx, in("docs/sub")); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove(ctx, in("nope")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("remove missing: %v", err)
	}

	pack := t.TempDir()
	makeTree(t, pack, map[string]string{"folder/x.txt": "x", "folder/y/z.txt": "z", "single.txt": "s"})
	if err := r.AddLocal(ctx, in("new"), topLevel(t, pack)); err != nil {
		t.Fatal(err)
	}

	l, err := r.List(ctx, zp)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "root after", names(l), []string{".hidden", "docs/", "new/", "top.bin"})
	l, _ = r.List(ctx, in("docs"))
	eq(t, "docs after", names(l), []string{"renamed-on-upload.txt"})
	l, _ = r.List(ctx, in("new"))
	eq(t, "new after", names(l), []string{"deep/", "folder/", "single.txt"})
	if got := readAll(t, r, in("new/folder/y/z.txt")); got != "z" {
		t.Errorf("packed tree content %q", got)
	}

	// The archive on disk is a valid zip with the same content.
	zr, err := ziparc.Open(zp, archive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zr.Stat("new/deep/c.txt"); err != nil {
		t.Error("zip on disk lacks the created file")
	}
	if _, err := zr.Stat("docs/sub/a2.txt"); err == nil {
		t.Error("zip on disk still has a deleted folder")
	}
	// Close it before the archive is rewritten: Windows cannot replace a
	// file that is still open.
	zr.Close()
	// Packing into the archive file path itself goes to its root.
	if err := r.AddLocal(ctx, zp, []string{local}); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, r, in("up.txt")); got != "uploaded" {
		t.Errorf("packed into root: %q", got)
	}
}

func TestArchiveTarFamily(t *testing.T) {
	for _, ext := range []string{"tar", "tar.gz", "tgz", "tar.bz2", "tar.xz", "tar.zst"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			ap := filepath.Join(dir, "t."+ext)
			makeTar(t, ap, sampleFiles)
			r := newTestRouter(t)
			ctx := context.Background()
			in := func(p string) string { return filepath.Join(ap, filepath.FromSlash(p)) }

			l, err := r.List(ctx, ap)
			if err != nil {
				t.Fatal(err)
			}
			eq(t, "root", names(l), []string{".hidden", "docs/", "top.bin"})
			l, err = r.List(ctx, in("docs"))
			if err != nil {
				t.Fatal(err)
			}
			eq(t, "docs", names(l), []string{"a.txt", "empty/", "sub/"})
			if got := readAll(t, r, in("docs/sub/b.txt")); got != "bravo" {
				t.Errorf("content %q", got)
			}
			if !r.Caps(in("docs")).Write {
				t.Fatal("tar archive not writable")
			}
			local := filepath.Join(dir, "local-name.txt")
			os.WriteFile(local, []byte("staged"), 0o644)
			if _, err := r.Upload(ctx, local, in("docs/other-name.txt"), TransferOptions{}); err != nil {
				t.Fatal(err)
			}
			if err := r.Mkdir(ctx, in("made")); err != nil {
				t.Fatal(err)
			}
			if err := r.Rename(ctx, in("top.bin"), in("made/top.bin")); err != nil {
				t.Fatal(err)
			}
			if err := r.RemoveAll(ctx, in("docs/sub")); err != nil {
				t.Fatal(err)
			}
			writeVia(t, r, in("made/new.txt"), "new")
			l, _ = r.List(ctx, ap)
			eq(t, "root after", names(l), []string{".hidden", "docs/", "made/"})
			l, _ = r.List(ctx, in("docs"))
			eq(t, "docs after", names(l), []string{"a.txt", "empty/", "other-name.txt"})
			l, _ = r.List(ctx, in("made"))
			eq(t, "made after", names(l), []string{"new.txt", "top.bin"})
			if got := readAll(t, r, in("docs/other-name.txt")); got != "staged" {
				t.Errorf("staged upload content %q", got)
			}
			if got := readAll(t, r, in("made/top.bin")); got != "0123456789" {
				t.Errorf("renamed content %q", got)
			}
		})
	}
}

func TestArchiveSingleFileGzReadOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt")
	os.WriteFile(src, []byte("gzipped text"), 0o644)
	gz := filepath.Join(dir, "notes.txt.gz")
	if err := tarfmt.Create(context.Background(), gz, []string{src}, tarfmt.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	r := newTestRouter(t)
	l, err := r.List(context.Background(), gz)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 1 {
		t.Fatalf("entries %v", names(l))
	}
	name := l.Entries[0].Name
	if got := readAll(t, r, filepath.Join(gz, name)); got != "gzipped text" {
		t.Errorf("content %q", got)
	}
	if r.Caps(filepath.Join(gz, name)).Write {
		t.Error("single-file gz reported writable")
	}
	if err := r.Mkdir(context.Background(), filepath.Join(gz, "d")); !errors.Is(err, ErrReadOnly) {
		t.Errorf("mkdir in .gz: %v", err)
	}
	if _, err := r.Create(context.Background(), filepath.Join(gz, "x")); !errors.Is(err, ErrReadOnly) {
		t.Errorf("create in .gz: %v", err)
	}
}

func TestArchive7zReadOnly(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "archive", "sevenrar", "testdata", "7z", "lzma2.7z"))
	if err != nil {
		t.Skip("7z sample not available:", err)
	}
	dir := t.TempDir()
	ap := filepath.Join(dir, "sample.7z")
	os.WriteFile(ap, b, 0o644)
	r := newTestRouter(t)
	ctx := context.Background()

	// Walk down to the first file and read it.
	cur := ap
	var file string
	for depth := 0; depth < 10 && file == ""; depth++ {
		l, err := r.List(ctx, cur)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Entries) == 0 {
			t.Fatal("empty listing at", cur)
		}
		next := ""
		for _, e := range l.Entries {
			if e.Flags&FlagDir == 0 {
				file = filepath.Join(cur, e.Name)
				break
			}
			if next == "" {
				next = filepath.Join(cur, e.Name)
			}
		}
		cur = next
	}
	if file == "" {
		t.Fatal("no file in sample")
	}
	st, err := r.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, r, file); int64(len(got)) != st.Size {
		t.Errorf("read %d bytes, stat says %d", len(got), st.Size)
	}
	info, _ := r.Archives.Info(ap)
	if info.Format != "7z" || !info.ReadOnly {
		t.Errorf("info %+v", info)
	}
	if err := r.Mkdir(ctx, filepath.Join(ap, "x")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("mkdir in 7z: %v", err)
	}
	if c := r.Caps(ap + string(os.PathSeparator) + "x"); c.Write || !c.Read {
		t.Errorf("caps %+v", c)
	}
}

func TestArchiveNested(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(t.TempDir(), "inner.tar.gz")
	makeTar(t, inner, map[string]string{"pkg/main.go": "package main", "README": "read me"})
	zp := filepath.Join(dir, "outer.zip")
	makeZip(t, zp, map[string]string{"old/keep.txt": "k"})
	r := newTestRouter(t)
	ctx := context.Background()
	if err := r.AddLocal(ctx, filepath.Join(zp, "old"), []string{inner}); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(zp, "old", "inner.tar.gz")

	// Going straight to a folder inside the nested archive works.
	l, err := r.List(ctx, filepath.Join(nested, "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "nested pkg", names(l), []string{"main.go"})
	if l.Parent != nested {
		t.Errorf("parent %q", l.Parent)
	}
	l, err = r.List(ctx, nested)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "nested root", names(l), []string{"README", "pkg/"})
	if got := readAll(t, r, filepath.Join(nested, "pkg", "main.go")); got != "package main" {
		t.Errorf("nested content %q", got)
	}
	// The nested archive itself is still a file of the outer archive.
	if e, err := r.Stat(nested); err != nil || e.Flags&FlagDir != 0 {
		t.Errorf("stat nested archive = %+v, %v", e, err)
	}
	if r.Kind(filepath.Join(nested, "pkg")) != KindArchive {
		t.Error("nested path not an archive path")
	}
	info, err := r.Archives.Info(filepath.Join(nested, "pkg"))
	if err != nil || !info.Nested || !info.ReadOnly || info.Root != nested {
		t.Errorf("nested info %+v %v", info, err)
	}
	if err := r.Mkdir(ctx, filepath.Join(nested, "x")); !errors.Is(err, ErrReadOnly) || !errors.Is(err, ErrNestedReadOnly) {
		t.Errorf("write into nested archive: %v", err)
	}
	// Board #143: at the nested archive's own path, Info sees the file in the outer archive and FolderInfo the
	// entered inner archive's root.
	if info, err := r.Archives.Info(nested); err != nil || info.Nested || info.ReadOnly || info.Inner != "old/inner.tar.gz" {
		t.Errorf("Info at the nested archive's path %+v %v", info, err)
	}
	if info, err := r.Archives.FolderInfo(nested); err != nil || !info.Nested || !info.ReadOnly || info.Inner != "" || info.Root != nested {
		t.Errorf("FolderInfo at the nested archive's path %+v %v", info, err)
	}
	if info, err := r.Archives.FolderInfo(filepath.Join(zp, "old")); err != nil || info.Nested || info.Inner != "old" {
		t.Errorf("FolderInfo of a plain folder %+v %v", info, err)
	}
	if got := r.Parent(nested); got != filepath.Join(zp, "old") {
		t.Errorf("parent of nested root %q", got)
	}

	// Replacing the inner archive in the outer one shows the new content.
	inner2 := filepath.Join(t.TempDir(), "inner.tar.gz")
	makeTar(t, inner2, map[string]string{"v2.txt": "two"})
	if _, err := r.Upload(ctx, inner2, nested, TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	l, err = r.List(ctx, nested)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "nested after replace", names(l), []string{"v2.txt"})

	// A file inside an archive that is not an archive cannot be entered.
	if _, err := r.List(ctx, filepath.Join(zp, "old", "keep.txt")); !errors.Is(err, ErrNotFolder) {
		t.Errorf("enter plain file in archive: %v", err)
	}
	if _, err := r.List(ctx, filepath.Join(zp, "old", "keep.txt", "x")); !errors.Is(err, ErrNotFolder) {
		t.Errorf("path below plain file in archive: %v", err)
	}
}

func TestArchiveReloadOnChange(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "c.zip")
	makeZip(t, zp, map[string]string{"one.txt": "1"})
	r := newTestRouter(t)
	ctx := context.Background()
	l, err := r.List(ctx, zp)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "before", names(l), []string{"one.txt"})
	// Rewrite the zip behind the cache's back with a different size and date.
	os.Remove(zp)
	makeZip(t, zp, map[string]string{"two.txt": "22", "three.txt": "333"})
	future := time.Now().Add(time.Hour)
	os.Chtimes(zp, future, future)
	l, err = r.List(ctx, zp)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "after", names(l), []string{"three.txt", "two.txt"})
}

func TestArchiveOpenReaderSurvivesWrite(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "r.zip")
	big := strings.Repeat("0123456789", 100000)
	makeZip(t, zp, map[string]string{"big.txt": big})
	r := newTestRouter(t)
	r.Archives.MaxOpen = 1
	ctx := context.Background()
	rc, err := r.Open(ctx, filepath.Join(zp, "big.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Open other archives to push this one out of the cache meanwhile.
	for i := 0; i < 3; i++ {
		other := filepath.Join(dir, "o"+string(rune('a'+i))+".zip")
		makeZip(t, other, map[string]string{"x": "y"})
		if _, err := r.List(ctx, other); err != nil {
			t.Fatal(err)
		}
	}
	b, err := io.ReadAll(rc)
	if err != nil || string(b) != big {
		t.Fatalf("read after eviction: %d bytes, %v", len(b), err)
	}
	rc.Close()
}

func TestCopyFileAcross(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	tp := filepath.Join(dir, "b.tar")
	makeZip(t, zp, map[string]string{"f.txt": "from zip"})
	makeTar(t, tp, map[string]string{"keep": "k"})
	r := newTestRouter(t)
	ctx := context.Background()

	// zip -> local uses ArchiveFS.Download.
	out := filepath.Join(dir, "out.txt")
	n, err := r.Download(ctx, filepath.Join(zp, "f.txt"), out, TransferOptions{PreserveTime: true})
	if err != nil || n != 8 {
		t.Fatalf("download: %d %v", n, err)
	}
	if b, _ := os.ReadFile(out); string(b) != "from zip" {
		t.Errorf("downloaded %q", b)
	}
	// zip -> tar streams through Open/Create.
	sfs, src := r.Resolve(filepath.Join(zp, "f.txt"))
	dfs, dst := r.Resolve(filepath.Join(tp, "copied.txt"))
	if _, err := CopyFile(ctx, sfs, src, dfs, dst, r.Local, TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, r, filepath.Join(tp, "copied.txt")); got != "from zip" {
		t.Errorf("copied %q", got)
	}
	// local -> zip uses Upload.
	if _, err := CopyFile(ctx, r.Local, out, r.Archives, filepath.Join(zp, "back.txt"), r.Local, TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, r, filepath.Join(zp, "back.txt")); got != "from zip" {
		t.Errorf("uploaded %q", got)
	}
	// Folders are refused.
	if _, err := CopyFile(ctx, r.Archives, zp, r.Local, filepath.Join(dir, "x"), nil, TransferOptions{}); !errors.Is(err, ErrIsFolder) {
		t.Errorf("copy folder: %v", err)
	}
}

func TestIsArchiveName(t *testing.T) {
	a := NewArchiveFS()
	tests := []struct {
		name string
		want bool
	}{
		{"a.zip", true}, {"A.ZIP", true}, {"x.tar.gz", true}, {"x.tgz", true}, {"x.7z", true},
		{"x.rar", true}, {"x.part2.rar", true}, {"x.r07", true}, {"x.z03", true}, {"x.7z.003", true},
		{"x.zip.002", true}, {"x.gz", true}, {"x.tar.zst", true}, {"x.cbz", true},
		{"x.txt", false}, {"x.docx", false}, {"zip", false}, {".zip", false}, {"x.001", false},
		{"x.r", false}, {"x.rxx", false},
	}
	for _, tt := range tests {
		if got := a.IsArchive(tt.name); got != tt.want {
			t.Errorf("IsArchive(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
	a.EnterExtensions = []string{"docx"}
	if !a.IsArchive("x.docx") || a.IsArchive("x.zip") {
		t.Error("EnterExtensions not honoured")
	}
}
