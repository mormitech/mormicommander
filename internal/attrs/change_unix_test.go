//go:build unix

// Unix-only tests for planning and applying permission, ownership and
// macOS hidden-flag changes, including the deepest-first order that lets a
// recursive change remove a folder's own access bits.
package attrs

import (
	"context"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func perm(t *testing.T, p string) fs.FileMode {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode() & PermMask
}

func TestApplyPermissionsRecursive(t *testing.T) {
	root := tree(t)
	sh := filepath.Join(root, "sub", "run.sh")
	write(t, sh, []byte("#!/bin/sh\n"), 0o644)
	// Make *.sh executable for everyone, with a mask.
	p, err := MakePlan(context.Background(), Request{Paths: []string{root}, Recursive: true,
		Filter: ParseMask("*.sh"), Perm: PermChange{Expr: "a+x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := relPaths(t, root, p, true); strings.Join(got, ",") != "sub/run.sh" {
		t.Errorf("changed = %v", got)
	}
	if ch := p.Items[0].Changes[0]; ch != "permissions rw-r--r-- (0644) → rwxr-xr-x (0755)" {
		t.Errorf("preview = %q", ch)
	}
	p.Apply(context.Background(), nil)
	if perm(t, sh) != 0o755 || perm(t, filepath.Join(root, "a.txt")) != 0o644 {
		t.Errorf("perms after apply: %v %v", perm(t, sh), perm(t, filepath.Join(root, "a.txt")))
	}
}

func TestApplyPermissionsDeepestFirst(t *testing.T) {
	root := tree(t)
	sub := filepath.Join(root, "sub")
	// Taking away read and execute from everything, folders included, only
	// works if the contents are handled before their folders.
	p, err := MakePlan(context.Background(), Request{Paths: []string{sub}, Recursive: true, Perm: PermChange{Expr: "go-rx"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { // let t.TempDir clean up
		_ = os.Chmod(filepath.Join(sub, "deep"), 0o755)
		_ = os.Chmod(sub, 0o755)
	})
	var order []string
	res := p.Apply(context.Background(), func(_, _ int, it *Item, _ error) {
		r, _ := filepath.Rel(root, it.Path)
		order = append(order, filepath.ToSlash(r))
	})
	if len(res.Failed) != 0 {
		t.Fatalf("failures: %v", res.Failed)
	}
	if order[len(order)-1] != "sub" || order[0] != "sub/deep/d.txt" {
		t.Errorf("order = %v", order)
	}
	if perm(t, sub) != 0o700 || perm(t, filepath.Join(sub, "deep", "d.txt")) != 0o600 {
		t.Errorf("perms: sub %v d %v", perm(t, sub), perm(t, filepath.Join(sub, "deep", "d.txt")))
	}
	// Now set exact permissions with Set/Clear (octal 0750 for folders only).
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{sub}, Recursive: true, ApplyTo: FoldersOnly, Perm: PermExact(0o750)})
	p2.Apply(context.Background(), nil)
	if perm(t, sub) != 0o750 || perm(t, filepath.Join(sub, "deep")) != 0o750 {
		t.Errorf("folder perms: %v %v", perm(t, sub), perm(t, filepath.Join(sub, "deep")))
	}
}

func TestApplySpecialBits(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "shared")
	if err := os.Mkdir(d, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o777); err != nil {
		t.Fatal(err)
	}
	p, _ := MakePlan(context.Background(), Request{Paths: []string{d}, Perm: PermChange{Set: fs.ModeSticky | fs.ModeSetgid}})
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	got := perm(t, d)
	if got&fs.ModeSticky == 0 || got&fs.ModeSetgid == 0 {
		t.Errorf("special bits not set: %v", FormatPerm(got))
	}
	info, _ := Stat(d)
	if c := ColumnString(info); c != "drwxrwsrwt" {
		t.Errorf("column = %q", c)
	}
}

func TestSymlinkPermissionsNote(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	write(t, target, []byte("x"), 0o644)
	link := filepath.Join(dir, "l")
	if err := os.Symlink("t", link); err != nil {
		t.Fatal(err)
	}
	p, _ := MakePlan(context.Background(), Request{Paths: []string{link}, Perm: PermChange{Set: 0o111}})
	it := p.Items[0]
	if it.Changed() || len(it.Notes) != 1 {
		t.Errorf("link item = %+v", it)
	}
	p.Apply(context.Background(), nil)
	if perm(t, target) != 0o644 {
		t.Error("target changed through link")
	}
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{link}, FollowLinks: true, Perm: PermChange{Set: 0o111}})
	p2.Apply(context.Background(), nil)
	if perm(t, target) != 0o755 {
		t.Errorf("target with follow = %v", perm(t, target))
	}
}

func TestApplyGroupChange(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	gids, err := u.GroupIds()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	write(t, f, []byte("x"), 0o644)
	info, _ := Stat(f)
	other := -1
	for _, g := range gids {
		if n, err := strconv.Atoi(g); err == nil && n != info.GID {
			other = n
			break
		}
	}
	// Changing to the current owner and group by name is always allowed and
	// a no-op in the plan.
	p0, err := MakePlan(context.Background(), Request{Paths: []string{f}, Owner: OwnerChange{Owner: info.Owner, Group: info.Group}})
	if err != nil {
		t.Fatal(err)
	}
	if p0.ChangedCount() != 0 {
		t.Errorf("same owner planned a change: %v", p0.Items[0].Changes)
	}
	if _, err := MakePlan(context.Background(), Request{Paths: []string{f}, Owner: OwnerChange{Owner: "no-such-user-mormi"}}); err == nil {
		t.Error("unknown user accepted")
	}
	if other < 0 {
		t.Skip("user belongs to only one group")
	}
	p, err := MakePlan(context.Background(), Request{Paths: []string{f}, Owner: OwnerChange{Group: strconv.Itoa(other)}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Items[0].Changes[0], "group ") {
		t.Errorf("preview = %v", p.Items[0].Changes)
	}
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	after, _ := Stat(f)
	if after.GID != other || after.UID != info.UID {
		t.Errorf("gid = %d want %d (uid %d→%d)", after.GID, other, info.UID, after.UID)
	}
}

func TestHiddenFlag(t *testing.T) {
	if runtime.GOOS != "darwin" {
		r := Request{Paths: []string{"x"}, Attrs: WinAttrChange{Set: AttrHidden}}
		if err := Validate(r); err == nil {
			t.Error("hidden attribute accepted without a hidden flag")
		}
		return
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "visible.txt")
	dot := filepath.Join(dir, ".dot")
	write(t, f, []byte("x"), 0o644)
	write(t, dot, []byte("x"), 0o644)
	p, err := MakePlan(context.Background(), Request{Paths: []string{f}, Attrs: WinAttrChange{Set: AttrHidden}})
	if err != nil {
		t.Fatal(err)
	}
	if ch := p.Items[0].Changes; len(ch) != 1 || ch[0] != "hidden flag off → on" {
		t.Errorf("preview = %q", ch)
	}
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	info, _ := Stat(f)
	if !info.HiddenFlag || !info.Hidden || info.Win&AttrHidden == 0 {
		t.Errorf("hidden flag not set: %+v", info)
	}
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{f, dot}, Attrs: WinAttrChange{Clear: AttrHidden}})
	if p2.Items[1].Changed() || len(p2.Items[1].Notes) != 1 {
		t.Errorf("dot file item = %+v", p2.Items[1])
	}
	p2.Apply(context.Background(), nil)
	info, _ = Stat(f)
	if info.HiddenFlag || info.Hidden {
		t.Errorf("hidden flag not cleared: %+v", info)
	}
	// The flag can be set on a symbolic link itself.
	link := filepath.Join(dir, "lnk")
	if err := os.Symlink("visible.txt", link); err != nil {
		t.Fatal(err)
	}
	p3, _ := MakePlan(context.Background(), Request{Paths: []string{link}, Attrs: WinAttrChange{Set: AttrHidden}})
	if res := p3.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	li, _ := Stat(link)
	ti, _ := Stat(f)
	if !li.HiddenFlag || ti.HiddenFlag {
		t.Errorf("link flag %v target flag %v", li.HiddenFlag, ti.HiddenFlag)
	}
}
