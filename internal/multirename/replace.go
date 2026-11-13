// This file implements the "Search for / Replace with" part of the dialog.
// In plain mode the search text may contain the wildcards * and ?, and
// several pairs can be given at once separated by a vertical bar: searching
// for "a|b" and replacing with "x|y" turns every a into x and every b into y
// in a single pass, so one replacement never feeds the next. A single
// replacement is used for all search strings. In regular-expression mode the
// bar is ordinary regex alternation and the replacement may refer to groups
// as $1, $2 or ${name}; we rewrite $1 to ${1} first because Go would read
// "$1x" as a group named "1x", while Total Commander users expect group 1
// followed by "x". Searches ignore case unless the "^" box is ticked, and
// "Only first" stops after the first match.

package multirename

import (
	"regexp"
	"strings"
)

// replacer is a compiled search-and-replace setting.
type replacer struct {
	re        *regexp.Regexp
	regex     bool
	template  string   // regex mode: the expanded-ready replacement
	repls     []string // plain mode: replacement per search alternative
	onlyFirst bool
}

var dollarNum = regexp.MustCompile(`\$(\d+)`)

// newReplacer compiles the search settings. It returns nil when there is
// nothing to search for.
func newReplacer(s Settings) (*replacer, error) {
	if s.Search == "" {
		return nil, nil
	}
	flags := ""
	if !s.CaseSensitive {
		flags = "(?i)"
	}
	r := &replacer{onlyFirst: s.OnlyFirst}
	if s.RegEx {
		re, err := regexp.Compile(flags + s.Search)
		if err != nil {
			return nil, err
		}
		r.re, r.regex = re, true
		r.template = dollarNum.ReplaceAllString(s.Replace, "$${$1}")
		return r, nil
	}
	pats := strings.Split(s.Search, "|")
	reps := strings.Split(s.Replace, "|")
	var alts []string
	for i, p := range pats {
		if p == "" {
			continue
		}
		rep := ""
		switch {
		case len(reps) == 1:
			rep = reps[0]
		case i < len(reps):
			rep = reps[i]
		}
		alts = append(alts, "("+wildcardToRegex(p)+")")
		r.repls = append(r.repls, rep)
	}
	if len(alts) == 0 {
		return nil, nil
	}
	re, err := regexp.Compile(flags + strings.Join(alts, "|"))
	if err != nil {
		return nil, err
	}
	r.re = re
	return r, nil
}

// wildcardToRegex quotes p for a regular expression, turning * into "any
// run of characters" and ? into "any one character".
func wildcardToRegex(p string) string {
	var b strings.Builder
	for _, c := range p {
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// apply performs the replacement on s.
func (r *replacer) apply(s string) string {
	n := -1
	if r.onlyFirst {
		n = 1
	}
	matches := r.re.FindAllStringSubmatchIndex(s, n)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m[0]])
		if r.regex {
			b.Write(r.re.ExpandString(nil, r.template, s, m))
		} else {
			for g := 0; g < len(r.repls); g++ {
				if m[2+2*g] >= 0 {
					b.WriteString(r.repls[g])
					break
				}
			}
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}
