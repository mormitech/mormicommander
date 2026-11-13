// These tests run real renames in temporary folders: simple batches, chains
// and swaps that need temporary names, folders renamed together with their
// contents, rollback after an injected failure, targets that appear after
// the preview, and undo both directly and through a saved log file.

package multirename

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// listTree returns "path=content" for every file and "path/" for every
// folder under root, sorted, so a whole tree can be compared in one string.
func listTree(t *testing.T, root string) string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			out = append(out, rel+"/")
		} else {
			b, _ := os.ReadFile(p)
			out = append(out, rel+"="+string(b))
		}
		return nil
	})
	sort.Strings(out)
	return strings.Join(out, " ")
}

func previewOrFail(t *testing.T, items []Item, s Settings) *Result {
	t.Helper()
	res, err := Preview(items, s, PreviewOptions{CaseRule: CaseRuleSensitive})
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("unexpected conflicts: %+v", res.Entries)
	}
	return res
}

func TestExecuteAndUndo(t *testing.T) {
	cases := []struct {
		name   string
		files  []string
		mask   string
		ext    string
		before string
		after  string
	}{
		{"simple", []string{"a.txt", "b.txt"}, "[N]_[C]", "[E]",
			"a.txt=a.txt b.txt=b.txt", "a_1.txt=a.txt b_2.txt=b.txt"},
		{"swap", []string{"1.txt", "2.txt"}, "[C2+-1]", "[E]",
			"1.txt=1.txt 2.txt=2.txt", "1.txt=2.txt 2.txt=1.txt"},
		{"chain", []string{"a", "a_x", "a_x_x"}, "[N]_x", "",
			"a=a a_x=a_x a_x_x=a_x_x", "a_x=a a_x_x=a_x a_x_x_x=a_x_x"},
		{"rotate three", []string{"1", "2", "3"}, "[C2]", "",
			"1=1 2=2 3=3", "2=1 3=2 4=3"},
		{"unchanged skipped", []string{"keep.txt", "x.txt"}, "[N]", "md",
			"keep.txt=keep.txt x.txt=x.txt", "keep.md=keep.txt x.md=x.txt"},
	}
	for _, c := range cases {
		dir, items := makeFiles(t, c.files...)
		if got := listTree(t, dir); got != c.before {
			t.Fatalf("%s: setup %q", c.name, got)
		}
		s := DefaultSettings()
		s.NameMask, s.ExtMask = c.mask, c.ext
		res := previewOrFail(t, items, s)
		var progress []int
		log, err := Execute(res, ExecOptions{Progress: func(d, total int) { progress = append(progress, d) }})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := listTree(t, dir); got != c.after {
			t.Errorf("%s: after = %q, want %q", c.name, got, c.after)
		}
		if len(progress) != res.Changes || (len(progress) > 0 && progress[len(progress)-1] != res.Changes) {
			t.Errorf("%s: progress %v for %d changes", c.name, progress, res.Changes)
		}
		logPath := filepath.Join(t.TempDir(), "undo", "last.json")
		if err := SaveUndoLog(logPath, log); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadUndoLog(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := Undo(loaded, ExecOptions{}); err != nil {
			t.Fatalf("%s: undo: %v", c.name, err)
		}
		if got := listTree(t, dir); got != c.before {
			t.Errorf("%s: after undo = %q, want %q", c.name, got, c.before)
		}
	}
}

func TestExecuteFoldersWithContents(t *testing.T) {
	dir, _ := makeFiles(t, "top/", "top/sub/", "top/sub/f.txt", "top/g.txt")
	items := []Item{
		{Dir: dir, Name: "top", IsDir: true},
		{Dir: filepath.Join(dir, "top"), Name: "sub", IsDir: true},
		{Dir: filepath.Join(dir, "top", "sub"), Name: "f.txt"},
		{Dir: filepath.Join(dir, "top"), Name: "g.txt"},
	}
	s := DefaultSettings()
	s.Case = CaseUpper
	res := previewOrFail(t, items, s)
	before := listTree(t, dir)
	log, err := Execute(res, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "TOP/ TOP/G.TXT=top/g.txt TOP/SUB/ TOP/SUB/F.TXT=top/sub/f.txt"
	if got := listTree(t, dir); got != want {
		t.Errorf("after = %q, want %q", got, want)
	}
	for _, op := range log.Ops {
		if op.OldName == "f.txt" && op.Dir != filepath.Join(dir, "TOP", "SUB") {
			t.Errorf("undo log keeps the old folder path for f.txt: %q", op.Dir)
		}
	}
	if err := Undo(log, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := listTree(t, dir); got != before {
		t.Errorf("after undo = %q, want %q", got, before)
	}
}

// failFS wraps the real file system and fails the n-th rename.
type failFS struct {
	OSFS
	failAt int
	calls  int
}

func (f *failFS) Rename(o, n string) error {
	f.calls++
	if f.calls == f.failAt {
		return errors.New("injected failure")
	}
	return os.Rename(o, n)
}

func TestExecuteRollback(t *testing.T) {
	for _, failAt := range []int{1, 2, 3, 4} {
		// A swap needs four raw renames (two to temporary names, two back).
		dir, items := makeFiles(t, "1.txt", "2.txt")
		s := DefaultSettings()
		s.NameMask = "[C2+-1]"
		res := previewOrFail(t, items, s)
		before := listTree(t, dir)
		_, err := Execute(res, ExecOptions{FS: &failFS{failAt: failAt}})
		var ee *ExecError
		if !errors.As(err, &ee) || len(ee.RollbackErrs) != 0 {
			t.Fatalf("failAt %d: want clean ExecError, got %v", failAt, err)
		}
		if !strings.Contains(err.Error(), "injected failure") || !strings.Contains(err.Error(), "rolled back") {
			t.Errorf("error text %q", err)
		}
		if got := listTree(t, dir); got != before {
			t.Errorf("failAt %d: tree after rollback %q, want %q", failAt, got, before)
		}
	}
}

func TestExecuteRefusesConflictsAndLateTargets(t *testing.T) {
	_, items := makeFiles(t, "a.txt", "b.txt")
	s := DefaultSettings()
	s.NameMask = "same"
	res, _ := Preview(items, s, PreviewOptions{CaseRule: CaseRuleSensitive})
	if _, err := Execute(res, ExecOptions{}); !errors.Is(err, ErrHasConflicts) {
		t.Errorf("want ErrHasConflicts, got %v", err)
	}

	dir, items := makeFiles(t, "a.txt", "b.txt")
	s.NameMask = "[N]_new"
	res = previewOrFail(t, items, s)
	// Someone creates a target after the preview was shown.
	if err := os.WriteFile(filepath.Join(dir, "b_new.txt"), []byte("intruder"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(res, ExecOptions{})
	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("want ErrTargetExists, got %v", err)
	}
	want := "a.txt=a.txt b.txt=b.txt b_new.txt=intruder"
	if got := listTree(t, dir); got != want {
		t.Errorf("tree = %q, want %q", got, want)
	}
}

func TestUndoRefusesToOverwrite(t *testing.T) {
	dir, items := makeFiles(t, "a.txt")
	s := DefaultSettings()
	s.NameMask = "b"
	log, err := Execute(previewOrFail(t, items, s), ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(log, ExecOptions{}); !errors.Is(err, ErrTargetExists) {
		t.Errorf("want ErrTargetExists, got %v", err)
	}
	if got := listTree(t, dir); got != "a.txt=new b.txt=a.txt" {
		t.Errorf("tree = %q", got)
	}
	if err := Undo(nil, ExecOptions{}); err == nil {
		t.Error("Undo(nil) should fail")
	}
}

func TestCaseOnlyRenameOnDisk(t *testing.T) {
	dir, items := makeFiles(t, "photo.JPG")
	s := DefaultSettings()
	s.Case = CaseLower
	res, err := Preview(items, s, PreviewOptions{})
	if err != nil || res.HasErrors() {
		t.Fatalf("preview: %v %+v", err, res)
	}
	if _, err := Execute(res, ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := listTree(t, dir); got != "photo.jpg=photo.JPG" {
		t.Errorf("tree = %q", got)
	}
}

func TestLoadUndoLogErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{"), 0o644)
	if _, err := LoadUndoLog(bad); err == nil {
		t.Error("want error for bad JSON")
	}
	v2 := filepath.Join(dir, "v2.json")
	os.WriteFile(v2, []byte(`{"version":2}`), 0o644)
	if _, err := LoadUndoLog(v2); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("want version error, got %v", err)
	}
	if _, err := LoadUndoLog(filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want not-exist, got %v", err)
	}
}
