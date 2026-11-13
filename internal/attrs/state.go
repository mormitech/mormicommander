// Initial checkbox states for the attributes dialog. When several files are
// selected TC shows each checkbox ticked if the attribute is set on all of
// them, empty if it is set on none, and grey (third state) if they differ; a
// grey box that the user leaves grey means "do not touch". Summarize computes
// those states for permissions, Windows attributes and the hidden flag, plus
// the common owner, group and dates, and the *FromTri helpers turn the
// dialog's final checkbox states back into a change request.
package attrs

import (
	"io/fs"
	"time"
)

// Tri is a three-state checkbox value.
type Tri uint8

const (
	Off Tri = iota
	On
	Mixed
)

// String returns "off", "on" or "mixed".
func (t Tri) String() string {
	switch t {
	case On:
		return "on"
	case Mixed:
		return "mixed"
	default:
		return "off"
	}
}

// Selection summarises several entries for the dialog.
type Selection struct {
	Count int

	permAll, permAny fs.FileMode
	winAll, winAny   WinAttr
	hidAll, hidAny   bool

	Owner, Group           string // empty when the entries differ
	OwnerMixed, GroupMixed bool

	// Common dates; zero with the *Mixed flag set when the entries differ.
	Modified, Accessed, Created                time.Time
	ModifiedMixed, AccessedMixed, CreatedMixed bool

	Files, Folders, Links int
}

// Summarize builds the dialog state for a selection.
func Summarize(infos []Info) Selection {
	s := Selection{Count: len(infos)}
	for i, in := range infos {
		switch in.Kind {
		case KindDir:
			s.Folders++
		case KindSymlink:
			s.Links++
		default:
			s.Files++
		}
		if i == 0 {
			s.permAll, s.permAny = in.Perm(), in.Perm()
			s.winAll, s.winAny = in.Win, in.Win
			s.hidAll, s.hidAny = in.HiddenFlag, in.HiddenFlag
			s.Owner, s.Group = in.Owner, in.Group
			s.Modified, s.Accessed, s.Created = in.Modified, in.Accessed, in.Created
			continue
		}
		s.permAll &= in.Perm()
		s.permAny |= in.Perm()
		s.winAll &= in.Win
		s.winAny |= in.Win
		s.hidAll = s.hidAll && in.HiddenFlag
		s.hidAny = s.hidAny || in.HiddenFlag
		if in.Owner != s.Owner && !s.OwnerMixed {
			s.Owner, s.OwnerMixed = "", true
		}
		if in.Group != s.Group && !s.GroupMixed {
			s.Group, s.GroupMixed = "", true
		}
		mix := func(t *time.Time, mixed *bool, v time.Time) {
			if !*mixed && !t.Equal(v) {
				*t, *mixed = time.Time{}, true
			}
		}
		mix(&s.Modified, &s.ModifiedMixed, in.Modified)
		mix(&s.Accessed, &s.AccessedMixed, in.Accessed)
		mix(&s.Created, &s.CreatedMixed, in.Created)
	}
	return s
}

func tri(all, any bool) Tri {
	switch {
	case all:
		return On
	case !any:
		return Off
	default:
		return Mixed
	}
}

// PermBit returns the checkbox state of one permission bit.
func (s Selection) PermBit(bit fs.FileMode) Tri {
	if s.Count == 0 {
		return Off
	}
	return tri(s.permAll&bit == bit, s.permAny&bit != 0)
}

// Attr returns the checkbox state of one Windows attribute.
func (s Selection) Attr(a WinAttr) Tri {
	if s.Count == 0 {
		return Off
	}
	return tri(s.winAll&a == a, s.winAny&a != 0)
}

// HiddenFlag returns the checkbox state of the macOS hidden flag.
func (s Selection) HiddenFlag() Tri {
	if s.Count == 0 {
		return Off
	}
	return tri(s.hidAll, s.hidAny)
}

// CommonPerm returns the permissions when every bit is either on or off for
// the whole selection, so the octal field can show them; ok is false when
// some bit is mixed.
func (s Selection) CommonPerm() (fs.FileMode, bool) {
	return s.permAll, s.Count > 0 && s.permAll == s.permAny
}

// PermChangeFromTri turns the final checkbox states into a change: On sets,
// Off clears and Mixed (or a missing bit) leaves the bit alone.
func PermChangeFromTri(bits map[fs.FileMode]Tri) PermChange {
	var c PermChange
	for b, t := range bits {
		switch t {
		case On:
			c.Set |= b & PermMask
		case Off:
			c.Clear |= b & PermMask
		}
	}
	return c
}

// WinAttrChangeFromTri does the same for Windows attributes.
func WinAttrChangeFromTri(attrs map[WinAttr]Tri) WinAttrChange {
	var c WinAttrChange
	for a, t := range attrs {
		switch t {
		case On:
			c.Set |= a
		case Off:
			c.Clear |= a
		}
	}
	return c
}

// ChangedOnly keeps only the checkboxes the user actually changed compared
// with the initial selection state, so that pressing OK without touching a
// box never rewrites it. This matters for recursive changes, where files deep
// in the tree may differ from the selected entries the boxes were built from.
func ChangedOnly[K comparable](initial, final map[K]Tri) map[K]Tri {
	out := map[K]Tri{}
	for k, v := range final {
		if iv, ok := initial[k]; !ok || iv != v {
			out[k] = v
		}
	}
	return out
}
