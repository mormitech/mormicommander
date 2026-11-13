// Total Commander file masks, as used by internal associations ("*.txt *.md", "*.jp*g;*.png | thumb*"). A mask
// list holds one or more masks separated by spaces or semicolons (a mask with spaces in it is put in double quotes);
// a '|' starts a second list of masks to exclude. '*' matches any run of characters and '?' exactly one. "*.*" and
// "*" match every name, "*." matches names without an extension, and a mask ending in a slash or backslash matches
// folders only. Matching ignores case, as it does in Total Commander, on every platform.
package launch

import "strings"

// Mask is a compiled mask list.
type Mask struct {
	include, exclude []maskPart
}

type maskPart struct {
	pattern string
	dirOnly bool
}

// ParseMask compiles a mask list. An empty list matches nothing.
func ParseMask(s string) Mask {
	inc, exc, _ := strings.Cut(s, "|")
	return Mask{include: splitMasks(inc), exclude: splitMasks(exc)}
}

func splitMasks(s string) []maskPart {
	var parts []maskPart
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		mp := maskPart{}
		if strings.HasSuffix(p, `\`) || strings.HasSuffix(p, "/") {
			mp.dirOnly = true
			p = strings.TrimRight(p, `\/`)
			if p == "" {
				p = "*"
			}
		}
		mp.pattern = strings.ToLower(p)
		parts = append(parts, mp)
	}
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == ';' || r == '\t'):
			add(cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	add(cur.String())
	return parts
}

// Empty says whether the mask list has no include masks.
func (m Mask) Empty() bool { return len(m.include) == 0 }

// Match says whether a name (not a path) matches the mask list. isDir tells whether the name is a folder, for masks
// that end in a separator.
func (m Mask) Match(name string, isDir bool) bool {
	lname := strings.ToLower(name)
	hit := false
	for _, p := range m.include {
		if p.matches(lname, isDir) {
			hit = true
			break
		}
	}
	if !hit {
		return false
	}
	for _, p := range m.exclude {
		if p.matches(lname, isDir) {
			return false
		}
	}
	return true
}

// MatchMask is ParseMask(mask).Match(name, isDir), for one-off checks.
func MatchMask(mask, name string, isDir bool) bool { return ParseMask(mask).Match(name, isDir) }

func (p maskPart) matches(lname string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	switch p.pattern {
	case "*", "*.*":
		return true
	case "*.":
		_, ext := splitExt(lname)
		return ext == "" && !strings.HasSuffix(lname, ".")
	}
	return wildMatch(p.pattern, lname)
}

// wildMatch matches s against a pattern of '*' and '?' wildcards, working on runes so '?' is one character even
// outside ASCII.
func wildMatch(pattern, s string) bool {
	pr, sr := []rune(pattern), []rune(s)
	pi, si := 0, 0
	starP, starS := -1, 0
	for si < len(sr) {
		switch {
		case pi < len(pr) && (pr[pi] == '?' || pr[pi] == sr[si]):
			pi++
			si++
		case pi < len(pr) && pr[pi] == '*':
			starP, starS = pi, si
			pi++
		case starP >= 0:
			pi = starP + 1
			starS++
			si = starS
		default:
			return false
		}
	}
	for pi < len(pr) && pr[pi] == '*' {
		pi++
	}
	return pi == len(pr)
}
