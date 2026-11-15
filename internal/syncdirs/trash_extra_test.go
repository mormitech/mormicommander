// Tests for the two opt-in trash options of Execute (board issue #99): with
// TrashEmptyDirs the emptied folders a sync removes go through the trash hook
// instead of being removed directly, and with TrashReplacedLinks a link that a
// link copy replaces goes through the hook instead of being overwritten. The
// hook here moves entries into a temp folder, so the real trash is never
// touched. Without the options the old behaviour (direct removal) must stay.
package syncdirs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// binHook returns a trash hook that moves entries of the side rooted at root
// into a fresh temp folder, and the list of names it was given.
func binHook(t *testing.T, root string) (func(WritableFS, string) error, string, *[]string) {
	t.Helper()
	bin := t.TempDir()
	var got []string
	return func(f WritableFS, name string) error {
		got = append(got, name)
		return os.Rename(filepath.Join(root, filepath.FromSlash(name)), filepath.Join(bin, filepath.Base(name)))
	}, bin, &got
}

func TestTrashEmptyDirsSendsFoldersToTheHook(t *testing.T) {
	right := map[string]spec{"dir/x.txt": file("x", 0), "dir/sub/": {}}
	o := DefaultOptions()
	o.Asymmetric = true

	// Off (the default): the folder is removed directly, only the file is trashed.
	l, r := trees(t, map[string]spec{}, right)
	res := mustCompare(t, l, r, o)
	res.SelectTree("", true)
	hook, bin, got := binHook(t, r)
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{Trash: hook})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if strings.Join(*got, ",") != "dir/x.txt" {
		t.Errorf("default: trashed %v, want only dir/x.txt", *got)
	}
	if _, err := os.Lstat(filepath.Join(r, "dir")); err == nil {
		t.Error("default: folder not removed")
	}
	if _, err := os.Lstat(filepath.Join(bin, "dir")); err == nil {
		t.Error("default: folder went to the trash")
	}

	// On: the emptied folders go through the hook too.
	l, r = trees(t, map[string]spec{}, right)
	res = mustCompare(t, l, r, o)
	res.SelectTree("", true)
	hook, bin, got = binHook(t, r)
	rep = syncAll(t, res, l, r, AllOps(), ExecOptions{Trash: hook, TrashEmptyDirs: true})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	for _, want := range []string{"dir/x.txt", "dir/sub", "dir"} {
		found := false
		for _, g := range *got {
			found = found || g == want
		}
		if !found {
			t.Errorf("on: %s not trashed (got %v)", want, *got)
		}
	}
	if _, err := os.Lstat(filepath.Join(r, "dir")); err == nil {
		t.Error("on: folder still there")
	}
	if fi, err := os.Lstat(filepath.Join(bin, "dir")); err != nil || !fi.IsDir() {
		t.Errorf("on: folder not in the trash: %v", err)
	}

	// TrashEmptyDirs without a Trash hook changes nothing (permanent sync).
	l, r = trees(t, map[string]spec{}, right)
	res = mustCompare(t, l, r, o)
	res.SelectTree("", true)
	rep = syncAll(t, res, l, r, AllOps(), ExecOptions{TrashEmptyDirs: true})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if _, err := os.Lstat(filepath.Join(r, "dir")); err == nil {
		t.Error("no hook: folder not removed")
	}
}

func TestTrashEmptyDirsNeverTakesAFolderWithSomethingLeft(t *testing.T) {
	l, r := trees(t, map[string]spec{}, map[string]spec{"dir/x.txt": file("x", 0)})
	o := DefaultOptions()
	o.Asymmetric = true
	res := mustCompare(t, l, r, o)
	res.SelectTree("", true)
	var got []string
	// The file can't be trashed; the folder could be, but must not be, as x.txt is still in it.
	hook := func(f WritableFS, name string) error {
		got = append(got, name)
		if name == "dir/x.txt" {
			return ErrNoTrash
		}
		return os.RemoveAll(filepath.Join(r, filepath.FromSlash(name)))
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{Trash: hook, TrashEmptyDirs: true})
	if strings.Join(got, ",") != "dir/x.txt" {
		t.Errorf("hook got %v, want only dir/x.txt", got)
	}
	if len(rep.Errors) != 2 || !errors.Is(rep.Errors[1].Err, errDirNotEmpty) {
		t.Errorf("errors %v", rep.Errors)
	}
	if readFile(t, filepath.Join(r, "dir", "x.txt")) != "x" {
		t.Error("file lost")
	}
}

func TestTrashReplacedLinks(t *testing.T) {
	setup := func() (string, string, *Result) {
		l, r := trees(t, map[string]spec{"lnk": {link: "new-target"}}, map[string]spec{"lnk": {link: "old-target"}})
		res := mustCompare(t, l, r, DefaultOptions())
		if err := res.SetAction("lnk", CopyLeftToRight); err != nil {
			t.Fatal(err)
		}
		_ = res.SetSelected("lnk", true)
		return l, r, res
	}

	// Off (the default): the old link is overwritten, the hook is not called.
	l, r, res := setup()
	hook, _, got := binHook(t, r)
	rep := syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Trash: hook, Force: true})
	if len(rep.Errors) != 0 || rep.Done == 0 {
		t.Fatalf("default: %+v", rep)
	}
	if len(*got) != 0 {
		t.Errorf("default: hook called for %v", *got)
	}
	if tgt, _ := os.Readlink(filepath.Join(r, "lnk")); tgt != "new-target" {
		t.Errorf("default: link points to %q", tgt)
	}

	// On: the old link goes to the trash as a link, then the new one is made.
	l, r, res = setup()
	hook, bin, got := binHook(t, r)
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Trash: hook, TrashReplacedLinks: true, Force: true})
	if len(rep.Errors) != 0 || rep.Done == 0 {
		t.Fatalf("on: %+v", rep)
	}
	if strings.Join(*got, ",") != "lnk" {
		t.Errorf("on: hook got %v", *got)
	}
	if tgt, _ := os.Readlink(filepath.Join(r, "lnk")); tgt != "new-target" {
		t.Errorf("on: link points to %q", tgt)
	}
	if tgt, err := os.Readlink(filepath.Join(bin, "lnk")); err != nil || tgt != "old-target" {
		t.Errorf("on: old link not in the trash: %q %v", tgt, err)
	}

	// A failing trash leaves the old link alone and reports the error; it is never overwritten anyway.
	l, r, res = setup()
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Force: true, TrashReplacedLinks: true,
		Trash: func(WritableFS, string) error { return ErrNoTrash }})
	if len(rep.Errors) != 1 || !errors.Is(rep.Errors[0].Err, ErrNoTrash) {
		t.Errorf("failing trash: %+v", rep)
	}
	if tgt, _ := os.Readlink(filepath.Join(r, "lnk")); tgt != "old-target" {
		t.Errorf("failing trash: link changed to %q", tgt)
	}
}

func TestTrashReplacedFiles(t *testing.T) {
	setup := func() (string, string, *Result) {
		l, r := trees(t, map[string]spec{"a.txt": file("new contents", 0)}, map[string]spec{"a.txt": file("old", 0)})
		res := mustCompare(t, l, r, DefaultOptions())
		if err := res.SetAction("a.txt", CopyLeftToRight); err != nil {
			t.Fatal(err)
		}
		_ = res.SetSelected("a.txt", true)
		return l, r, res
	}

	// Off (the default): the old file is replaced in place, the hook is not called.
	l, r, res := setup()
	hook, _, got := binHook(t, r)
	rep := syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Trash: hook, Force: true})
	if len(rep.Errors) != 0 || rep.Done == 0 {
		t.Fatalf("default: %+v", rep)
	}
	if len(*got) != 0 {
		t.Errorf("default: hook called for %v", *got)
	}
	if readFile(t, filepath.Join(r, "a.txt")) != "new contents" {
		t.Error("default: file not replaced")
	}

	// On: the old file goes to the trash with its contents, the new one takes its name.
	l, r, res = setup()
	hook, bin, got := binHook(t, r)
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Trash: hook, TrashReplacedFiles: true, Force: true})
	if len(rep.Errors) != 0 || rep.Done == 0 {
		t.Fatalf("on: %+v", rep)
	}
	if strings.Join(*got, ",") != "a.txt" {
		t.Errorf("on: hook got %v", *got)
	}
	if readFile(t, filepath.Join(r, "a.txt")) != "new contents" {
		t.Error("on: file not replaced")
	}
	if readFile(t, filepath.Join(bin, "a.txt")) != "old" {
		t.Error("on: old file not in the trash")
	}

	// A failing trash leaves the old file as it was and reports the error; no temporary file is left behind.
	l, r, res = setup()
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Force: true, TrashReplacedFiles: true,
		Trash: func(WritableFS, string) error { return ErrNoTrash }})
	if len(rep.Errors) != 1 || !errors.Is(rep.Errors[0].Err, ErrNoTrash) {
		t.Errorf("failing trash: %+v", rep)
	}
	if readFile(t, filepath.Join(r, "a.txt")) != "old" {
		t.Error("failing trash: old file changed")
	}
	if ents, _ := os.ReadDir(r); len(ents) != 1 {
		t.Errorf("failing trash: left %d entries, want only a.txt", len(ents))
	}

	// A new file (nothing to replace) never calls the hook.
	l, r = trees(t, map[string]spec{"b.txt": file("b", 0)}, map[string]spec{})
	res = mustCompare(t, l, r, DefaultOptions())
	res.SelectTree("", true)
	hook, _, got = binHook(t, r)
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Trash: hook, TrashReplacedFiles: true})
	if len(rep.Errors) != 0 || len(*got) != 0 {
		t.Errorf("new file: %+v, hook got %v", rep, *got)
	}
}
