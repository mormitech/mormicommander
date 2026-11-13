// This file implements the case conversions of the Multi-Rename Tool. The
// dialog has an "Upper/lower case" drop-down that is applied to the whole new
// name after the masks and search-and-replace have run, with the same five
// choices as Total Commander: unchanged, all lower case, ALL UPPER CASE,
// First letter upper case, and First Letter Of Each Word Upper Case. The same
// word logic also drives the inline [F] placeholder. A "word" starts after
// any character that is not a letter or digit, except that an apostrophe
// does not break a word, so "don't stop" becomes "Don't Stop" and not
// "Don'T Stop".

package multirename

import (
	"strings"
	"unicode"
)

// CaseMode is the value of the case drop-down. It is stored as a string so
// presets saved as JSON remain readable and stable.
type CaseMode string

const (
	CaseUnchanged  CaseMode = ""      // leave the case alone
	CaseLower      CaseMode = "lower" // all lower case
	CaseUpper      CaseMode = "upper" // ALL UPPER CASE
	CaseFirstUpper CaseMode = "first" // First letter upper, the rest lower
	CaseEachWord   CaseMode = "words" // First Letter Of Each Word Upper
)

// ApplyCase converts s according to mode. Unknown modes leave s unchanged so
// a preset written by a newer version cannot break an older one.
func ApplyCase(s string, mode CaseMode) string {
	switch mode {
	case CaseLower:
		return strings.ToLower(s)
	case CaseUpper:
		return strings.ToUpper(s)
	case CaseFirstUpper:
		rs := []rune(strings.ToLower(s))
		for i, r := range rs {
			if unicode.IsLetter(r) {
				rs[i] = unicode.ToUpper(r)
				break
			}
		}
		return string(rs)
	case CaseEachWord:
		return titleWords(s, ' ')
	}
	return s
}

// titleWords upper-cases the first letter of each word and lower-cases the
// rest. prev is the character just before s, used to decide whether s begins
// inside a word.
func titleWords(s string, prev rune) string {
	var b strings.Builder
	b.Grow(len(s))
	inWord := isWordRune(prev)
	for _, r := range s {
		if isWordRune(r) {
			if inWord {
				b.WriteRune(unicode.ToLower(r))
			} else {
				b.WriteRune(unicode.ToUpper(r))
			}
			inWord = true
			continue
		}
		b.WriteRune(r)
		inWord = r == '\'' && inWord
	}
	return b.String()
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) }
