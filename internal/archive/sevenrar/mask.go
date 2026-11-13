// This file implements Total Commander's file masks for the unpack dialog
// (Alt+F9, "Unpack specific files"). A mask is a list of wildcard patterns
// separated by spaces or semicolons, such as "*.txt *.doc", optionally
// followed by "|" and patterns to exclude: "*.* | *.bak *.tmp". Matching is
// case-insensitive and only looks at the file name, never the folder. As in
// Total Commander, "*.*" also matches names without a dot, "*" matches any
// run of characters and "?" exactly one character; there are no character
// classes, so "[" is an ordinary character.

package sevenrar

import (
	"strings"
	"unicode/utf8"
)

// Mask is a compiled Total Commander file mask. The zero value matches
// everything.
type Mask struct {
	include []string
	exclude []string
}

// ParseMask compiles a mask string. An empty mask matches everything.
func ParseMask(s string) Mask {
	inc, exc, _ := strings.Cut(s, "|")
	return Mask{include: splitPatterns(inc), exclude: splitPatterns(exc)}
}

// splitPatterns splits on spaces, tabs and semicolons, keeping text inside
// double quotes together so "my file.txt" is one pattern.
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

// Match reports whether the file name (not a path) is selected by the mask.
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

// wildMatch matches name against a pattern of literal characters, "*" and
// "?". "*.*" is special-cased to match everything, like in Total Commander.
func wildMatch(p, name string) bool {
	if p == "*.*" || p == "*" {
		return true
	}
	// Iterative matcher with single-star backtracking (linear for the
	// usual patterns, no recursion on hostile input).
	pi, ni := 0, 0
	starP, starN := -1, 0
	for ni < len(name) {
		if pi < len(p) {
			pc, pw := utf8.DecodeRuneInString(p[pi:])
			nc, nw := utf8.DecodeRuneInString(name[ni:])
			switch {
			case pc == '*':
				starP, starN = pi, ni
				pi += pw
				continue
			case pc == '?' || pc == nc:
				pi += pw
				ni += nw
				continue
			}
		}
		if starP < 0 {
			return false
		}
		_, nw := utf8.DecodeRuneInString(name[starN:])
		starN += nw
		pi, ni = starP+1, starN
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	// A trailing ".*" also matches a name without an extension.
	if strings.HasPrefix(p[pi:], ".*") && strings.Trim(p[pi+2:], "*") == "" && !strings.Contains(name[max(0, starN):], ".") {
		return true
	}
	return pi == len(p)
}
