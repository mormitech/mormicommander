// This file implements the "Search for" field of Total Commander's Find Files dialog
// (F-525, F-837). The field holds one or more wildcard masks separated by semicolons or
// spaces ("*.txt;*.doc", or "*.txt *.doc"), names with spaces go in double quotes, and
// everything after a single "|" is an exclusion list: "*.txt|old*" finds text files
// whose names do not start with "old". An exclusion that ends with a slash or
// backslash ("|vendor/" or "|.git\") excludes folders instead: the walker neither
// lists nor enters them, which is how TC users keep searches out of build output.
//
// The matching rules follow Total Commander and agree with the shared masks package:
// names compare without regard to case and in Unicode NFC form (macOS hands out
// decomposed names, while the user types composed ones, so "árok*" must still find the
// file Finder shows as "árok.txt"), "*" is any run of characters and "?" exactly one,
// "*.*" means every name (even names without a dot), a mask ending in ".*" also matches
// the bare name ("readme.*" finds "readme", the DOS rule), and a mask ending in "."
// ("*.") matches only names without an extension. A mask with no wildcard and no dot
// is searched as "*text*", so typing "report" finds "Q3 report final.pdf"; a mask with
// a dot but no wildcard ("readme.txt") names one exact file. A mask that contains a
// path separator in the middle ("src/*.go") is matched against the path relative to
// the search folder.

package find

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Mask is a parsed "Search for" string.
type Mask struct {
	Include     []Pattern // names to find; empty means every name
	Exclude     []Pattern // names to leave out of the results
	ExcludeDirs []Pattern // folders to skip entirely (neither listed nor entered)
}

// Pattern is one wildcard mask.
type Pattern struct {
	Raw   string // the mask as typed, for display
	glob  string // lower-case glob actually matched
	path  bool   // match against the relative path instead of the name
	all   bool   // "*.*" or "*": everything
	noExt bool   // the mask ended in ".": the name must have no extension
	bare  string // set when the mask ends in ".*": the part before it, also tried on names without an extension
}

// ParseMask parses a Total Commander "Search for" string.
func ParseMask(s string) (*Mask, error) {
	inc, exc, err := splitExclusions(s)
	if err != nil {
		return nil, err
	}
	m := &Mask{}
	for _, t := range tokenize(inc) {
		m.Include = append(m.Include, newPattern(t, true))
	}
	for _, t := range tokenize(exc) {
		if strings.HasSuffix(t, "/") || strings.HasSuffix(t, `\`) {
			d := strings.TrimRight(t, `/\`)
			if d == "" {
				continue
			}
			m.ExcludeDirs = append(m.ExcludeDirs, newPattern(d, false))
			continue
		}
		m.Exclude = append(m.Exclude, newPattern(t, true))
	}
	return m, nil
}

// MustParseMask is ParseMask for masks known to be valid (tests, constants).
func MustParseMask(s string) *Mask {
	m, err := ParseMask(s)
	if err != nil {
		panic(err)
	}
	return m
}

// splitExclusions splits at the first "|" outside quotes. A second "|" is an error,
// because Total Commander rejects it too and silently guessing would surprise users.
func splitExclusions(s string) (inc, exc string, err error) {
	inQuote := false
	cut := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '|':
			if inQuote {
				continue
			}
			if cut >= 0 {
				return "", "", fmt.Errorf("find: more than one | in mask %q", s)
			}
			cut = i
		}
	}
	if inQuote {
		return "", "", fmt.Errorf("find: unbalanced quote in mask %q", s)
	}
	if cut < 0 {
		return s, "", nil
	}
	return s[:cut], s[cut+1:], nil
}

// tokenize splits on semicolons and white space, keeping quoted runs together.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, quoted := false, false
	flush := func() {
		if cur.Len() > 0 || quoted {
			if t := cur.String(); t != "" {
				out = append(out, t)
			}
		}
		cur.Reset()
		quoted = false
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			quoted = true
		case !inQuote && (r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// newPattern normalises one mask. autoStar applies TC's "no wildcard and no dot means
// *text*" rule; folder exclusions do not use it, so "|bin/" skips "bin" but not "cabinet".
func newPattern(raw string, autoStar bool) Pattern {
	p := Pattern{Raw: raw}
	g := foldName(strings.ReplaceAll(raw, `\`, "/"))
	if g == "*.*" || g == "*" {
		p.all = true
		return p
	}
	if strings.Contains(strings.Trim(g, "/"), "/") {
		p.path = true
		g = strings.Trim(g, "/")
	}
	if !p.path && len(g) > 1 && strings.HasSuffix(g, ".") && strings.ContainsAny(g, "*?") {
		p.noExt = true
		g = strings.TrimSuffix(g, ".")
	}
	if autoStar && !strings.ContainsAny(g, "*?") && !strings.Contains(g, ".") {
		g = "*" + g + "*"
	}
	if !p.path && len(g) > 2 && strings.HasSuffix(g, ".*") {
		p.bare = strings.TrimSuffix(g, ".*")
	}
	p.glob = g
	return p
}

// foldName prepares a name or mask for comparison: NFC first, so composed and
// decomposed spellings of the same letter are equal, then lower case.
func foldName(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// hasExt says whether a (folded) name has an extension: text after the last dot,
// where a leading dot (".bashrc") does not start one and a trailing dot leaves none.
// It is the same rule as masks.SplitExt.
func hasExt(n string) bool {
	i := strings.LastIndexByte(n, '.')
	return i > 0 && i < len(n)-1
}

// Match reports whether a name (or, for path masks, the slash-separated relative path)
// matches this pattern.
func (p Pattern) Match(name, rel string) bool {
	if p.all {
		return true
	}
	if p.path {
		return Glob(p.glob, foldName(strings.ReplaceAll(rel, `\`, "/")))
	}
	n := foldName(name)
	if p.noExt {
		if hasExt(n) {
			return false
		}
	}
	if Glob(p.glob, n) {
		return true
	}
	return p.bare != "" && !strings.Contains(n, ".") && Glob(p.bare, n)
}

// MatchName reports whether a file or folder name is selected by the mask: it matches
// at least one include pattern (or there are none) and no exclude pattern. rel is the
// path relative to the search folder, used only by masks that contain a separator.
func (m *Mask) MatchName(name, rel string) bool {
	if m == nil {
		return true
	}
	ok := len(m.Include) == 0
	for _, p := range m.Include {
		if p.Match(name, rel) {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	for _, p := range m.Exclude {
		if p.Match(name, rel) {
			return false
		}
	}
	return true
}

// SkipDir reports whether a folder is excluded by a "name/" exclusion.
func (m *Mask) SkipDir(name, rel string) bool {
	if m == nil {
		return false
	}
	for _, p := range m.ExcludeDirs {
		if p.Match(name, rel) {
			return true
		}
	}
	return false
}

// Glob matches s against a pattern where "*" matches any run of characters and "?"
// exactly one character. It is case-sensitive; callers lower-case both sides. Unlike
// path.Match, "*" also crosses slashes and there are no character classes, because
// "[" is a perfectly ordinary file name character for Total Commander users.
func Glob(pattern, s string) bool {
	px, sx := 0, 0
	starP, starS := -1, -1
	for sx < len(s) {
		if px < len(pattern) {
			switch pattern[px] {
			case '*':
				starP, starS = px, sx
				px++
				continue
			case '?':
				_, w := utf8.DecodeRuneInString(s[sx:])
				px++
				sx += w
				continue
			default:
				pr, pw := utf8.DecodeRuneInString(pattern[px:])
				sr, sw := utf8.DecodeRuneInString(s[sx:])
				if pr == sr {
					px += pw
					sx += sw
					continue
				}
			}
		}
		if starP < 0 {
			return false
		}
		// Backtrack: let the last star swallow one more character.
		_, w := utf8.DecodeRuneInString(s[starS:])
		starS += w
		px, sx = starP+1, starS
	}
	for px < len(pattern) && pattern[px] == '*' {
		px++
	}
	return px == len(pattern)
}
