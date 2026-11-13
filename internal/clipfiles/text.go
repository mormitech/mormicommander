// This file formats selected files as text for Total Commander's "copy to clipboard" commands and parses text back
// into names. cm_CopyNamesToClip copies bare names one per line, cm_CopyFullNamesToClip full paths,
// cm_CopyDetailsToClip and cm_CopyFullDetailsToClip add size, date and attributes separated by tabs (so the text
// pastes into a spreadsheet as columns), cm_CopyNetNamesToClip turns mapped drive letters into \\server\share names
// on Windows, and cm_CopySrcPathToClip / cm_CopyTrgPathToClip copy a panel's folder. TC also offers forward-slash and
// file:// URL variants, which help when pasting into a shell or a browser, and cm_CopyFullNamesQuotedToClip (ours)
// copies full paths in double quotes, so a path with spaces pastes into a shell or a Windows prompt as one argument.
// In the other direction,
// cm_LoadSelectionFromClip and the multi-rename tool's "load names from clipboard" read lines of text; ParseNameList
// and Lines do that, tolerating CRLF, quotes and file:// URIs. Formatting is pure so the UI can preview it and the
// tests can check it without touching the real clipboard.

package clipfiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Entry is one selected file or folder as the text formatters see it.
type Entry struct {
	Path    string      // full native path
	Name    string      // name shown in the panel; empty means filepath.Base(Path)
	Size    int64       // size in bytes (ignored for folders)
	ModTime time.Time   // last modification time
	IsDir   bool        // folder or not
	Mode    fs.FileMode // permission and type bits, used for the attribute column outside Windows
	// Attr is a preformatted attribute string. When empty, the details formatters derive one: TC-style "rahs"
	// letters on Windows, "-rw-r--r--" style elsewhere.
	Attr string
}

// name returns the display name of e.
func (e Entry) name() string {
	if e.Name != "" {
		return e.Name
	}
	return filepath.Base(e.Path)
}

// FromPaths makes entries that carry only paths, enough for the name formatters.
func FromPaths(paths []string) []Entry {
	out := make([]Entry, len(paths))
	for i, p := range paths {
		out[i] = Entry{Path: p}
	}
	return out
}

// StatPaths makes entries with size, date and attributes read from disk. Links are described as links (Lstat), as
// in the panel. A path that cannot be read still gets an entry, with only its name filled in.
func StatPaths(paths []string) []Entry {
	out := make([]Entry, len(paths))
	for i, p := range paths {
		out[i] = Entry{Path: p}
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		out[i].Size = fi.Size()
		out[i].ModTime = fi.ModTime()
		out[i].IsDir = fi.IsDir()
		out[i].Mode = fi.Mode()
		out[i].Attr = attrString(fi)
	}
	return out
}

// Format controls how entries become text.
type Format struct {
	// Newline separates lines. Empty means the platform's own: CRLF on Windows (as TC does), LF elsewhere.
	Newline string
	// TrailingNewline ends the last line too. Off by default so that pasting a single path into a terminal does not
	// run it.
	TrailingNewline bool
	// DateLayout is the time.Format layout of the date column in details. Empty means "2006-01-02 15:04:05".
	DateLayout string
	// DirSuffix appends a path separator to folder names, which makes folders recognisable in a plain list.
	DirSuffix bool
	// DirSizeText is shown in the size column for folders. Empty means "<DIR>", as in TC's panels.
	DirSizeText string
}

func (f Format) newline() string {
	if f.Newline != "" {
		return f.Newline
	}
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// TextKind picks what FormatText produces.
type TextKind int

const (
	Names       TextKind = iota // bare names (cm_CopyNamesToClip)
	FullNames                   // full native paths (cm_CopyFullNamesToClip)
	Details                     // name, size, date, attributes, tab separated (cm_CopyDetailsToClip)
	FullDetails                 // the same with full paths (cm_CopyFullDetailsToClip)
	NetNames                    // full paths with mapped drives as UNC names on Windows (cm_CopyNetNamesToClip)
	UnixPaths                   // full paths with forward slashes
	URLs                        // file:// URLs
	QuotedNames                 // full native paths in double quotes (cm_CopyFullNamesQuotedToClip, ours)
)

// commands maps TC command names to text kinds so the UI's command registry can look them up.
var commands = map[string]TextKind{
	"cm_CopyNamesToClip":           Names,
	"cm_CopyFullNamesToClip":       FullNames,
	"cm_CopyDetailsToClip":         Details,
	"cm_CopyFullDetailsToClip":     FullDetails,
	"cm_CopyNetNamesToClip":        NetNames,
	"cm_CopyUnixNamesToClip":       UnixPaths,
	"cm_CopyURLNamesToClip":        URLs,
	"cm_CopyFullNamesQuotedToClip": QuotedNames,
}

// CommandKind returns the text kind for a TC command name such as "cm_CopyNamesToClip".
func CommandKind(cm string) (TextKind, bool) {
	k, ok := commands[cm]
	return k, ok
}

// FormatText renders entries as one line each.
func FormatText(kind TextKind, entries []Entry, f Format) string {
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = formatLine(kind, e, f)
	}
	nl := f.newline()
	s := strings.Join(lines, nl)
	if f.TrailingNewline && len(lines) > 0 {
		s += nl
	}
	return s
}

func formatLine(kind TextKind, e Entry, f Format) string {
	suffix := ""
	if f.DirSuffix && e.IsDir {
		suffix = string(filepath.Separator)
	}
	switch kind {
	case Names:
		return e.name() + suffix
	case FullNames:
		return e.Path + suffix
	case NetNames:
		return uncName(e.Path) + suffix
	case UnixPaths:
		p := strings.ReplaceAll(e.Path, `\`, "/")
		if f.DirSuffix && e.IsDir {
			p += "/"
		}
		return p
	case QuotedNames:
		return QuotePath(e.Path+suffix, runtime.GOOS)
	case URLs:
		u := FileURI(e.Path)
		if f.DirSuffix && e.IsDir {
			u += "/"
		}
		return u
	case Details, FullDetails:
		n := e.name()
		if kind == FullDetails {
			n = e.Path
		}
		size := strconv.FormatInt(e.Size, 10)
		if e.IsDir {
			size = f.DirSizeText
			if size == "" {
				size = "<DIR>"
			}
		}
		layout := f.DateLayout
		if layout == "" {
			layout = "2006-01-02 15:04:05"
		}
		date := ""
		if !e.ModTime.IsZero() {
			date = e.ModTime.Format(layout)
		}
		attr := e.Attr
		if attr == "" && e.Mode != 0 {
			attr = e.Mode.String()
		}
		return n + suffix + "\t" + size + "\t" + date + "\t" + attr
	}
	return e.Path
}

// QuotePath puts a path in double quotes for pasting into a shell. On Windows a path can't hold a double quote, so the
// quotes are all it needs, as Explorer's "Copy as path" does. Elsewhere the characters a POSIX shell still treats
// specially inside double quotes (backslash, double quote, dollar and backquote) get a backslash in front, so the
// pasted text always names exactly this path.
func QuotePath(p, goos string) string {
	if goos == "windows" {
		return `"` + p + `"`
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range p {
		switch r {
		case '\\', '"', '$', '`':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// FolderPath formats a panel folder for cm_CopySrcPathToClip and cm_CopyTrgPathToClip. TC copies the folder with a
// trailing backslash; trailingSep does the same with the platform's separator. A root such as "/" or "C:\" is never
// given a second separator.
func FolderPath(dir string, trailingSep bool) string {
	d := filepath.Clean(dir)
	if trailingSep && !strings.HasSuffix(d, string(filepath.Separator)) {
		d += string(filepath.Separator)
	}
	return d
}

// Lines splits clipboard text into lines for the multi-rename tool, which pairs names with files line by line, so
// blank lines inside the text are kept. CRLF and CR line ends are accepted and one trailing newline is dropped.
func Lines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// ParseNameList reads clipboard text as a list of file names or paths for cm_LoadSelectionFromClip. Blank lines are
// skipped, surrounding whitespace and double quotes are removed, file:// URIs become paths, and a detail line copied
// by cm_CopyDetailsToClip keeps only its first (name) column.
func ParseNameList(text string) []string {
	var out []string
	for _, line := range Lines(text) {
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if len(line) >= 2 && line[0] == '"' && line[len(line)-1] == '"' {
			line = line[1 : len(line)-1]
		}
		if line == "" {
			continue
		}
		if p, ok := PathFromURI(line); ok {
			line = p
		}
		out = append(out, line)
	}
	return out
}

// SelectionNames turns a parsed name list into the names to select in folder dir: bare names are kept, absolute paths
// inside dir become their names, and absolute paths elsewhere are dropped because they cannot match a file in this
// panel. A trailing separator (from DirSuffix) is removed.
func SelectionNames(list []string, dir string) []string {
	dir = filepath.Clean(dir)
	var out []string
	for _, n := range list {
		n = strings.TrimRight(n, `/\`)
		if n == "" {
			continue
		}
		if filepath.IsAbs(n) {
			if !samePath(filepath.Dir(filepath.Clean(n)), dir) {
				continue
			}
			n = filepath.Base(n)
		}
		out = append(out, n)
	}
	return out
}

// samePath compares two cleaned paths, ignoring case on the platforms whose file systems usually do.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
