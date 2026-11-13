// Package masks implements Total Commander style wildcard masks and the selection engine built on them.
//
// A single mask is a pattern such as "*.txt", "report??.doc" or "[a-c]*.go". This file compiles one mask and matches
// it against a file name. The rules follow Total Commander, which inherited them from DOS, because users type masks
// from muscle memory and expect the same results: "*" matches any run of characters (also none), "?" matches exactly
// one character, and "[...]" matches one character out of a set ("[abc]", ranges like "[a-z]", negated with "[!...]"
// or "[^...]"). Two DOS conveniences are kept on purpose: a mask that ends in ".*" also matches names without any
// extension (so "*.*" really means every file and "readme.*" matches "readme"), and a mask that ends in a single "."
// matches only names without an extension (so "*." selects extensionless files). Names and masks are compared in
// Unicode NFC form, because macOS file systems may hand back decomposed (NFD) names while the user types composed
// ones; without that, "café*" would silently fail to match a file the user can plainly see.
package masks

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// tokKind says what one compiled pattern element matches.
type tokKind uint8

const (
	tLit  tokKind = iota // one literal character
	tOne                 // "?": any single character
	tStar                // "*": any run of characters
	tSet                 // "[...]": one character out of a set
)

// runeRange is an inclusive range of characters inside a "[...]" set; a single character is a range of length one.
type runeRange struct{ lo, hi rune }

// tok is one element of a compiled mask.
type tok struct {
	kind   tokKind
	r      rune        // the character for tLit
	ranges []runeRange // the members for tSet
	negate bool        // tSet: "[!...]" or "[^...]"
}

// Mask is one compiled wildcard pattern. It is safe for concurrent use once compiled.
type Mask struct {
	raw      string         // the mask as the user typed it (without a folder marker)
	toks     []tok          // the compiled pattern
	dirOnly  bool           // the user wrote a trailing "\" or "/": the mask applies to folders only
	cs       bool           // compare case-sensitively
	pathMode bool           // match a whole path: "*" and "?" never cross "/", which only a literal "/" can match
	dotStar  []tok          // when the mask ends in ".*": the same pattern without it, tried for extensionless names
	dotEnd   []tok          // when the mask ends in a single ".": the pattern without it, tried for extensionless names
	literal  bool           // no wildcards at all, so a plain (folded) string compare is enough
	litFold  string         // the folded literal for the fast path
	segments [][]tok        // pathMode: the pattern split at literal "/" characters
	re       *regexp.Regexp // set for a "<regex" mask; the other fields are then unused
}

// CompileMask compiles one mask. caseSensitive decides how letters compare; use CaseRule.Sensitive to follow the
// platform default. A trailing "\" or "/" marks a folder-only mask and is removed from the pattern. An empty mask is
// valid and matches only an empty name, which in practice means nothing; callers that want "everything" use "*".
func CompileMask(mask string, caseSensitive bool) *Mask {
	m := &Mask{cs: caseSensitive}
	if n := len(mask); n > 1 && (mask[n-1] == '\\' || mask[n-1] == '/') {
		m.dirOnly = true
		mask = mask[:n-1]
	}
	m.raw = mask
	m.compile(norm.NFC.String(mask))
	return m
}

// CompilePathMask compiles a mask that is matched against a whole slash-separated relative path, such as "src/*.go"
// in a synchronize filter. "*" and "?" never cross a "/"; only a literal "/" in the mask matches one. Backslashes are
// turned into slashes first, and a trailing separator makes it a folder-only mask, as with CompileMask.
func CompilePathMask(mask string, caseSensitive bool) *Mask { return compilePath(mask, caseSensitive) }

// compilePath compiles a mask that is matched against a whole slash-separated path (used by the ignore list for
// entries such as "C:\Windows\*.dll"). Backslashes are turned into slashes first, so the same list works everywhere.
func compilePath(mask string, caseSensitive bool) *Mask {
	m := &Mask{cs: caseSensitive, pathMode: true}
	mask = strings.ReplaceAll(mask, "\\", "/")
	if n := len(mask); n > 1 && mask[n-1] == '/' {
		m.dirOnly = true
		mask = mask[:n-1]
	}
	m.raw = mask
	m.compile(norm.NFC.String(mask))
	return m
}

// compile turns the (already normalised) pattern text into tokens and prepares the DOS special cases.
func (m *Mask) compile(s string) {
	m.toks = parseToks(s)
	m.literal = true
	for _, t := range m.toks {
		if t.kind != tLit {
			m.literal = false
			break
		}
	}
	if m.literal {
		m.litFold = s
		if !m.cs {
			m.litFold = foldString(s)
		}
	}
	n := len(m.toks)
	switch {
	case n >= 2 && m.toks[n-1].kind == tStar && isLit(m.toks[n-2], '.'):
		m.dotStar = m.toks[:n-2]
	case n >= 1 && isLit(m.toks[n-1], '.') && !(n >= 2 && isLit(m.toks[n-2], '.')):
		m.dotEnd = m.toks[:n-1]
	}
	if m.pathMode {
		var seg []tok
		for _, t := range m.toks {
			if isLit(t, '/') {
				m.segments = append(m.segments, seg)
				seg = nil
				continue
			}
			seg = append(seg, t)
		}
		m.segments = append(m.segments, seg)
	}
}

// isLit says whether t is the literal character r.
func isLit(t tok, r rune) bool { return t.kind == tLit && t.r == r }

// parseToks splits a pattern into tokens. A "[" without a closing "]" is an ordinary character, so names such as
// "[draft" can still be typed literally; a "]" right after "[" (or "[!") is a member of the set, as in shell globs.
func parseToks(s string) []tok {
	rs := []rune(s)
	var out []tok
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '*':
			if len(out) > 0 && out[len(out)-1].kind == tStar {
				continue // "**" is the same as "*"; collapsing keeps matching linear
			}
			out = append(out, tok{kind: tStar})
		case '?':
			out = append(out, tok{kind: tOne})
		case '[':
			if t, next, ok := parseSet(rs, i); ok {
				out = append(out, t)
				i = next
				continue
			}
			out = append(out, tok{kind: tLit, r: r})
		default:
			out = append(out, tok{kind: tLit, r: r})
		}
	}
	return out
}

// parseSet reads a "[...]" set starting at rs[start] == '['. It returns the token, the index of the closing "]",
// and false when the set is not closed.
func parseSet(rs []rune, start int) (tok, int, bool) {
	t := tok{kind: tSet}
	i := start + 1
	if i < len(rs) && (rs[i] == '!' || rs[i] == '^') {
		t.negate = true
		i++
	}
	first := i
	for ; i < len(rs); i++ {
		if rs[i] == ']' && i > first {
			return t, i, true
		}
		lo := rs[i]
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != ']' {
			hi := rs[i+2]
			if hi < lo {
				lo, hi = hi, lo
			}
			t.ranges = append(t.ranges, runeRange{lo, hi})
			i += 2
			continue
		}
		t.ranges = append(t.ranges, runeRange{lo, lo})
	}
	return tok{}, 0, false
}

// String returns the mask as typed, with a trailing "/" when it is a folder-only mask.
func (m *Mask) String() string {
	if m.re != nil {
		return "<" + m.raw
	}
	if m.dirOnly {
		return m.raw + "/"
	}
	return m.raw
}

// DirOnly says whether the mask was written with a trailing "\" or "/" and so applies to folders only.
func (m *Mask) DirOnly() bool { return m.dirOnly }

// HasWildcards says whether the mask contains "*", "?" or a set; a mask without them names one exact file.
func (m *Mask) HasWildcards() bool { return m.re != nil || !m.literal }

// Match reports whether name matches the mask. It does not look at DirOnly; List.Match applies that rule.
func (m *Mask) Match(name string) bool {
	name = norm.NFC.String(name)
	if m.re != nil {
		return m.re.MatchString(name)
	}
	if m.pathMode {
		return m.matchPath(name)
	}
	if m.literal {
		if m.cs {
			if name == m.litFold {
				return true
			}
		} else if foldString(name) == m.litFold {
			return true
		}
		// A literal mask can still use the "name." form for extensionless names.
		if m.dotEnd != nil {
			return !hasExt(name) && matchToks(m.dotEnd, []rune(name), m.cs)
		}
		return false
	}
	rs := []rune(name)
	if matchToks(m.toks, rs, m.cs) {
		return true
	}
	if m.dotStar != nil && !strings.Contains(name, ".") && matchToks(m.dotStar, rs, m.cs) {
		return true
	}
	if m.dotEnd != nil && !hasExt(name) && matchToks(m.dotEnd, rs, m.cs) {
		return true
	}
	return false
}

// matchPath matches a slash-separated path segment by segment. Because wildcards never match "/", every "/" in the
// name has to line up with a literal "/" in the mask, so splitting both sides gives the exact answer cheaply.
func (m *Mask) matchPath(p string) bool {
	segs := strings.Split(p, "/")
	if len(segs) != len(m.segments) {
		return false
	}
	for i, s := range segs {
		if !matchToks(m.segments[i], []rune(s), m.cs) {
			return false
		}
	}
	return true
}

// matchToks is the classic linear wildcard matcher: it walks the name once and, on a mismatch, lets the most recent
// "*" absorb one more character. This never backtracks exponentially, so hostile masks like "*a*a*a*a*b" are fast.
func matchToks(toks []tok, name []rune, cs bool) bool {
	ti, ni := 0, 0
	starT, starN := -1, 0
	for ni < len(name) {
		if ti < len(toks) {
			switch t := toks[ti]; t.kind {
			case tStar:
				starT, starN = ti, ni
				ti++
				continue
			default:
				if t.matches(name[ni], cs) {
					ti++
					ni++
					continue
				}
			}
		}
		if starT < 0 {
			return false
		}
		starN++
		ni = starN
		ti = starT + 1
	}
	for ti < len(toks) && toks[ti].kind == tStar {
		ti++
	}
	return ti == len(toks)
}

// matches says whether the single-character token t accepts r.
func (t tok) matches(r rune, cs bool) bool {
	switch t.kind {
	case tOne:
		return true
	case tLit:
		return r == t.r || (!cs && equalFoldRune(r, t.r))
	case tSet:
		in := t.inSet(r)
		if !in && !cs {
			in = t.inSet(unicode.ToLower(r)) || t.inSet(unicode.ToUpper(r))
		}
		return in != t.negate
	}
	return false
}

// inSet says whether r falls in one of the set's ranges.
func (t tok) inSet(r rune) bool {
	for _, rg := range t.ranges {
		if r >= rg.lo && r <= rg.hi {
			return true
		}
	}
	return false
}

// equalFoldRune compares two characters under Unicode simple case folding, so "Ä" equals "ä" and "K" (Kelvin) "k".
func equalFoldRune(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

// foldString maps a string to a canonical case-folded form so two names that differ only in case compare equal.
func foldString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

// foldRune picks the smallest member of r's case-folding orbit as its canonical form.
func foldRune(r rune) rune {
	min := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < min {
			min = f
		}
	}
	return min
}

// SplitExt splits a file name into base name and extension the way the panels show them: the extension is the part
// after the last dot. A leading dot does not start an extension, so ".bashrc" has no extension and "a.tar.gz" has
// "gz". The returned extension has no dot.
func SplitExt(name string) (base, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// hasExt says whether name has an extension in the SplitExt sense (a trailing dot counts as none).
func hasExt(name string) bool {
	_, ext := SplitExt(name)
	return ext != ""
}

// sameName compares two names under the given case rule, after NFC normalisation.
func sameName(a, b string, cs bool) bool {
	if a == b {
		return true
	}
	a, b = norm.NFC.String(a), norm.NFC.String(b)
	if cs {
		return a == b
	}
	return foldString(a) == foldString(b)
}
