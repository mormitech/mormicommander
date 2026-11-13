// Display width of characters. The Lister lays text out on a grid of
// fixed-width cells (the fixed font of Total Commander), so line wrapping and
// tab stops need to know how many cells a character takes: Chinese, Japanese
// and Korean ideographs and most emoji take two, combining accents take none,
// everything else one. This is a compact version of the Unicode East Asian
// Width rules, good enough for wrapping; the UI's font decides the final look.

package lister

import "unicode"

var wideRanges = [][2]rune{
	{0x1100, 0x115F}, {0x231A, 0x231B}, {0x2329, 0x232A}, {0x23E9, 0x23EC},
	{0x2E80, 0x303E}, {0x3041, 0x33FF}, {0x3400, 0x4DBF}, {0x4E00, 0x9FFF},
	{0xA000, 0xA4CF}, {0xA960, 0xA97F}, {0xAC00, 0xD7A3}, {0xF900, 0xFAFF},
	{0xFE10, 0xFE19}, {0xFE30, 0xFE6F}, {0xFF00, 0xFF60}, {0xFFE0, 0xFFE6},
	{0x1F300, 0x1F64F}, {0x1F900, 0x1F9FF}, {0x20000, 0x2FFFD}, {0x30000, 0x3FFFD},
}

// RuneWidth returns the number of fixed-width cells r occupies (0, 1 or 2).
func RuneWidth(r rune) int {
	if r < 0x300 {
		return 1
	}
	if unicode.In(r, unicode.Mn, unicode.Me) || r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF {
		return 0
	}
	lo, hi := 0, len(wideRanges)
	for lo < hi {
		m := (lo + hi) / 2
		switch {
		case r < wideRanges[m][0]:
			hi = m
		case r > wideRanges[m][1]:
			lo = m + 1
		default:
			return 2
		}
	}
	return 1
}

// StringWidth returns the cell width of s (tabs count as one cell here).
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}
