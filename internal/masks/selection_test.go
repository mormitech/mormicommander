// Tests for the selection engine: group select, kinds, invert, same extension/name, restore and slots.

package masks

import (
	"reflect"
	"testing"
)

func panel() []Entry {
	return []Entry{
		{Name: "..", IsDir: true, Size: -1},
		{Name: "docs", IsDir: true, Size: -1},
		{Name: "src.d", IsDir: true, Size: 100},
		{Name: "a.txt", Size: 10},
		{Name: "B.TXT", Size: 20},
		{Name: "a.go", Size: 30},
		{Name: "readme", Size: 40},
		{Name: "a", Size: 50},
	}
}

func newSel(foldersToo bool) *Selection {
	return NewSelection(panel(), SelectionOptions{Case: CaseInsensitive, FoldersToo: foldersToo})
}

func TestSelectGroupKinds(t *testing.T) {
	tests := []struct {
		name string
		expr string
		kind Kind
		want []string
	}{
		{"files only", "*", KindFiles, []string{"a.txt", "B.TXT", "a.go", "readme", "a"}},
		{"folders only", "*", KindFolders, []string{"docs", "src.d"}},
		{"both, never ..", "*", KindBoth, []string{"docs", "src.d", "a.txt", "B.TXT", "a.go", "readme", "a"}},
		{"case-insensitive ext", "*.txt", KindFiles, []string{"a.txt", "B.TXT"}},
		{"exclusion", "*|*.go", KindFiles, []string{"a.txt", "B.TXT", "readme", "a"}},
		{"folder mask", "*.d\\", KindBoth, []string{"src.d"}},
	}
	for _, tt := range tests {
		s := newSel(false)
		s.SelectGroup(MustParse(tt.expr, Options{Case: CaseInsensitive}), tt.kind)
		if got := s.SelectedNames(); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGroupKindOption(t *testing.T) {
	if k := newSel(false).GroupKind(false); k != KindFiles {
		t.Errorf("Num+ default = %v", k)
	}
	if k := newSel(false).GroupKind(true); k != KindBoth {
		t.Errorf("Shift+Num+ default = %v", k)
	}
	if k := newSel(true).GroupKind(false); k != KindBoth {
		t.Errorf("Num+ with folders = %v", k)
	}
	if k := newSel(true).GroupKind(true); k != KindFiles {
		t.Errorf("Shift+Num+ with folders = %v", k)
	}
}

func TestUnselectInvertClear(t *testing.T) {
	s := newSel(false)
	s.SelectAll(KindBoth)
	s.UnselectGroup(MustParse("a*", Options{Case: CaseInsensitive}), KindFiles)
	if got, want := s.SelectedNames(), []string{"docs", "src.d", "B.TXT", "readme"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unselect: %v", got)
	}
	s.Invert(KindFiles)
	if got, want := s.SelectedNames(), []string{"docs", "src.d", "a.txt", "a.go", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invert files: %v", got)
	}
	s.Invert(KindFolders)
	if got, want := s.SelectedNames(), []string{"a.txt", "a.go", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invert folders: %v", got)
	}
	s.SelectAll(KindFolders)
	s.ClearAll(KindFiles)
	if got, want := s.SelectedNames(), []string{"docs", "src.d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("clear files: %v", got)
	}
	if s.IsSelected(0) {
		t.Fatal(".. selected")
	}
	if s.Toggle(0) {
		t.Fatal(".. toggled")
	}
}

func TestSameExtAndName(t *testing.T) {
	s := newSel(false)
	if n := s.SelectSameExt(3, true, KindFiles); n != 2 { // a.txt -> a.txt, B.TXT
		t.Errorf("same ext selected %d", n)
	}
	s.SelectSameExt(4, false, KindFiles)
	if s.Count() != 0 {
		t.Errorf("unselect same ext left %v", s.SelectedNames())
	}
	s.SelectSameExt(6, true, KindFiles) // readme: no extension -> readme, a
	if got, want := s.SelectedNames(), []string{"readme", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("no-ext: %v", got)
	}
	s.Clear()
	s.SelectSameName(3, true, KindFiles) // base "a" -> a.txt, a.go, a
	if got, want := s.SelectedNames(), []string{"a.txt", "a.go", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same name: %v", got)
	}
	s.Clear()
	s.SelectSameNameExt(4, true, KindFiles)
	if got, want := s.SelectedNames(), []string{"B.TXT"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same name+ext: %v", got)
	}
	if s.SelectSameExt(0, true, KindBoth) != 0 {
		t.Error("acting on .. must do nothing")
	}
}

func TestRangeStatsRestoreSlots(t *testing.T) {
	s := newSel(false)
	if n := s.SetRange(5, 0, true); n != 5 { // .. skipped
		t.Fatalf("range marked %d", n)
	}
	st := s.Stats()
	if st.Files != 3 || st.Folders != 2 || st.Bytes != 160 || st.FoldersSized != 1 {
		t.Fatalf("stats %+v", st)
	}
	before := s.SelectedNames()
	s.ClearAll(KindBoth)
	if s.Count() != 0 || s.Restore() != len(before) || !reflect.DeepEqual(s.SelectedNames(), before) {
		t.Fatalf("restore: %v", s.SelectedNames())
	}
	s.SaveSlot()
	s.Clear()
	s.Set(7, true)
	if s.LoadSlot() != len(before) || !reflect.DeepEqual(s.SelectedNames(), before) {
		t.Fatalf("slot: %v", s.SelectedNames())
	}
	if !reflect.DeepEqual(s.Slot(), before) {
		t.Fatal("Slot copy")
	}
}

func TestSetEntriesKeepsMarks(t *testing.T) {
	s := newSel(false)
	s.SelectNames([]string{"a.go", "readme"}, true)
	reordered := []Entry{{Name: "readme"}, {Name: "new"}, {Name: "a.go"}}
	s.SetEntries(reordered)
	if got := s.Selected(); !reflect.DeepEqual(got, []int{0, 2}) {
		t.Errorf("after refresh: %v", got)
	}
	s.SetMarks([]bool{false, true})
	if got := s.SelectedNames(); !reflect.DeepEqual(got, []string{"new"}) {
		t.Errorf("SetMarks: %v", got)
	}
	if got := len(s.SelectedEntries()); got != 1 {
		t.Errorf("SelectedEntries: %d", got)
	}
}

func TestSelectNamesFromOtherPanelCase(t *testing.T) {
	cs := NewSelection(panel(), SelectionOptions{Case: CaseSensitive})
	if n := cs.SelectNames([]string{"b.txt"}, true); n != 0 {
		t.Errorf("case-sensitive matched %d", n)
	}
	ci := newSel(false)
	if n := ci.SelectNames([]string{"b.txt", "DOCS"}, true); n != 2 {
		t.Errorf("case-insensitive matched %d", n)
	}
}
