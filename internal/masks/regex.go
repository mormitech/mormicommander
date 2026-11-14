// This file adds regular-expression masks. Total Commander's wiki documents that a selection expression starting
// with "<" is a regular expression, and mormiCommander supports that form with Go's RE2 syntax (spec area-3, mask
// syntax rule 5): "<^IMG_\d{4}\.jpe?g$" selects camera pictures that no wildcard mask can describe exactly. RE2 has
// no back-references or look-arounds, but it always runs in linear time, so a hostile pattern can never freeze the
// panel. The expression is matched against the NFC-normalised name and, like every other mask, ignores case unless
// the caller asks for exact case. A broken expression is reported as "Invalid regular expression: <detail>", the text
// the selection dialog shows before it stays open for a correction.

package masks

import (
	"regexp/syntax"

	"golang.org/x/text/unicode/norm"

	"mormicommander/internal/rxflags"
)

// RegexpError is returned when a "<regex" mask does not compile. Its message is the one the dialog shows.
type RegexpError struct {
	Expr   string // the expression without the leading "<"
	Detail string // what the regexp parser complained about
}

func (e *RegexpError) Error() string { return "Invalid regular expression: " + e.Detail }

// CompileRegexMask compiles expr (without the leading "<") as a regular-expression mask. caseSensitive false adds
// the (?i) flag. The mask matches when the expression finds a match anywhere in the name; anchor it with ^ and $ to
// demand the whole name.
func CompileRegexMask(expr string, caseSensitive bool) (*Mask, error) {
	src := norm.NFC.String(expr)
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	re, err := rxflags.Compile(flags, src)
	if err != nil {
		detail := err.Error()
		if se, ok := err.(*syntax.Error); ok {
			detail = se.Code.String() + ": " + se.Expr
		}
		return nil, &RegexpError{Expr: expr, Detail: detail}
	}
	return &Mask{raw: expr, cs: caseSensitive, re: re}, nil
}

// IsRegex says whether the mask is a regular-expression mask.
func (m *Mask) IsRegex() bool { return m.re != nil }
