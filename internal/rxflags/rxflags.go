// Package rxflags compiles a user-typed regular expression with inline flags such as "(?i)" or "(?m)" put in front
// of it by the program, while keeping those internal flags out of any error message. Several places (selection
// masks, Find Files, the Lister search, Multi-Rename) add "(?i)" when the user did not ask for exact case. Go's
// regexp parser quotes the offending part of the pattern in its error, and for some mistakes (an unclosed "(")
// that part is the whole pattern, so the user saw "missing closing ): (?i)(" for a pattern they typed as "(".
// Compile reports errors as they would read for the pattern the user actually typed.

package rxflags

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

// Compile compiles flags+expr. flags is the program's own inline-flag prefix (for example "(?i)" or "(?i)(?m)",
// possibly empty) and expr is the user's text. On success it behaves exactly like regexp.Compile(flags+expr). On
// failure the returned error describes expr alone, never mentioning the prefix.
func Compile(flags, expr string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(flags + expr)
	if err == nil || flags == "" {
		return re, err
	}
	// A flag prefix never turns a valid pattern into an invalid one, so the bare pattern normally fails with the
	// same complaint, quoting only what the user typed.
	if _, bareErr := regexp.Compile(expr); bareErr != nil {
		return nil, bareErr
	}
	// Defensive fallback: strip the prefix from the quoted expression of the original error.
	if se, ok := err.(*syntax.Error); ok {
		return nil, &syntax.Error{Code: se.Code, Expr: strings.TrimPrefix(se.Expr, flags)}
	}
	return nil, err
}
