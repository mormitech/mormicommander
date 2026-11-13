// Two shortcut formats are small INI-style text files. Windows Internet
// shortcuts (.url) keep the address under [InternetShortcut] URL=, and TC
// opens them in the browser when Enter is pressed (F-746). Linux desktops use
// freedesktop.org desktop entries (.desktop): a [Desktop Entry] group whose
// Type is Link (with a URL, which may be file:// for a local file), Directory,
// or Application (with an Exec command line). Smart Enter (F-738) needs the
// target of both, and Ctrl+Shift+F5 on Linux writes a Type=Link entry as the
// desktop equivalent of a Windows shortcut. This file reads and writes both
// formats and converts between local paths and file:// URLs, which both of
// them (and .webloc) use for shortcuts to files.

package links

import (
	"bufio"
	"bytes"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// IniShortcut is the parsed content of a .url or .desktop file.
type IniShortcut struct {
	// Type is the desktop entry Type (Link, Application, Directory); "Link"
	// for .url files.
	Type       string
	Name       string
	URL        string
	Exec       string
	WorkingDir string
	Icon       string
	Comment    string
}

// readIniGroup returns the key=value pairs of the named group. Keys are
// matched exactly, as both formats are case-sensitive in practice; localized
// keys such as Name[de] are kept under their full name.
func readIniGroup(data []byte, group string) (map[string]string, bool) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	in, found := false, false
	out := map[string]string{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			in = strings.EqualFold(line[1:len(line)-1], group)
			found = found || in
			continue
		}
		if !in {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			if _, dup := out[k]; !dup { // first value wins, as in GLib
				out[k] = strings.TrimSpace(v)
			}
		}
	}
	return out, found
}

// ParseURLShortcut parses a Windows .url file.
func ParseURLShortcut(data []byte) (*IniShortcut, error) {
	m, ok := readIniGroup(data, "InternetShortcut")
	if !ok || m["URL"] == "" {
		return nil, ErrBadShortcut
	}
	return &IniShortcut{Type: "Link", URL: m["URL"], WorkingDir: m["WorkingDirectory"], Icon: m["IconFile"]}, nil
}

// ParseDesktopEntry parses a freedesktop.org .desktop file.
func ParseDesktopEntry(data []byte) (*IniShortcut, error) {
	m, ok := readIniGroup(data, "Desktop Entry")
	if !ok {
		return nil, ErrBadShortcut
	}
	s := &IniShortcut{
		Type:       desktopUnescape(m["Type"]),
		Name:       desktopUnescape(m["Name"]),
		URL:        desktopUnescape(m["URL"]),
		Exec:       desktopUnescape(m["Exec"]),
		WorkingDir: desktopUnescape(m["Path"]),
		Icon:       desktopUnescape(m["Icon"]),
		Comment:    desktopUnescape(m["Comment"]),
	}
	if s.Type == "" {
		return nil, ErrBadShortcut
	}
	return s, nil
}

func desktopUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func desktopEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	s = r.Replace(s)
	if strings.HasPrefix(s, " ") {
		s = `\s` + s[1:]
	}
	return s
}

// ExecArgs splits a desktop entry Exec value into program and arguments,
// honouring double quotes and dropping field codes (%f, %U, ...), which a
// file manager fills only when passing files to an application.
func ExecArgs(exec string) []string {
	var args []string
	var cur strings.Builder
	inQuote, have := false, false
	for i := 0; i < len(exec); i++ {
		c := exec[i]
		switch {
		case inQuote && c == '\\' && i+1 < len(exec):
			i++
			cur.WriteByte(exec[i])
		case c == '"':
			inQuote = !inQuote
			have = true
		case !inQuote && (c == ' ' || c == '\t'):
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if have {
		args = append(args, cur.String())
	}
	out := args[:0]
	for _, a := range args {
		if len(a) == 2 && a[0] == '%' && a[1] != '%' {
			continue // a lone field code
		}
		out = append(out, strings.ReplaceAll(a, "%%", "%"))
	}
	return out
}

// URLShortcutData returns the content of a Windows .url file for u.
func URLShortcutData(u string) []byte {
	return []byte("[InternetShortcut]\r\nURL=" + u + "\r\n")
}

// DesktopLinkData returns a Type=Link desktop entry named name for u.
func DesktopLinkData(name, u string) []byte {
	return []byte("[Desktop Entry]\nVersion=1.0\nType=Link\nName=" + desktopEscape(name) + "\nURL=" + desktopEscape(u) + "\n")
}

// FileURL returns the file:// URL for an absolute local path. Windows drive
// paths become file:///C:/..., UNC paths file://server/share/...
func FileURL(path string) string {
	// Backslashes are separators on Windows, and in Windows-style paths
	// (drive letter or UNC) met on other systems; elsewhere they are
	// ordinary file name characters.
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" || len(path) >= 2 && path[1] == ':' || strings.HasPrefix(path, `\\`) {
		p = strings.ReplaceAll(path, `\`, "/")
	}
	u := url.URL{Scheme: "file"}
	switch {
	case strings.HasPrefix(p, "//"):
		host, rest, _ := strings.Cut(p[2:], "/")
		u.Host, u.Path = host, "/"+rest
	case len(p) >= 2 && p[1] == ':':
		u.Path = "/" + p
	default:
		u.Path = p
	}
	return u.String()
}

// PathFromFileURL converts a file:// URL back to a local path. ok is false
// for other schemes.
func PathFromFileURL(s string) (path string, ok bool) {
	u, err := url.Parse(s)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		return `\\` + u.Host + strings.ReplaceAll(p, "/", `\`), true
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' { // /C:/x
		return strings.ReplaceAll(p[1:], "/", `\`), true
	}
	return filepath.FromSlash(p), true
}

// IsURL reports whether s looks like an absolute URL with a scheme (http:,
// https:, ftp:, mailto:, file:, ...) rather than a file path. A Windows drive
// letter ("C:") is not a scheme.
func IsURL(s string) bool {
	i := strings.IndexByte(s, ':')
	if i < 2 {
		return false
	}
	for j := 0; j < i; j++ {
		c := s[j]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return false
		}
	}
	return true
}
