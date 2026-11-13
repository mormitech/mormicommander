// Tests for saving and loading selections: line endings, encodings, full paths, wildcards and replace vs add.

package masks

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWriteReadSelectionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a.txt", "with space.doc", "ünï.md"}
	p := filepath.Join(dir, "sel.txt")
	var buf bytes.Buffer
	if err := WriteSelection(&buf, dir, names, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lines, err := ReadSelection(f)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSelection([]Entry{{Name: "a.txt"}, {Name: "with space.doc"}, {Name: "ünï.md"}, {Name: "other"}},
		SelectionOptions{Case: CaseSensitive})
	if n := s.ApplyList(lines, dir, true); n != 3 || !reflect.DeepEqual(s.SelectedNames(), names) {
		t.Fatalf("applied %d: %v", n, s.SelectedNames())
	}
	buf.Reset()
	_ = WriteSelection(&buf, dir, names[:1], false)
	if got := buf.String(); got != "a.txt"+lineEnding {
		t.Errorf("bare names: %q", got)
	}
}

func TestReadSelectionEncodings(t *testing.T) {
	want := []string{"a.txt", "b c.txt", " lead"}
	text := "a.txt\r\n  b c.txt  \r\n\r\n\" lead\"\n"
	u := utf16.Encode([]rune(text))
	le := []byte{0xFF, 0xFE}
	be := []byte{0xFE, 0xFF}
	for _, c := range u {
		le = append(le, byte(c), byte(c>>8))
		be = append(be, byte(c>>8), byte(c))
	}
	inputs := map[string][]byte{"utf8": []byte(text), "bom": append([]byte{0xEF, 0xBB, 0xBF}, text...), "le": le, "be": be}
	for name, in := range inputs {
		got, err := ReadSelection(bytes.NewReader(in))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}

func TestApplyList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "panel")
	other := filepath.Join(filepath.Dir(dir), "other")
	entries := []Entry{{Name: ".."}, {Name: "a.jpg"}, {Name: "b.jpg"}, {Name: "c.txt"}, {Name: "sub", IsDir: true}}
	s := NewSelection(entries, SelectionOptions{Case: CaseInsensitive})
	s.Set(3, true)
	lines := []string{
		filepath.Join(dir, "A.JPG"),   // full path in this folder, other case
		filepath.Join(other, "b.jpg"), // other folder: ignored
		filepath.Join(dir, "sub") + string(filepath.Separator),
		"missing",
		"..",
	}
	if n := s.ApplyList(lines, dir, false); n != 2 {
		t.Errorf("added %d", n)
	}
	if got, want := s.SelectedNames(), []string{"a.jpg", "c.txt", "sub"}; !reflect.DeepEqual(got, want) {
		t.Errorf("add: %v", got)
	}
	s.ApplyList([]string{"*.jpg"}, dir, true)
	if got, want := s.SelectedNames(), []string{"a.jpg", "b.jpg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("replace with mask: %v", got)
	}
	if got := SplitLines("x\r\ny\rz\n"); !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
		t.Errorf("SplitLines: %q", got)
	}
	// Windows-style path in a list read on any system still splits at the backslash.
	if strings.Contains(dir, `\`) {
		return
	}
	s.ApplyList([]string{strings.ReplaceAll(filepath.Join(dir, "c.txt"), "/", `\`)}, dir, true)
	if got := s.SelectedNames(); !reflect.DeepEqual(got, []string{"c.txt"}) {
		t.Errorf("backslash path: %v", got)
	}
}
