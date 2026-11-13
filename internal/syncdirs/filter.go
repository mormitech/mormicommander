// This file implements the file-type filter of the Synchronize dialog
// (the "Search for" box of FEATURES.md F-546). Spec area-5 9.2 says the box
// uses the same syntax as Find Files 3.1, so the strings users already know
// keep working, and the matching itself is done by the shared masks package
// so all mask boxes in the program agree:
//
//	*.txt *.doc            only these files
//	*.* | *.bak *.tmp      everything except backups and temp files
//	* | .git\ node_modules\   everything, but skip those folders entirely
//	src\                   only the folder "src" (and everything below it)
//	"my file*.txt"         quotes keep spaces inside one pattern
//	report                 no wildcard and no dot: any name containing "report"
//	*.                     names without an extension ("Makefile")
//	>Sources               a saved Find Files template, see CompileFilter
//
// Patterns are separated by spaces or semicolons; those after "|" exclude.
// A pattern ending in "\" or "/" applies to folders, the others to files.
// A pattern containing a slash in the middle is matched against the
// relative path instead of the bare name ("*" and "?" do not cross a slash
// there). Matching ignores letter case and compares names in Unicode NFC,
// "*.*" means every file, even one without a dot, and "name.*" also matches
// the bare "name", as in Total Commander. When the include side names only
// folders, files are taken only from inside those folders, so `src\` really
// means "nothing but src".
package syncdirs

import (
	"fmt"
	"strings"

	"mormicommander/internal/masks"
)

// Filter decides which names take part in the compare.
type Filter struct {
	include, exclude []pattern
	inclDirs         bool // include list has folder patterns
	inclFiles        bool // include list has file patterns
}

type pattern struct {
	m      *masks.Mask
	dir    bool // folder pattern (ended in a slash)
	byPath bool // matched against the relative path
}

// maxTemplateDepth stops saved templates that refer to each other in a loop.
const maxTemplateDepth = 8

// ParseFilter parses a Total Commander filter string. An empty string (or
// "*" or "*.*") matches everything. Words of the form ">name" need a
// template resolver and are ignored here; use CompileFilter for them.
func ParseFilter(s string) *Filter {
	f, err := CompileFilter(s, nil)
	if err != nil {
		f = &Filter{}
		_ = f.add(s, nil, 0, true)
		f.finish()
	}
	return f
}

// CompileFilter parses a filter string. resolve looks up ">name" words (saved
// Find Files templates): it returns the template's "Search for" string and
// whether the template exists. A nil resolve, an unknown name, templates
// nested too deep or an unbalanced quote are errors.
func CompileFilter(s string, resolve func(name string) (string, bool)) (*Filter, error) {
	f := &Filter{}
	if err := f.add(s, resolve, 0, false); err != nil {
		return nil, err
	}
	f.finish()
	return f, nil
}

func (f *Filter) finish() {
	for _, p := range f.include {
		if p.dir {
			f.inclDirs = true
		} else {
			f.inclFiles = true
		}
	}
}

// add parses s and appends its patterns. lenient skips bad template words
// instead of failing (ParseFilter's behaviour).
func (f *Filter) add(s string, resolve func(string) (string, bool), depth int, lenient bool) error {
	incl, excl, err := splitFilter(s)
	if err != nil && !lenient {
		return err
	}
	for side, words := range [2][]string{incl, excl} {
		for _, w := range words {
			if strings.HasPrefix(w, ">") && len(w) > 1 {
				sub, err := lookupTemplate(w[1:], resolve, depth)
				if err != nil {
					if lenient {
						continue
					}
					return err
				}
				if side == 1 {
					// A template on the exclude side excludes what it would include.
					si, _, _ := splitFilter(sub)
					sub = "|" + quoteWords(si)
				}
				if err := f.add(sub, resolve, depth+1, lenient); err != nil {
					return err
				}
				continue
			}
			p, ok := newFilterPattern(w)
			if !ok {
				continue
			}
			if side == 0 {
				f.include = append(f.include, p)
			} else {
				f.exclude = append(f.exclude, p)
			}
		}
	}
	return nil
}

func lookupTemplate(name string, resolve func(string) (string, bool), depth int) (string, error) {
	if resolve == nil {
		return "", fmt.Errorf("syncdirs: filter template %q: no templates available", name)
	}
	if depth >= maxTemplateDepth {
		return "", fmt.Errorf("syncdirs: filter template %q: too many nested templates", name)
	}
	sub, ok := resolve(name)
	if !ok {
		return "", fmt.Errorf("syncdirs: filter template %q not found", name)
	}
	return sub, nil
}

// newFilterPattern compiles one word. File patterns without a wildcard and
// without a dot become "*word*" (Find Files rule); folder patterns never do,
// so `| bin\` skips "bin" but not "cabinet".
func newFilterPattern(tok string) (pattern, bool) {
	tok = strings.ReplaceAll(tok, `\`, "/")
	p := pattern{}
	if strings.HasSuffix(tok, "/") {
		p.dir = true
		tok = strings.TrimRight(tok, "/")
	}
	tok = strings.TrimPrefix(tok, "./")
	if tok == "" {
		return p, false
	}
	if strings.Contains(tok, "/") {
		p.byPath = true
		p.m = masks.CompilePathMask(tok, false)
		return p, true
	}
	if !p.dir && !strings.ContainsAny(tok, "*?") && !strings.Contains(tok, ".") {
		tok = "*" + tok + "*"
	}
	p.m = masks.CompileMask(tok, false)
	return p, true
}

// splitFilter splits a filter into include and exclude words: separators are
// spaces, tabs and semicolons, double quotes keep a word together, and the
// first unquoted "|" switches to the exclude side.
func splitFilter(s string) (incl, excl []string, err error) {
	var cur strings.Builder
	quoted, side := false, 0
	out := [2][]string{}
	flush := func() {
		if cur.Len() > 0 {
			out[side] = append(out[side], cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case quoted:
			cur.WriteRune(r)
		case r == ' ' || r == '\t' || r == ';':
			flush()
		case r == '|':
			flush()
			side = 1
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	if quoted {
		err = fmt.Errorf("syncdirs: unbalanced quote in filter %q", s)
	}
	return out[0], out[1], err
}

// quoteWords joins words back into a filter, quoting those with separators.
func quoteWords(ws []string) string {
	var b strings.Builder
	for i, w := range ws {
		if i > 0 {
			b.WriteByte(' ')
		}
		if strings.ContainsAny(w, " \t;|") {
			b.WriteString(`"` + w + `"`)
		} else {
			b.WriteString(w)
		}
	}
	return b.String()
}

func (p pattern) match(rel, name string) bool {
	if p.byPath {
		return p.m.Match(rel)
	}
	return p.m.Match(name)
}

// File reports whether the file at rel (base name name) is included.
// inIncludedDir says whether the file lies inside a folder that matched an
// include folder pattern; when the include side has folder patterns only,
// that is the only way a file gets in.
func (f *Filter) File(rel, name string, inIncludedDir bool) bool {
	if f == nil {
		return true
	}
	for _, p := range f.exclude {
		if !p.dir && p.match(rel, name) {
			return false
		}
	}
	if !f.inclFiles {
		return !f.inclDirs || inIncludedDir
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
