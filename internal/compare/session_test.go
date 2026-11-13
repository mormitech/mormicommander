// Tests for the compare window model: block copying in both directions,
// line edits, undo/redo, the modified flag, saving and reloading in another
// encoding. Files live in t.TempDir().
package compare

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func texts(d *Document) []string { return d.Texts() }

func TestSessionCopyBlocks(t *testing.T) {
	tests := []struct {
		name      string
		left      string
		right     string
		block     int
		from      Side
		wantLeft  string
		wantRight string
	}{
		{"change L->R", "a\nb\nc\n", "a\nX\nc\n", 0, Left, "a\nb\nc\n", "a\nb\nc\n"},
		{"change R->L", "a\nb\nc\n", "a\nX\nc\n", 0, Right, "a\nX\nc\n", "a\nX\nc\n"},
		{"delete L->R inserts", "a\nb\nc\n", "a\nc\n", 0, Left, "a\nb\nc\n", "a\nb\nc\n"},
		{"delete R->L removes", "a\nb\nc\n", "a\nc\n", 0, Right, "a\nc\n", "a\nc\n"},
		{"insert at end R->L", "a\n", "a\nb\nc\n", 0, Right, "a\nb\nc\n", "a\nb\nc\n"},
		{"insert at start R->L", "c\n", "a\nb\nc\n", 0, Right, "a\nb\nc\n", "a\nb\nc\n"},
		{"crlf target keeps crlf", "a\r\nb\r\nc\r\n", "a\nX\nY\nc\n", 0, Right, "a\r\nX\r\nY\r\nc\r\n", "a\nX\nY\nc\n"},
		{"no final newline kept", "a\nb", "a\nX", 0, Right, "a\nX", "a\nX"},
		{"second block", "1\nsame\n2\n", "A\nsame\nB\n", 1, Left, "1\nsame\n2\n", "A\nsame\n2\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSession(NewDocument(tc.left), NewDocument(tc.right), Options{})
			if err := s.CopyBlock(tc.block, tc.from); err != nil {
				t.Fatal(err)
			}
			if got := s.Doc(Left).String(); got != tc.wantLeft {
				t.Errorf("left %q want %q", got, tc.wantLeft)
			}
			if got := s.Doc(Right).String(); got != tc.wantRight {
				t.Errorf("right %q want %q", got, tc.wantRight)
			}
			if !s.Modified(tc.from.Other()) || s.Modified(tc.from) {
				t.Errorf("modified flags wrong")
			}
			// Undo restores the original text exactly.
			if side, ok := s.Undo(); !ok || side != tc.from.Other() {
				t.Fatal("undo failed")
			}
			if s.Doc(Left).String() != tc.left || s.Doc(Right).String() != tc.right {
				t.Fatalf("undo did not restore: %q %q", s.Doc(Left).String(), s.Doc(Right).String())
			}
			if s.Modified(Left) || s.Modified(Right) {
				t.Error("modified after undo to original")
			}
			s.Redo()
			if s.Doc(Left).String() != tc.wantLeft || s.Doc(Right).String() != tc.wantRight {
				t.Fatal("redo mismatch")
			}
		})
	}
}

func TestSessionCopyAllMakesIdentical(t *testing.T) {
	s := NewSession(NewDocument("a\nb\nc\nd\n"), NewDocument("x\nb\ny\nz\nd\ne\n"), Options{})
	if s.Identical() || len(s.Blocks()) != 3 {
		t.Fatalf("blocks %+v", s.Blocks())
	}
	if err := s.CopyAllBlocks(Left); err != nil {
		t.Fatal(err)
	}
	if !s.Identical() || s.Doc(Right).String() != "a\nb\nc\nd\n" {
		t.Fatalf("not identical: %q", s.Doc(Right).String())
	}
	// Copying blocks one by one reaches the same state.
	s2 := NewSession(NewDocument("a\nb\nc\nd\n"), NewDocument("x\nb\ny\nz\nd\ne\n"), Options{})
	for !s2.Identical() {
		if err := s2.CopyBlock(0, Right); err != nil {
			t.Fatal(err)
		}
	}
	if s2.Doc(Left).String() != "x\nb\ny\nz\nd\ne\n" {
		t.Fatalf("left %q", s2.Doc(Left).String())
	}
}

func TestSessionLineEdits(t *testing.T) {
	s := NewSession(NewDocument("a\r\nb\r\nc"), NewDocument("a\r\nB\r\nc"), Options{})
	if len(s.Blocks()) != 1 {
		t.Fatal("expected one block")
	}
	l, r := s.InlineSpans(s.Blocks()[0].First)
	if render("b", l) != "[b]" || render("B", r) != "[B]" {
		t.Fatalf("spans %v %v", l, r)
	}
	if err := s.SetLine(Right, 1, "b"); err != nil {
		t.Fatal(err)
	}
	if !s.Identical() || s.Doc(Right).Lines[1].EOL != "\r\n" {
		t.Fatalf("after SetLine: %+v", s.Doc(Right).Lines)
	}
	if err := s.SetLine(Right, 2, "c edited"); err != nil || s.Doc(Right).Lines[2].EOL != "" {
		t.Fatalf("last line EOL: %+v %v", s.Doc(Right).Lines, err)
	}
	if err := s.InsertLines(Left, 1, "new1", "new2"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(texts(s.Doc(Left)), []string{"a", "new1", "new2", "b", "c"}) || s.Doc(Left).Lines[1].EOL != "\r\n" {
		t.Fatalf("insert: %+v", s.Doc(Left).Lines)
	}
	if err := s.DeleteLines(Left, 1, 2); err != nil {
		t.Fatal(err)
	}
	if s.Doc(Left).String() != "a\r\nb\r\nc" {
		t.Fatalf("delete: %q", s.Doc(Left).String())
	}
	if err := s.DeleteLines(Left, 2, 5); err == nil {
		t.Fatal("out of range delete must fail")
	}
	if err := s.SetLine(Left, 9, "x"); err == nil {
		t.Fatal("out of range set must fail")
	}
	if err := s.CopyBlock(7, Left); err == nil {
		t.Fatal("bad block must fail")
	}
	if err := s.CopyRows(3, 1, Left); err == nil {
		t.Fatal("bad rows must fail")
	}
	// Undo everything.
	n := 0
	for s.CanUndo() {
		s.Undo()
		n++
	}
	if n != 4 || s.Doc(Left).String() != "a\r\nb\r\nc" || s.Doc(Right).String() != "a\r\nB\r\nc" {
		t.Fatalf("undo all (%d): %q %q", n, s.Doc(Left).String(), s.Doc(Right).String())
	}
	if !s.CanRedo() {
		t.Fatal("redo expected")
	}
	s.InsertLines(Left, 0, "z")
	if s.CanRedo() {
		t.Fatal("new edit must clear redo")
	}
	if _, ok := (&Session{}).Undo(); ok {
		t.Fatal("empty undo")
	}
	if _, ok := (&Session{}).Redo(); ok {
		t.Fatal("empty redo")
	}
}

func TestSessionOptionsAndRows(t *testing.T) {
	s := NewSession(NewDocument("Hello\nworld\n"), NewDocument("hello\nworld\n"), Options{})
	if s.Identical() {
		t.Fatal("case differs")
	}
	s.SetOptions(Options{IgnoreCase: true})
	if !s.Identical() || !s.Options().IgnoreCase {
		t.Fatal("ignore case not applied")
	}
	if s.LineText(0, Left) != "Hello" || s.LineText(0, Right) != "hello" {
		t.Fatal("LineText")
	}
	l, r := s.InlineSpans(0)
	if l[0].Changed || r[0].Changed {
		t.Fatal("equal row must not be highlighted")
	}
	s2 := NewSession(NewDocument("a\n"), NewDocument("a\nb\n"), Options{})
	if s2.LineText(1, Left) != "" {
		t.Fatal("padding cell must be empty")
	}
	_, r = s2.InlineSpans(1)
	if !r[0].Changed {
		t.Fatal("inserted row must be highlighted")
	}
	if st := s2.Stats(); st.Inserted != 1 || st.Blocks != 1 {
		t.Fatalf("stats %+v", st)
	}
	if Left.String() != "left" || Right.String() != "right" || Left.Other() != Right {
		t.Fatal("Side helpers")
	}
}

func TestSessionFiles(t *testing.T) {
	dir := t.TempDir()
	pa := filepath.Join(dir, "a.txt")
	pb := filepath.Join(dir, "b.txt")
	la, _ := Encode("közös\nbal oldal\nvége\n", EncWindows1250, false)
	os.WriteFile(pa, la, 0o644)
	os.WriteFile(pb, []byte("közös\njobb oldal\nvége\n"), 0o644)

	s, err := OpenFiles(pa, pb, Options{}, DetectOptions{ANSI: EncWindows1250})
	if err != nil {
		t.Fatal(err)
	}
	if s.Binary() || s.Detection(Left).Encoding != EncWindows1250 || s.Detection(Right).Encoding != EncUTF8 {
		t.Fatalf("detection %+v %+v", s.Detection(Left), s.Detection(Right))
	}
	if len(s.Blocks()) != 1 {
		t.Fatal("one block expected")
	}
	if err := s.CopyBlock(0, Right); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Left, ""); err != nil {
		t.Fatal(err)
	}
	if s.Modified(Left) {
		t.Fatal("saved side still modified")
	}
	// Saved in the left file's own encoding (Windows-1250).
	back, _ := os.ReadFile(pa)
	want, _ := Encode("közös\njobb oldal\nvége\n", EncWindows1250, false)
	if string(back) != string(want) {
		t.Fatalf("saved bytes %q", back)
	}
	// Undo after save makes it modified again; redo returns to saved state.
	s.Undo()
	if !s.Modified(Left) {
		t.Fatal("undo after save must be modified")
	}
	if err := s.ReloadSide(Left, EncAuto, DetectOptions{ANSI: EncWindows1250}, false); err != ErrModified {
		t.Fatalf("reload with edits: %v", err)
	}
	s.Redo()
	if s.Modified(Left) {
		t.Fatal("redo to saved state must be clean")
	}
	// Reload in another encoding: Windows-1250 bytes read as Windows-1252.
	if err := s.ReloadSide(Left, EncWindows1252, DetectOptions{}, false); err != nil {
		t.Fatal(err)
	}
	if s.Doc(Left).Encoding != EncWindows1252 || s.Doc(Left).Lines[0].Text != "közös" {
		// ö is the same byte in both code pages.
		t.Fatalf("reloaded %+v", s.Doc(Left))
	}
	if s.CanUndo() {
		t.Fatal("reload must drop the side's history")
	}
	// Save as a new file.
	pc := filepath.Join(dir, "c.txt")
	if err := s.Save(Right, pc); err != nil || s.Doc(Right).Path != pc {
		t.Fatal(err)
	}
	if err := NewSession(NewDocument("x"), NewDocument("y"), Options{}).Save(Left, ""); err == nil {
		t.Fatal("save without path must fail")
	}
	if err := NewSession(NewDocument("x"), NewDocument("y"), Options{}).ReloadSide(Left, EncAuto, DetectOptions{}, false); err == nil {
		t.Fatal("reload without path must fail")
	}
	if _, err := OpenFiles(filepath.Join(dir, "nope"), pb, Options{}, DetectOptions{}); err == nil {
		t.Fatal("missing file must fail")
	}

	// Binary detection.
	pbin := filepath.Join(dir, "bin")
	os.WriteFile(pbin, []byte{0, 1, 2, 3, 0, 0}, 0o644)
	s, err = OpenFiles(pa, pbin, Options{}, DetectOptions{})
	if err != nil || !s.Binary() {
		t.Fatalf("binary not detected: %v", err)
	}
}
