// Tests for mask expressions: separators, quotes, exclusions, folder masks, saved filters and case rules.

package masks

import (
	"errors"
	"testing"
)

func TestListMatch(t *testing.T) {
	type probe struct {
		name  string
		isDir bool
		want  bool
	}
	tests := []struct {
		expr   string
		probes []probe
	}{
		{"", []probe{{"a", false, true}, {"d", true, true}}},
		{"*.jpg;*.png *.gif", []probe{{"a.jpg", false, true}, {"b.png", false, true}, {"c.gif", false, true}, {"d.bmp", false, false}}},
		{`"my file*.txt";x`, []probe{{"my file 2.txt", false, true}, {"x", false, true}, {"my", false, false}}},
		{"*.txt|read*", []probe{{"a.txt", false, true}, {"readme.txt", false, false}}},
		{"|*.bak", []probe{{"a.txt", false, true}, {"a.bak", false, false}, {"dir", true, true}}},
		{"*|.git\\", []probe{{".git", true, false}, {".git", false, true}, {"src", true, true}}},
		{"src/", []probe{{"src", true, true}, {"src", false, false}}},
		{"*.c|*.h|x*", []probe{{"a.c", false, true}, {"a.h", false, false}, {"x.c", false, false}}},
		{" ;; *.go ; ", []probe{{"a.go", false, true}, {"a.md", false, false}}},
		{`""`, []probe{{"a", false, true}}},
	}
	for _, tt := range tests {
		l, err := Parse(tt.expr, Options{Case: CaseSensitive})
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.expr, err)
		}
		for _, p := range tt.probes {
			if got := l.Match(p.name, p.isDir); got != p.want {
				t.Errorf("Parse(%q).Match(%q, dir=%v) = %v, want %v", tt.expr, p.name, p.isDir, got, p.want)
			}
		}
	}
}

func TestListCase(t *testing.T) {
	ci := MustParse("*.JPG", Options{Case: CaseInsensitive})
	cs := MustParse("*.JPG", Options{Case: CaseSensitive})
	if !ci.Match("a.jpg", false) || cs.Match("a.jpg", false) {
		t.Error("case rule not applied")
	}
	def := MustParse("*.JPG", Options{})
	if def.CaseSensitive() != DefaultCaseSensitive || def.Match("a.jpg", false) == DefaultCaseSensitive {
		t.Error("default case rule not applied")
	}
}

func TestListErrors(t *testing.T) {
	if _, err := Parse(`"open`, Options{}); !errors.Is(err, ErrUnclosedQuote) {
		t.Errorf("unclosed quote: %v", err)
	}
	if _, err := Parse(">pics", Options{}); err == nil {
		t.Error("saved filter without resolver must fail")
	}
	res := func(n string) (string, bool) { return map[string]string{"loop": ">loop"}[n], n == "loop" }
	if _, err := Parse(">missing", Options{Resolve: res}); err == nil {
		t.Error("unknown saved filter must fail")
	}
	if _, err := Parse(">loop", Options{Resolve: res}); err == nil {
		t.Error("recursive saved filter must fail")
	}
}

func TestListSavedFilters(t *testing.T) {
	saved := map[string]string{"pics": "*.jpg;*.png|thumb*", "junk": "*.tmp *.bak"}
	res := func(n string) (string, bool) { s, ok := saved[n]; return s, ok }
	l, err := Parse(">pics;*.gif|>junk", Options{Case: CaseSensitive, Resolve: res})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"a.jpg": true, "a.gif": true, "thumb.jpg": false, "a.txt": false, "a.tmp": false} {
		if got := l.Match(name, false); got != want {
			t.Errorf("Match(%q) = %v, want %v", name, got, want)
		}
	}
	// A saved filter on the exclude side excludes what it includes, here *.tmp.
	l2 := MustParse("*|>junk", Options{Case: CaseSensitive, Resolve: res})
	if l2.Match("x.tmp", false) || !l2.Match("x.txt", false) {
		t.Error("exclude-side saved filter")
	}
}

func TestMatchesAllAndExcludesDir(t *testing.T) {
	for expr, want := range map[string]bool{"": true, "*": true, "*.*": true, "*.txt": false, "*|a": false, "*/": false} {
		if got := MustParse(expr, Options{}).MatchesAll(); got != want {
			t.Errorf("MatchesAll(%q) = %v, want %v", expr, got, want)
		}
	}
	l := MustParse("*|.git\\ node_modules", Options{Case: CaseSensitive})
	if !l.ExcludesDir(".git") || !l.ExcludesDir("node_modules") || l.ExcludesDir("src") {
		t.Error("ExcludesDir")
	}
	if ok, err := Match("*.go", "main.go"); !ok || err != nil {
		t.Errorf("Match: %v %v", ok, err)
	}
}
