// This file prepares what Ctrl+V does in a panel. When the clipboard holds files, PlanPaste turns them into a copy or
// move job for the file operations engine: cut files are moved, copied files are copied, and the plan refuses to paste
// a folder into itself or one of its own subfolders, which would recurse forever. Pasting copied files into the folder
// they came from is allowed (Explorer makes "name - Copy" files then), so the plan flags it and the copy engine picks
// the new names. When the clipboard holds text or an image instead, Total Commander 10 and later asks for a file name
// and saves the data as a new file in the current folder; SuggestName proposes a free name and SaveText / SaveImage
// write the file without ever overwriting an existing one.

package clipfiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Plan is a paste turned into a file job.
type Plan struct {
	Sources []string // absolute paths from the clipboard, in clipboard order
	Dest    string   // target folder
	Move    bool     // true when the files were cut: move them, then clear the clipboard
	// SameDir is true when at least one source already lives in Dest. For a copy, the engine must give the copy a
	// new name; for a move it is a no-op for that file.
	SameDir bool
}

// ErrIntoItself is returned when a folder would be pasted into itself or one of its subfolders.
var ErrIntoItself = errors.New("cannot paste a folder into itself")

// PlanPaste builds the job for pasting the files on the clipboard into dest.
func (c *Clipboard) PlanPaste(dest string) (Plan, error) {
	paths, op, err := c.ReadFiles()
	if err != nil {
		return Plan{}, err
	}
	return NewPlan(paths, op, dest)
}

// PlanPaste builds the job for pasting the files on the system clipboard into dest.
func PlanPaste(dest string) (Plan, error) { return system.PlanPaste(dest) }

// NewPlan checks a paste of paths into dest and returns the job. It is separate from PlanPaste so callers that
// already read the clipboard, and tests, can use it.
func NewPlan(paths []string, op Op, dest string) (Plan, error) {
	if len(paths) == 0 {
		return Plan{}, ErrEmpty
	}
	if !filepath.IsAbs(dest) {
		return Plan{}, fmt.Errorf("%s: target folder must be absolute", dest)
	}
	dest = filepath.Clean(dest)
	p := Plan{Dest: dest, Move: op == Cut}
	for _, s := range paths {
		s = filepath.Clean(s)
		if samePath(s, dest) || isInside(dest, s) {
			return Plan{}, fmt.Errorf("%s: %w", s, ErrIntoItself)
		}
		if samePath(filepath.Dir(s), dest) {
			p.SameDir = true
		}
		p.Sources = append(p.Sources, s)
	}
	return p, nil
}

// isInside says whether path lies below dir.
func isInside(path, dir string) bool {
	if samePath("A", "a") { // case-insensitive platform
		path, dir = strings.ToLower(path), strings.ToLower(dir)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// SuggestName proposes a free file name in dir for pasted text or images: base+ext if free, otherwise base (2)+ext,
// base (3)+ext and so on.
func SuggestName(dir, base, ext string) string {
	name := base + ext
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			return name
		}
		name = base + " (" + strconv.Itoa(i) + ")" + ext
	}
}

// SaveText writes text to dir/name and returns the full path. It never overwrites: an existing file gives an error
// wrapping os.ErrExist so the UI can ask for another name.
func SaveText(dir, name, text string) (string, error) { return saveNew(dir, name, []byte(text)) }

// SaveImage writes PNG bytes to dir/name and returns the full path, with the same no-overwrite rule as SaveText.
func SaveImage(dir, name string, png []byte) (string, error) { return saveNew(dir, name, png) }

func saveNew(dir, name string, data []byte) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("%q is not a plain file name", name)
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// PasteData saves whatever non-file data the clipboard holds as a new file in dir, for TC 10's "paste text or image
// as a file". An image is preferred over text, since programs that copy an image often add a text description too.
// name may be empty, in which case "clipboard.png" or "clipboard.txt" (made unique) is used. It returns the new path.
func (c *Clipboard) PasteData(dir, name string) (string, error) {
	if img, err := c.ReadImage(); err == nil && len(img) > 0 {
		if name == "" {
			name = SuggestName(dir, "clipboard", ".png")
		}
		return SaveImage(dir, name, img)
	}
	text, err := c.ReadText()
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", ErrEmpty
	}
	if name == "" {
		name = SuggestName(dir, "clipboard", ".txt")
	}
	return SaveText(dir, name, text)
}

// PasteData saves the system clipboard's text or image as a new file in dir (see Clipboard.PasteData).
func PasteData(dir, name string) (string, error) { return system.PasteData(dir, name) }
