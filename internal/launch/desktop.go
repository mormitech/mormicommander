// Freedesktop .desktop entries. On Linux and the BSDs an application announces itself with a .desktop file: its
// name, the command that starts it (Exec, with field codes such as %f for one file and %F for several), the file
// types it opens (MimeType) and whether it needs a terminal. The "Open with" list, opening a file with a chosen app,
// and the smart Enter on .desktop shortcuts (Type=Link) all read these files, so the parsing lives here, free of any
// platform code, where it can be tested anywhere.
package launch

import (
	"strings"
)

// DesktopEntry is the useful part of a .desktop file's [Desktop Entry] group.
type DesktopEntry struct {
	Type      string   // "Application", "Link" or "Directory"
	Name      string   // display name
	Exec      string   // command template with field codes
	TryExec   string   // program that must exist for the entry to count
	URL       string   // for Type=Link
	Icon      string   // icon name or path
	MimeTypes []string // MIME types the application opens
	Terminal  bool     // the program must run in a terminal
	NoDisplay bool     // not shown in menus (still a valid handler)
	Hidden    bool     // deleted by the user or the system: ignore it
}

// ParseDesktopEntry reads a .desktop file. Localised names are used when one matches lang (such as "de_DE" or
// "de"); otherwise the plain Name is used.
func ParseDesktopEntry(data []byte, lang string) DesktopEntry {
	var e DesktopEntry
	s := ParseINI(data).Section("Desktop Entry")
	if s == nil {
		return e
	}
	get := func(k string) string {
		v, _ := s.Get(k)
		return unescapeDesktop(v)
	}
	e.Type = get("Type")
	e.Name = get("Name")
	if lang != "" {
		short, _, _ := strings.Cut(lang, "_")
		for _, l := range []string{lang, short} {
			if v, ok := s.Get("Name[" + l + "]"); ok && v != "" {
				e.Name = unescapeDesktop(v)
				break
			}
		}
	}
	e.Exec = get("Exec")
	e.TryExec = get("TryExec")
	e.URL = get("URL")
	e.Icon = get("Icon")
	for _, m := range strings.Split(get("MimeType"), ";") {
		if m = strings.TrimSpace(m); m != "" {
			e.MimeTypes = append(e.MimeTypes, m)
		}
	}
	e.Terminal = strings.EqualFold(get("Terminal"), "true")
	e.NoDisplay = strings.EqualFold(get("NoDisplay"), "true")
	e.Hidden = strings.EqualFold(get("Hidden"), "true")
	return e
}

// unescapeDesktop undoes the escapes the desktop entry format allows in values: \s \n \t \r \\.
func unescapeDesktop(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			switch v[i+1] {
			case 's':
				b.WriteByte(' ')
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			case '\\':
				// Exec then has its own second level of quoting, which SplitArgs undoes.
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// Handles says whether the entry opens files of this MIME type, either exactly or through a "type/*" wildcard.
func (e DesktopEntry) Handles(mimeType string) bool {
	major, _, _ := strings.Cut(mimeType, "/")
	for _, m := range e.MimeTypes {
		if strings.EqualFold(m, mimeType) || m == major+"/*" {
			return true
		}
	}
	return false
}

// ExpandDesktopExec turns an Exec template into an argument list for the given files. %f and %u take the first
// file (an argument that is only that code disappears when there is no file), %F and %U take all files as separate
// arguments, %i becomes "--icon <icon>", %c the name, %k the location of the .desktop file, %% a percent sign; the
// deprecated codes are dropped. When the template has no file code at all, the files are appended, so "Open with"
// still passes the file to programs with a sloppy Exec line.
func ExpandDesktopExec(exec string, files []string, name, icon, location string) ([]string, error) {
	words, err := SplitArgs(exec)
	if err != nil {
		return nil, err
	}
	var out []string
	usedFiles := false
	for _, w := range words {
		switch w {
		case "%f", "%u":
			usedFiles = true
			if len(files) > 0 {
				out = append(out, files[0])
			}
			continue
		case "%F", "%U":
			usedFiles = true
			out = append(out, files...)
			continue
		case "%i":
			if icon != "" {
				out = append(out, "--icon", icon)
			}
			continue
		}
		var b strings.Builder
		for i := 0; i < len(w); i++ {
			if w[i] != '%' || i+1 >= len(w) {
				b.WriteByte(w[i])
				continue
			}
			i++
			switch w[i] {
			case '%':
				b.WriteByte('%')
			case 'f', 'u':
				usedFiles = true
				if len(files) > 0 {
					b.WriteString(files[0])
				}
			case 'F', 'U':
				usedFiles = true
				b.WriteString(strings.Join(files, " "))
			case 'c':
				b.WriteString(name)
			case 'k':
				b.WriteString(location)
			case 'i':
				b.WriteString(icon)
			default:
				// Deprecated or unknown codes (%d %D %n %N %v %m) are removed.
			}
		}
		if b.Len() == 0 && len(w) == 2 && w[0] == '%' {
			continue // a dropped code standing alone leaves no empty argument behind
		}
		out = append(out, b.String())
	}
	if !usedFiles {
		out = append(out, files...)
	}
	return out, nil
}
