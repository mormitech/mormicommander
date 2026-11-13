// Tests for quoting and splitting: every quoted string must split back to
// the original argument, in both styles, including the awkward cases
// (trailing backslashes on Windows, embedded quotes, empty arguments).
package params

import (
	"errors"
	"reflect"
	"testing"
)

func TestNeedsQuote(t *testing.T) {
	cases := []struct {
		s    string
		st   Style
		want bool
	}{
		{"plain.txt", StyleWindows, false},
		{"my file.txt", StyleWindows, true},
		{"", StyleWindows, true},
		{"a&b", StyleWindows, false}, // TC only quotes blanks
		{`a"b`, StyleWindows, true},
		{"plain.txt", StylePOSIX, false},
		{"/usr/local/bin", StylePOSIX, false},
		{"my file", StylePOSIX, true},
		{"a&b", StylePOSIX, true},
		{"it's", StylePOSIX, true},
		{"$HOME", StylePOSIX, true},
		{"árvíztűrő.txt", StylePOSIX, false},
		{"~x", StylePOSIX, true},
	}
	for _, c := range cases {
		if got := NeedsQuote(c.s, c.st); got != c.want {
			t.Errorf("NeedsQuote(%q,%d) = %v, want %v", c.s, c.st, got, c.want)
		}
	}
}

func TestQuote(t *testing.T) {
	cases := []struct {
		s    string
		st   Style
		want string
	}{
		{"a b", StyleWindows, `"a b"`},
		{`C:\My dir\`, StyleWindows, `"C:\My dir\\"`},
		{`a"b c`, StyleWindows, `"a\"b c"`},
		{`x\"y z`, StyleWindows, `"x\\\"y z"`},
		{"ab", StyleWindows, "ab"},
		{"", StyleWindows, `""`},
		{"a b", StylePOSIX, `'a b'`},
		{"it's", StylePOSIX, `'it'\''s'`},
		{"", StylePOSIX, `''`},
	}
	for _, c := range cases {
		if got := Quote(c.s, c.st); got != c.want {
			t.Errorf("Quote(%q) = %s, want %s", c.s, got, c.want)
		}
	}
}

func TestSplitWindows(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`a b  c`, []string{"a", "b", "c"}},
		{`"a b" c`, []string{"a b", "c"}},
		{`a\b c\\d`, []string{`a\b`, `c\\d`}},
		{`"C:\My dir\\" x`, []string{`C:\My dir\`, "x"}},
		{`a\"b`, []string{`a"b`}},
		{`a\\\"b`, []string{`a\"b`}},
		{`"a""b"`, []string{`a"b`}},
		{`""`, []string{""}},
		{`"open`, []string{"open"}},
		{`x"y z"w`, []string{"xy zw"}},
		{"   ", nil},
	}
	for _, c := range cases {
		got, err := SplitArgs(c.in, StyleWindows)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitArgs(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestSplitPOSIX(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		err  error
	}{
		{`a b  c`, []string{"a", "b", "c"}, nil},
		{`'a b' c`, []string{"a b", "c"}, nil},
		{`"a \"b\" \$x \q"`, []string{`a "b" $x \q`}, nil},
		{`a\ b`, []string{"a b"}, nil},
		{`'it'\''s'`, []string{"it's"}, nil},
		{`''`, []string{""}, nil},
		{`x'y z'w`, []string{"xy zw"}, nil},
		{`'open`, nil, ErrUnterminatedQuote},
		{`"open`, nil, ErrUnterminatedQuote},
		{`trail\`, nil, ErrUnterminatedQuote},
	}
	for _, c := range cases {
		got, err := SplitArgs(c.in, StylePOSIX)
		if !errors.Is(err, c.err) || !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitArgs(%q) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestQuoteRoundTrip(t *testing.T) {
	args := []string{"plain", "with space", `C:\dir with space\`, `back\slash`, "", `q"uote`,
		"it's", "$HOME", "tab\there", `\\server\share\`, "ünï cödé", `end\\`}
	for _, st := range []Style{StyleWindows, StylePOSIX} {
		line := JoinArgs(args, st)
		got, err := SplitArgs(line, st)
		if err != nil || !reflect.DeepEqual(got, args) {
			t.Errorf("style %d: round trip of %s = %q, %v", st, line, got, err)
		}
	}
}

func TestInsertName(t *testing.T) {
	cases := []struct {
		line, name string
		st         Style
		want       string
	}{
		{"", "a.txt", StyleWindows, "a.txt"},
		{"notepad", "my file.txt", StyleWindows, `notepad "my file.txt"`},
		{"notepad ", "b.txt", StyleWindows, "notepad b.txt"},
		{`"a b"`, "c d", StyleWindows, `"a b" "c d"`},
		{"vi", "my file", StylePOSIX, "vi 'my file'"},
	}
	for _, c := range cases {
		if got := InsertName(c.line, c.name, c.st); got != c.want {
			t.Errorf("InsertName(%q,%q) = %q, want %q", c.line, c.name, got, c.want)
		}
	}
}

func TestDefaultStyleSep(t *testing.T) {
	if Style(0).orDefault() != DefaultStyle {
		t.Fatal("zero style must mean DefaultStyle")
	}
	if StyleWindows.Sep() != `\` || StylePOSIX.Sep() != "/" {
		t.Fatal("wrong separators")
	}
}
