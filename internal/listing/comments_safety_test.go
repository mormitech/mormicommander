// Regression tests for descript.ion saving: a save must keep the comment
// file's permission bits (a shared folder's comments stay readable for other
// users), and names that start with or contain a double quote, which are
// legal on Unix, must survive a save and reload.
package listing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommentsSaveKeepsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	dir := t.TempDir()
	c := NewComments()
	c.Set("a.txt", "one")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, CommentFileName)
	if err := os.Chmod(p, 0o664); err != nil {
		t.Fatal(err)
	}
	c.Set("b.txt", "two")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o664 {
		t.Fatalf("mode after save = %v, want 0664", st.Mode().Perm())
	}
}

func TestCommentsQuoteNamesRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("names with quotes are not allowed on Windows")
	}
	names := []string{`"quoted`, `"quoted"`, `a"b`, `"a b`, `x y"`, `plain`, `with space`,
		`a" b`, `a"	b`, `x" y" z`, `a"" b`, `""`, `" "`, `end"`, `q"" `}
	dir := t.TempDir()
	c := NewComments()
	for i, n := range names {
		c.Set(n, "c"+string(rune('0'+i)))
	}
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	r, err := LoadComments(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		if got, want := r.Get(n), "c"+string(rune('0'+i)); got != want {
			t.Errorf("Get(%q) = %q, want %q", n, got, want)
		}
	}
}

func TestParseCommentsQuoteRules(t *testing.T) {
	c := ParseComments([]byte("\"a b\" one\r\n\"open two\r\n\"x\"y three\r\n\"only\"\r\n"))
	cases := map[string]string{"a b": "one", `"open`: "two", `"x"y`: "three", "only": ""}
	for n, want := range cases {
		if got := c.Get(n); got != want {
			t.Errorf("Get(%q) = %q, want %q", n, got, want)
		}
	}
}

// TestCommentsQuoteEscaping pins the written form: only a quote that would
// end the name early (followed by a space, a tab, another quote or the end of
// the name) is doubled, so names that always read back keep their old form.
func TestCommentsQuoteEscaping(t *testing.T) {
	cases := map[string]string{
		`with space`: `"with space" c`,
		`"quoted`:    `""quoted" c`,
		`a" b`:       `"a"" b" c`,
		`x y"`:       `"x y""" c`,
		`plain`:      `plain c`,
		`a"b`:        `a"b c`,
	}
	for name, want := range cases {
		c := NewComments()
		c.Set(name, "c")
		if got := strings.TrimSuffix(string(c.Bytes()), "\r\n"); got != want {
			t.Errorf("Bytes for %q = %q, want %q", name, got, want)
		}
		if got := ParseComments(c.Bytes()).Get(name); got != "c" {
			t.Errorf("round trip of %q = %q", name, got)
		}
	}
}

// TestParseCommentsOldQuoteForm checks that lines written before quotes were
// doubled still read back the same way.
func TestParseCommentsOldQuoteForm(t *testing.T) {
	c := ParseComments([]byte("\"x y\"\" one\r\n\"\"quoted\"\" two\r\n"))
	for n, want := range map[string]string{`x y"`: "one", `"quoted"`: "two"} {
		if got := c.Get(n); got != want {
			t.Errorf("Get(%q) = %q, want %q", n, got, want)
		}
	}
}
