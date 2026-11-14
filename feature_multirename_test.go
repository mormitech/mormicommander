package main

// Tests of the Multi-Rename tool's bound object: a whole session in a temporary folder (preview, sort, F5 step,
// loaded names, Start!, undo, presets), with the settings folder pointed at a temporary folder too.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"mormicommander/internal/multirename"
)

func mrSetup(t *testing.T, names ...string) (*MultiRename, string) {
	t.Helper()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &MultiRename{app: &App{}}, dir
}

func mrList(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func mrNews(p mrPreview) []string {
	var out []string
	for _, r := range p.Rows {
		out = append(out, r.New)
	}
	return out
}

func TestMultiRenameSession(t *testing.T) {
	m, dir := mrSetup(t, "b.txt", "a.txt")
	st, err := m.Start(dir, []string{"b.txt", "a.txt"})
	if err != nil || len(st.Rows) != 2 || st.Undo.Available {
		t.Fatalf("start: %v %+v", err, st)
	}
	s := multirename.DefaultSettings()
	s.NameMask = "[N]_[C]"
	p := m.Preview(s)
	if got := mrNews(p); got[0] != "b_1.txt" || got[1] != "a_2.txt" || p.Changes != 2 {
		t.Fatalf("preview: %v", got)
	}
	m.Sort("name", false) // F-519: the counter follows the list order
	if got := mrNews(m.Preview(s)); got[0] != "a_1.txt" || got[1] != "b_2.txt" {
		t.Fatalf("after sort: %v", got)
	}
	if err := m.Reorder([]int{1, 0}); err != nil {
		t.Fatal(err)
	}
	if got := mrNews(m.Preview(s)); got[0] != "b_1.txt" {
		t.Fatalf("after reorder: %v", got)
	}
	m.Preview(s)
	r, err := m.Execute()
	if err != nil || r.Renamed != 2 || !r.UndoSaved {
		t.Fatalf("execute: %v %+v", err, r)
	}
	if got := mrList(t, dir); got[0] != "a_2.txt" || got[1] != "b_1.txt" {
		t.Fatalf("on disk: %v", got)
	}
	// the list now shows the new names as the old ones
	if p := m.Preview(multirename.DefaultSettings()); p.Rows[0].Old != "b_1.txt" || p.Changes != 0 {
		t.Fatalf("after execute: %+v", p.Rows)
	}
	if m.ResultText() == "" || !m.UndoInfo().Available {
		t.Fatal("no result text or undo")
	}
	if n, err := m.Undo(); err != nil || n != 2 {
		t.Fatalf("undo: %d %v", n, err)
	}
	if got := mrList(t, dir); got[0] != "a.txt" || got[1] != "b.txt" {
		t.Fatalf("after undo: %v", got)
	}
	if m.UndoInfo().Available {
		t.Fatal("undo log kept after undo")
	}
}

func TestMultiRenameConflictsAndMaskErrors(t *testing.T) {
	m, dir := mrSetup(t, "a.txt", "b.txt", "x.txt")
	if _, err := m.Start(dir, []string{"a.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}
	s := multirename.DefaultSettings()
	s.NameMask = "same"
	p := m.Preview(s)
	if p.Errors != 2 || len(p.Rows[0].Problems) == 0 {
		t.Fatalf("duplicates not reported: %+v", p)
	}
	if _, err := m.Execute(); err == nil {
		t.Fatal("execute ran with conflicts")
	}
	s.NameMask = "x"
	s.ExtMask = "txt"
	if p := m.Preview(s); p.Errors == 0 {
		t.Fatal("existing target not reported")
	}
	s.NameMask = "[N"
	if p := m.Preview(s); p.Field != "nameMask" || p.FieldError == "" || p.Rows[0].New != "<Error!>" {
		t.Fatalf("mask error: %+v", p)
	}
	s = multirename.DefaultSettings()
	s.RegEx, s.Search = true, "("
	if p := m.Preview(s); p.Field != "search" {
		t.Fatalf("regex error: %+v", p)
	}
}

func TestMultiRenameStepsNamesAndPresets(t *testing.T) {
	m, dir := mrSetup(t, "one.txt", "two.txt")
	if _, err := m.Start(dir, []string{"one.txt", "two.txt"}); err != nil {
		t.Fatal(err)
	}
	s := multirename.DefaultSettings()
	s.Case = multirename.CaseUpper
	if err := m.NextStep(s); err != nil { // F5
		t.Fatal(err)
	}
	s2 := multirename.DefaultSettings()
	s2.NameMask = "[N]-x"
	p := m.Preview(s2)
	if !p.Steps || p.Rows[0].Old != "ONE.TXT" || p.Rows[0].New != "ONE-x.TXT" {
		t.Fatalf("steps: %+v", p.Rows[0])
	}
	m.Reset()
	names := filepath.Join(t.TempDir(), "names.txt")
	os.WriteFile(names, []byte("first.md\n"), 0o644)
	if n, err := m.LoadNamesFrom(names); err != nil || n != 1 {
		t.Fatalf("names: %d %v", n, err)
	}
	p = m.Preview(multirename.DefaultSettings())
	if p.Rows[0].New != "first.md" || p.Rows[1].New != "two.txt" || len(p.Warnings) == 0 || p.Names != 1 {
		t.Fatalf("loaded names: %+v", p)
	}
	t.Setenv("MORMICOMMANDER_NO_EDITOR", "1")
	path, err := m.EditNames(multirename.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("x1.txt\nx2.txt\n"), 0o644)
	if n, err := m.ReadEditedNames(); err != nil || n != 2 {
		t.Fatalf("edited names: %d %v", n, err)
	}
	if got := mrNews(m.Preview(multirename.DefaultSettings())); got[1] != "x2.txt" {
		t.Fatalf("edited preview: %v", got)
	}
	m.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("temporary names file kept after close")
	}

	s.NameMask = "[N]_[C]"
	if err := m.SavePreset("Numbered", s); err != nil {
		t.Fatal(err)
	}
	if list, _ := m.Presets(); len(list) != 1 || list[0] != "Numbered" {
		t.Fatalf("presets: %v", list)
	}
	if got, err := m.LoadPreset("Numbered"); err != nil || got.NameMask != "[N]_[C]" || got.Case != multirename.CaseUpper {
		t.Fatalf("load preset: %+v %v", got, err)
	}
	if err := m.DeletePreset("Numbered"); err != nil {
		t.Fatal(err)
	}
	if list, _ := m.Presets(); len(list) != 0 {
		t.Fatalf("preset not deleted: %v", list)
	}
}

func TestMultiRenameFolderWithContents(t *testing.T) {
	m, dir := mrSetup(t)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "f.txt"), nil, 0o644)
	if _, err := m.Start(dir, []string{"sub", filepath.Join("sub", "f.txt")}); err != nil {
		t.Fatal(err)
	}
	s := multirename.DefaultSettings()
	s.Case = multirename.CaseUpper
	m.Preview(s)
	if _, err := m.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SUB", "F.TXT")); err != nil {
		t.Fatalf("folder and file not renamed: %v", err)
	}
	if p := m.Preview(multirename.DefaultSettings()); p.Rows[1].Dir != filepath.Join(dir, "SUB") || p.Rows[1].Old != "F.TXT" {
		t.Fatalf("list not updated: %+v", p.Rows)
	}
}

func TestMultiRenameMapPath(t *testing.T) {
	sep := string(filepath.Separator)
	moved := map[string]string{sep + "a": sep + "A", sep + "a" + sep + "b": sep + "a" + sep + "B"}
	if got := mrMapPath(sep+"a"+sep+"b"+sep+"c", moved); got != sep+"A"+sep+"B"+sep+"c" {
		t.Fatalf("got %s", got)
	}
}

// TestMultiRenameInBranchView is F-511: the files of a branch view (feature_branch.go), named by their paths relative
// to the folder it lists, are renamed each in its own folder. [P] is each file's own parent folder, the same new name
// in two folders is fine while two files of one folder with the same new name are a conflict, and undo puts every
// file back in its folder.
func TestMultiRenameInBranchView(t *testing.T) {
	m, dir := mrSetup(t)
	for _, n := range []string{"top.txt", "jan/a.txt", "jan/b.txt", "feb/a.txt"} {
		findWriteFile(t, filepath.Join(dir, filepath.FromSlash(n)), n)
	}
	l, err := branchFS{}.List(t.Context(), "branch://"+dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range l.Entries {
		names = append(names, e.Name)
	}
	sort.Strings(names) // feb/a.txt, jan/a.txt, jan/b.txt, top.txt
	if len(names) != 4 {
		t.Fatalf("branch names %v", names)
	}
	st, err := m.Start(l.Parent, names)
	if err != nil {
		t.Fatal(err)
	}
	if st.Rows[0].Dir != filepath.Join(dir, "feb") || st.Rows[0].Old != "a.txt" || st.Rows[3].Dir != dir {
		t.Fatalf("rows %+v", st.Rows)
	}
	s := multirename.DefaultSettings()
	s.NameMask = "[P]"
	p := m.Preview(s)
	// feb/a → feb.txt, jan/a and jan/b → jan.txt twice (a conflict in jan), top → <base>.txt
	if p.Errors != 2 || len(p.Rows[1].Problems) == 0 || len(p.Rows[2].Problems) == 0 || len(p.Rows[0].Problems) != 0 {
		t.Fatalf("conflicts per folder: %+v", p)
	}
	s.NameMask = "[P]_[N]"
	p = m.Preview(s)
	if p.Errors != 0 || mrNews(p)[0] != "feb_a.txt" || mrNews(p)[2] != "jan_b.txt" {
		t.Fatalf("preview %v %+v", mrNews(p), p.Warnings)
	}
	s.NameMask = "same"
	if p := m.Preview(s); p.Errors != 2 {
		t.Fatalf("same name in jan twice should be the only conflict: %+v", p)
	}
	s.NameMask = "x_[N]"
	m.Preview(s)
	if _, err := m.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := mrList(t, filepath.Join(dir, "jan")); len(got) != 2 || got[0] != "x_a.txt" || got[1] != "x_b.txt" {
		t.Fatalf("jan after rename: %v", got)
	}
	if got := mrList(t, filepath.Join(dir, "feb")); len(got) != 1 || got[0] != "x_a.txt" {
		t.Fatalf("feb after rename: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "x_top.txt")); err != nil {
		t.Fatal("the file of the folder itself was not renamed")
	}
	if n, err := m.Undo(); err != nil || n != 4 {
		t.Fatalf("undo: %d %v", n, err)
	}
	for _, n := range []string{"top.txt", "jan/a.txt", "jan/b.txt", "feb/a.txt"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(n))); err != nil {
			t.Errorf("%s not back after undo", n)
		}
	}
}
