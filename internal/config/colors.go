// Colour schemes for the file panels (Configuration > Colors, F-417, and the
// per-file-type colours of F-435). Total Commander stores colours in the
// [Colors] section as Windows COLORREF numbers, that is decimal integers laid
// out as 0x00BBGGRR (blue in the high byte), with -1 meaning "use the system
// default". We keep that format on disk so a TC [Colors] section can be copied
// over unchanged, and convert to the #rrggbb form the web UI needs. File type
// colours are numbered pairs ColorFilter1=<mask>, ColorFilter1Color=<colour>,
// where the mask uses TC's mask syntax (see mask.go) and the first matching
// filter wins. Besides the active scheme in [Colors] there is an optional dark
// variant in [ColorsDark] (used when the OS is in dark mode) and any number of
// named user schemes in sections called "[ColorScheme <name>]", plus a few
// built-in presets.

package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Color is a COLORREF value (0x00BBGGRR); DefaultColor (-1) means unset.
type Color int32

// DefaultColor means "use the built-in/system colour".
const DefaultColor Color = -1

// RGB builds a Color from red, green and blue.
func RGB(r, g, b uint8) Color { return Color(int32(r) | int32(g)<<8 | int32(b)<<16) }

// IsDefault reports whether the colour is unset.
func (c Color) IsDefault() bool { return c < 0 }

// RGB returns the red, green and blue parts.
func (c Color) RGB() (r, g, b uint8) {
	return uint8(c), uint8(c >> 8), uint8(c >> 16)
}

// Hex returns "#rrggbb", or "" for DefaultColor.
func (c Color) Hex() string {
	if c.IsDefault() {
		return ""
	}
	r, g, b := c.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// INI returns the TC on-disk form (decimal COLORREF, -1 for default).
func (c Color) INI() string { return strconv.Itoa(int(c)) }

// ParseColor accepts "#rrggbb", "#rgb", "rgb(r,g,b)", a decimal COLORREF and
// "" or "-1" (default).
func ParseColor(s string) (Color, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "" || s == "-1":
		return DefaultColor, nil
	case strings.HasPrefix(s, "#"):
		h := s[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) != 6 {
			return DefaultColor, fmt.Errorf("config: bad colour %q", s)
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return DefaultColor, fmt.Errorf("config: bad colour %q", s)
		}
		return RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
	case strings.HasPrefix(strings.ToLower(s), "rgb(") && strings.HasSuffix(s, ")"):
		parts := strings.Split(s[4:len(s)-1], ",")
		if len(parts) != 3 {
			return DefaultColor, fmt.Errorf("config: bad colour %q", s)
		}
		var v [3]uint8
		for i, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 || n > 255 {
				return DefaultColor, fmt.Errorf("config: bad colour %q", s)
			}
			v[i] = uint8(n)
		}
		return RGB(v[0], v[1], v[2]), nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < -1 || n > 0xFFFFFF {
		return DefaultColor, fmt.Errorf("config: bad colour %q", s)
	}
	return Color(n), nil
}

// ColorFilter colours files that match Mask (TC "define colours by file
// type"). Mask may also be ">name" to refer to a saved search in TC; we keep
// such masks as text and never match them.
type ColorFilter struct {
	Mask  string
	Color Color
}

// ColorScheme is one complete set of panel colours.
type ColorScheme struct {
	Name             string
	Fore             Color // text
	Back             Color // background
	Back2            Color // alternate (every second line) background
	Mark             Color // selected (marked) files
	Cursor           Color // cursor bar
	CursorText       Color // text under the cursor
	InactiveCursor   Color // cursor bar in the inactive panel
	InactiveMark     Color // marked files in the inactive panel
	InactiveBack     Color // background of the inactive panel (ours)
	InverseCursor    bool  // draw the cursor as an inverted bar
	InverseSelection bool  // draw selection by inverting
	Filters          []ColorFilter
}

// FileColor returns the text colour for a file by the first matching filter,
// or s.Fore when none matches.
func (s ColorScheme) FileColor(name, fullPath string, isDir bool) Color {
	for _, f := range s.Filters {
		if strings.HasPrefix(f.Mask, ">") {
			continue
		}
		if ParseMask(f.Mask).Match(name, fullPath, isDir) {
			return f.Color
		}
	}
	return s.Fore
}

var colorKeys = []struct {
	key string
	get func(*ColorScheme) *Color
}{
	{"ForeColor", func(s *ColorScheme) *Color { return &s.Fore }},
	{"BackColor", func(s *ColorScheme) *Color { return &s.Back }},
	{"BackColor2", func(s *ColorScheme) *Color { return &s.Back2 }},
	{"MarkColor", func(s *ColorScheme) *Color { return &s.Mark }},
	{"CursorColor", func(s *ColorScheme) *Color { return &s.Cursor }},
	{"CursorText", func(s *ColorScheme) *Color { return &s.CursorText }},
	{"InactiveCursorColor", func(s *ColorScheme) *Color { return &s.InactiveCursor }},
	{"InactiveMarkColor", func(s *ColorScheme) *Color { return &s.InactiveMark }},
	{"InactiveBackColor", func(s *ColorScheme) *Color { return &s.InactiveBack }},
}

// LoadColorScheme reads a scheme from a section. Missing colours stay
// DefaultColor. Filters are read from ColorFilterN/ColorFilterNColor until
// the first missing number.
func LoadColorScheme(f *File, section string) ColorScheme {
	s := ColorScheme{Name: section}
	m := f.Map(section)
	for _, k := range colorKeys {
		c := DefaultColor
		if v, ok := m[strings.ToLower(k.key)]; ok {
			if pc, err := ParseColor(v); err == nil {
				c = pc
			}
		}
		*k.get(&s) = c
	}
	s.InverseCursor = m["inversecursor"] == "1"
	s.InverseSelection = m["inverseselection"] == "1"
	for i := 1; ; i++ {
		mask, ok := m[fmt.Sprintf("colorfilter%d", i)]
		if !ok {
			break
		}
		c, err := ParseColor(m[fmt.Sprintf("colorfilter%dcolor", i)])
		if err != nil {
			c = DefaultColor
		}
		s.Filters = append(s.Filters, ColorFilter{Mask: mask, Color: c})
	}
	return s
}

// SaveColorScheme writes a scheme into a section, replacing its old keys
// (other keys of that section are kept, comments too).
func SaveColorScheme(f *File, section string, s ColorScheme) {
	var kvs []KV
	known := map[string]bool{"inversecursor": true, "inverseselection": true}
	for _, k := range colorKeys {
		known[strings.ToLower(k.key)] = true
		if c := *k.get(&s); !c.IsDefault() {
			kvs = append(kvs, KV{k.key, c.INI()})
		}
	}
	if s.InverseCursor {
		kvs = append(kvs, KV{"InverseCursor", "1"})
	}
	if s.InverseSelection {
		kvs = append(kvs, KV{"InverseSelection", "1"})
	}
	for i, flt := range s.Filters {
		kvs = append(kvs, KV{fmt.Sprintf("ColorFilter%d", i+1), flt.Mask})
		kvs = append(kvs, KV{fmt.Sprintf("ColorFilter%dColor", i+1), flt.Color.INI()})
	}
	// Keep unknown keys (e.g. TC options we don't model) after ours.
	for _, kv := range f.Entries(section) {
		low := strings.ToLower(kv.Key)
		if known[low] || strings.HasPrefix(low, "colorfilter") {
			continue
		}
		kvs = append(kvs, kv)
	}
	f.ReplaceSection(section, kvs)
}

// Section names.
const (
	SectionColors     = "Colors"
	SectionColorsDark = "ColorsDark"
	colorSchemePrefix = "ColorScheme "
)

// NamedSchemes lists the user's saved schemes ("[ColorScheme <name>]"),
// sorted by name.
func NamedSchemes(f *File) []ColorScheme {
	var out []ColorScheme
	for _, sec := range f.Sections() {
		if len(sec) > len(colorSchemePrefix) && strings.EqualFold(sec[:len(colorSchemePrefix)], colorSchemePrefix) {
			s := LoadColorScheme(f, sec)
			s.Name = sec[len(colorSchemePrefix):]
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// SaveNamedScheme stores a scheme under its name.
func SaveNamedScheme(f *File, s ColorScheme) { SaveColorScheme(f, colorSchemePrefix+s.Name, s) }

// DeleteNamedScheme removes a saved scheme.
func DeleteNamedScheme(f *File, name string) bool { return f.DeleteSection(colorSchemePrefix + name) }

// BuiltinSchemes returns the presets offered on the Colors page.
func BuiltinSchemes() []ColorScheme {
	return []ColorScheme{
		{
			Name: "Default", Fore: RGB(0, 0, 0), Back: RGB(255, 255, 255), Back2: DefaultColor,
			Mark: RGB(255, 0, 0), Cursor: RGB(0, 120, 215), CursorText: RGB(255, 255, 255),
			InactiveCursor: RGB(204, 204, 204), InactiveMark: DefaultColor, InactiveBack: DefaultColor,
		},
		{
			Name: "Dark", Fore: RGB(220, 220, 220), Back: RGB(30, 30, 30), Back2: RGB(38, 38, 38),
			Mark: RGB(255, 110, 100), Cursor: RGB(38, 79, 120), CursorText: RGB(255, 255, 255),
			InactiveCursor: RGB(60, 60, 60), InactiveMark: DefaultColor, InactiveBack: DefaultColor,
		},
		{
			Name: "Classic blue", Fore: RGB(0, 255, 255), Back: RGB(0, 0, 128), Back2: DefaultColor,
			Mark: RGB(255, 255, 0), Cursor: RGB(0, 128, 128), CursorText: RGB(0, 0, 0),
			InactiveCursor: RGB(0, 64, 96), InactiveMark: DefaultColor, InactiveBack: DefaultColor,
		},
	}
}
