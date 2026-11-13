// Package clipfiles puts files and file names on the operating system clipboard and reads them back, which is what
// Total Commander's Ctrl+C, Ctrl+X and Ctrl+V do with files (cm_CopyToClipboard, cm_CutToClipboard,
// cm_PasteFromClipboard) and what its "copy names to clipboard" commands do with text. Files go on the clipboard in the
// format the platform's own file manager uses, so a file copied in Finder, Explorer or Nautilus can be pasted into a
// panel and the other way round: NSPasteboard file URLs on macOS, CF_HDROP plus "Preferred DropEffect" on Windows, and
// x-special/gnome-copied-files or text/uri-list on Linux. Nothing here needs cgo: macOS goes through osascript
// (JavaScript for Automation), Linux through wl-copy/wl-paste or xclip, and Windows calls user32 directly. When a tool
// is missing the functions return a plain error that says what to install instead of failing silently.
//
// The package is UI-free. The UI wires its commands to the package-level functions (which act on the system
// clipboard) or to a *Clipboard made by New. Pure helpers that build or parse the clipboard formats (uri.go, hdrop.go,
// dib.go) and the text formatters (text.go) work on every platform so they can be tested anywhere.
package clipfiles

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Op says whether files on the clipboard were copied (Ctrl+C) or cut (Ctrl+X). A paste of cut files moves them.
type Op int

const (
	// Copy means the files stay where they are and a paste makes copies.
	Copy Op = iota
	// Cut means a paste moves the files; after a successful move the clipboard should be cleared, as Explorer does.
	Cut
)

// String returns "copy" or "cut", the words the GNOME clipboard format uses.
func (o Op) String() string {
	if o == Cut {
		return "cut"
	}
	return "copy"
}

// Content tells which kinds of data the clipboard currently holds, so the UI can decide what Ctrl+V does: paste files,
// or (TC 10+) save text or an image as a new file.
type Content struct {
	Files bool // a file list from a file manager
	Text  bool // plain text
	Image bool // a bitmap (PNG, TIFF or DIB)
	// Types lists the raw format names the platform reported (UTIs, MIME types or Windows format names), for
	// diagnostics.
	Types []string
}

var (
	// ErrUnsupported is returned on platforms that have no clipboard support here.
	ErrUnsupported = errors.New("the clipboard is not supported on this platform")
	// ErrNoTool is returned on Linux when neither wl-clipboard nor xclip/xsel is installed. The full error text names
	// the packages to install.
	ErrNoTool = errors.New("no clipboard tool found")
	// ErrEmpty is returned when the clipboard holds no data of the asked kind (no files, no text or no image).
	ErrEmpty = errors.New("the clipboard holds no data of this kind")
)

// backend is one platform's clipboard implementation. Paths are always absolute and native.
type backend interface {
	writeFiles(paths []string, op Op) error
	readFiles() ([]string, Op, error)
	writeText(s string) error
	readText() (string, error)
	writeImage(png []byte) error
	readImage() ([]byte, error)
	content() (Content, error)
	clear() error
}

// Clipboard is a handle on one clipboard. The zero value is not usable; call New.
type Clipboard struct {
	b backend
}

// New returns a handle on the system clipboard.
func New() *Clipboard { return &Clipboard{b: newSystemBackend()} }

// system is the handle the package-level functions use.
var system = New()

// WriteFiles puts paths on the clipboard as a file list, marked as copied or cut. Paths must be absolute.
func (c *Clipboard) WriteFiles(paths []string, op Op) error {
	if len(paths) == 0 {
		return errors.New("no files to put on the clipboard")
	}
	clean := make([]string, len(paths))
	for i, p := range paths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("%s: path must be absolute", p)
		}
		clean[i] = filepath.Clean(p)
	}
	return c.b.writeFiles(clean, op)
}

// ReadFiles returns the file list on the clipboard and whether it was copied or cut. It returns ErrEmpty when the
// clipboard holds no files.
func (c *Clipboard) ReadFiles() ([]string, Op, error) {
	paths, op, err := c.b.readFiles()
	if err == nil && len(paths) == 0 {
		return nil, Copy, ErrEmpty
	}
	return paths, op, err
}

// WriteText puts plain text on the clipboard.
func (c *Clipboard) WriteText(s string) error { return c.b.writeText(s) }

// ReadText returns the text on the clipboard, or ErrEmpty when there is none.
func (c *Clipboard) ReadText() (string, error) { return c.b.readText() }

// WriteImage puts a PNG image on the clipboard.
func (c *Clipboard) WriteImage(png []byte) error {
	if len(png) == 0 {
		return errors.New("no image data")
	}
	return c.b.writeImage(png)
}

// ReadImage returns the image on the clipboard encoded as PNG, or ErrEmpty when there is none.
func (c *Clipboard) ReadImage() ([]byte, error) { return c.b.readImage() }

// Content reports which kinds of data the clipboard holds.
func (c *Clipboard) Content() (Content, error) { return c.b.content() }

// Clear empties the clipboard. The UI calls it after cut files were moved by a paste.
func (c *Clipboard) Clear() error { return c.b.clear() }

// CopyAsText formats entries as the given kind of text (see TextKind) and puts the result on the clipboard. It backs
// cm_CopyNamesToClip, cm_CopyFullNamesToClip, cm_CopyDetailsToClip and their relatives.
func (c *Clipboard) CopyAsText(kind TextKind, entries []Entry, f Format) error {
	if len(entries) == 0 {
		return errors.New("nothing selected")
	}
	return c.WriteText(FormatText(kind, entries, f))
}

// WriteFiles puts paths on the system clipboard (see Clipboard.WriteFiles).
func WriteFiles(paths []string, op Op) error { return system.WriteFiles(paths, op) }

// ReadFiles reads the file list from the system clipboard (see Clipboard.ReadFiles).
func ReadFiles() ([]string, Op, error) { return system.ReadFiles() }

// WriteText puts text on the system clipboard.
func WriteText(s string) error { return system.WriteText(s) }

// ReadText reads text from the system clipboard.
func ReadText() (string, error) { return system.ReadText() }

// WriteImage puts a PNG image on the system clipboard.
func WriteImage(png []byte) error { return system.WriteImage(png) }

// ReadImage reads an image from the system clipboard as PNG.
func ReadImage() ([]byte, error) { return system.ReadImage() }

// ReadContent reports which kinds of data the system clipboard holds.
func ReadContent() (Content, error) { return system.Content() }

// Clear empties the system clipboard.
func Clear() error { return system.Clear() }

// CopyAsText formats entries and puts the text on the system clipboard (see Clipboard.CopyAsText).
func CopyAsText(kind TextKind, entries []Entry, f Format) error {
	return system.CopyAsText(kind, entries, f)
}
