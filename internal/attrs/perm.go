// Unix permissions in all the shapes the attributes dialog needs. The dialog
// shows a grid of checkboxes (read, write, execute for owner, group and
// others, plus setuid, setgid and sticky), an octal field such as 4755 that
// stays in sync with the grid, and the familiar rwxr-xr-x text that the panel's
// attribute column displays. Power users also expect chmod expressions like
// "u+x,go-w" or "a+X", so those are parsed here too. Everything works on Go's
// fs.FileMode, where setuid, setgid and sticky live in high bits, and the
// helpers convert to and from the classic Unix octal layout.
package attrs

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// PermMask selects the permission part of an fs.FileMode: the nine rwx bits
// and the three special bits.
const PermMask = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// PermBits lists every permission bit in dialog order (owner rwx, group rwx,
// others rwx, then setuid, setgid, sticky), so the UI can build its checkbox
// grid from one list.
var PermBits = []fs.FileMode{
	0o400, 0o200, 0o100,
	0o040, 0o020, 0o010,
	0o004, 0o002, 0o001,
	fs.ModeSetuid, fs.ModeSetgid, fs.ModeSticky,
}

// PermBitName returns a short label for one of the PermBits entries.
func PermBitName(bit fs.FileMode) string {
	names := map[fs.FileMode]string{
		0o400: "owner read", 0o200: "owner write", 0o100: "owner execute",
		0o040: "group read", 0o020: "group write", 0o010: "group execute",
		0o004: "others read", 0o002: "others write", 0o001: "others execute",
		fs.ModeSetuid: "set user ID", fs.ModeSetgid: "set group ID", fs.ModeSticky: "sticky",
	}
	return names[bit]
}

// ToUnix converts the permission part of a Go mode to classic Unix bits
// (04000 setuid, 02000 setgid, 01000 sticky, 0777 rwx).
func ToUnix(m fs.FileMode) uint32 {
	u := uint32(m & fs.ModePerm)
	if m&fs.ModeSetuid != 0 {
		u |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		u |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		u |= 0o1000
	}
	return u
}

// FromUnix converts classic Unix permission bits to a Go mode (permission
// part only).
func FromUnix(u uint32) fs.FileMode {
	m := fs.FileMode(u & 0o777)
	if u&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if u&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if u&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}

// FormatOctal returns the four-digit octal form, e.g. "0644" or "4755".
func FormatOctal(m fs.FileMode) string {
	return fmt.Sprintf("%04o", ToUnix(m))
}

// ParseOctal parses one to four octal digits ("755", "0644", "4755"). A
// leading "0o" is accepted too. Fewer digits mean the special bits are clear,
// as with chmod.
func ParseOctal(s string) (fs.FileMode, error) {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(strings.TrimPrefix(t, "0o"), "0O")
	if t == "" || len(t) > 4 {
		return 0, fmt.Errorf("octal permissions %q: want 1 to 4 octal digits", s)
	}
	v, err := strconv.ParseUint(t, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("octal permissions %q: %w", s, errors.Unwrap(err))
	}
	return FromUnix(uint32(v)), nil
}

// FormatPerm returns the nine-character ls-style text, e.g. "rwsr-x--T".
// A lower-case s/t means the execute bit under it is set, upper case means it
// is not, exactly like ls.
func FormatPerm(m fs.FileMode) string {
	b := []byte("---------")
	for i, c := range "rwxrwxrwx" {
		if m&(1<<uint(8-i)) != 0 {
			b[i] = byte(c)
		}
	}
	special := func(pos int, set bool, lower, upper byte) {
		if !set {
			return
		}
		if b[pos] == 'x' {
			b[pos] = lower
		} else {
			b[pos] = upper
		}
	}
	special(2, m&fs.ModeSetuid != 0, 's', 'S')
	special(5, m&fs.ModeSetgid != 0, 's', 'S')
	special(8, m&fs.ModeSticky != 0, 't', 'T')
	return string(b)
}

// FormatMode returns the ten-character text with a leading type letter, as
// ls -l and the panel attribute column show it: d folder, l link, p pipe,
// s socket, c character device, b block device, - everything else.
func FormatMode(m fs.FileMode) string {
	t := byte('-')
	switch kindOf(m) {
	case KindDir:
		t = 'd'
	case KindSymlink:
		t = 'l'
	case KindNamedPipe:
		t = 'p'
	case KindSocket:
		t = 's'
	case KindCharDevice:
		t = 'c'
	case KindDevice:
		t = 'b'
	}
	return string(t) + FormatPerm(m)
}

// ParsePerm parses the nine-character ls text, optionally with the leading
// type letter ("rwxr-x---", "-rw-r--r--", "drwxrwxrwt"). It returns only the
// permission part.
func ParsePerm(s string) (fs.FileMode, error) {
	t := strings.TrimSpace(s)
	if len(t) == 10 {
		t = t[1:]
	}
	if len(t) != 9 {
		return 0, fmt.Errorf("permissions %q: want 9 characters like rwxr-xr-x", s)
	}
	var m fs.FileMode
	for i := 0; i < 9; i++ {
		c := t[i]
		bit := fs.FileMode(1) << uint(8-i)
		want := "rwx"[i%3]
		switch {
		case c == '-':
		case c == want:
			m |= bit
		case i%3 == 2 && (c == 's' || c == 't'):
			m |= bit
			m |= specialFor(i)
			if (c == 't') != (i == 8) {
				return 0, fmt.Errorf("permissions %q: %c not allowed at position %d", s, c, i+1)
			}
		case i%3 == 2 && (c == 'S' || c == 'T'):
			m |= specialFor(i)
			if (c == 'T') != (i == 8) {
				return 0, fmt.Errorf("permissions %q: %c not allowed at position %d", s, c, i+1)
			}
		default:
			return 0, fmt.Errorf("permissions %q: unexpected %q at position %d", s, c, i+1)
		}
	}
	return m, nil
}

func specialFor(i int) fs.FileMode {
	switch i {
	case 2:
		return fs.ModeSetuid
	case 5:
		return fs.ModeSetgid
	default:
		return fs.ModeSticky
	}
}

// ParsePermAny accepts whatever a user is likely to type into the
// permissions field: octal digits or ls text.
func ParsePermAny(s string) (fs.FileMode, error) {
	t := strings.TrimSpace(s)
	if t != "" && strings.Trim(strings.TrimPrefix(t, "0o"), "01234567") == "" {
		return ParseOctal(t)
	}
	return ParsePerm(t)
}

// ApplyChmod applies a chmod-style symbolic expression to a mode and returns
// the new permission part. It understands comma-separated clauses of the form
// [ugoa]*[+-=][rwxXst]* or [ugoa]*[+-=][ugo], and plain octal ("644").
// Unlike the chmod command no umask is applied when the "who" part is empty;
// the dialog should do exactly what the user typed. X sets execute only for
// folders and for files that already have some execute bit, which is the
// usual way to make a tree readable without making every file executable.
func ApplyChmod(m fs.FileMode, expr string, isDir bool) (fs.FileMode, error) {
	e := strings.TrimSpace(expr)
	if e == "" {
		return m & PermMask, nil
	}
	if strings.Trim(e, "01234567") == "" {
		return ParseOctal(e)
	}
	cur := ToUnix(m)
	for _, clause := range strings.Split(e, ",") {
		var err error
		cur, err = applyClause(cur, strings.TrimSpace(clause), isDir)
		if err != nil {
			return 0, fmt.Errorf("chmod %q: %w", expr, err)
		}
	}
	return FromUnix(cur), nil
}

// applyClause applies one clause like "go-w" or "u=rwx,+t" pieces to Unix bits.
func applyClause(cur uint32, clause string, isDir bool) (uint32, error) {
	if clause == "" {
		return 0, errors.New("empty clause")
	}
	i := 0
	var who uint32 // mask of rwx bits addressed
	var whoSpecial uint32
	for i < len(clause) && strings.IndexByte("ugoa", clause[i]) >= 0 {
		switch clause[i] {
		case 'u':
			who |= 0o700
			whoSpecial |= 0o4000
		case 'g':
			who |= 0o070
			whoSpecial |= 0o2000
		case 'o':
			who |= 0o007
			whoSpecial |= 0o1000
		case 'a':
			who |= 0o777
			whoSpecial |= 0o7000
		}
		i++
	}
	if who == 0 {
		who, whoSpecial = 0o777, 0o7000
	}
	if i >= len(clause) {
		return 0, fmt.Errorf("clause %q has no operator", clause)
	}
	// A clause can chain several operations: "u+r-w".
	for i < len(clause) {
		op := clause[i]
		if op != '+' && op != '-' && op != '=' {
			return 0, fmt.Errorf("clause %q: expected +, - or = at %q", clause, clause[i:])
		}
		i++
		start := i
		for i < len(clause) && clause[i] != '+' && clause[i] != '-' && clause[i] != '=' {
			i++
		}
		perms := clause[start:i]
		var bits uint32
		if len(perms) == 1 && strings.IndexByte("ugo", perms[0]) >= 0 {
			// Copy form: g=u copies the owner's rwx onto the group.
			var src uint32
			switch perms[0] {
			case 'u':
				src = (cur >> 6) & 7
			case 'g':
				src = (cur >> 3) & 7
			case 'o':
				src = cur & 7
			}
			bits = (src<<6 | src<<3 | src) & who
		} else {
			for _, c := range perms {
				switch c {
				case 'r':
					bits |= 0o444 & who
				case 'w':
					bits |= 0o222 & who
				case 'x':
					bits |= 0o111 & who
				case 'X':
					if isDir || cur&0o111 != 0 {
						bits |= 0o111 & who
					}
				case 's':
					bits |= whoSpecial & 0o6000
				case 't':
					// Sticky only makes sense for "others" or "all"; chmod
					// accepts +t with no who, which we treat as all.
					if whoSpecial&0o1000 != 0 {
						bits |= 0o1000
					}
				default:
					return 0, fmt.Errorf("clause %q: unknown permission %q", clause, c)
				}
			}
		}
		switch op {
		case '+':
			cur |= bits
		case '-':
			cur &^= bits
		case '=':
			// "=" resets the addressed rwx bits and their special bit, but a
			// folder keeps setuid/setgid as GNU chmod does, because those
			// bits on folders are usually inherited policy, not a choice.
			clear := who | (whoSpecial & 0o1000)
			if !isDir {
				clear |= whoSpecial & 0o6000
			}
			cur = cur&^clear | bits
		}
	}
	return cur, nil
}

// PermChange describes how the dialog wants permissions changed. Set bits are
// switched on, Clear bits switched off and all other bits stay as they are
// (the "grey" checkbox state for a mixed selection). Expr, when not empty, is
// a chmod expression applied after Set and Clear.
type PermChange struct {
	Set   fs.FileMode
	Clear fs.FileMode
	Expr  string
}

// PermExact returns a change that sets the permissions to exactly m.
func PermExact(m fs.FileMode) PermChange {
	return PermChange{Set: m & PermMask, Clear: PermMask &^ m}
}

// IsZero reports whether the change leaves permissions alone.
func (c PermChange) IsZero() bool {
	return c.Set&PermMask == 0 && c.Clear&PermMask == 0 && strings.TrimSpace(c.Expr) == ""
}

// Validate checks the change for contradictions and a bad expression.
func (c PermChange) Validate() error {
	if c.Set&c.Clear&PermMask != 0 {
		return fmt.Errorf("permission bits %s are both set and cleared", FormatPerm(c.Set&c.Clear))
	}
	if strings.TrimSpace(c.Expr) != "" {
		if _, err := ApplyChmod(0, c.Expr, false); err != nil {
			return err
		}
	}
	return nil
}

// Apply returns the permission part after the change.
func (c PermChange) Apply(m fs.FileMode, isDir bool) (fs.FileMode, error) {
	p := (m&PermMask)&^(c.Clear&PermMask) | (c.Set & PermMask)
	if strings.TrimSpace(c.Expr) == "" {
		return p, nil
	}
	return ApplyChmod(p, c.Expr, isDir)
}
