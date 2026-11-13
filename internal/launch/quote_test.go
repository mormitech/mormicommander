// Tests for command line splitting and quoting in the three dialects.
package launch

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		err  bool
	}{
		{"", nil, false},
		{"  a  b\tc ", []string{"a", "b", "c"}, false},
		{`"a b" c`, []string{"a b", "c"}, false},
		{`'a "b"' c`, []string{`a "b"`, "c"}, false},
		{`a\ b`, []string{"a b"}, false},
		{`"a\"b"`, []string{`a"b`}, false},
		{`'a\b'`, []string{`a\b`}, false},
		{`x""`, []string{"x"}, false},
		{`""`, []string{""}, false},
		{`pre"mid dle"post`, []string{"premid dlepost"}, false},
		{`"open`, nil, true},
		{`'open`, nil, true},
	}
	for _, tt := range tests {
		got, err := SplitArgs(tt.in)
		if (err != nil) != tt.err {
			t.Errorf("SplitArgs(%q) error = %v, want error %v", tt.in, err, tt.err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitArgs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeArgSurvivesSplitting(t *testing.T) {
	for _, s := range []string{"plain", "with space", `quo"te`, `back\slash`, "it's", "tab\there", "", "  "} {
		for _, wrap := range []string{"%s", `"%s"`, "pre%spost"} {
			var text string
			switch wrap {
			case "%s":
				text = EscapeArg(s)
			case `"%s"`:
				text = `"` + EscapeArg(s) + `"`
			default:
				text = "pre" + EscapeArg(s) + "post"
			}
			got, err := SplitArgs(text)
			if err != nil {
				t.Fatalf("SplitArgs(%q): %v", text, err)
			}
			want := s
			if wrap == "pre%spost" {
				want = "pre" + s + "post"
			}
			if s == "" && wrap == "%s" {
				if len(got) != 0 {
					t.Errorf("empty escape gave %q", got)
				}
				continue
			}
			if len(got) != 1 || got[0] != want {
				t.Errorf("escape %q in %s: split gave %q", s, wrap, got)
			}
		}
	}
}

func TestQuoteWindowsArgRoundTrip(t *testing.T) {
	tests := []struct{ in, quoted string }{
		{`C:\x\y.txt`, `C:\x\y.txt`},
		{`C:\Program Files\a.exe`, `"C:\Program Files\a.exe"`},
		{``, `""`},
		{`a"b`, `"a\"b"`},
		{`C:\dir with space\`, `"C:\dir with space\\"`},
		{`a\\"b`, `"a\\\\\"b"`},
	}
	for _, tt := range tests {
		q := QuoteWindowsArg(tt.in)
		if q != tt.quoted {
			t.Errorf("QuoteWindowsArg(%q) = %s, want %s", tt.in, q, tt.quoted)
		}
		back := SplitWindowsArgs("prog " + q + " next")
		if len(back) != 3 || back[1] != tt.in || back[2] != "next" {
			t.Errorf("round trip of %q gave %q", tt.in, back)
		}
	}
}

func TestSplitWindowsArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`a b  c`, []string{"a", "b", "c"}},
		{`"C:\Program Files\x.exe" /k`, []string{`C:\Program Files\x.exe`, "/k"}},
		{`a\\b`, []string{`a\\b`}},
		{`"a""b"`, []string{`a"b`}},
		{`\"x`, []string{`"x`}},
	}
	for _, tt := range tests {
		if got := SplitWindowsArgs(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitWindowsArgs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShellQuoteAndQuoteName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abc", "abc"},
		{"/usr/bin/x-1.2", "/usr/bin/x-1.2"},
		{"a b", "'a b'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
		{"$HOME", "'$HOME'"},
	}
	for _, tt := range tests {
		if got := ShellQuote(tt.in); got != tt.want {
			t.Errorf("ShellQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
	if got := QuoteName("windows", "My File.txt"); got != `"My File.txt"` {
		t.Errorf("QuoteName windows = %s", got)
	}
	if got := QuoteName("windows", "a&b.txt"); got != "a&b.txt" {
		t.Errorf("QuoteName windows without space = %s", got)
	}
	if got := QuoteName("linux", "a&b.txt"); got != "'a&b.txt'" {
		t.Errorf("QuoteName linux = %s", got)
	}
}
