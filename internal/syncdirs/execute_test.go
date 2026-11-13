// Tests for the synchronize half: plans and their dialog checkboxes, full
// two-way and asymmetric syncs followed by a re-compare that must show
// everything equal (the F-551 acceptance check), the "changed since compare"
// protection, overwrite and error hooks, cancellation without leftovers,
// folder and symlink safety, and progress reporting.
package syncdirs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func syncAll(t *testing.T, res *Result, l, r string, f PlanFilter, o ExecOptions) *Report {
	t.Helper()
	rep, err := Execute(context.Background(), res.Plan(f), DirFS(l), DirFS(r), o)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return rep
}

func TestSyncThenRecompareIsEqual(t *testing.T) {
	left := map[string]spec{
		"same.txt":        file("same", 0),
		"newer-left.txt":  file("left wins", time.Hour),
		"older-left.txt":  file("old", 0),
		"left-only.txt":   file("L", 0),
		"tree/a/b/c.txt":  file("deep", 0),
		"tree/empty/":     {},
		"shared/l.txt":    file("l", 0),
		"shared/both.txt": file("1", 0),
	}
	right := map[string]spec{
		"same.txt":        file("same", 0),
		"newer-left.txt":  file("right", 0),
		"older-left.txt":  file("right is newer", time.Hour),
		"right-only.txt":  file("R", 0),
		"rtree/x/y.txt":   file("deep right", 0),
		"shared/r.txt":    file("r", 0),
		"shared/both.txt": file("1", 0),
	}
	for _, asym := range []bool{false, true} {
		name := "symmetric"
		if asym {
			name = "asymmetric"
		}
		t.Run(name, func(t *testing.T) {
			l, r := trees(t, left, right)
			o := DefaultOptions()
			o.Asymmetric = asym
			res := mustCompare(t, l, r, o)
			rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
			if len(rep.Errors) > 0 {
				t.Fatalf("errors: %v", rep.Errors)
			}
			again := mustCompare(t, l, r, o)
			if s := again.Summary(); !s.AllEqual() {
				t.Fatalf("not equal after sync: %+v\n%s", s, strings.Join(rows(again), "\n"))
			}
			if got := readFile(t, filepath.Join(r, "newer-left.txt")); got != "left wins" {
				t.Errorf("right newer-left.txt = %q", got)
			}
			if asym {
				if got := readFile(t, filepath.Join(r, "older-left.txt")); got != "old" {
					t.Errorf("asymmetric did not overwrite right: %q", got)
				}
				for _, gone := range []string{"right-only.txt", "rtree", "shared/r.txt"} {
					if _, err := os.Lstat(filepath.Join(r, gone)); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("%s still on the right (%v)", gone, err)
					}
				}
				if _, err := os.Lstat(filepath.Join(l, "right-only.txt")); err == nil {
					t.Errorf("asymmetric changed the left side")
				}
			} else {
				if got := readFile(t, filepath.Join(l, "older-left.txt")); got != "right is newer" {
					t.Errorf("left older-left.txt = %q", got)
				}
				if got := readFile(t, filepath.Join(l, "rtree", "x", "y.txt")); got != "deep right" {
					t.Errorf("deep copy right->left = %q", got)
				}
				if fi, err := os.Stat(filepath.Join(r, "tree", "empty")); err != nil || !fi.IsDir() {
					t.Errorf("empty folder not copied: %v", err)
				}
			}
			// Copied files keep their dates.
			fi, err := os.Stat(filepath.Join(r, "tree", "a", "b", "c.txt"))
			if err != nil || !fi.ModTime().Equal(t0) {
				t.Errorf("copied file date = %v (%v), want %v", fi.ModTime(), err, t0)
			}
			src, _ := os.Stat(filepath.Join(l, "tree", "a"))
			if fi, err := os.Stat(filepath.Join(r, "tree", "a")); err != nil || !fi.ModTime().Equal(src.ModTime()) {
				t.Errorf("copied folder date not restored: %v", err)
			}
		})
	}
}

func TestPlanFilterAndTotals(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"lo": file("12345", 0), "ln": file("ab", time.Hour), "del": file("x", 0)},
		map[string]spec{"ro": file("123", 0), "ln": file("a", 0)})
	res := mustCompare(t, l, r, DefaultOptions())
	if err := res.SetAction("del", DeleteLeft); err != nil {
		t.Fatal(err)
	}
	p := res.Plan(AllOps())
	if p.FilesLR != 2 || p.BytesLR != 7 || p.FilesRL != 1 || p.BytesRL != 3 || p.DeletesLeft != 1 || p.DeletesRight != 0 {
		t.Errorf("plan totals = %+v", p)
	}
	if last := p.Ops[len(p.Ops)-1]; last.Kind != OpDelete || last.Dst != "del" {
		t.Errorf("deletes are not last: %+v", last)
	}
	only := res.Plan(PlanFilter{LeftToRight: true})
	if len(only.Ops) != 2 || only.NeedsWriteLeft || !only.NeedsWriteRight {
		t.Errorf("left-to-right plan = %+v", only)
	}
	syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{})
	if _, err := os.Stat(filepath.Join(l, "ro")); err == nil {
		t.Errorf("right->left copy ran although unchecked")
	}
	if _, err := os.Stat(filepath.Join(l, "del")); err != nil {
		t.Errorf("delete ran although unchecked")
	}
	if got := readFile(t, filepath.Join(r, "lo")); got != "12345" {
		t.Errorf("lo = %q", got)
	}
}

func TestChangedSinceCompareIsSkipped(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"over": file("new", time.Hour), "fresh": file("left", 0), "del": file("d", 0)},
		map[string]spec{"over": file("old", 0), "del2": file("d", 0)})
	res := mustCompare(t, l, r, DefaultOptions())
	if err := res.SetAction("del", DeleteLeft); err != nil {
		t.Fatal(err)
	}
	if err := res.SetAction("del2", DeleteRight); err != nil {
		t.Fatal(err)
	}
	// Someone edits the right file, creates the would-be target and
	// changes a file marked for deletion after the compare.
	build(t, r, map[string]spec{"over": file("edited meanwhile", 2*time.Hour), "fresh": file("other", 0)})
	build(t, l, map[string]spec{"del": file("changed!", 0)})
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 3 {
		t.Fatalf("errors = %v", rep.Errors)
	}
	for _, e := range rep.Errors {
		if !errors.Is(e.Err, ErrChanged) {
			t.Errorf("error %v is not ErrChanged", e)
		}
	}
	if got := readFile(t, filepath.Join(r, "over")); got != "edited meanwhile" {
		t.Errorf("edited file overwritten: %q", got)
	}
	if got := readFile(t, filepath.Join(r, "fresh")); got != "other" {
		t.Errorf("new file overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(l, "del")); err != nil {
		t.Errorf("changed file deleted")
	}
	if _, err := os.Stat(filepath.Join(r, "del2")); err == nil {
		t.Errorf("unchanged del2 not deleted")
	}
	// Force overrides the checks.
	rep = syncAll(t, res, l, r, PlanFilter{LeftToRight: true}, ExecOptions{Force: true})
	if len(rep.Errors) != 0 || readFile(t, filepath.Join(r, "over")) != "new" {
		t.Errorf("force failed: %v", rep.Errors)
	}
}

func TestDeleteAlreadyGoneCountsAsDone(t *testing.T) {
	l, r := trees(t, map[string]spec{}, map[string]spec{"x": file("1", 0)})
	o := DefaultOptions()
	o.Asymmetric = true
	res := mustCompare(t, l, r, o)
	if err := os.Remove(filepath.Join(r, "x")); err != nil {
		t.Fatal(err)
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if rep.Done != 1 || len(rep.Errors) != 0 {
		t.Errorf("report = %+v", rep)
	}
}

func TestOverwriteConfirmation(t *testing.T) {
	left := map[string]spec{"a": file("A2", time.Hour), "b": file("B2", time.Hour), "c": file("C2", time.Hour), "new": file("N", 0)}
	right := map[string]spec{"a": file("A1", 0), "b": file("B1", 0), "c": file("C1", 0)}
	cases := []struct {
		name    string
		answers []OverwriteDecision
		asked   int
		want    string // contents of a,b,c on the right
		skipped int
		err     error
	}{
		{"overwrite each", []OverwriteDecision{Overwrite, Overwrite, Overwrite}, 3, "A2B2C2", 0, nil},
		{"skip one", []OverwriteDecision{Overwrite, SkipFile, Overwrite}, 3, "A2B1C2", 1, nil},
		{"overwrite all", []OverwriteDecision{OverwriteAll}, 1, "A2B2C2", 0, nil},
		{"skip all", []OverwriteDecision{Overwrite, SkipAll}, 2, "A2B1C1", 2, nil},
		{"cancel", []OverwriteDecision{Overwrite, CancelSync}, 2, "A2B1C1", 0, context.Canceled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, r := trees(t, left, right)
			res := mustCompare(t, l, r, DefaultOptions())
			asked := 0
			rep, err := Execute(context.Background(), res.Plan(AllOps()), DirFS(l), DirFS(r), ExecOptions{
				ConfirmOverwrite: func(op *Op) OverwriteDecision {
					d := c.answers[asked]
					asked++
					return d
				},
			})
			if !errors.Is(err, c.err) || (c.err == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.err)
			}
			got := readFile(t, filepath.Join(r, "a")) + readFile(t, filepath.Join(r, "b")) + readFile(t, filepath.Join(r, "c"))
			if got != c.want || asked != c.asked || rep.Skipped != c.skipped {
				t.Errorf("contents %s asked %d skipped %d; want %s %d %d", got, asked, rep.Skipped, c.want, c.asked, c.skipped)
			}
		})
	}
}

func TestErrorHookRetryAndAbort(t *testing.T) {
	t.Run("retry", func(t *testing.T) {
		l, r := trees(t, map[string]spec{"x": file("1", 0)}, map[string]spec{})
		res := mustCompare(t, l, r, DefaultOptions())
		block := filepath.Join(r, "x")
		build(t, r, map[string]spec{"x": file("blocker", 0)})
		calls := 0
		rep, err := Execute(context.Background(), res.Plan(AllOps()), DirFS(l), DirFS(r), ExecOptions{
			OnError: func(op *Op, err error) ErrorDecision {
				calls++
				_ = os.Remove(block)
				return Retry
			},
		})
		if err != nil || calls != 1 || rep.Done != 1 || readFile(t, block) != "1" {
			t.Errorf("err=%v calls=%d rep=%+v", err, calls, rep)
		}
	})
	t.Run("abort", func(t *testing.T) {
		l, r := trees(t, map[string]spec{"a": file("1", 0), "b": file("2", 0)}, map[string]spec{})
		res := mustCompare(t, l, r, DefaultOptions())
		build(t, r, map[string]spec{"a": file("blocker", 0)})
		rep, err := Execute(context.Background(), res.Plan(AllOps()), DirFS(l), DirFS(r), ExecOptions{
			OnError: func(*Op, error) ErrorDecision { return Abort },
		})
		if !errors.Is(err, ErrAborted) || len(rep.Errors) != 1 {
			t.Fatalf("err=%v rep=%+v", err, rep)
		}
		if _, err := os.Stat(filepath.Join(r, "b")); err == nil {
			t.Errorf("b copied after abort")
		}
	})
}

func TestCancelMidCopyLeavesNothing(t *testing.T) {
	big := strings.Repeat("0123456789abcdef", 4*chunkSize/16)
	l, r := trees(t, map[string]spec{"big": file(big, 0), "after": file("x", 0)}, map[string]spec{})
	res := mustCompare(t, l, r, DefaultOptions())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := Execute(ctx, res.Plan(AllOps()), DirFS(l), DirFS(r), ExecOptions{
		Progress: func(p Progress) {
			if p.FileDone > 0 {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	ents, _ := os.ReadDir(r)
	for _, e := range ents {
		if e.Name() != "after" {
			t.Errorf("left behind: %s", e.Name())
		}
	}
	if len(ents) != 0 {
		t.Errorf("right side not empty: %d entries", len(ents))
	}
}

func TestProgressReports(t *testing.T) {
	l, r := trees(t, map[string]spec{"a": file(strings.Repeat("a", chunkSize+10), 0), "d/b": file("bb", 0)}, map[string]spec{})
	res := mustCompare(t, l, r, DefaultOptions())
	var last Progress
	var prev int64
	mono := true
	syncAll(t, res, l, r, AllOps(), ExecOptions{Progress: func(p Progress) {
		if p.BytesDone < prev {
			mono = false
		}
		prev = p.BytesDone
		last = p
	}})
	if !mono {
		t.Errorf("BytesDone went backwards")
	}
	if last.BytesDone != last.BytesTotal || last.BytesTotal != int64(chunkSize+12) || last.OpsDone != last.OpsTotal || last.OpsTotal != 3 || last.Op != nil {
		t.Errorf("final progress = %+v", last)
	}
}

func TestFolderDeleteOnlyWhenEmpty(t *testing.T) {
	l, r := trees(t, map[string]spec{"d/keep": file("1", 0)}, map[string]spec{})
	res := mustCompare(t, l, r, DefaultOptions())
	if err := res.SetAction("d", DeleteLeft); err != nil {
		t.Fatal(err)
	}
	if err := res.SetAction("d/keep", NoAction); err != nil {
		t.Fatal(err)
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 1 || rep.Errors[0].Op.Kind != OpRemoveDir {
		t.Errorf("errors = %v", rep.Errors)
	}
	if readFile(t, filepath.Join(l, "d", "keep")) != "1" {
		t.Errorf("file inside folder deleted")
	}
	// With the whole tree marked, the folder goes after its contents.
	res = mustCompare(t, l, r, DefaultOptions())
	if _, err := res.SetActionTree("d", DeleteLeft); err != nil {
		t.Fatal(err)
	}
	rep = syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 0 || rep.Done != 2 {
		t.Errorf("tree delete: %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(l, "d")); err == nil {
		t.Errorf("folder still there")
	}
}

func TestSymlinkCopyAndDeleteAreLinkOnly(t *testing.T) {
	outside := t.TempDir()
	build(t, outside, map[string]spec{"precious.txt": file("keep me", 0)})
	l, r := trees(t,
		map[string]spec{"lnk": {link: "target.txt"}, "out": {link: outside}},
		map[string]spec{"rout": {link: outside}})
	o := DefaultOptions()
	o.Asymmetric = true // rout is marked for deletion
	res := mustCompare(t, l, r, o)
	if it := res.Find("rout"); it.Action != DeleteRight || it.Right.Kind != KindLink {
		t.Fatalf("rout = %+v", it)
	}
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 0 {
		t.Fatalf("errors: %v", rep.Errors)
	}
	if tgt, err := os.Readlink(filepath.Join(r, "lnk")); err != nil || tgt != "target.txt" {
		t.Errorf("link not recreated: %q %v", tgt, err)
	}
	if tgt, err := os.Readlink(filepath.Join(r, "out")); err != nil || tgt != outside {
		t.Errorf("folder link not recreated as link: %q %v", tgt, err)
	}
	if _, err := os.Lstat(filepath.Join(r, "rout")); err == nil {
		t.Errorf("link not deleted")
	}
	if readFile(t, filepath.Join(outside, "precious.txt")) != "keep me" {
		t.Errorf("link target touched")
	}
	if s := mustCompare(t, l, r, o).Summary(); !s.AllEqual() {
		t.Errorf("links not equal after sync: %+v", s)
	}
}

func TestOSFSRefusesWritesThroughSymlinks(t *testing.T) {
	outside := t.TempDir()
	build(t, outside, map[string]spec{"victim.txt": file("keep", 0)})
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "esc")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	f := DirFS(root)
	checks := []struct {
		name string
		do   func() error
	}{
		{"mkdir", func() error { return f.Mkdir("esc/new", 0o755) }},
		{"remove", func() error { return f.Remove("esc/victim.txt") }},
		{"createtemp", func() error { _, _, err := f.CreateTemp("esc"); return err }},
		{"chtimes", func() error { return f.Chtimes("esc/victim.txt", t0) }},
		{"chmod", func() error { return f.Chmod("esc/victim.txt", 0o600) }},
		{"symlink", func() error { return f.Symlink("x", "esc/l") }},
		{"rename", func() error { return f.Rename("esc/victim.txt", "stolen") }},
		{"dotdot", func() error { return f.Remove("../x") }},
		{"root", func() error { return f.Remove(".") }},
	}
	for _, c := range checks {
		if err := c.do(); err == nil {
			t.Errorf("%s through symlink succeeded", c.name)
		} else if c.name != "dotdot" && c.name != "root" && !errors.Is(err, ErrSymlinkInPath) {
			t.Errorf("%s: err = %v, want ErrSymlinkInPath", c.name, err)
		}
	}
	if readFile(t, filepath.Join(outside, "victim.txt")) != "keep" {
		t.Errorf("victim changed")
	}
	// Removing the link itself is allowed and leaves the target alone.
	if err := f.Remove("esc"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(outside, "victim.txt")) != "keep" {
		t.Errorf("victim changed by link removal")
	}
}

func TestReadOnlyFilesCopyAndOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	l, r := trees(t, map[string]spec{"ro": file("new", time.Hour)}, map[string]spec{"ro": file("old", 0)})
	if err := os.Chmod(filepath.Join(l, "ro"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(r, "ro"), 0o444); err != nil {
		t.Fatal(err)
	}
	res := mustCompare(t, l, r, DefaultOptions())
	rep := syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if len(rep.Errors) != 0 {
		t.Fatalf("errors: %v", rep.Errors)
	}
	fi, err := os.Stat(filepath.Join(r, "ro"))
	if err != nil || fi.Mode().Perm() != 0o444 || readFile(t, filepath.Join(r, "ro")) != "new" {
		t.Errorf("mode %v err %v", fi.Mode(), err)
	}
}

func TestCaseMismatchCopiesIntoExistingFolder(t *testing.T) {
	o := DefaultOptions()
	o.Case = CaseInsensitive
	l, r := trees(t, map[string]spec{"Docs/new.txt": file("n", 0)}, map[string]spec{"docs/": {}})
	res := mustCompare(t, l, r, o)
	syncAll(t, res, l, r, AllOps(), ExecOptions{})
	if readFile(t, filepath.Join(r, "docs", "new.txt")) != "n" {
		t.Errorf("not copied into the existing folder")
	}
	ents, _ := os.ReadDir(r)
	if len(ents) != 1 || ents[0].Name() != "docs" {
		t.Errorf("right root = %v", ents)
	}
}

func TestOpErrorText(t *testing.T) {
	e := OpError{Op: Op{Kind: OpCopyFile, Side: RightSide, Dst: "a/b"}, Err: ErrChanged}
	if !strings.Contains(e.Error(), "copyFile right a/b") || !errors.Is(e, ErrChanged) {
		t.Errorf("OpError = %q", e.Error())
	}
}
