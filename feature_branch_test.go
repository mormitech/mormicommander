package main

// Tests of the branch view's folder (F-086, F-087): a branch path lists every file under the folder with relative
// names, ".." goes back to the folder, a name joins to the real file, the selection view lists only what was
// selected, a copy target in a branch view is the real folder, and the panels see it as a local place.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mormicommander/internal/vfs"
)

func branchTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "sub/b.txt", "sub/deeper/c.txt", "other/d.txt"} {
		findWriteFile(t, filepath.Join(dir, filepath.FromSlash(n)), n)
	}
	return dir
}

func branchNames(l vfs.Listing) []string {
	var out []string
	for _, e := range l.Entries {
		out = append(out, filepath.ToSlash(e.Name))
	}
	sort.Strings(out)
	return out
}

func TestBranchListAndPaths(t *testing.T) {
	dir := branchTree(t)
	p := "branch://" + dir
	fsys := vfs.For(p, nil)
	if _, ok := fsys.(branchFS); !ok {
		t.Fatalf("branch:// is answered by %T", fsys)
	}
	l, err := fsys.List(context.Background(), p+string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(branchNames(l), ","), "a.txt,other/d.txt,sub/b.txt,sub/deeper/c.txt"; got != want {
		t.Errorf("names = %s, want %s", got, want)
	}
	if l.Path != p || l.Parent != dir {
		t.Errorf("path %q parent %q, want %q and %q", l.Path, l.Parent, p, dir)
	}
	if got := fsys.Join(p, filepath.Join("sub", "b.txt")); got != filepath.Join(dir, "sub", "b.txt") {
		t.Errorf("Join = %q", got)
	}
	if got := fsys.Parent(p); got != dir {
		t.Errorf("Parent = %q, want the folder", got)
	}
	if e, err := fsys.Stat(p + string(filepath.Separator) + filepath.Join("sub", "b.txt")); err != nil || e.Size != int64(len("sub/b.txt")) {
		t.Errorf("Stat of a name = %+v, %v", e, err)
	}
	if real, ok := branchReal(p); !ok || real != dir {
		t.Errorf("branchReal = %q, %v", real, ok)
	}
	if _, ok := branchReal(dir); ok {
		t.Error("a plain folder counts as a branch view")
	}
	b := &Branch{app: &App{}}
	if info := b.Info(p); info.Files != 4 || info.Truncated || info.Base != dir {
		t.Errorf("Info = %+v", info)
	}
	if pr := b.Progress(); len(pr) != 0 {
		t.Errorf("Progress after the listing = %+v", pr)
	}
	if _, err := fsys.List(context.Background(), "branch://"+filepath.Join(dir, "missing")); err == nil {
		t.Error("the branch view of a missing folder gave no error")
	}
}

func TestBranchSelection(t *testing.T) {
	dir := branchTree(t)
	b := &Branch{app: &App{}}
	if _, err := b.Select(dir, nil); err == nil {
		t.Error("an empty selection gave no error")
	}
	if _, err := b.Select("ftp://host/x", []string{"a"}); err == nil {
		t.Error("a server folder gave no error")
	}
	p, err := b.Select(dir, []string{"a.txt", "sub", "../escape"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, "branchsel://") || !branchPath(p) {
		t.Fatalf("path %q", p)
	}
	l, err := vfs.For(p, nil).List(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(branchNames(l), ","), "a.txt,sub/b.txt,sub/deeper/c.txt"; got != want {
		t.Errorf("names = %s, want %s", got, want)
	}
	if l.Parent != dir || (branchFS{}).Parent(p) != dir {
		t.Errorf("parent %q, want %q", l.Parent, dir)
	}
	if got := (branchFS{}).Join(p, "sub/b.txt"); got != filepath.Join(dir, "sub", "b.txt") {
		t.Errorf("Join = %q", got)
	}
	if _, err := (branchFS{}).List(context.Background(), "branchsel://999999"); err == nil {
		t.Error("a selection that is gone gave no error")
	}
}

func TestBranchCopyTargetAndPathInfo(t *testing.T) {
	dir := branchTree(t)
	src := t.TempDir()
	j := newTestJobs(t)
	tg, err := j.ParseTarget("branch://"+dir+string(filepath.Separator), 2, src)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Dir != dir || !tg.DirExists {
		t.Errorf("target %+v, want the real folder %s", tg, dir)
	}
	if info := branchTestFS(t).PathInfo("branch://" + dir); info.Kind != "local" || info.Remote {
		t.Errorf("PathInfo = %+v, want a local place", info)
	}
	if err := os.RemoveAll(filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	l, err := vfs.For("branch://"+dir, nil).List(context.Background(), "branch://"+dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(branchNames(l), ","); got != "a.txt,other/d.txt" {
		t.Errorf("after deleting a folder the list is %s", got)
	}
}

func branchTestFS(t *testing.T) *FS {
	t.Helper()
	f, _ := vfsTestApp(t)
	return f
}
