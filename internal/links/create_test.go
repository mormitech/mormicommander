// Tests for creating symbolic links, hard links, junctions and batches, all
// inside temp dirs: the happy paths, the relative-target option, the
// Overwrite rules (existing links replaced, real folders never), and the
// user-facing errors for hard links to folders and missing targets.

package links

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateSymlink(t *testing.T) {
	dir := realTemp(t)
	target := filepath.Join(dir, "data", "file.txt")
	writeFile(t, target, "hello")
	if err := os.MkdirAll(filepath.Join(dir, "links"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		opt      CreateOptions
		wantText string
	}{
		{"absolute", CreateOptions{}, target},
		{"relative", CreateOptions{Relative: true}, filepath.Join("..", "data", "file.txt")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			link := filepath.Join(dir, "links", tc.name)
			if err := CreateSymlink(target, link, tc.opt); err != nil {
				if isPrivilegeErr(err) || errors.Is(err, ErrPrivilege) {
					t.Skip("symlinks not permitted")
				}
				t.Fatal(err)
			}
			got, err := ReadLink(link)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.wantText {
				t.Errorf("link text = %q, want %q", got, tc.wantText)
			}
			b, err := os.ReadFile(link)
			if err != nil || string(b) != "hello" {
				t.Errorf("read through link = %q, %v", b, err)
			}
		})
	}
}

func TestCreateSymlinkOverwrite(t *testing.T) {
	dir := realTemp(t)
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	writeFile(t, a, "A")
	writeFile(t, b, "B")
	link := filepath.Join(dir, "link")
	symlinkOrSkip(t, a, link)

	err := CreateSymlink(b, link, CreateOptions{})
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("without Overwrite: err = %v, want ErrExist", err)
	}
	if err := CreateSymlink(b, link, CreateOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(link); string(got) != "B" {
		t.Errorf("after overwrite link reads %q", got)
	}
	// A real folder is never replaced.
	realDir := filepath.Join(dir, "realdir")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CreateSymlink(b, realDir, CreateOptions{Overwrite: true}); !errors.Is(err, fs.ErrExist) {
		t.Errorf("overwrite real dir: err = %v, want ErrExist", err)
	}
	if fi, err := os.Lstat(realDir); err != nil || !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 {
		t.Errorf("real folder was touched: %v %v", fi, err)
	}
	// No temp files are left behind.
	ents, _ := os.ReadDir(dir)
	if len(ents) != 4 {
		t.Errorf("unexpected entries: %v", ents)
	}
}

func TestCreateSymlinkToDir(t *testing.T) {
	dir := realTemp(t)
	sub := filepath.Join(dir, "sub")
	writeFile(t, filepath.Join(sub, "x"), "x")
	link := filepath.Join(dir, "sublink")
	if err := CreateSymlink(sub, link, CreateOptions{}); err != nil {
		if isPrivilegeErr(err) || errors.Is(err, ErrPrivilege) {
			t.Skip("symlinks not permitted")
		}
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(link, "x")); err != nil {
		t.Errorf("cannot reach file through dir link: %v", err)
	}
}

func TestCreateHardLink(t *testing.T) {
	dir := realTemp(t)
	target := filepath.Join(dir, "orig.txt")
	writeFile(t, target, "data")
	link := filepath.Join(dir, "second.txt")
	if err := CreateHardLink(target, link, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(target)
	b, _ := os.Stat(link)
	if !os.SameFile(a, b) {
		t.Error("hard link is not the same file")
	}
	if runtime.GOOS != "js" && runtime.GOOS != "wasip1" {
		info, err := Inspect(link)
		if err != nil {
			t.Fatal(err)
		}
		if info.Kind != KindHardLink || info.HardLinks != 2 {
			t.Errorf("Inspect = kind %v links %d, want hardlink 2", info.Kind, info.HardLinks)
		}
	}
	if err := CreateHardLink(target, link, CreateOptions{}); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second create: err = %v, want ErrExist", err)
	}
	if err := CreateHardLink(target, link, CreateOptions{Overwrite: true}); err != nil {
		t.Errorf("overwrite: %v", err)
	}
}

func TestCreateHardLinkErrors(t *testing.T) {
	dir := realTemp(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		target string
		want   error
	}{
		{"folder", sub, ErrIsDir},
		{"missing", filepath.Join(dir, "nope"), fs.ErrNotExist},
		{"empty", "", fs.ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CreateHardLink(tc.target, filepath.Join(dir, "l-"+tc.name), CreateOptions{})
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateHardLinkThroughSymlink(t *testing.T) {
	dir := realTemp(t)
	real := filepath.Join(dir, "real")
	writeFile(t, real, "r")
	sl := filepath.Join(dir, "sl")
	symlinkOrSkip(t, real, sl)
	hl := filepath.Join(dir, "hl")
	if err := CreateHardLink(sl, hl, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(hl)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		t.Error("hard link was made to the symlink, not the file")
	}
	rfi, _ := os.Stat(real)
	if !os.SameFile(fi, rfi) {
		t.Error("hard link is not the real file")
	}
}

func TestCreateJunctionNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the Windows test")
	}
	dir := realTemp(t)
	err := CreateJunction(dir, filepath.Join(dir, "j"), CreateOptions{})
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "j")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("junction attempt left something behind")
	}
}

func TestCreateDispatch(t *testing.T) {
	dir := realTemp(t)
	target := filepath.Join(dir, "t.txt")
	writeFile(t, target, "t")
	tests := []struct {
		kind    Kind
		name    string
		wantErr error
	}{
		{KindHardLink, "h", nil},
		{KindURL, "u.url", nil},
		{KindWebloc, "w.webloc", nil},
		{KindDesktop, "d.desktop", nil},
		{KindLnk, "l.lnk", ErrUnsupported},
		{KindAlias, "a", ErrUnsupported},
		{KindNone, "n", ErrUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.kind.String(), func(t *testing.T) {
			err := Create(tc.kind, target, filepath.Join(dir, tc.name), CreateOptions{})
			if tc.wantErr == nil && err != nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCreateLinksBatch(t *testing.T) {
	dir := realTemp(t)
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	writeFile(t, filepath.Join(src, "b.txt"), "b")
	writeFile(t, filepath.Join(dst, "b.txt"), "existing")
	res := CreateLinks(context.Background(), BatchRequest{
		Kind:    KindHardLink,
		Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt"), filepath.Join(src, "missing")},
		DestDir: dst,
	})
	if len(res) != 3 {
		t.Fatalf("got %d results", len(res))
	}
	if res[0].Err != nil || res[0].Link != filepath.Join(dst, "a.txt") {
		t.Errorf("a: %+v", res[0])
	}
	if !errors.Is(res[1].Err, fs.ErrExist) {
		t.Errorf("b: err = %v, want ErrExist", res[1].Err)
	}
	if !errors.Is(res[2].Err, fs.ErrNotExist) {
		t.Errorf("missing: err = %v, want ErrNotExist", res[2].Err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res = CreateLinks(ctx, BatchRequest{Kind: KindHardLink, Sources: []string{filepath.Join(src, "a.txt")}, DestDir: dir,
		Name: func(string) string { return "renamed" }})
	if !errors.Is(res[0].Err, context.Canceled) || res[0].Link != filepath.Join(dir, "renamed") {
		t.Errorf("cancelled batch: %+v", res[0])
	}
}

func TestDefaultLinkName(t *testing.T) {
	tests := []struct {
		src  string
		kind Kind
		want string
	}{
		{filepath.Join("a", "b", "file.txt"), KindSymlink, "file.txt"},
		{filepath.Join("a", "folder") + string(filepath.Separator), KindHardLink, "folder"},
		{filepath.Join("a", "doc.pdf"), KindURL, "doc.pdf.url"},
		{filepath.Join("a", "doc.pdf"), KindWebloc, "doc.pdf.webloc"},
		{filepath.Join("a", "doc.pdf"), KindDesktop, "doc.pdf.desktop"},
		{string(filepath.Separator), KindSymlink, "link"},
	}
	for _, tc := range tests {
		if got := DefaultLinkName(tc.src, tc.kind); got != tc.want {
			t.Errorf("DefaultLinkName(%q, %v) = %q, want %q", tc.src, tc.kind, got, tc.want)
		}
	}
}

func TestUniqueName(t *testing.T) {
	dir := realTemp(t)
	if got := UniqueName(dir, "x.txt"); got != "x.txt" {
		t.Errorf("free name: %q", got)
	}
	writeFile(t, filepath.Join(dir, "x.txt"), "")
	writeFile(t, filepath.Join(dir, "x (2).txt"), "")
	if got := UniqueName(dir, "x.txt"); got != "x (3).txt" {
		t.Errorf("taken name: %q", got)
	}
}

func TestSupportedKinds(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		ks := SupportedKinds(goos)
		if len(ks) < 3 || ks[0] != KindSymlink || ks[1] != KindHardLink {
			t.Errorf("%s: %v", goos, ks)
		}
		hasJunction := false
		for _, k := range ks {
			hasJunction = hasJunction || k == KindJunction
		}
		if hasJunction != (goos == "windows") {
			t.Errorf("%s: junction offered = %v", goos, hasJunction)
		}
	}
}

func TestKindHelpers(t *testing.T) {
	if !KindSymlink.IsFileSystemLink() || !KindJunction.IsFileSystemLink() || KindLnk.IsFileSystemLink() {
		t.Error("IsFileSystemLink")
	}
	for _, k := range []Kind{KindLnk, KindURL, KindWebloc, KindDesktop, KindAlias} {
		if !k.IsShortcut() {
			t.Errorf("%v should be a shortcut", k)
		}
	}
	if KindHardLink.IsShortcut() || Kind(99).String() != "Kind(99)" || KindJunction.String() != "junction" {
		t.Error("kind helpers")
	}
}
