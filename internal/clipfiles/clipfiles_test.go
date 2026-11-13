// These tests check the public Clipboard wrapper with an in-memory backend: argument checks (absolute paths, empty
// lists), the ErrEmpty contract, copy-as-text, paste planning from the clipboard and TC 10's paste-data-as-file, which
// must prefer an image over text and pick a free name. The real platform backends are tested in their own files.

package clipfiles

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// memBackend is a clipboard held in memory.
type memBackend struct {
	paths []string
	op    Op
	text  *string
	png   []byte
}

func (m *memBackend) writeFiles(p []string, op Op) error {
	*m = memBackend{paths: p, op: op}
	return nil
}
func (m *memBackend) readFiles() ([]string, Op, error) { return m.paths, m.op, nil }
func (m *memBackend) writeText(s string) error         { *m = memBackend{text: &s}; return nil }
func (m *memBackend) readText() (string, error) {
	if m.text == nil {
		return "", ErrEmpty
	}
	return *m.text, nil
}
func (m *memBackend) writeImage(b []byte) error { *m = memBackend{png: b}; return nil }
func (m *memBackend) readImage() ([]byte, error) {
	if m.png == nil {
		return nil, ErrEmpty
	}
	return m.png, nil
}
func (m *memBackend) content() (Content, error) {
	return Content{Files: len(m.paths) > 0, Text: m.text != nil, Image: m.png != nil}, nil
}
func (m *memBackend) clear() error { *m = memBackend{}; return nil }

func TestClipboardFiles(t *testing.T) {
	m := &memBackend{}
	c := &Clipboard{b: m}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")

	if err := c.WriteFiles(nil, Copy); err == nil {
		t.Error("empty list accepted")
	}
	if err := c.WriteFiles([]string{"rel.txt"}, Copy); err == nil {
		t.Error("relative path accepted")
	}
	if _, _, err := c.ReadFiles(); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty read: %v", err)
	}
	if err := c.WriteFiles([]string{a + string(filepath.Separator)}, Cut); err != nil {
		t.Fatal(err)
	}
	paths, op, err := c.ReadFiles()
	if err != nil || op != Cut || len(paths) != 1 || paths[0] != a {
		t.Fatalf("read %q %v %v (paths must be cleaned)", paths, op, err)
	}
	plan, err := c.PlanPaste(filepath.Join(dir, "dest"))
	if err != nil || !plan.Move || plan.Sources[0] != a {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PlanPaste(dir); !errors.Is(err, ErrEmpty) {
		t.Fatalf("plan after clear: %v", err)
	}
}

func TestClipboardCopyAsText(t *testing.T) {
	m := &memBackend{}
	c := &Clipboard{b: m}
	if err := c.CopyAsText(Names, nil, Format{}); err == nil {
		t.Error("empty selection accepted")
	}
	if err := c.CopyAsText(Names, FromPaths([]string{filepath.Join("x", "a"), filepath.Join("x", "b")}),
		Format{Newline: "\n"}); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.ReadText(); s != "a\nb" {
		t.Fatalf("text %q", s)
	}
}

func TestClipboardPasteData(t *testing.T) {
	dir := t.TempDir()
	m := &memBackend{}
	c := &Clipboard{b: m}
	if _, err := c.PasteData(dir, ""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty clipboard: %v", err)
	}
	c.WriteText("")
	if _, err := c.PasteData(dir, ""); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty text: %v", err)
	}
	c.WriteText("hello")
	p, err := c.PasteData(dir, "")
	if err != nil || filepath.Base(p) != "clipboard.txt" {
		t.Fatalf("text paste %q %v", p, err)
	}
	p, err = c.PasteData(dir, "")
	if err != nil || filepath.Base(p) != "clipboard(2).txt" {
		t.Fatalf("second text paste %q %v", p, err)
	}
	if _, err := c.PasteData(dir, "clipboard.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("named paste over existing file: %v", err)
	}
	m.png = []byte("\x89PNG fake")
	p, err = c.PasteData(dir, "")
	if err != nil || filepath.Base(p) != "clipboard.png" {
		t.Fatalf("image paste %q %v", p, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "\x89PNG fake" {
		t.Fatalf("image content %q", b)
	}
	if err := c.WriteImage(nil); err == nil {
		t.Fatal("empty image accepted")
	}
}

func TestOpString(t *testing.T) {
	if Copy.String() != "copy" || Cut.String() != "cut" {
		t.Fatal("Op.String must match the GNOME format words")
	}
}

func TestMemoryClipboard(t *testing.T) {
	c := NewMemory()
	if _, _, err := c.ReadFiles(); !errors.Is(err, ErrEmpty) {
		t.Errorf("new memory clipboard: %v", err)
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := c.WriteFiles([]string{a}, Cut); err != nil {
		t.Fatal(err)
	}
	if p, op, err := c.ReadFiles(); err != nil || op != Cut || len(p) != 1 || p[0] != a {
		t.Errorf("read back: %v %v %v", p, op, err)
	}
	c.WriteText("x")
	if ct, _ := c.Content(); ct.Files || !ct.Text {
		t.Errorf("text replaces files: %+v", ct)
	}
	c.Clear()
	if ct, _ := c.Content(); ct.Files || ct.Text || ct.Image {
		t.Errorf("cleared: %+v", ct)
	}
}
