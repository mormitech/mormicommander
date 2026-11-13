// This file builds and parses the text formats Linux desktops use for files on the clipboard. A file manager puts
// file:// URIs on the clipboard, one per line: text/uri-list (RFC 2483, CRLF line ends, '#' comment lines) is the
// common format that KDE, GTK 4 and most toolkits read, and x-special/gnome-copied-files is the GNOME, Cinnamon, MATE
// and Xfce format whose first line says "copy" or "cut", which is the only way those desktops tell a cut from a copy.
// KDE marks a cut with a separate application/x-kde-cutselection target holding "1". The URI encoding follows glib's
// g_filename_to_uri so the strings match what Nautilus produces byte for byte, which matters because some file
// managers compare clipboard contents to decide whether they still own a cut. The helpers are platform-neutral so they
// are tested everywhere; Windows drive and UNC paths are handled when the code runs on Windows.

package clipfiles

import (
	"net/url"
	"path/filepath"
	"strings"
)

// MIME types and X11 targets used on Linux.
const (
	MimeURIList     = "text/uri-list"
	MimeGnomeCopied = "x-special/gnome-copied-files"
	MimeKDECut      = "application/x-kde-cutselection"
	MimeText        = "text/plain;charset=utf-8"
	MimePNG         = "image/png"
)

// uriSafe holds the bytes glib leaves unescaped in the path part of a file URI, besides letters and digits.
const uriSafe = "-._~!$&'()*+,;=:@/"

// FileURI turns an absolute path into a file:// URI with every other byte percent-encoded, the way
// g_filename_to_uri does. On Windows, "C:\a b" becomes file:///C:/a%20b and "\\srv\share\x" becomes file://srv/share/x.
func FileURI(path string) string {
	p := filepath.ToSlash(path)
	host := ""
	if strings.HasPrefix(p, "//") { // UNC path on Windows: //server/share/...
		rest := p[2:]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			host, p = rest[:i], rest[i:]
		} else {
			host, p = rest, "/"
		}
	} else if hasDrive(p) {
		p = "/" + p
	}
	var b strings.Builder
	b.WriteString("file://")
	b.WriteString(host)
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(uriSafe, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		const hex = "0123456789ABCDEF"
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// hasDrive says whether p starts with a Windows drive letter such as "C:".
func hasDrive(p string) bool {
	return len(p) >= 2 && p[1] == ':' && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
}

// PathFromURI turns a file:// URI back into a native path. It accepts file:///path, file://localhost/path and the
// single-slash file:/path some tools write. A URI for another host is only accepted on Windows, where it becomes a UNC
// path; elsewhere such a file cannot be opened, so ok is false. Non-file URIs (http, smb, trash) also give ok false.
func PathFromURI(uri string) (path string, ok bool) {
	uri = strings.TrimSpace(uri)
	if len(uri) < 5 || !strings.EqualFold(uri[:5], "file:") {
		return "", false
	}
	rest := uri[5:]
	host := ""
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			return "", false
		}
		host, rest = rest[:i], rest[i:]
	}
	if !strings.HasPrefix(rest, "/") {
		return "", false
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 { // a query or fragment is not part of the file name
		rest = rest[:i]
	}
	p, err := url.PathUnescape(rest)
	if err != nil || strings.IndexByte(p, 0) >= 0 {
		return "", false
	}
	windows := filepath.Separator == '\\'
	if host != "" && !strings.EqualFold(host, "localhost") {
		if !windows {
			return "", false
		}
		return filepath.FromSlash("//" + host + p), true
	}
	if windows && len(p) >= 3 && hasDrive(p[1:]) {
		p = p[1:] // "/C:/x" -> "C:/x"
	}
	return filepath.FromSlash(p), true
}

// EncodeURIList builds a text/uri-list payload: one URI per line with CRLF line ends, as RFC 2483 asks.
func EncodeURIList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(FileURI(p))
		b.WriteString("\r\n")
	}
	return b.String()
}

// ParseURIList reads a text/uri-list payload and returns the local paths in it. Comment lines, blank lines and URIs
// that are not local files are skipped. It accepts LF as well as CRLF line ends, since many tools write LF.
func ParseURIList(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || line[0] == '#' {
			continue
		}
		if p, ok := PathFromURI(line); ok {
			out = append(out, p)
		}
	}
	return out
}

// EncodeGnomeCopied builds an x-special/gnome-copied-files payload: "copy" or "cut" on the first line, then one URI
// per line, LF separated and without a trailing newline, exactly as Nautilus writes it.
func EncodeGnomeCopied(paths []string, op Op) string {
	lines := make([]string, 0, len(paths)+1)
	lines = append(lines, op.String())
	for _, p := range paths {
		lines = append(lines, FileURI(p))
	}
	return strings.Join(lines, "\n")
}

// ParseGnomeCopied reads an x-special/gnome-copied-files payload. ok is false when the first line is neither "copy"
// nor "cut", which means the data is not in this format at all.
func ParseGnomeCopied(text string) (paths []string, op Op, ok bool) {
	first, rest, _ := strings.Cut(text, "\n")
	switch strings.TrimSpace(first) {
	case "copy":
		op = Copy
	case "cut":
		op = Cut
	default:
		return nil, Copy, false
	}
	return ParseURIList(rest), op, true
}
