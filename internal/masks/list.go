// This file parses a full mask expression as typed into Total Commander's "Select files" (Num +), "Unselect files"
// (Num -), custom panel filter (Ctrl+F12) or colour-by-type dialogs. An expression holds several masks separated by
// ";" or spaces, for example "*.jpg;*.png *.gif". Masks that contain spaces are put in double quotes. A "|" splits
// the expression into masks to include (left) and masks to exclude (right), so "*.txt|read*" means every text file
// except those starting with "read", and "|*.bak" (nothing on the left) means everything except backups. A mask that
// ends in "\" or "/" applies to folders only, which lets one expression say "all files but skip .git folders":
// "*|.git\". A word starting with ">" names a saved filter, which the caller looks up through Options.Resolve and
// whose masks are merged in, the way TC lets ">Pictures" stand for a stored search template. Case rules follow the
// platform (case-insensitive on Windows and macOS, case-sensitive on Linux and other Unix) unless the caller says
// otherwise, because that is how the file systems behave by default.

package masks

import (
	"errors"
	"fmt"
	"strings"
)

// CaseRule says how letters compare. The zero value follows the platform default.
type CaseRule uint8

const (
	CaseDefault     CaseRule = iota // case-insensitive on Windows and macOS, case-sensitive elsewhere
	CaseSensitive                   // "A" and "a" differ
	CaseInsensitive                 // "A" and "a" are the same letter
)

// Sensitive resolves the rule to a yes/no answer for this platform.
func (c CaseRule) Sensitive() bool {
	switch c {
	case CaseSensitive:
		return true
	case CaseInsensitive:
		return false
	}
	return DefaultCaseSensitive
}

// Options controls how an expression is parsed.
type Options struct {
	Case CaseRule
	// Resolve looks up a saved filter by name for words written as ">name". It returns the stored expression and
	// whether the name exists. Nil means saved filters are not available and any ">name" word is an error.
	Resolve func(name string) (string, bool)
}

// maxResolveDepth stops saved filters that refer to each other in a loop.
const maxResolveDepth = 8

// ErrUnclosedQuote is returned when a double quote in the expression has no partner.
var ErrUnclosedQuote = errors.New("masks: unclosed quote")

// List is a parsed mask expression: masks to include and masks to exclude.
type List struct {
	Include []*Mask // empty means "every name"
	Exclude []*Mask
	raw     string
	cs      bool
}

// Parse parses a mask expression. An empty or blank expression gives a list that matches everything, which is what
// TC does when the "Select files" box is left empty with "*.*".
func Parse(expr string, opt Options) (*List, error) {
	l := &List{raw: expr, cs: opt.Case.Sensitive()}
	if err := l.add(expr, opt, 0); err != nil {
		return nil, err
	}
	return l, nil
}

// MustParse is Parse for expressions known to be valid, such as constants in code; it panics on an error.
func MustParse(expr string, opt Options) *List {
	l, err := Parse(expr, opt)
	if err != nil {
		panic(err)
	}
	return l
}

// add parses expr and appends its masks to l, resolving saved filters recursively.
func (l *List) add(expr string, opt Options, depth int) error {
	incWords, excWords, err := splitExpr(expr)
	if err != nil {
		return err
	}
	for side, words := range [2][]string{incWords, excWords} {
		for _, w := range words {
			if strings.HasPrefix(w, ">") && len(w) > 1 {
				name := w[1:]
				if opt.Resolve == nil {
					return fmt.Errorf("masks: saved filter %q: no saved filters available", name)
				}
				sub, ok := opt.Resolve(name)
				if !ok {
					return fmt.Errorf("masks: saved filter %q not found", name)
				}
				if depth >= maxResolveDepth {
					return fmt.Errorf("masks: saved filter %q: too many nested filters", name)
				}
				if side == 1 {
					// A saved filter on the exclude side excludes everything it would include.
					si, _, err := splitExpr(sub)
					if err != nil {
						return err
					}
					sub = "|" + joinQuoted(si)
				}
				if err := l.add(sub, opt, depth+1); err != nil {
					return err
				}
				continue
			}
			m := CompileMask(w, l.cs)
			if side == 0 {
				l.Include = append(l.Include, m)
			} else {
				l.Exclude = append(l.Exclude, m)
			}
		}
	}
	return nil
}

// joinQuoted rebuilds an expression from words, quoting the ones that contain separators.
func joinQuoted(words []string) string {
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteByte(';')
		}
		if strings.ContainsAny(w, " ;|") {
			b.WriteString(`"` + w + `"`)
		} else {
			b.WriteString(w)
		}
	}
	return b.String()
}

// splitExpr splits an expression into include and exclude words. Quotes group a word and are removed; the first
// unquoted "|" switches to the exclude side (later "|" characters are treated as separators).
func splitExpr(expr string) (inc, exc []string, err error) {
	var cur strings.Builder
	inQuote, quoted, excl := false, false, false
	flush := func() {
		if cur.Len() > 0 || quoted {
			if excl {
				exc = append(exc, cur.String())
			} else {
				inc = append(inc, cur.String())
			}
		}
		cur.Reset()
		quoted = false
	}
	for _, r := range expr {
		switch {
		case r == '"':
			inQuote = !inQuote
			quoted = true
		case inQuote:
			cur.WriteRune(r)
		case r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		case r == '|':
			flush()
			excl = true
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, nil, ErrUnclosedQuote
	}
	flush()
	// An empty quoted word ("") is not a useful mask; drop it.
	inc, exc = dropEmpty(inc), dropEmpty(exc)
	return inc, exc, nil
}

// dropEmpty removes empty words.
func dropEmpty(ws []string) []string {
	out := ws[:0]
	for _, w := range ws {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// String returns the expression the list was parsed from.
func (l *List) String() string { return l.raw }

// CaseSensitive says whether the list compares letters case-sensitively.
func (l *List) CaseSensitive() bool { return l.cs }

// MatchesAll says whether the list accepts every name (no include masks, or only "*"/"*.*", and no exclusions).
// The panel uses this to tell whether a filter is really active.
func (l *List) MatchesAll() bool {
	if len(l.Exclude) > 0 {
		return false
	}
	if len(l.Include) == 0 {
		return true
	}
	for _, m := range l.Include {
		if !m.dirOnly && (m.raw == "*" || m.raw == "*.*") {
			return true
		}
	}
	return false
}

// Match reports whether an entry with this name passes the list. Folder-only masks ("name\") are considered only for
// folders. A list whose include side holds only folder masks therefore never accepts a file, while plain masks apply
// to files and folders alike; callers that want "files only" filter by kind themselves, as the selection does.
func (l *List) Match(name string, isDir bool) bool {
	if !l.included(name, isDir) {
		return false
	}
	for _, m := range l.Exclude {
		if (!m.dirOnly || isDir) && m.Match(name) {
			return false
		}
	}
	return true
}

// included checks the include side only.
func (l *List) included(name string, isDir bool) bool {
	if len(l.Include) == 0 {
		return true
	}
	for _, m := range l.Include {
		if (!m.dirOnly || isDir) && m.Match(name) {
			return true
		}
	}
	return false
}

// ExcludesDir says whether a folder of this name is excluded, so a recursive walk (search, compare with subfolders,
// size calculation) can skip it without descending.
func (l *List) ExcludesDir(name string) bool {
	for _, m := range l.Exclude {
		if m.Match(name) {
			return true
		}
	}
	return false
}

// Match parses expr with the platform case rule and reports whether a file called name matches it.
func Match(expr, name string) (bool, error) {
	l, err := Parse(expr, Options{})
	if err != nil {
		return false, err
	}
	return l.Match(name, false), nil
}
