// This file builds the text that the "copy names" commands put on the clipboard: bare names (cm_CopyNamesToClip),
// full paths (cm_CopyFullNamesToClip), names with size, date and attributes separated by tabs (cm_CopyDetailsToClip,
// cm_CopyFullDetailsToClip), and the variants for other programs: forward-slash Unix paths, file:// URLs and, on
// Windows, UNC network names (cm_CopyNetNamesToClip). The panel decides which entries to pass, normally the selection
// or the entry under the cursor when nothing is selected. Lines are joined with the platform line ending so the text
// pastes cleanly into Notepad as well as into a terminal; the clipboard call itself lives in the UI layer.

package masks

import (
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// NameFormat picks the clipboard format.
type NameFormat uint8

const (
	FormatNames       NameFormat = iota // "file.txt"
	FormatFullNames                     // "/home/me/file.txt" or "C:\Users\me\file.txt"
	FormatDetails                       // "file.txt\t1234\t2024-05-01 12:00\t-a--"
	FormatFullDetails                   // like FormatDetails with the full path
	FormatUnixPaths                     // full path with "/" separators, also on Windows
	FormatURLs                          // "file:///home/me/file.txt"
)

// ClipOptions tune the details formats.
type ClipOptions struct {
	TimeLayout string // Go time layout for details; empty means "2006-01-02 15:04:05"
	// MapNet, when set, turns a local path into its network name (a mapped drive "Z:\x" into "\\server\share\x") for
	// cm_CopyNetNamesToClip; the platform layer provides it on Windows. Paths it does not know come back unchanged.
	MapNet func(path string) string
}

// FormatEntries returns the clipboard text for entries in folder dir.
func FormatEntries(dir string, entries []Entry, f NameFormat, opt ClipOptions) string {
	layout := opt.TimeLayout
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		full := e.Path
		if full == "" {
			full = filepath.Join(dir, e.Name)
		}
		if opt.MapNet != nil {
			full = opt.MapNet(full)
		}
		var ln string
		switch f {
		case FormatNames:
			ln = e.Name
		case FormatFullNames:
			ln = full
		case FormatDetails, FormatFullDetails:
			name := e.Name
			if f == FormatFullDetails {
				name = full
			}
			size := strconv.FormatInt(e.Size, 10)
			if e.IsDir && e.Size < 0 {
				size = "<DIR>"
			}
			date := ""
			if !e.ModTime.IsZero() {
				date = e.ModTime.Format(layout)
			}
			ln = strings.Join([]string{name, size, date, e.Attr}, "\t")
		case FormatUnixPaths:
			ln = filepath.ToSlash(full)
		case FormatURLs:
			ln = fileURL(full)
		}
		if e.IsDir && (f == FormatFullNames || f == FormatUnixPaths) {
			// Folders get a trailing separator in full-path copies so scripts reading the list can tell them apart.
			if f == FormatUnixPaths {
				ln = strings.TrimRight(ln, "/") + "/"
			} else {
				ln = strings.TrimRight(ln, string(filepath.Separator)) + string(filepath.Separator)
			}
		}
		lines = append(lines, ln)
	}
	return strings.Join(lines, lineEnding)
}

// fileURL turns a path into a file:// URL: "C:\a b" becomes "file:///C:/a%20b", "\\srv\share\x" becomes
// "file://srv/share/x".
func fileURL(p string) string {
	s := filepath.ToSlash(p)
	if strings.HasPrefix(s, "//") {
		rest := strings.TrimPrefix(s, "//")
		host, tail, _ := strings.Cut(rest, "/")
		u := url.URL{Scheme: "file", Host: host, Path: "/" + tail}
		return u.String()
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	u := url.URL{Scheme: "file", Path: path.Clean(s)}
	return "file://" + u.EscapedPath()
}
