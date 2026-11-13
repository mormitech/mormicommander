// Regression tests for the milestone fixes: asymmetric actions that start
// unselected, the Show filter limiting Synchronize, deletions through the
// trash hook, NFC name pairing, folder-only include filters, the shared mask
// syntax in the filter (bare words, "*.", templates) and partial results
// after a cancelled compare.
package syncdirs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAsymmetricDestructiveActionsStartUnselected(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"a.txt": file("left", 0), "b.txt": file("left newer", time.Hour)},
		map[string]spec{"a.txt": file("right newer", 5*time.Hour), "b.txt": file("r", 0), "extra.txt": file("x", 0)})
	o := DefaultOptions()
	o.Asymmetric = true
	res := mustCompare(t, l, r, o)
	a, b, extra := res.Find("a.txt"), res.Find("b.txt"), res.Find("extra.txt")
	if a.Action != CopyLeftToRight || !a.Unselected {
		t.Errorf("right-newer a.txt: %v unselected=%v", a.Action, a.Unselected)
	}
	if extra.Action != DeleteRight || !extra.Unselected {
		t.Errorf("right-only extra.txt: %v unselected=%v", extra.Action, extra.Unselected)
	}
	if b.Action != CopyLeftToRight || b.Unselected {
		t.Errorf("left-newer b.txt must stay selected: %v unselected=%v", b.Action, b.Unselected)
	}
	p := res.Plan(AllOps())
	if len(p.Ops) != 1 || p.Ops[0].Dst != "b.txt" || p.Unselected != 2 {
		t.Fatalf("default plan: %+v", p)
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if readFile(t, filepath.Join(r, "a.txt")) != "right newer" {
		t.Error("newer right file overwritten by default")
	}
	if _, err := os.Stat(filepath.Join(r, "extra.txt")); err != nil {
		t.Error("right-only file deleted by default")
	}
	// Selecting a row, or choosing an action for it, includes it; a reset
	// brings the unselected default back.
	if err := res.SetSelected("extra.txt", true); err != nil {
		t.Fatal(err)
	}
	if err := res.SetAction("a.txt", CopyLeftToRight); err != nil {
		t.Fatal(err)
	}
	if p := res.Plan(AllOps()); p.Unselected != 0 || p.DeletesRight != 1 {
		t.Errorf("after selecting: %+v", p)
	}
	res.ResetActions()
	if !res.Find("a.txt").Unselected || !res.Find("extra.txt").Unselected {
		t.Error("ResetActions did not restore the unselected defaults")
	}
	// Symmetric mode never starts anything unselected.
	o.Asymmetric = false
	for _, it := range mustCompare(t, l, r, o).Items {
		if it.Unselected {
			t.Errorf("symmetric %s unselected", it.Path)
		}
	}
}

func TestPlanHonoursShowFilter(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"newer.txt": file("L2", time.Hour), "lonly.txt": file("L", 0), "sub/deep.txt": file("d", 0), "neq.txt": file("aaa", 0)},
		map[string]spec{"newer.txt": file("R", 0), "ronly.txt": file("R", 0), "neq.txt": file("bb", 0)})
	res := mustCompare(t, l, r, DefaultOptions())
	if err := res.SetAction("neq.txt", CopyLeftToRight); err != nil {
		t.Fatal(err)
	}
	all := res.Plan(AllOps())
	if all.FilesLR != 4 || all.FilesRL != 1 {
		t.Fatalf("unfiltered plan: LR %d RL %d", all.FilesLR, all.FilesRL)
	}
	// Hide "!=" rows: neq.txt must not be copied although it has an action.
	show := ShowAll()
	show.Unequal = false
	f := AllOps()
	f.Show = &show
	p := res.Plan(f)
	if p.FilesLR != 3 || p.FilesRL != 1 {
		t.Errorf("hiding != : LR %d RL %d", p.FilesLR, p.FilesRL)
	}
	for _, op := range p.Ops {
		if op.Dst == "neq.txt" {
			t.Error("hidden unequal row synchronized")
		}
	}
	// Hide singles: only the duplicate pairs remain (newer.txt; neq.txt is a
	// duplicate too).
	show = ShowAll()
	show.Singles = false
	f.Show = &show
	p = res.Plan(f)
	var dsts []string
	for _, op := range p.Ops {
		dsts = append(dsts, op.Dst)
	}
	if strings.Join(dsts, ",") != "neq.txt,newer.txt" {
		t.Errorf("hiding singles: ops %v", dsts)
	}
}

func TestDeletesGoThroughTrashHook(t *testing.T) {
	l, r := trees(t, map[string]spec{"keep.txt": file("k", 0)},
		map[string]spec{"keep.txt": file("k", 0), "gone.txt": file("g", 0), "dir/x.txt": file("x", 0)})
	o := DefaultOptions()
	o.Asymmetric = true
	res := mustCompare(t, l, r, o)
	res.SelectTree("", true)
	bin := t.TempDir()
	var trashed []string
	hook := func(f WritableFS, name string) error {
		trashed = append(trashed, name)
		return os.Rename(filepath.Join(r, filepath.FromSlash(name)), filepath.Join(bin, filepath.Base(name)))
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{Trash: hook})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if strings.Join(trashed, ",") != "gone.txt,dir/x.txt" && strings.Join(trashed, ",") != "dir/x.txt,gone.txt" {
		t.Errorf("trashed %v", trashed)
	}
	for _, n := range []string{"gone.txt", "x.txt"} {
		if _, err := os.Stat(filepath.Join(bin, n)); err != nil {
			t.Errorf("%s not in the trash: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(r, "dir")); err == nil {
		t.Error("emptied folder not removed")
	}
	// A failing trash never falls back to a permanent delete.
	l, r = trees(t, map[string]spec{}, map[string]spec{"y.txt": file("y", 0)})
	res = mustCompare(t, l, r, o)
	res.SelectTree("", true)
	rep = syncAll(t, res, l, r, AllOps(), ExecOptions{Trash: func(WritableFS, string) error { return ErrNoTrash }})
	if len(rep.Errors) != 1 || !errors.Is(rep.Errors[0].Err, ErrNoTrash) {
		t.Errorf("report %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(r, "y.txt")); err != nil {
		t.Error("file deleted although the trash failed")
	}
	if err := SystemTrash(&nonOSFS{}, "a"); !errors.Is(err, ErrNoTrash) {
		t.Errorf("SystemTrash on a non-local side: %v", err)
	}
}

// nonOSFS is a WritableFS that is not on the local disk.
type nonOSFS struct{ *OSFS }

func TestNFCPairing(t *testing.T) {
	nfd, nfc := "é.txt", "é.txt"
	l, r := trees(t, map[string]spec{nfd: file("same", 0)}, map[string]spec{nfc: file("same", 0)})
	for _, cm := range []CaseMode{CaseSensitive, CaseInsensitive} {
		o := DefaultOptions()
		o.Case = cm
		res := mustCompare(t, l, r, o)
		if len(res.Items) != 1 {
			t.Fatalf("case %v: NFD and NFC names not paired: %v", cm, rows(res))
		}
		it := res.Items[0]
		if it.Category != Equal || it.Left == nil || it.Right == nil {
			t.Errorf("case %v: %v", cm, rows(res))
		}
	}
}

func TestFolderOnlyIncludeFilter(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"a.txt": file("a", 0), "src/b.go": file("b", 0), "src/deep/c.go": file("c", 0), "other/c.go": file("c", 0)},
		map[string]spec{})
	o := DefaultOptions()
	o.Filter = `src\`
	equalLines(t, rows(mustCompare(t, l, r, o)), []string{
		"src/ leftOnly copyLeftToRight",
		"src/b.go leftOnly copyLeftToRight",
		"src/deep/ leftOnly copyLeftToRight",
		"src/deep/c.go leftOnly copyLeftToRight",
	})
}

func TestFilterUsesSharedMaskSyntax(t *testing.T) {
	cases := []struct {
		filter, rel string
		want        bool
	}{
		{"*.", "Makefile", true},
		{"*.", "main.go", false},
		{"report", "annual_report.pdf", true},
		{"report", "summary.pdf", false},
		{"readme.txt", "old_readme.txt.bak", false}, // dotted mask: exact
		{"readme.*", "readme", true},                // DOS rule
		{"*.TXT", "é.txt", true},
		{"é*", "été.doc", true}, // NFC on both sides
		{"[ab]*.go", "b.go", true},
		{"src/*.go", "src/deep/a.go", false}, // "*" does not cross "/" in path masks
	}
	for _, c := range cases {
		f, err := CompileFilter(c.filter, nil)
		if err != nil {
			t.Fatalf("%q: %v", c.filter, err)
		}
		if got := f.File(c.rel, baseName(c.rel), false); got != c.want {
			t.Errorf("filter %q file %q = %v, want %v", c.filter, c.rel, got, c.want)
		}
	}
	// Folder patterns never get the bare-word star.
	f := ParseFilter(`| bin\`)
	if enter, _ := f.Dir("cabinet", "cabinet", false); !enter {
		t.Error(`"| bin\" skipped "cabinet"`)
	}
}

func TestFilterTemplates(t *testing.T) {
	tmpl := map[string]string{"Sources": "*.go *.c | *_test.go", "Junk": "*.tmp *.bak", "Loop": ">Loop"}
	resolve := func(n string) (string, bool) { s, ok := tmpl[n]; return s, ok }
	f, err := CompileFilter(">Sources", resolve)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]bool{"a.go": true, "b.c": true, "a_test.go": false, "x.txt": false} {
		if got := f.File(rel, rel, false); got != want {
			t.Errorf(">Sources %s = %v", rel, got)
		}
	}
	f, err = CompileFilter("* | >Junk", resolve)
	if err != nil {
		t.Fatal(err)
	}
	if f.File("a.tmp", "a.tmp", false) || !f.File("a.txt", "a.txt", false) {
		t.Error("template on the exclude side")
	}
	for _, bad := range []string{">Missing", ">Loop", `"open`} {
		if _, err := CompileFilter(bad, resolve); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
	if _, err := CompileFilter(">Sources", nil); err == nil {
		t.Error("template without resolver must fail")
	}
	// Compare passes the resolver through and reports bad filters.
	l, r := trees(t, map[string]spec{"a.go": file("a", 0), "b.txt": file("b", 0)}, map[string]spec{})
	o := DefaultOptions()
	o.Filter, o.Templates = ">Sources", resolve
	equalLines(t, rows(mustCompare(t, l, r, o)), []string{"a.go leftOnly copyLeftToRight"})
	o.Filter = ">Missing"
	if _, err := Compare(context.Background(), DirFS(l), DirFS(r), o, nil); err == nil {
		t.Error("Compare accepted an unknown template")
	}
}

func TestCancelledCompareKeepsPartialResults(t *testing.T) {
	left := map[string]spec{}
	for _, d := range []string{"a", "b", "c", "d", "e", "f"} {
		left[d+"/x.txt"] = file("x", 0)
	}
	l, r := trees(t, left, map[string]spec{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The first progress report comes while the root folder is listed; Esc
	// there must keep whatever rows were found before the walk stopped.
	res, err := Compare(ctx, DirFS(l), DirFS(r), DefaultOptions(), func(CompareProgress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if res == nil || len(res.Items) == 0 {
		t.Fatal("partial results discarded")
	}
	if it := res.Find("a"); it == nil || it.Action != CopyLeftToRight {
		t.Errorf("partial rows lack their default actions: %v", rows(res))
	}
	if len(res.Items) >= 12 {
		t.Errorf("compare did not stop early: %d rows", len(res.Items))
	}
}
