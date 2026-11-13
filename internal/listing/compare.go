// This file holds the name comparison rules a panel sorts with. Total Commander offers several: a plain string sort,
// Windows "word sort" (hyphens and apostrophes are ignored, so "co-op" sits next to "coop"), natural sort (numbers
// compared as numbers, so "file2" comes before "file10"), case-sensitive or case-insensitive order, and the system
// locale's collation (so "Äpfel" sorts with "Apfel" in German). Collator packs these choices into one comparison
// function so the sorter and the custom-column sorter agree on what "by name" means.

package listing

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Collation describes how names are compared.
type Collation struct {
	Natural       bool   // compare runs of digits by numeric value ("file2" < "file10")
	CaseSensitive bool   // "B" and "b" differ; upper case sorts before lower case of the same letter, as in TC
	WordSort      bool   // ignore '-' and '\'' like Windows CompareString without SORT_STRINGSORT
	Locale        string // BCP 47 tag ("de", "sv", "en-US") for linguistic order; empty means code point order
}

// Collator compares strings according to a Collation. The zero value is a case-insensitive code point compare. A
// Collator is safe for concurrent use.
type Collator struct {
	c    Collation
	once sync.Once
	mu   sync.Mutex
	col  *collate.Collator
}

// NewCollator returns a Collator for c.
func NewCollator(c Collation) *Collator { return &Collator{c: c} }

// Collation returns the settings the collator was built with.
func (k *Collator) Collation() Collation { return k.c }

func (k *Collator) locale() *collate.Collator {
	k.once.Do(func() {
		if k.c.Locale == "" {
			return
		}
		tag, err := language.Parse(k.c.Locale)
		if err != nil {
			return
		}
		var opts []collate.Option
		if !k.c.CaseSensitive {
			opts = append(opts, collate.IgnoreCase)
		}
		if k.c.Natural {
			opts = append(opts, collate.Numeric)
		}
		k.col = collate.New(tag, opts...)
	})
	return k.col
}

// Compare returns -1, 0 or +1 as a sorts before, equal to or after b.
func (k *Collator) Compare(a, b string) int {
	if k.c.WordSort {
		a, b = stripWordChars(a), stripWordChars(b)
	}
	if col := k.locale(); col != nil {
		// collate.Collator keeps internal buffers and is not safe for concurrent use.
		k.mu.Lock()
		r := col.CompareString(a, b)
		k.mu.Unlock()
		if r != 0 || !k.c.CaseSensitive {
			return r
		}
		return strings.Compare(a, b)
	}
	if k.c.Natural {
		return naturalCompare(a, b, k.c.CaseSensitive)
	}
	return runeCompare(a, b, k.c.CaseSensitive)
}

// stripWordChars drops the characters Windows word sort ignores. Only allocates when one is present.
func stripWordChars(s string) string {
	if !strings.ContainsAny(s, "-'") {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '\'' {
			return -1
		}
		return r
	}, s)
}

// foldRune maps a rune to its case-insensitive comparison key.
func foldRune(r rune) rune { return unicode.ToLower(unicode.ToUpper(r)) }

// caseRank orders letters that are equal ignoring case: TC (and Windows) put upper case first.
func caseRank(r rune) int {
	if unicode.IsUpper(r) {
		return 0
	}
	return 1
}

// runeCompare compares two strings rune by rune. When case-sensitive, letters still sort alphabetically first
// ("apple" < "Banana"), and case only breaks ties ("Apple" < "apple"), which is how TC orders names; a raw byte
// compare would put every capital before every lower-case letter.
func runeCompare(a, b string, caseSensitive bool) int {
	tie := 0
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		a, b = a[na:], b[nb:]
		if ra == rb {
			continue
		}
		fa, fb := foldRune(ra), foldRune(rb)
		if fa != fb {
			if fa < fb {
				return -1
			}
			return 1
		}
		if tie == 0 && caseSensitive {
			tie = caseRank(ra) - caseRank(rb)
			if tie == 0 {
				tie = cmpInt(int64(ra), int64(rb))
			}
		}
	}
	switch {
	case a == "" && b != "":
		return -1
	case a != "" && b == "":
		return 1
	}
	return sign(tie)
}

// naturalCompare compares with digit runs treated as numbers. Leading zeros do not change the value ("007" == "7"),
// but when everything else is equal the run with fewer leading zeros sorts first so the order stays total.
func naturalCompare(a, b string, caseSensitive bool) int {
	tie := 0
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if isDigit(ra) && isDigit(rb) {
			da, restA := digitRun(a)
			db, restB := digitRun(b)
			a, b = restA, restB
			ta, tb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if len(ta) != len(tb) {
				return cmpInt(int64(len(ta)), int64(len(tb)))
			}
			if c := strings.Compare(ta, tb); c != 0 {
				return c
			}
			if tie == 0 {
				tie = cmpInt(int64(len(da)), int64(len(db)))
			}
			continue
		}
		a, b = a[na:], b[nb:]
		if ra == rb {
			continue
		}
		fa, fb := foldRune(ra), foldRune(rb)
		if fa != fb {
			if fa < fb {
				return -1
			}
			return 1
		}
		if tie == 0 && caseSensitive {
			tie = caseRank(ra) - caseRank(rb)
		}
	}
	switch {
	case a == "" && b != "":
		return -1
	case a != "" && b == "":
		return 1
	}
	return sign(tie)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func digitRun(s string) (run, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
