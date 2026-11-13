// Shortcut files: the "smart Enter" on .lnk, .url, .webloc and .desktop files (F-738), and the target shown by
// cm_GoToDir (F-075). Each platform has its own little file that points elsewhere: Windows shell links (.lnk, a
// binary format documented as MS-SHLLINK) and Internet shortcuts (.url, an INI file), macOS web locations (.webloc,
// a property list) and freedesktop links (.desktop with Type=Link or Type=Application). ReadShortcut reads any of
// them on any platform, so a .lnk copied to a Mac still leads somewhere, and tells the caller what it points to:
// a web address to open, a path to go to or open, or a command to run.
package launch

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// ShortcutKind says what a shortcut points to.
type ShortcutKind int

const (
	ShortcutNone    ShortcutKind = iota // not a shortcut we understand
	ShortcutURL                         // Target is a web address (or any URL)
	ShortcutPath                        // Target is a file or folder path
	ShortcutCommand                     // Target is a program; Args and Dir complete it
)

// Shortcut is what a shortcut file points to.
type Shortcut struct {
	Kind   ShortcutKind
	Target string
	Args   string // .lnk command line arguments, or the rest of a .desktop Exec line
	Dir    string // working folder, when the shortcut has one
}

// IsShortcutName says whether a file name has one of the shortcut extensions ReadShortcut understands.
func IsShortcutName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".lnk", ".url", ".webloc", ".desktop":
		return true
	}
	return false
}

// ReadShortcut reads the shortcut file at path.
func ReadShortcut(path string) (Shortcut, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Shortcut{}, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".url":
		return parseURLShortcut(data)
	case ".webloc":
		return parseWebloc(data)
	case ".desktop":
		return parseDesktopShortcut(data, filepath.Dir(path))
	case ".lnk":
		return ParseLnk(data, filepath.Dir(path))
	}
	return Shortcut{}, errors.New("launch: not a shortcut file")
}

func parseURLShortcut(data []byte) (Shortcut, error) {
	s := ParseINI(data).Section("InternetShortcut")
	if s != nil {
		if u, ok := s.Get("URL"); ok && u != "" {
			if strings.HasPrefix(strings.ToLower(u), "file:///") {
				return Shortcut{Kind: ShortcutPath, Target: fileURLPath(u)}, nil
			}
			return Shortcut{Kind: ShortcutURL, Target: u}, nil
		}
	}
	return Shortcut{}, errors.New("launch: .url file without URL")
}

// fileURLPath turns file:///C:/x%20y or file:///home/x into a path.
func fileURLPath(u string) string {
	p := u[len("file://"):]
	p = percentDecode(p)
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		return strings.ReplaceAll(p[1:], "/", `\`)
	}
	return p
}

func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, ok := unhex(s[i+1], s[i+2]); ok {
				b = append(b, v)
				i += 2
				continue
			}
		}
		b = append(b, s[i])
	}
	return string(b)
}

func unhex(a, b byte) (byte, bool) {
	h := func(c byte) (byte, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	x, ok1 := h(a)
	y, ok2 := h(b)
	return x<<4 | y, ok1 && ok2
}

func parseWebloc(data []byte) (Shortcut, error) {
	v, err := ParsePlist(data)
	if err != nil {
		return Shortcut{}, err
	}
	if m, ok := v.(map[string]any); ok {
		if u, ok := m["URL"].(string); ok && u != "" {
			if strings.HasPrefix(u, "file://") {
				return Shortcut{Kind: ShortcutPath, Target: fileURLPath(u)}, nil
			}
			return Shortcut{Kind: ShortcutURL, Target: u}, nil
		}
	}
	return Shortcut{}, errors.New("launch: .webloc file without URL")
}

func parseDesktopShortcut(data []byte, dir string) (Shortcut, error) {
	e := ParseDesktopEntry(data, "")
	switch e.Type {
	case "Link":
		if e.URL == "" {
			break
		}
		if strings.HasPrefix(e.URL, "file://") {
			return Shortcut{Kind: ShortcutPath, Target: fileURLPath(e.URL)}, nil
		}
		if strings.HasPrefix(e.URL, "/") {
			return Shortcut{Kind: ShortcutPath, Target: e.URL}, nil
		}
		return Shortcut{Kind: ShortcutURL, Target: e.URL}, nil
	case "Application":
		argv, err := ExpandDesktopExec(e.Exec, nil, e.Name, e.Icon, "")
		if err != nil || len(argv) == 0 {
			break
		}
		q := make([]string, 0, len(argv)-1)
		for _, a := range argv[1:] {
			q = append(q, EscapeArg(a))
		}
		return Shortcut{Kind: ShortcutCommand, Target: argv[0], Args: strings.Join(q, " ")}, nil
	}
	return Shortcut{}, errors.New("launch: .desktop file is neither a link nor an application")
}

// Shell link flags (MS-SHLLINK 2.1.1).
const (
	lnkHasIDList     = 1 << 0
	lnkHasLinkInfo   = 1 << 1
	lnkHasName       = 1 << 2
	lnkHasRelPath    = 1 << 3
	lnkHasWorkingDir = 1 << 4
	lnkHasArguments  = 1 << 5
	lnkHasIcon       = 1 << 6
	lnkIsUnicode     = 1 << 7
)

var errLnk = errors.New("launch: malformed .lnk file")

// ParseLnk reads a Windows shell link. dir is the folder the .lnk file is in, used when the link only stores a
// path relative to itself.
func ParseLnk(data []byte, dir string) (Shortcut, error) {
	if len(data) < 0x4C || binary.LittleEndian.Uint32(data) != 0x4C {
		return Shortcut{}, errLnk
	}
	flags := binary.LittleEndian.Uint32(data[0x14:])
	pos := 0x4C
	if flags&lnkHasIDList != 0 {
		if pos+2 > len(data) {
			return Shortcut{}, errLnk
		}
		pos += 2 + int(binary.LittleEndian.Uint16(data[pos:]))
	}
	target := ""
	if flags&lnkHasLinkInfo != 0 {
		if pos+4 > len(data) {
			return Shortcut{}, errLnk
		}
		size := int(binary.LittleEndian.Uint32(data[pos:]))
		if size < 0x1C || pos+size > len(data) {
			return Shortcut{}, errLnk
		}
		target = linkInfoPath(data[pos : pos+size])
		pos += size
	}
	unicode := flags&lnkIsUnicode != 0
	readString := func() (string, error) {
		if pos+2 > len(data) {
			return "", errLnk
		}
		n := int(binary.LittleEndian.Uint16(data[pos:]))
		pos += 2
		if unicode {
			if pos+2*n > len(data) {
				return "", errLnk
			}
			u := make([]uint16, n)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(data[pos+2*i:])
			}
			pos += 2 * n
			return string(utf16.Decode(u)), nil
		}
		if pos+n > len(data) {
			return "", errLnk
		}
		s := string(data[pos : pos+n])
		pos += n
		return s, nil
	}
	var rel, work, args string
	for _, f := range []uint32{lnkHasName, lnkHasRelPath, lnkHasWorkingDir, lnkHasArguments, lnkHasIcon} {
		if flags&f == 0 {
			continue
		}
		s, err := readString()
		if err != nil {
			return Shortcut{}, err
		}
		switch f {
		case lnkHasRelPath:
			rel = s
		case lnkHasWorkingDir:
			work = s
		case lnkHasArguments:
			args = s
		}
	}
	if target == "" && rel != "" {
		target = joinPath("windows", dir, rel)
		if !strings.Contains(dir, `\`) {
			// The .lnk sits on a non-Windows file system: resolve the relative path with its conventions.
			target = filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		}
	}
	if target == "" {
		return Shortcut{}, errors.New("launch: .lnk file without a file system target")
	}
	kind := ShortcutPath
	if args != "" {
		kind = ShortcutCommand
	}
	return Shortcut{Kind: kind, Target: target, Args: args, Dir: work}, nil
}

// linkInfoPath reads the target path out of a LinkInfo structure (MS-SHLLINK 2.3): a local base path, or a network
// share name, followed by the common path suffix. The Unicode copies are preferred when present.
func linkInfoPath(li []byte) string {
	u32 := func(off int) int {
		if off+4 > len(li) {
			return 0
		}
		return int(binary.LittleEndian.Uint32(li[off:]))
	}
	headerSize := u32(4)
	liFlags := u32(8)
	localOff, netOff, suffixOff := u32(16), u32(20), u32(24)
	localOffU, suffixOffU := 0, 0
	if headerSize >= 0x24 {
		localOffU, suffixOffU = u32(28), u32(32)
	}
	suffix := cString(li, suffixOff)
	if suffixOffU > 0 {
		suffix = wString(li, suffixOffU)
	}
	if liFlags&1 != 0 {
		base := cString(li, localOff)
		if localOffU > 0 {
			base = wString(li, localOffU)
		}
		return joinSuffix(base, suffix)
	}
	if liFlags&2 != 0 && netOff > 0 && netOff < len(li) {
		cnr := li[netOff:]
		if len(cnr) < 0x14 {
			return ""
		}
		nameOff := int(binary.LittleEndian.Uint32(cnr[8:]))
		name := cString(cnr, nameOff)
		if nameOff > 0x14 && len(cnr) >= 0x1C {
			if uo := int(binary.LittleEndian.Uint32(cnr[0x14:])); uo > 0 {
				name = wString(cnr, uo)
			}
		}
		return joinSuffix(name, suffix)
	}
	return ""
}

func joinSuffix(base, suffix string) string {
	if suffix == "" {
		return base
	}
	if strings.HasSuffix(base, `\`) {
		return base + suffix
	}
	return base + `\` + suffix
}

func cString(b []byte, off int) string {
	if off <= 0 || off >= len(b) {
		return ""
	}
	end := off
	for end < len(b) && b[end] != 0 {
		end++
	}
	return string(b[off:end])
}

func wString(b []byte, off int) string {
	if off <= 0 || off >= len(b) {
		return ""
	}
	var u []uint16
	for i := off; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
