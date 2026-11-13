// Tests for "<regex" masks (spec area-3 mask syntax rule 5 and rule 9) and for the platform-independent default case
// rule (rule 6): masks ignore case on every platform, Linux included.
package masks

import (
	"errors"
	"strings"
	"testing"
)

func TestRegexMasks(t *testing.T) {
	cases := []struct {
		expr, name string
		want       bool
	}{
		{`<^img_\d{4}\.jpe?g$`, "IMG_0042.JPG", true},
		{`<^img_\d{4}\.jpe?g$`, "IMG_42.jpg", false},
		{`<report`, "annual_report.pdf", true},
		{`<\.(txt|md)$`, "notes.md", true}, // "|" is alternation here, not the exclude separator
		{`<\.(txt|md)$`, "notes.doc", false},
		{`<a b`, "xa by", true},            // spaces belong to the expression
		{"<^árok", "a\u0301rok.txt", true}, // NFD names are normalised before matching
		{`  <^x$`, "X", true},              // leading blanks are ignored
	}
	for _, c := range cases {
		l, err := Parse(c.expr, Options{})
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.expr, err)
		}
		if got := l.Match(c.name, false); got != c.want {
			t.Errorf("%q matching %q = %v, want %v", c.expr, c.name, got, c.want)
		}
	}
	cs := MustParse(`<^A`, Options{Case: CaseSensitive})
	if cs.Match("abc", false) || !cs.Match("Abc", false) {
		t.Error("case-sensitive regex ignored the case rule")
	}
	m, _ := CompileRegexMask("x+", false)
	if !m.IsRegex() || !m.HasWildcards() || m.String() != "<x+" {
		t.Errorf("regex mask accessors: %v %v %q", m.IsRegex(), m.HasWildcards(), m.String())
	}
}

func TestRegexInvalid(t *testing.T) {
	_, err := Parse("<a(b", Options{})
	var re *RegexpError
	if !errors.As(err, &re) {
		t.Fatalf("want RegexpError, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "Invalid regular expression: ") {
		t.Errorf("message %q", err.Error())
	}
}

func TestDefaultCaseInsensitiveEverywhere(t *testing.T) {
	if DefaultCaseSensitive {
		t.Fatal("masks must ignore case by default on every platform")
	}
	if !MustParse("*.TXT", Options{}).Match("a.txt", false) {
		t.Error("*.TXT should select a.txt with CaseDefault")
	}
}
