package main

// The simple built-in text editor (FEATURES.md F-594, our addition: TC uses Notepad). The page shows the text in an
// editor window (frontend/src/features/textedit.js) and reads and saves it here. Only local UTF-8 text files up to
// textEditMax are taken; anything else is left to the external editor (F4). The line ends (CRLF or LF) and a UTF-8 byte
// order mark are kept as the file had them. A save checks that the file was not changed by something else since it was
// read, so the built-in editor never silently overwrites another program's changes.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// textEditMax is the largest file the built-in editor opens.
const textEditMax = 4 << 20

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TextFile is a file read for the built-in editor: Text has "\n" line ends; CRLF and BOM say how it is written back;
// MTime and Size are what the file looked like when it was read (a save checks them).
type TextFile struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Text  string `json:"text"`
	CRLF  bool   `json:"crlf"`
	BOM   bool   `json:"bom"`
	MTime int64  `json:"mtime"` // nanoseconds
	Size  int64  `json:"size"`
}

// decodeTextFile turns file bytes into a TextFile's text, or says why the built-in editor can't take them.
func decodeTextFile(b []byte) (text string, crlf, bom bool, err error) {
	if bytes.HasPrefix(b, utf8BOM) {
		bom, b = true, b[len(utf8BOM):]
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", false, false, errors.New("not plain UTF-8 text")
	}
	s := string(b)
	crlf = strings.Contains(s, "\r\n")
	if crlf {
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	return s, crlf, bom, nil
}

// encodeTextFile is decodeTextFile backwards.
func encodeTextFile(text string, crlf, bom bool) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if bom {
		return append(append([]byte{}, utf8BOM...), text...)
	}
	return []byte(text)
}

func (e *ListerEdit) textPath(path string) error {
	if err := e.check(path); err != nil {
		return err
	}
	if r := vfsRouter(e.app); r != nil && !vfsIsLocal(r, path) {
		return errors.New("The built-in editor edits files on this computer only. Use F4 for files inside archives or on servers.")
	}
	return nil
}

// ReadText reads a local text file for the built-in editor.
func (e *ListerEdit) ReadText(path string) (TextFile, error) {
	if err := e.textPath(path); err != nil {
		return TextFile{}, err
	}
	name := filepath.Base(path)
	fi, err := os.Stat(path)
	if err != nil {
		return TextFile{}, textEditErr("open", name, err)
	}
	if !fi.Mode().IsRegular() {
		return TextFile{}, fmt.Errorf("%s is not a file.", name)
	}
	if fi.Size() > textEditMax {
		return TextFile{}, fmt.Errorf("%s is too large for the built-in editor (more than %d MB). Use the external editor.", name, textEditMax>>20)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return TextFile{}, textEditErr("open", name, err)
	}
	text, crlf, bom, err := decodeTextFile(b)
	if err != nil {
		return TextFile{}, fmt.Errorf("%s is not a plain UTF-8 text file, so the built-in editor can't change it safely. Use the external editor.", name)
	}
	return TextFile{Path: path, Name: name, Text: text, CRLF: crlf, BOM: bom, MTime: fi.ModTime().UnixNano(), Size: fi.Size()}, nil
}

// WriteText saves the built-in editor's text over the file f was read from, with f's line ends and byte order mark.
// Unless force, it refuses when the file changed since f was read (the page then asks whether to overwrite). It gives
// back f with the new time and size, for the next save.
func (e *ListerEdit) WriteText(f TextFile, text string, force bool) (TextFile, error) {
	if err := e.textPath(f.Path); err != nil {
		return f, err
	}
	name := filepath.Base(f.Path)
	fi, err := os.Stat(f.Path)
	switch {
	case err == nil && !fi.Mode().IsRegular():
		return f, fmt.Errorf("%s is not a file.", name)
	case err == nil && !force && (fi.ModTime().UnixNano() != f.MTime || fi.Size() != f.Size):
		return f, errTextChanged
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return f, textEditErr("save", name, err)
	}
	mode := fs.FileMode(0o644)
	if err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(f.Path, encodeTextFile(text, f.CRLF, f.BOM), mode); err != nil {
		return f, textEditErr("save", name, err)
	}
	if fi, err := os.Stat(f.Path); err == nil {
		f.MTime, f.Size = fi.ModTime().UnixNano(), fi.Size()
	}
	f.Text = ""
	return f, nil
}

// errTextChanged is WriteText's answer when the file changed on disk; its text is what the page looks for.
var errTextChanged = errors.New("changed on disk")

func textEditErr(what, name string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("Can't %s %s: it is not there.", what, name)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("Can't %s %s: you don't have permission.", what, name)
	}
	return fmt.Errorf("Can't %s %s: %v", what, name, err)
}

// CreateNew makes sure the file name (relative to dir, or a full path) exists, creating it empty when it does not,
// without opening an editor (Shift+F4 with the built-in editor).
func (e *ListerEdit) CreateNew(dir, name string) (ListerNewFile, error) {
	return e.createNew(dir, name)
}
