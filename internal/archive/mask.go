// This file implements Total Commander's file mask syntax, used by the Unpack
// dialog ("Unpack specific files") and by the Pack dialog's exclude list. A
// mask is a list of wildcard patterns separated by spaces or semicolons, where
// "*" matches any run of characters and "?" a single one, matching is
// case-insensitive, and everything after a "|" is an exclude list, so
// "*.txt *.doc | old*" means text and Word files except those starting with
// "old". "*.*" matches every name, including names without a dot, exactly as
// in Total Commander. Patterns may be quoted to include spaces.
package archive

import (
	"strings"
	"unicode/utf8"
)

// Mask is a parsed file mask. The zero value matches everything.
type Mask struct {
	include []string
	exclude []string
}

// ParseMask parses a Total Commander style mask. An empty mask matches all
// names.
func ParseMask(s string) Mask {
	var m Mask
	inc, exc, _ := strings.Cut(s, "|")
	m.include = splitPatterns(inc)
	m.exclude = splitPatterns(exc)
	return m
}

func splitPatterns(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && (r == ' ' || r == ';' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// IsEmpty reports whether the mask has no include and no exclude patterns.
func (m Mask) IsEmpty() bool { return len(m.include) == 0 && len(m.exclude) == 0 }

// Match reports whether a base file name passes the mask.
func (m Mask) Match(name string) bool {
	name = strings.ToLower(name)
	ok := len(m.include) == 0
	for _, p := range m.include {
		if wildMatch(p, name) {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	for _, p := range m.exclude {
		if wildMatch(p, name) {
			return false
		}
	}
	return true
}

// wildMatch matches name against a pattern of literal runes, "*" and "?".
// It uses the classic greedy algorithm with one backtrack point, which runs in
// linear time for practical patterns and never recurses.
func wildMatch(pattern, name string) bool {
	if pattern == "*.*" || pattern == "*" {
		return true
	}
	p, n := 0, 0
	starP, starN := -1, 0
	for n < len(name) {
		if p < len(pattern) {
			pr, psz := utf8.DecodeRuneInString(pattern[p:])
			nr, nsz := utf8.DecodeRuneInString(name[n:])
			switch {
			case pr == '*':
				starP, starN = p, n
				p += psz
				continue
			case pr == '?' || pr == nr:
				p += psz
				n += nsz
				continue
			}
		}
		if starP < 0 {
			return false
		}
		// Let the last star swallow one more rune and retry.
		_, nsz := utf8.DecodeRuneInString(name[starN:])
		starN += nsz
		n = starN
		p = starP + 1
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	// A trailing ".*" also matches names without an extension ("readme"
	// matches "readme.*"), as in Total Commander.
	if strings.HasSuffix(pattern[p:], ".*") && strings.Trim(pattern[p:], ".*") == "" {
		return true
	}
	return p == len(pattern)
}
