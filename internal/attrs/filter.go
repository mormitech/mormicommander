// Filters for recursive attribute changes. When "Process subfolders" is on,
// TC lets the user restrict the change with a file mask or a saved search
// template, for example "make all *.sh files executable but leave .git
// alone". The mask uses TC's syntax: patterns separated by spaces or
// semicolons, an optional "|" followed by patterns to exclude, and a trailing
// slash or backslash marking a pattern that applies to folders only. Matching
// is case-insensitive like TC. An excluded folder pattern also stops the walk
// from descending into that folder. For search templates (attribute, size and
// date conditions from the Find Files engine) the filter has a Match hook so
// the UI can plug the template in without this package depending on it.
package attrs

import (
	"path"
	"strings"
)

// Filter decides which entries a recursive change touches. A nil *Filter
// matches everything.
type Filter struct {
	Include []string // patterns; empty means every name
	Exclude []string // patterns that win over Include
	// Match, when set, must also return true; it receives the full path and
	// the entry's Info (used for search templates).
	Match func(path string, info Info) bool
}

// ParseMask parses TC's mask syntax, e.g. "*.txt *.doc | *.bak .git\".
// Patterns containing spaces can be quoted with double quotes.
func ParseMask(s string) *Filter {
	inc, exc, _ := strings.Cut(s, "|")
	f := &Filter{Include: splitPatterns(inc), Exclude: splitPatterns(exc)}
	return f
}

// splitPatterns splits on spaces and semicolons, honouring double quotes.
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
		case !quoted && (r == ' ' || r == ';' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// String renders the filter back in mask syntax (without the Match hook).
func (f *Filter) String() string {
	if f == nil {
		return ""
	}
	q := func(list []string) string {
		parts := make([]string, len(list))
		for i, p := range list {
			if strings.ContainsAny(p, " ;") {
				p = `"` + p + `"`
			}
			parts[i] = p
		}
		return strings.Join(parts, " ")
	}
	s := q(f.Include)
	if len(f.Exclude) > 0 {
		s += " | " + q(f.Exclude)
	}
	return s
}

// matchPattern matches one TC pattern against a name. "*.*" means any name,
// with or without a dot, as in TC. Folder-only patterns (trailing / or \)
// never match files.
func matchPattern(pat, name string, isDir bool) bool {
	dirOnly := strings.HasSuffix(pat, "/") || strings.HasSuffix(pat, `\`)
	if dirOnly {
		if !isDir {
			return false
		}
		pat = strings.TrimRight(pat, `/\`)
	}
	if pat == "*.*" || pat == "*" || pat == "" {
		return true
	}
	p, n := strings.ToLower(pat), strings.ToLower(name)
	ok, err := path.Match(p, n)
	if err != nil {
		return p == n
	}
	return ok
}

// matchesName reports whether a name passes the include and exclude lists.
func (f *Filter) matchesName(name string, isDir bool) bool {
	if f == nil {
		return true
	}
	for _, p := range f.Exclude {
		if matchPattern(p, name, isDir) {
			return false
		}
	}
	if len(f.Include) == 0 {
		return true
	}
	for _, p := range f.Include {
		dirOnly := strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`)
		// A file-style include pattern like "*.txt" says nothing about
		// folders, so folders only match folder patterns or catch-alls.
		if isDir && !dirOnly && p != "*" && p != "*.*" {
			continue
		}
		if matchPattern(p, name, isDir) {
			return true
		}
	}
	return false
}

// Matches reports whether the entry should be changed.
func (f *Filter) Matches(info Info) bool {
	if f == nil {
		return true
	}
	if !f.matchesName(info.Name, info.IsDir()) {
		return false
	}
	if f.Match != nil && !f.Match(info.Path, info) {
		return false
	}
	return true
}

// Descend reports whether a recursive walk should enter a folder. Only
// folder exclusions stop descent; include patterns never do, because the
// files inside may still match.
func (f *Filter) Descend(name string) bool {
	if f == nil {
		return true
	}
	for _, p := range f.Exclude {
		if (strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`)) && matchPattern(p, name, true) {
			return false
		}
	}
	return true
}
