// Regression tests for descript.ion saving: a save must keep the comment
// file's permission bits (a shared folder's comments stay readable for other
// users), and names that start with or contain a double quote, which are
// legal on Unix, must survive a save and reload.
package listing

import (
	"os"
	"path/filepath"
	"runtime"
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
	names := []string{`"quoted`, `"quoted"`, `a"b`, `"a b`, `x y"`, `plain`, `with space`}
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
