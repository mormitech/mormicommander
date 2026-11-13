// Tests for the intra-line (word and character) diff.
package compare

import (
	"strings"
	"testing"
)

// render marks changed spans with [brackets] for readable expectations.
func render(s string, spans []Span) string {
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp.Start != pos {
			return "GAP"
		}
		if sp.Changed {
			b.WriteString("[" + s[sp.Start:sp.End] + "]")
		} else {
			b.WriteString(s[sp.Start:sp.End])
		}
		pos = sp.End
	}
	if pos != len(s) {
		return "SHORT"
	}
	return b.String()
}

func TestInlineDiff(t *testing.T) {
	tests := []struct {
		name         string
		a, b         string
		opts         Options
		wantA, wantB string
	}{
		{"word change", "the quick fox", "the slow fox", Options{}, "the [quick] fox", "the [slow] fox"},
		{"word insert", "a b", "a x b", Options{}, "a b", "a [x ]b"},
		{"identical", "same", "same", Options{}, "same", "same"},
		{"empty left", "", "new", Options{}, "", "[new]"},
		{"char mode", "colour", "color", Options{Inline: InlineChars}, "colo[u]r", "color"},
		{"char typo", "recieve", "receive", Options{Inline: InlineChars}, "rec[ie]ve", "rec[ei]ve"},
		{"ignore case words", "Hello World", "hello there", Options{IgnoreCase: true}, "Hello [World]", "hello [there]"},
		{"ws ignored", "a  b", "a b", Options{Whitespace: WhitespaceIgnoreChange}, "a  b", "a b"},
		{"ws exact", "a  b", "a b", Options{}, "a[  ]b", "a[ ]b"},
		{"punctuation", "f(x, y)", "f(x; y)", Options{}, "f(x[,] y)", "f(x[;] y)"},
		{"islands absorbed", "one two three", "uno dos tres", Options{}, "[one two three]", "[uno dos tres]"},
		{"utf8 chars", "árvíz", "árvűz", Options{Inline: InlineChars}, "árv[í]z", "árv[ű]z"},
		{"none mode", "abc", "abd", Options{Inline: InlineNone}, "[abc]", "[abd]"},
		{"none mode equal", "abc", "ABC", Options{Inline: InlineNone, IgnoreCase: true}, "abc", "ABC"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sa, sb := InlineDiff(tc.a, tc.b, tc.opts)
			if got := render(tc.a, sa); got != tc.wantA {
				t.Errorf("left: got %q want %q", got, tc.wantA)
			}
			if got := render(tc.b, sb); got != tc.wantB {
				t.Errorf("right: got %q want %q", got, tc.wantB)
			}
		})
	}
}
