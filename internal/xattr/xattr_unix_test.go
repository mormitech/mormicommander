//go:build darwin || linux

// Tests of listing and removing extended attributes on real files in temporary folders. Linux only allows user.*
// names for ordinary users and some file systems (older tmpfs) have none at all; the tests skip there.
package xattr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// testName is an attribute name an ordinary user may set on this system.
func testName(s string) string {
	if runtime.GOOS == "linux" {
		return "user.mc." + s
	}
	return "com.mormitech.test." + s
}

func set(t *testing.T, p, name, val string) {
	t.Helper()
	if err := unix.Lsetxattr(p, name, []byte(val), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			t.Skip("this file system has no extended attributes")
		}
		t.Fatal(err)
	}
}

func file(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListHasRemove(t *testing.T) {
	if !Supported() {
		t.Fatal("Supported is false on a system with extended attributes")
	}
	p := file(t, t.TempDir(), "a.txt")
	set(t, p, testName("b"), "12345")
	set(t, p, testName("a"), "")
	l, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	var mine []Attr
	for _, a := range l {
		if a.Name == testName("a") || a.Name == testName("b") {
			mine = append(mine, a)
		}
	}
	if len(mine) != 2 || mine[0].Name != testName("a") || mine[0].Size != 0 || mine[1].Size != 5 {
		t.Fatalf("List = %+v", l)
	}
	if !Has(p, testName("b")) || Has(p, testName("none")) {
		t.Fatal("Has is wrong")
	}
	if ok, err := Remove(p, testName("b")); !ok || err != nil {
		t.Fatalf("Remove = %v, %v", ok, err)
	}
	if ok, err := Remove(p, testName("b")); ok || err != nil {
		t.Fatalf("second Remove = %v, %v; want false, nil", ok, err)
	}
	if Has(p, testName("b")) || !Has(p, testName("a")) {
		t.Fatal("the wrong attribute went")
	}
}

func TestListPlainFile(t *testing.T) {
	p := file(t, t.TempDir(), "plain")
	l, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range l {
		if a.Name == testName("a") {
			t.Fatalf("unexpected %+v", a)
		}
	}
	if _, err := List(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("List of a missing file gave no error")
	}
}

func TestRemoveAll(t *testing.T) {
	dir := t.TempDir()
	top := filepath.Join(dir, "App")
	sub := filepath.Join(top, "Contents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f1 := file(t, sub, "bin")
	f2 := file(t, dir, "loose.txt")
	other := file(t, dir, "other.txt")
	name := testName("q")
	for _, p := range []string{top, f1, f2, other} {
		set(t, p, name, "0081;00000000;Safari;")
	}
	// a link to a marked file: the link is looked at, its target is not touched through it
	link := filepath.Join(top, "link")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}

	r, err := RemoveAll(context.Background(), []string{top, f2}, name, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Removed != 3 || len(r.Failed) != 0 || r.Checked != 5 { // App, Contents, bin, link, loose.txt
		t.Fatalf("RemoveAll = %+v", r)
	}
	for _, p := range []string{top, f1, f2} {
		if Has(p, name) {
			t.Errorf("%s still has it", p)
		}
	}
	if !Has(other, name) {
		t.Error("the link's target lost it")
	}

	// not recursive: only the paths themselves
	set(t, top, name, "x")
	set(t, f1, name, "x")
	r, err = RemoveAll(context.Background(), []string{top}, name, false, nil)
	if err != nil || r.Removed != 1 || r.Checked != 1 || !Has(f1, name) {
		t.Fatalf("not recursive: %+v, %v", r, err)
	}

	// a missing path is a failure, the rest goes on
	r, err = RemoveAll(context.Background(), []string{filepath.Join(dir, "gone"), other}, name, true, nil)
	if err != nil || len(r.Failed) != 1 || r.Removed != 1 {
		t.Fatalf("missing path: %+v, %v", r, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RemoveAll(ctx, []string{top}, name, true, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestFindAny(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := file(t, dir, "plain")
	tagged := file(t, sub, "tagged")
	if p, n := FindAny(context.Background(), []string{plain, dir}, 100); n == testName("tag") {
		t.Fatalf("found %s on %s before it was set", n, p)
	}
	set(t, tagged, testName("tag"), "red")
	p, n := FindAny(context.Background(), []string{plain, dir}, 100)
	if p == "" || n == "" {
		t.Fatal("FindAny found nothing")
	}
	if p, _ := FindAny(context.Background(), []string{sub}, 0); p != "" {
		t.Fatalf("limit 0 still looked: %s", p)
	}
}

func TestQuarantineDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	p := file(t, t.TempDir(), "download.zip")
	set(t, p, Quarantine, "0081;65f00000;Safari;")
	if !Has(p, Quarantine) {
		t.Fatal("quarantine not set")
	}
	r, err := RemoveAll(context.Background(), []string{p}, Quarantine, true, nil)
	if err != nil || r.Removed != 1 || Has(p, Quarantine) {
		t.Fatalf("RemoveAll = %+v, %v", r, err)
	}
	// only the quarantine flag is not worth a warning, so FindAny ignores it
	set(t, p, Quarantine, "0081;65f00000;Safari;")
	if f, n := FindAny(context.Background(), []string{p}, 10); f != "" && n == Quarantine {
		t.Fatal("FindAny reported the quarantine flag")
	}
}
