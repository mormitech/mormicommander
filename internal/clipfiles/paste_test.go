// These tests check the paste planning (copy versus move, refusing to paste a folder into itself, flagging a paste
// into the source folder) and the TC 10 "paste text or image as a file" helpers: free-name suggestions and saving that
// never overwrites an existing file. Everything happens in temporary folders.

package clipfiles

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewPlan(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	f := filepath.Join(a, "f.txt")
	tests := []struct {
		name    string
		paths   []string
		op      Op
		dest    string
		want    Plan
		wantErr error
	}{
		{"copy", []string{f}, Copy, b, Plan{Sources: []string{f}, Dest: b}, nil},
		{"cut moves", []string{f}, Cut, b, Plan{Sources: []string{f}, Dest: b, Move: true}, nil},
		{"same folder", []string{f}, Copy, a, Plan{Sources: []string{f}, Dest: a, SameDir: true}, nil},
		{"folder into parent is fine", []string{a}, Copy, root, Plan{Sources: []string{a}, Dest: root, SameDir: true}, nil},
		{"sibling with shared prefix", []string{a}, Copy, a + "bc", Plan{Sources: []string{a}, Dest: a + "bc"}, nil},
		{"into itself", []string{a}, Copy, a, Plan{}, ErrIntoItself},
		{"into own subfolder", []string{f, a}, Cut, filepath.Join(a, "x", "y"), Plan{}, ErrIntoItself},
		{"nothing", nil, Copy, b, Plan{}, ErrEmpty},
	}
	for _, tt := range tests {
		got, err := NewPlan(tt.paths, tt.op, tt.dest)
		if tt.wantErr != nil {
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
	if _, err := NewPlan([]string{f}, Copy, "relative"); err == nil {
		t.Error("relative target accepted")
	}
}

func TestSuggestName(t *testing.T) {
	dir := t.TempDir()
	if got := SuggestName(dir, "clipboard", ".txt"); got != "clipboard.txt" {
		t.Fatalf("first suggestion %q", got)
	}
	for _, n := range []string{"clipboard.txt", "clipboard (2).txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := SuggestName(dir, "clipboard", ".txt"); got != "clipboard (3).txt" {
		t.Fatalf("suggestion with two taken: %q", got)
	}
}

func TestSaveNew(t *testing.T) {
	dir := t.TempDir()
	p, err := SaveText(dir, "note.txt", "hello\nworld")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "hello\nworld" {
		t.Fatalf("content %q", b)
	}
	if _, err := SaveText(dir, "note.txt", "other"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrite: err = %v, want ErrExist", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "hello\nworld" {
		t.Fatal("existing file was changed")
	}
	if _, err := SaveImage(dir, "pic.png", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", filepath.Join("x", "y")} {
		if _, err := SaveText(dir, bad, "x"); err == nil || !strings.Contains(err.Error(), "plain file name") {
			t.Errorf("name %q: err = %v", bad, err)
		}
	}
}
