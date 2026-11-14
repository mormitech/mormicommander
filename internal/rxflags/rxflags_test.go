// Tests for rxflags.Compile: valid patterns behave like regexp.Compile with the prefix, and errors for broken
// patterns never show the program's internal flag prefix (board issue #96).

package rxflags

import (
	"regexp/syntax"
	"strings"
	"testing"
)

func TestCompileValidKeepsFlags(t *testing.T) {
	re, err := Compile("(?i)", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("xABCx") {
		t.Error("(?i) flag was not applied")
	}
	re, err = Compile("", "abc")
	if err != nil || re.MatchString("ABC") {
		t.Errorf("no flags should stay case-sensitive: %v", err)
	}
}

func TestCompileErrorHidesPrefix(t *testing.T) {
	for _, flags := range []string{"(?i)", "(?i)(?m)", "(?m)"} {
		for _, expr := range []string{"(", "a(b", "a)", "[a", "*", "a{2,1}"} {
			_, err := Compile(flags, expr)
			if err == nil {
				t.Fatalf("%q%q compiled", flags, expr)
			}
			if strings.Contains(err.Error(), "(?") {
				t.Errorf("%q%q: error leaks flags: %v", flags, expr, err)
			}
			if _, ok := err.(*syntax.Error); !ok {
				t.Errorf("%q%q: want *syntax.Error, got %T", flags, expr, err)
			}
		}
	}
	_, err := Compile("(?i)", "(")
	if se := err.(*syntax.Error); se.Code != syntax.ErrMissingParen || se.Expr != "(" {
		t.Errorf("got %v / %q", se.Code, se.Expr)
	}
}
