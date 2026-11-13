// Total Commander file masks. The same small language is used all over the
// settings: file colours by type ("*.exe *.com"), the ignore list, tooltip
// rules, column set auto-switching and selection masks. A mask is a list of
// wildcard patterns separated by spaces or semicolons (a pattern with spaces
// is put in double quotes); a "|" splits it into patterns to include and
// patterns to exclude ("*.txt | readme.txt"); a pattern that ends in a slash
// or backslash matches folders only; "*" and "?" are the wildcards and "*.*"
// also matches names without a dot, as in DOS. Matching ignores case, as TC
// does on every platform. A pattern containing a path separator is matched
// against the whole path instead of the bare name.

package config

import "strings"

// Mask is a parsed TC file mask.
type Mask struct {
	Include []string
	Exclude []string
}

// ParseMask parses mask text. An empty include list means "everything" when
// there is an exclude part ("| *.bak"), and "nothing" otherwise.
func ParseMask(s string) Mask {
	var m Mask
	inc, exc, hasExc := strings.Cut(s, "|")
	m.Include = splitPatterns(inc)
	if hasExc {
		m.Exclude = splitPatterns(exc)
		if len(m.Include) == 0 {
			m.Include = []string{"*"}
		}
	}
	return m
}

func splitPatterns(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == ';' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// String renders the mask back to text.
func (m Mask) String() string {
	q := func(ps []string) string {
		var parts []string
		for _, p := range ps {
			if strings.ContainsAny(p, " ;") {
				p = `"` + p + `"`
			}
			parts = append(parts, p)
		}
		return strings.Join(parts, " ")
	}
	s := q(m.Include)
	if len(m.Exclude) > 0 {
		s += " | " + q(m.Exclude)
	}
	return s
}

// Match reports whether a file matches. fullPath may be "" when only the
// name is known; isDir tells whether it is a folder.
func (m Mask) Match(name, fullPath string, isDir bool) bool {
	return matchAny(m.Include, name, fullPath, isDir) && !matchAny(m.Exclude, name, fullPath, isDir)
}

// MatchMask is a shortcut for ParseMask(mask).Match(name, "", isDir).
func MatchMask(mask, name string, isDir bool) bool {
	return ParseMask(mask).Match(name, "", isDir)
}

func matchAny(patterns []string, name, fullPath string, isDir bool) bool {
	for _, p := range patterns {
		if matchPattern(p, name, fullPath, isDir) {
			return true
		}
	}
	return false
}

func matchPattern(p, name, fullPath string, isDir bool) bool {
	dirOnly := strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`)
	if dirOnly {
		if !isDir {
			return false
		}
		p = strings.TrimRight(p, `/\`)
	}
	subject := name
	if strings.ContainsAny(p, `/\`) {
		if fullPath == "" {
			return false
		}
		subject = strings.ReplaceAll(fullPath, `\`, "/")
		p = strings.ReplaceAll(p, `\`, "/")
		if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "*") {
			p = "*/" + p
		}
	}
	return wildMatch(strings.ToLower(p), strings.ToLower(subject))
}

// wildMatch matches * (any run of characters, slashes included) and ?
// (one character). "[" is literal, as in TC.
func wildMatch(p, s string) bool {
	if p == "*.*" || p == "*" {
		return true
	}
	if strings.HasSuffix(p, ".*") && !strings.Contains(s, ".") {
		// DOS rule: "name.*" also matches "name".
		if wildMatch(strings.TrimSuffix(p, ".*"), s) {
			return true
		}
	}
	pr, sr := []rune(p), []rune(s)
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(sr) {
		switch {
		case pi < len(pr) && (pr[pi] == '?' || pr[pi] == sr[si]):
			pi++
			si++
		case pi < len(pr) && pr[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(pr) && pr[pi] == '*' {
		pi++
	}
	return pi == len(pr)
}
