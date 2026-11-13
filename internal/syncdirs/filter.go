// This file implements the file-type filter of the Synchronize dialog
// (the "Search for" box of FEATURES.md F-546). It uses Total Commander's
// syntax so the strings users already know keep working:
//
//	*.txt *.doc            only these files
//	*.* | *.bak *.tmp      everything except backups and temp files
//	* | .git\ node_modules\   everything, but skip those folders entirely
//	src\                   only the folder "src" (and everything below it)
//	"my file*.txt"         quotes keep spaces inside one pattern
//
// Patterns are separated by spaces or semicolons; those after "|" exclude.
// A pattern ending in "\" or "/" applies to folders, the others to files.
// A pattern containing a slash in the middle is matched against the
// relative path instead of the bare name. "*" and "?" are the only
// wildcards, matching ignores letter case, and "*.*" means every file, even
// one without a dot, as in Total Commander.
package syncdirs

import (
	"strings"
)

// Filter decides which names take part in the compare.
type Filter struct {
	include, exclude []pattern
	inclDirs         bool // include list has folder patterns
	inclFiles        bool // include list has file patterns
}

type pattern struct {
	glob   string // lower-case, slashes normalised to "/"
	dir    bool   // folder pattern (ended in a slash)
	byPath bool   // matched against the relative path
}

// ParseFilter parses a Total Commander filter string. An empty string (or
// "*" or "*.*") matches everything.
func ParseFilter(s string) *Filter {
	f := &Filter{}
	incl, excl, _ := strings.Cut(s, "|")
	f.include = parsePatterns(incl)
	f.exclude = parsePatterns(excl)
	for _, p := range f.include {
		if p.dir {
			f.inclDirs = true
		} else {
			f.inclFiles = true
		}
	}
	return f
}

func parsePatterns(s string) []pattern {
	var out []pattern
	for _, tok := range splitPatterns(s) {
		tok = strings.ReplaceAll(tok, `\`, "/")
		p := pattern{}
		if strings.HasSuffix(tok, "/") {
			p.dir = true
			tok = strings.TrimRight(tok, "/")
		}
		if tok == "" {
			continue
		}
		tok = strings.TrimPrefix(tok, "./")
		if strings.Contains(tok, "/") {
			p.byPath = true
		}
		if tok == "*.*" {
			tok = "*"
		}
		p.glob = strings.ToLower(tok)
		out = append(out, p)
	}
	return out
}

// splitPatterns splits on spaces and semicolons, keeping quoted parts.
func splitPatterns(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && (r == ' ' || r == '\t' || r == ';'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func (p pattern) match(rel, name string) bool {
	if p.byPath {
		return globMatch(p.glob, strings.ToLower(rel))
	}
	return globMatch(p.glob, strings.ToLower(name))
}

// File reports whether the file at rel (base name name) is included.
func (f *Filter) File(rel, name string) bool {
	if f == nil {
		return true
	}
	for _, p := range f.exclude {
		if !p.dir && p.match(rel, name) {
			return false
		}
	}
	if !f.inclFiles {
		return true
	}
	for _, p := range f.include {
		if !p.dir && p.match(rel, name) {
			return true
		}
	}
	return false
}

// Dir reports whether the folder at rel is entered, and whether it matched
// an include folder pattern (then everything below it counts as included
// without checking the include folder patterns again). parentIncluded says
// whether an ancestor already matched.
func (f *Filter) Dir(rel, name string, parentIncluded bool) (enter, included bool) {
	if f == nil {
		return true, true
	}
	for _, p := range f.exclude {
		if p.dir && p.match(rel, name) {
			return false, false
		}
	}
	if !f.inclDirs || parentIncluded {
		return true, true
	}
	for _, p := range f.include {
		if p.dir && p.match(rel, name) {
			return true, true
		}
	}
	return false, false
}

// globMatch matches s against a pattern with "*" (any run of characters)
// and "?" (exactly one character). It works on runes, so "?" matches one
// accented letter, and runs in linear time with the usual backtracking
// over the last star.
func globMatch(pat, s string) bool {
	p := []rune(pat)
	t := []rune(s)
	pi, ti := 0, 0
	star, mark := -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]):
			pi++
			ti++
		case pi < len(p) && p[pi] == '*':
			star = pi
			mark = ti
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
