// Windows shell shortcuts (.lnk files) are what Explorer and TC on Windows
// create for "Create shortcut", and users meet them everywhere: on desktops,
// in Start menu folders, on USB sticks carried between machines. TC follows a
// .lnk when Enter is pressed on it and shows its target in Properties, so we
// read them on every platform, but never write them (see the package README).
// The format is Microsoft's documented MS-SHLLINK: a fixed 76-byte header,
// an optional shell item list, an optional LinkInfo block holding the local
// or network path, a run of counted strings (description, relative path,
// working folder, arguments, icon) and optional extra data blocks, of which
// the environment-variable block (a target such as %USERPROFILE%\x) is used
// as a fallback. The parser is defensive: every offset is bounds-checked and
// a truncated or hostile file returns ErrBadShortcut instead of panicking.
// Targets that exist only as a shell item list (special folders) are not
// decoded; such a shortcut reports its relative path or an empty target.

package links

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf16"
)

// Lnk holds the fields of a .lnk file that a file manager shows.
type Lnk struct {
	// Target is the absolute target path as stored (a Windows path, possibly
	// a UNC path or one with %VARIABLES%).
	Target string
	// Name is the shortcut's description ("Comment" in Explorer).
	Name         string
	RelativePath string
	WorkingDir   string
	Arguments    string
	IconLocation string
	IconIndex    int32
	// FileSize, Attributes and WriteTime describe the target as it was when
	// the shortcut was made.
	FileSize   uint32
	Attributes uint32
	WriteTime  time.Time
	// ShowCommand is 1 (normal), 3 (maximized) or 7 (minimized).
	ShowCommand uint32
	// HotKey is the raw HOTKEYFLAGS value (low byte key, high byte modifiers).
	HotKey uint16
	// EnvTarget is the target from the environment-variable block, unexpanded.
	EnvTarget string
}

// IsDir reports whether the target was a folder when the shortcut was made.
func (l *Lnk) IsDir() bool { return l.Attributes&0x10 != 0 }

const (
	lnkHasIDList     = 0x1
	lnkHasLinkInfo   = 0x2
	lnkHasName       = 0x4
	lnkHasRelPath    = 0x8
	lnkHasWorkingDir = 0x10
	lnkHasArguments  = 0x20
	lnkHasIcon       = 0x40
	lnkIsUnicode     = 0x80
	lnkHasExpString  = 0x200

	lnkEnvBlockSig = 0xA0000001
)

var lnkCLSID = []byte{0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}

// IsLnkData reports whether data starts like a shell link.
func IsLnkData(data []byte) bool {
	return len(data) >= 20 && binary.LittleEndian.Uint32(data) == 0x4C && bytes.Equal(data[4:20], lnkCLSID)
}

// ReadLnk reads and parses the .lnk file at path.
func ReadLnk(path string) (*Lnk, error) {
	data, err := readSmall(path, 1<<20)
	if err != nil {
		return nil, err
	}
	l, err := ParseLnk(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// ParseLnk parses the bytes of a .lnk file.
func ParseLnk(data []byte) (*Lnk, error) {
	if !IsLnkData(data) || len(data) < 76 {
		return nil, ErrBadShortcut
	}
	le := binary.LittleEndian
	flags := le.Uint32(data[20:])
	l := &Lnk{
		Attributes:  le.Uint32(data[24:]),
		WriteTime:   filetime(le.Uint64(data[44:])),
		FileSize:    le.Uint32(data[52:]),
		IconIndex:   int32(le.Uint32(data[56:])),
		ShowCommand: le.Uint32(data[60:]),
		HotKey:      le.Uint16(data[64:]),
	}
	pos := 76
	if flags&lnkHasIDList != 0 {
		if pos+2 > len(data) {
			return nil, ErrBadShortcut
		}
		pos += 2 + int(le.Uint16(data[pos:]))
	}
	if flags&lnkHasLinkInfo != 0 {
		if pos+4 > len(data) {
			return nil, ErrBadShortcut
		}
		size := int(le.Uint32(data[pos:]))
		if size < 0x1C || pos+size > len(data) {
			return nil, ErrBadShortcut
		}
		t, err := parseLinkInfo(data[pos : pos+size])
		if err != nil {
			return nil, err
		}
		l.Target = t
		pos += size
	}
	unicode := flags&lnkIsUnicode != 0
	for _, f := range []struct {
		bit uint32
		dst *string
	}{
		{lnkHasName, &l.Name}, {lnkHasRelPath, &l.RelativePath}, {lnkHasWorkingDir, &l.WorkingDir},
		{lnkHasArguments, &l.Arguments}, {lnkHasIcon, &l.IconLocation},
	} {
		if flags&f.bit == 0 {
			continue
		}
		if pos+2 > len(data) {
			return nil, ErrBadShortcut
		}
		n := int(le.Uint16(data[pos:]))
		pos += 2
		if unicode {
			if pos+2*n > len(data) {
				return nil, ErrBadShortcut
			}
			*f.dst = utf16le(data[pos : pos+2*n])
			pos += 2 * n
		} else {
			if pos+n > len(data) {
				return nil, ErrBadShortcut
			}
			*f.dst = latin1(data[pos : pos+n])
			pos += n
		}
	}
	// Extra data blocks: a sequence of (size, signature, payload) ending
	// with a block whose size is below 4.
	for pos+8 <= len(data) {
		size := int(le.Uint32(data[pos:]))
		if size < 8 || pos+size > len(data) {
			break
		}
		if le.Uint32(data[pos+4:]) == lnkEnvBlockSig && size >= 8+260+520 {
			b := data[pos+8:]
			if s := cstrUTF16(b[260 : 260+520]); s != "" {
				l.EnvTarget = s
			} else {
				l.EnvTarget = cstr(b[:260])
			}
		}
		pos += size
	}
	if l.Target == "" && flags&lnkHasExpString != 0 {
		l.Target = l.EnvTarget
	}
	return l, nil
}

// parseLinkInfo returns the target path stored in a LinkInfo structure.
func parseLinkInfo(b []byte) (string, error) {
	le := binary.LittleEndian
	hdr := int(le.Uint32(b[4:]))
	flags := le.Uint32(b[8:])
	at := func(off int) int {
		if off+4 > hdr || off+4 > len(b) {
			return 0
		}
		return int(le.Uint32(b[off:]))
	}
	str := func(off int, wide bool) string {
		if off <= 0 || off >= len(b) {
			return ""
		}
		if wide {
			return cstrUTF16(b[off:])
		}
		return cstr(b[off:])
	}
	wide := hdr >= 0x24
	suffix := str(at(0x18), false)
	if wide && at(0x20) != 0 {
		suffix = str(at(0x20), true)
	}
	if flags&1 != 0 { // VolumeIDAndLocalBasePath
		base := str(at(0x10), false)
		if wide && at(0x1C) != 0 {
			base = str(at(0x1C), true)
		}
		return joinWin(base, suffix), nil
	}
	if flags&2 != 0 { // CommonNetworkRelativeLinkAndPathSuffix
		off := at(0x14)
		if off <= 0 || off+20 > len(b) {
			return "", ErrBadShortcut
		}
		cn := b[off:]
		netOff := int(le.Uint32(cn[8:]))
		net := ""
		if netOff > 0x14 && len(cn) >= 0x1C {
			if u := int(le.Uint32(cn[0x14:])); u > 0 && u < len(cn) {
				net = cstrUTF16(cn[u:])
			}
		}
		if net == "" && netOff > 0 && netOff < len(cn) {
			net = cstr(cn[netOff:])
		}
		return joinWin(net, suffix), nil
	}
	return "", nil
}

func joinWin(base, suffix string) string {
	if suffix == "" {
		return base
	}
	if base == "" || strings.HasSuffix(base, `\`) {
		return base + suffix
	}
	return base + `\` + suffix
}

// ExpandWindowsEnv replaces %NAME% references using lookup (os.LookupEnv
// when nil). Unknown names are left as they are, like Windows does.
func ExpandWindowsEnv(s string, lookup func(string) (string, bool)) string {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	var out strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		out.WriteString(s[:i])
		if v, ok := lookup(name); ok && name != "" {
			out.WriteString(v)
		} else {
			out.WriteString(s[i : i+2+j])
		}
		s = s[i+2+j:]
	}
	out.WriteString(s)
	return out.String()
}

func filetime(ft uint64) time.Time {
	if ft == 0 {
		return time.Time{}
	}
	// FILETIME counts 100 ns ticks since 1601-01-01 UTC.
	const epochDiff = 116444736000000000
	if ft < epochDiff {
		return time.Time{}
	}
	return time.Unix(0, int64(ft-epochDiff)*100).UTC()
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

func cstrUTF16(b []byte) string {
	for i := 0; i+1 < len(b); i += 2 {
		if b[i] == 0 && b[i+1] == 0 {
			return utf16le(b[:i])
		}
	}
	return utf16le(b[:len(b)&^1])
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return latin1(b)
}

// latin1 decodes single-byte text. Real .lnk files use the system ANSI code
// page; Latin-1 is right for ASCII and close enough for display otherwise.
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// readSmall reads a whole file but refuses files larger than limit, so a
// mislabelled huge file is never slurped into memory just to show a target.
func readSmall(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w (file too large)", path, ErrBadShortcut)
	}
	return data, nil
}
