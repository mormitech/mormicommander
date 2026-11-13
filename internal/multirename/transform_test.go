// These tests cover the text transformations applied after the masks:
// search and replace in all its modes, the case drop-down, accent removal,
// and the order in which these steps run.

package multirename

import (
	"testing"
)

func TestSearchReplace(t *testing.T) {
	cases := []struct {
		name                        string
		search, replace             string
		regex, caseSens, first, ext bool
		in, want                    string
	}{
		{"plain", "foo", "bar", false, false, false, false, "foo-foo.txt", "bar-bar.txt"},
		{"ignore case", "FOO", "bar", false, false, false, false, "Foo-foo.txt", "bar-bar.txt"},
		{"case sensitive", "FOO", "bar", false, true, false, false, "Foo-FOO.txt", "Foo-bar.txt"},
		{"only first", "o", "0", false, false, true, false, "foo.txt", "f0o.txt"},
		{"not in ext", "t", "T", false, false, false, false, "test.txt", "TesT.txt"},
		{"in ext", "t", "T", false, false, false, true, "test.txt", "TesT.TxT"},
		{"remove dot via ext", ".", "_", false, false, false, true, "a.b.c", "a_b_c"},
		{"pairs", "a|b", "x|y", false, false, false, false, "abba.txt", "xyyx.txt"},
		{"pairs no cascade", "a|x", "x|z", false, false, false, false, "ax.txt", "xz.txt"},
		{"one repl for all", "a|b", "-", false, false, false, false, "abc.txt", "--c.txt"},
		{"fewer repls", "a|b|c", "1|2", false, false, false, false, "abc.txt", "12.txt"},
		{"empty alt skipped", "a||b", "1||2", false, false, false, false, "ab.txt", "12.txt"},
		{"wildcard star", "(*)", "", false, false, false, false, "song (live).mp3", "song .mp3"},
		{"wildcard q", "img?", "pic", false, false, false, false, "img1 img22.jpg", "pic pic2.jpg"},
		{"regex meta in plain", "a+b", "c", false, false, false, false, "a+b aab.txt", "c aab.txt"},
		{"delete", "_", "", false, false, false, false, "a_b_c.txt", "abc.txt"},
		{"regex groups", `(\d+)-(\w+)`, "$2_$1", true, false, false, false, "12-ab.txt", "ab_12.txt"},
		{"regex $1x", `(a)`, "$1x", true, false, false, false, "a.txt", "ax.txt"},
		{"regex named", `(?P<n>\d+)`, "#${n}", true, false, false, false, "x5.txt", "x#5.txt"},
		{"regex alternation", `foo|bar`, "z", true, false, false, false, "foobar.txt", "zz.txt"},
		{"regex case", `ABC`, "x", true, true, false, false, "abcABC.txt", "abcx.txt"},
		{"regex first", `\d`, "#", true, false, true, false, "a1b2.txt", "a#b2.txt"},
		{"regex anchor", `^`, "pre_", true, false, false, false, "a.txt", "pre_a.txt"},
		{"regex ext", `\.txt$`, ".md", true, false, false, true, "a.txt", "a.md"},
		{"no match", "zzz", "y", false, false, false, false, "a.txt", "a.txt"},
		{"accented ignore case", "Á", "a", false, false, false, false, "ár.txt", "ar.txt"},
	}
	for _, c := range cases {
		s := DefaultSettings()
		s.Search, s.Replace, s.RegEx, s.CaseSensitive, s.OnlyFirst, s.IncludeExt =
			c.search, c.replace, c.regex, c.caseSens, c.first, c.ext
		got := renderOne(t, s, []Item{newItem(c.in)}, 0)
		if got != c.want {
			t.Errorf("%s: %q -> %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestBadRegex(t *testing.T) {
	s := DefaultSettings()
	s.Search, s.RegEx = "(", true
	_, err := Compile(s)
	se, ok := err.(*SettingsError)
	if !ok || se.Field != "search" || se.Unwrap() == nil {
		t.Errorf("want search SettingsError, got %v", err)
	}
	s.RegEx = false // the same text is fine as a plain search
	if _, err := Compile(s); err != nil {
		t.Errorf("plain '(' failed: %v", err)
	}
	s.Search = "|"
	r, err := Compile(s)
	if err != nil || r.repl != nil {
		t.Errorf("all-empty alternatives should mean no replacer: %v", err)
	}
}

func TestApplyCase(t *testing.T) {
	cases := []struct {
		mode     CaseMode
		in, want string
	}{
		{CaseUnchanged, "MiXed Case.TXT", "MiXed Case.TXT"},
		{CaseLower, "MiXed ÁRVÍZ.TXT", "mixed árvíz.txt"},
		{CaseUpper, "mixed árvíz.txt", "MIXED ÁRVÍZ.TXT"},
		{CaseFirstUpper, "hELLO wORLD.TXT", "Hello world.txt"},
		{CaseFirstUpper, "01 track.mp3", "01 Track.mp3"},
		{CaseEachWord, "the QUICK brown_fox-jumps.txt", "The Quick Brown_Fox-Jumps.Txt"},
		{CaseEachWord, "o'neil's song", "O'neil's Song"},
		{CaseEachWord, "élet és irodalom", "Élet És Irodalom"},
		{CaseEachWord, "2nd place", "2nd Place"},
		{CaseMode("bogus"), "aB", "aB"},
	}
	for _, c := range cases {
		if got := ApplyCase(c.in, c.mode); got != c.want {
			t.Errorf("%q %q = %q, want %q", c.mode, c.in, got, c.want)
		}
	}
}

func TestRemoveAccents(t *testing.T) {
	cases := []struct{ in, want string }{
		{"árvíztűrő tükörfúrógép", "arvizturo tukorfurogep"},
		{"ÁRVÍZTŰRŐ TÜKÖRFÚRÓGÉP", "ARVIZTURO TUKORFUROGEP"},
		{"Straße Æsir Œuvre Łódź", "Strasse AEsir OEuvre Lodz"},
		{"crème brûlée façade naïve", "creme brulee facade naive"},
		{"Dvořák Škoda Čech", "Dvorak Skoda Cech"},
		{"árvíz", "arviz"}, // decomposed, as macOS stores names
		{"Ελλάδα Москва 東京", "Ελλάδα Москва 東京"},
		{"plain ascii 123", "plain ascii 123"},
	}
	for _, c := range cases {
		if got := RemoveAccents(c.in); got != c.want {
			t.Errorf("RemoveAccents(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPipelineOrder(t *testing.T) {
	// Search and replace sees the masked name, case runs after it, and
	// accent removal runs last so it also cleans replacement text.
	s := DefaultSettings()
	s.NameMask = "[N]_[C]"
	s.Search, s.Replace = "_1", "_Één"
	s.Case = CaseUpper
	s.RemoveAccents = true
	got := renderOne(t, s, []Item{newItem("ár.txt")}, 0)
	if got != "AR_EEN.TXT" {
		t.Errorf("got %q", got)
	}
}
