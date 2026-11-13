// Windows file attributes. Total Commander's attributes dialog has four
// checkboxes, Read-only, Archive, Hidden and System, and its attribute column
// shows them as the letters r, a, h and s ("-a--"). NTFS also reports
// compressed and encrypted files, which TC shows but which are switched with
// separate tools, so this package reports them and refuses to set them. The
// bit values deliberately equal the Win32 FILE_ATTRIBUTE_* constants so the
// Windows layer can pass them straight through. On macOS and Linux the same
// type is filled in from the permissions and the hidden state, so filters and
// columns written with Windows words still mean something there.
package attrs

import (
	"fmt"
	"strings"
)

// WinAttr is a set of Windows file attribute bits.
type WinAttr uint32

const (
	AttrReadOnly     WinAttr = 0x1
	AttrHidden       WinAttr = 0x2
	AttrSystem       WinAttr = 0x4
	AttrDirectory    WinAttr = 0x10
	AttrArchive      WinAttr = 0x20
	AttrNormal       WinAttr = 0x80
	AttrTemporary    WinAttr = 0x100
	AttrSparse       WinAttr = 0x200
	AttrReparsePoint WinAttr = 0x400
	AttrCompressed   WinAttr = 0x800
	AttrOffline      WinAttr = 0x1000
	AttrNotIndexed   WinAttr = 0x2000
	AttrEncrypted    WinAttr = 0x4000
)

// DialogAttrs are the attributes the dialog shows as checkboxes, in TC's
// order: read-only, archive, hidden, system, then the two reporting-only ones.
var DialogAttrs = []WinAttr{AttrReadOnly, AttrArchive, AttrHidden, AttrSystem, AttrCompressed, AttrEncrypted}

// letters maps each column letter to its bit, in column order.
var letters = []struct {
	c byte
	a WinAttr
}{
	{'r', AttrReadOnly}, {'a', AttrArchive}, {'h', AttrHidden}, {'s', AttrSystem},
	{'c', AttrCompressed}, {'e', AttrEncrypted},
}

// Has reports whether every bit of b is set.
func (a WinAttr) Has(b WinAttr) bool { return a&b == b && b != 0 }

// String returns TC's four-letter column form, e.g. "-a--" or "r-hs".
func (a WinAttr) String() string { return a.format(4) }

// LongString adds the compressed and encrypted positions: "-a--c-".
func (a WinAttr) LongString() string { return a.format(6) }

func (a WinAttr) format(n int) string {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		if a&letters[i].a != 0 {
			b[i] = letters[i].c
		} else {
			b[i] = '-'
		}
	}
	return string(b)
}

// Names returns the plain-English names of the set bits, for tooltips and
// the change preview.
func (a WinAttr) Names() []string {
	all := []struct {
		a WinAttr
		n string
	}{
		{AttrReadOnly, "read-only"}, {AttrArchive, "archive"}, {AttrHidden, "hidden"},
		{AttrSystem, "system"}, {AttrCompressed, "compressed"}, {AttrEncrypted, "encrypted"},
		{AttrDirectory, "folder"}, {AttrTemporary, "temporary"}, {AttrSparse, "sparse"},
		{AttrReparsePoint, "reparse point"}, {AttrOffline, "offline"}, {AttrNotIndexed, "not indexed"},
	}
	var out []string
	for _, x := range all {
		if a&x.a != 0 {
			out = append(out, x.n)
		}
	}
	return out
}

// ParseWinAttr parses the column form back into bits. Letters may come in any
// order and dashes are ignored, so "ra", "r-a-" and "-a-r" are all the same.
func ParseWinAttr(s string) (WinAttr, error) {
	var a WinAttr
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20 // lower case
		if s[i] == '-' || s[i] == ' ' {
			continue
		}
		found := false
		for _, l := range letters {
			if l.c == c {
				a |= l.a
				found = true
			}
		}
		if !found {
			return 0, fmt.Errorf("attributes %q: unknown letter %q", s, s[i])
		}
	}
	return a, nil
}

// WinAttrChange says which attributes to switch on (Set) and off (Clear);
// all others are left alone, like a grey checkbox.
type WinAttrChange struct {
	Set   WinAttr
	Clear WinAttr
}

// IsZero reports whether the change touches nothing.
func (c WinAttrChange) IsZero() bool { return c.Set == 0 && c.Clear == 0 }

// Apply returns the attributes after the change.
func (c WinAttrChange) Apply(a WinAttr) WinAttr { return a&^c.Clear | c.Set }

// ParseWinAttrChange parses the attrib-style notation "+r -h +a" (spaces
// optional: "+r-h+a"). A letter without a sign before it is an error, so a
// typo never silently clears something.
func ParseWinAttrChange(s string) (WinAttrChange, error) {
	var c WinAttrChange
	sign := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == ' ' || ch == ',':
		case ch == '+' || ch == '-':
			sign = ch
		default:
			a, err := ParseWinAttr(string(ch))
			if err != nil || a == 0 {
				return WinAttrChange{}, fmt.Errorf("attribute change %q: unknown letter %q", s, ch)
			}
			if sign == 0 {
				return WinAttrChange{}, fmt.Errorf("attribute change %q: %q needs + or - before it", s, ch)
			}
			if sign == '+' {
				c.Set |= a
				c.Clear &^= a
			} else {
				c.Clear |= a
				c.Set &^= a
			}
		}
	}
	return c, nil
}

// String renders the change in the same notation ParseWinAttrChange reads.
func (c WinAttrChange) String() string {
	var b strings.Builder
	for _, l := range letters {
		if c.Set&l.a != 0 {
			b.WriteByte('+')
			b.WriteByte(l.c)
		}
		if c.Clear&l.a != 0 {
			b.WriteByte('-')
			b.WriteByte(l.c)
		}
	}
	return b.String()
}
