// This file sorts the rename list. In the dialog the counter follows the
// order of the list (F-519), so users click a column header to sort by name,
// extension, size or date before numbering, or drag rows by hand. Names are
// compared "naturally": runs of digits compare by value, so "img2" comes
// before "img10", and letters compare without regard to case, which is how
// Total Commander and the system file managers order files.

package multirename

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SortKey selects the column to sort by.
type SortKey string

const (
	SortByName SortKey = "name"
	SortByExt  SortKey = "ext"
	SortBySize SortKey = "size"
	SortByDate SortKey = "date"
	SortByPath SortKey = "path" // folder first, then name; for branch views
)

// SortItems sorts items in place. Ties are broken by natural name order so
// the result is stable and predictable.
func SortItems(items []Item, key SortKey, descending bool) {
	less := func(a, b *Item) int {
		switch key {
		case SortByExt:
			_, ea := SplitExt(a.Name)
			_, eb := SplitExt(b.Name)
			if c := NaturalCompare(ea, eb); c != 0 {
				return c
			}
		case SortBySize:
			if a.Size != b.Size {
				if a.Size < b.Size {
					return -1
				}
				return 1
			}
		case SortByDate:
			if !a.ModTime.Equal(b.ModTime) {
				if a.ModTime.Before(b.ModTime) {
					return -1
				}
				return 1
			}
		case SortByPath:
			if c := NaturalCompare(filepath.ToSlash(a.Dir), filepath.ToSlash(b.Dir)); c != 0 {
				return c
			}
		}
		return NaturalCompare(a.Name, b.Name)
	}
	sort.SliceStable(items, func(i, j int) bool {
		c := less(&items[i], &items[j])
		if descending {
			return c > 0
		}
		return c < 0
	})
}

// NaturalCompare compares two strings with numbers ordered by value and
// letters without regard to case. It returns -1, 0 or 1. Strings that are
// equal apart from case fall back to a plain comparison so the order is
// total.
func NaturalCompare(a, b string) int {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		if isDigit(ca) && isDigit(cb) {
			ja, jb := ia, ib
			for ja < len(a) && isDigit(a[ja]) {
				ja++
			}
			for jb < len(b) && isDigit(b[jb]) {
				jb++
			}
			na := strings.TrimLeft(a[ia:ja], "0")
			nb := strings.TrimLeft(b[ib:jb], "0")
			if len(na) != len(nb) {
				return sign(len(na) - len(nb))
			}
			if na != nb {
				return strings.Compare(na, nb)
			}
			ia, ib = ja, jb
			continue
		}
		ra, sa := utf8.DecodeRuneInString(a[ia:])
		rb, sb := utf8.DecodeRuneInString(b[ib:])
		la, lb := unicode.ToLower(ra), unicode.ToLower(rb)
		if la != lb {
			return sign(int(la) - int(lb))
		}
		ia += sa
		ib += sb
	}
	if c := sign((len(a) - ia) - (len(b) - ib)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}
