package main

// Tests of the file tags service (feature_tags.go, issue #28) on temporary files: Change adds and removes tags, gives
// Finder's colour tags their colour, records one undo step that Undo.Run takes back, and the [=mc.tags] column field
// reads the names. Skipped where the system or the temporary folder's file system has no tags.

import (
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/fileops"
	lst "mormicommander/internal/listing"
	"mormicommander/internal/tags"
	"mormicommander/internal/xattr"
)

func tagsTempFiles(t *testing.T, names ...string) (string, []string) {
	t.Helper()
	if !tags.Supported() {
		t.Skip("this system has no file tags")
	}
	dir := t.TempDir()
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		mkfile(t, p, n)
		out = append(out, p)
	}
	if err := xattr.Set(out[0], "user.mc-probe", []byte("1")); err != nil {
		t.Skip("the temporary folder's file system has no extended attributes")
	}
	xattr.Remove(out[0], "user.mc-probe")
	return dir, out
}

func TestTagsChangeAndUndo(t *testing.T) {
	_, u := newUndoJobs(t)
	tg := &Tags{app: u.app}
	_, ps := tagsTempFiles(t, "a.txt", "b.txt")
	if !tg.Info().Supported {
		t.Fatal("Info should say tags are supported")
	}
	r, err := tg.Change(ps, []tags.Tag{{Name: " red "}, {Name: "Work"}, {Name: ""}}, nil)
	if err != nil || r.Changed != 2 || len(r.Failed) != 0 {
		t.Fatalf("Change = %+v, %v", r, err)
	}
	got := tg.Lookup(append([]string{filepath.Join(filepath.Dir(ps[0]), "none")}, ps...))
	if len(got) != 2 || len(got[ps[0]]) != 2 || got[ps[0]][0].Name != "red" {
		t.Fatalf("Lookup = %+v", got)
	}
	if tags.System().HasColors() && got[ps[0]][0].Color != 6 {
		t.Fatalf("red should be red: %+v", got[ps[0]])
	}
	// the custom column field
	it := lst.Item{Name: "a.txt", Dir: filepath.Dir(ps[0])}
	if fl, ok := lst.LookupField("mc.tags"); !ok || fl.Get(&it, "", nil).Text != "red, Work" {
		t.Fatal("the mc.tags field should read the names")
	}
	// adding what is there changes nothing and records nothing more
	if r, _ := tg.Change(ps, []tags.Tag{{Name: "WORK"}}, nil); r.Changed != 0 {
		t.Fatalf("Change again = %+v", r)
	}
	if r, _ := tg.Change(ps[:1], nil, []string{"Red"}); r.Changed != 1 {
		t.Fatalf("remove = %+v", r)
	}
	if l, _ := tags.Read(ps[0]); len(l) != 1 || l[0].Name != "Work" {
		t.Fatalf("after remove: %+v", l)
	}
	if n := len(u.j.List()); n != 2 {
		t.Fatalf("journal has %d entries", n)
	}
	// undo the removal, then the first change
	if res, err := u.Run(0); err != nil || res.Done != 1 {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if l, _ := tags.Read(ps[0]); len(l) != 2 {
		t.Fatalf("after undo: %+v", l)
	}
	if res, err := u.Run(0); err != nil || res.Done != 2 || res.What != "the tag change of 2 items" {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	for _, p := range ps {
		if l, _ := tags.Read(p); len(l) != 0 {
			t.Fatalf("%s still has %+v", p, l)
		}
	}
	// a server path is refused, the rest still works
	r, _ = tg.Change([]string{"sftp://host/x"}, []tags.Tag{{Name: "x"}}, nil)
	if len(r.Failed) != 1 {
		t.Fatalf("server path: %+v", r)
	}
}

// TestTagsKeptOnCopy: a copy job keeps the tags (they are extended attributes, which internal/fileops copies).
func TestTagsKeptOnCopy(t *testing.T) {
	j, u := newUndoJobs(t)
	dir, ps := tagsTempFiles(t, "a.txt")
	if err := tags.Write(ps[0], []tags.Tag{{Name: "Keep"}}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out")
	os.MkdirAll(dst, 0o755)
	runAndWait(t, j, u, fileops.Request{Kind: fileops.Copy, Target: dst, Sources: ps})
	if l, _ := tags.Read(filepath.Join(dst, "a.txt")); len(l) != 1 || l[0].Name != "Keep" {
		t.Fatalf("copy has %+v", l)
	}
}
