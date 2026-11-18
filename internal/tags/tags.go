// Package tags reads and writes the file tags the system's own file manager shows (issue #28, F-869): on macOS the
// Finder tags, kept in the extended attribute com.apple.metadata:_kMDItemUserTags as a binary property list of
// "name\ncolour" strings (plus the old colour label bits in com.apple.FinderInfo, which Finder keeps in step); on
// Linux the user.xdg.tags attribute, a comma-separated list that Dolphin, Baloo and other freedesktop tools use.
// Windows has no file tags, so Supported says false there and nothing is read or written.
//
// The tags live in the file itself (in its extended attributes), so a copy that keeps extended attributes keeps them
// (internal/fileops does), a move on one disk keeps them anyway, and a disk without extended attributes (FAT, exFAT)
// has none. Links are never followed: a link's own tags are used.
//
// The package has no UI; feature_tags.go exposes it to the page and internal/undo uses it to take a change back.
package tags

import (
	"bytes"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"mormicommander/internal/xattr"
)

// Tag is one tag: its name and its Finder colour (0 none, 1 grey, 2 green, 3 purple, 4 blue, 5 yellow, 6 red,
// 7 orange). Linux tags have no colour.
type Tag struct {
	Name  string `json:"name"`
	Color int    `json:"color"`
}

// Attribute names.
const (
	FinderAttr = "com.apple.metadata:_kMDItemUserTags"
	FinderInfo = "com.apple.FinderInfo"
	XDGAttr    = "user.xdg.tags"
)

// ColorNames are Finder's standard tags, by colour; a tag with one of these names gets that colour by default.
var ColorNames = [8]string{"", "Gray", "Green", "Purple", "Blue", "Yellow", "Red", "Orange"}

// Style says how tags are stored: "finder", "xdg" or "" (none).
type Style string

const (
	Finder Style = "finder"
	XDG    Style = "xdg"
	None   Style = ""
)

// System is how this system stores tags.
func System() Style {
	if !xattr.Supported() {
		return None
	}
	switch runtime.GOOS {
	case "darwin":
		return Finder
	case "linux":
		return XDG
	}
	return None
}

// Supported says whether this system has file tags.
func Supported() bool { return System() != None }

// HasColors says whether tags of this style have colours.
func (s Style) HasColors() bool { return s == Finder }

// DefaultColor is the colour a new tag of this name gets: the standard colour tags keep their colour (Finder does
// the same), any other name gets none.
func DefaultColor(name string) int {
	for i, n := range ColorNames {
		if i > 0 && strings.EqualFold(n, name) {
			return i
		}
	}
	return 0
}

// Clean tidies a tag name for a style: line breaks become spaces (Finder keeps the colour after one), and for
// user.xdg.tags commas too (they separate the tags there); spaces at the ends go. "" means the name is unusable.
func Clean(s Style, name string) string {
	name = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(name)
	if s == XDG {
		name = strings.ReplaceAll(name, ",", " ")
	}
	name = strings.TrimSpace(name)
	if len(name) > 255 {
		name = name[:255]
		for len(name) > 0 && !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}
	return name
}

// Read returns the tags of one file or folder (none when it has none, or the system has no tags).
func Read(path string) ([]Tag, error) { return ReadStyle(System(), path) }

// ReadStyle reads the tags as stored in one style (the tests read both styles on one system).
func ReadStyle(s Style, path string) ([]Tag, error) {
	switch s {
	case Finder:
		v, ok, err := xattr.Get(path, FinderAttr)
		if err != nil {
			return nil, err
		}
		if ok {
			return parseFinder(v), nil
		}
		// an old file with a colour label but no tag list: Finder shows the label as its colour's tag
		if fi, ok, _ := xattr.Get(path, FinderInfo); ok && len(fi) >= 10 {
			if c := int(fi[9]>>1) & 7; c > 0 {
				return []Tag{{Name: ColorNames[c], Color: c}}, nil
			}
		}
		return nil, nil
	case XDG:
		v, ok, err := xattr.Get(path, XDGAttr)
		if err != nil || !ok {
			return nil, err
		}
		return parseXDG(v), nil
	}
	return nil, nil
}

func parseFinder(v []byte) []Tag {
	list, err := decodeStrings(v)
	if err != nil {
		return nil
	}
	out := make([]Tag, 0, len(list))
	for _, s := range list {
		name, color := s, 0
		if i := strings.LastIndexByte(s, '\n'); i >= 0 {
			name = s[:i]
			if c, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil && c >= 0 && c < 8 {
				color = c
			}
		}
		if name != "" {
			out = append(out, Tag{Name: name, Color: color})
		}
	}
	return out
}

func parseXDG(v []byte) []Tag {
	var out []Tag
	for _, s := range strings.Split(string(bytes.TrimRight(v, "\x00")), ",") {
		if s = strings.TrimSpace(s); s != "" && !Contains(out, s) {
			out = append(out, Tag{Name: s})
		}
	}
	return out
}

// Write sets the tags of one file or folder to list (an empty list removes them).
func Write(path string, list []Tag) error { return WriteStyle(System(), path, list) }

// WriteStyle writes the tags in one style.
func WriteStyle(s Style, path string, list []Tag) error {
	switch s {
	case Finder:
		strs := make([]string, 0, len(list))
		label := 0
		for _, t := range list {
			if t.Color > 0 && t.Color < 8 {
				strs = append(strs, t.Name+"\n"+strconv.Itoa(t.Color))
				label = t.Color
			} else {
				strs = append(strs, t.Name)
			}
		}
		if len(strs) == 0 {
			if _, err := xattr.Remove(path, FinderAttr); err != nil {
				return err
			}
		} else if err := xattr.Set(path, FinderAttr, encodeStrings(strs)); err != nil {
			return err
		}
		return setLabel(path, label)
	case XDG:
		if len(list) == 0 {
			_, err := xattr.Remove(path, XDGAttr)
			return err
		}
		names := make([]string, len(list))
		for i, t := range list {
			names[i] = t.Name
		}
		return xattr.Set(path, XDGAttr, []byte(strings.Join(names, ",")))
	}
	return errNoTags
}

// setLabel keeps the old colour label bits of com.apple.FinderInfo in step with the tags (macOS does the same): the
// colour of the last coloured tag, or none. The other 31 bytes are left as they are; an attribute that ends up all
// zeros is removed, and none is made just to say "no colour".
func setLabel(path string, color int) error {
	fi, ok, err := xattr.Get(path, FinderInfo)
	if err != nil {
		return nil // can't read it: leave it alone, the tag list is what counts
	}
	if !ok || len(fi) < 32 {
		if color == 0 {
			return nil
		}
		fi = make([]byte, 32)
	}
	fi = append([]byte(nil), fi...)
	want := fi[9]&^0x0E | byte(color&7)<<1
	if ok && want == fi[9] {
		return nil
	}
	fi[9] = want
	if bytes.Count(fi, []byte{0}) == len(fi) {
		_, err := xattr.Remove(path, FinderInfo)
		return err
	}
	return xattr.Set(path, FinderInfo, fi)
}

type tagsError string

func (e tagsError) Error() string { return string(e) }

const errNoTags = tagsError("This system has no file tags.")

// Contains says whether the list has a tag of this name (names are compared without regard to case, as Finder does).
func Contains(list []Tag, name string) bool { return index(list, name) >= 0 }

func index(list []Tag, name string) int {
	for i, t := range list {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

// Change returns list with the tags of add added (a name already there keeps its place and takes add's colour when
// add gives one) and the names of remove taken away, and whether anything changed.
func Change(list []Tag, add []Tag, remove []string) ([]Tag, bool) {
	out := append([]Tag(nil), list...)
	changed := false
	for _, n := range remove {
		if i := index(out, n); i >= 0 {
			out = append(out[:i], out[i+1:]...)
			changed = true
		}
	}
	for _, t := range add {
		if t.Name == "" {
			continue
		}
		if i := index(out, t.Name); i >= 0 {
			if t.Color > 0 && out[i].Color != t.Color {
				out[i].Color = t.Color
				changed = true
			}
			continue
		}
		out = append(out, t)
		changed = true
	}
	return out, changed
}

// Equal says whether two lists hold the same tags in the same order.
func Equal(a, b []Tag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
