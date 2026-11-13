// Tests for descript.ion parsing and writing.
package listing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseComments(t *testing.T) {
	data := "\xEF\xBB\xBFreadme.txt Read this first\r\n" +
		"\"my file.doc\" Quoted name\r\n" +
		"multi.txt first\\nsecond" + multiLineMark + "\r\n" +
		"recoded.txt a\\nb\x04\xC3\x82\r\n" +
		"latin.txt caf\xE9\r\n" +
		"\r\n" +
		"bare.txt\r\n" +
		"tab.txt\tTabbed\n"
	c := ParseComments([]byte(data))
	tests := map[string]string{
		"README.TXT":  "Read this first",
		"my file.doc": "Quoted name",
		"multi.txt":   "first\nsecond",
		"recoded.txt": "a\nb",
		"latin.txt":   "café",
		"bare.txt":    "",
		"tab.txt":     "Tabbed",
		"missing":     "",
	}
	for name, want := range tests {
		if got := c.Get(name); got != want {
			t.Errorf("Get(%q) = %q want %q", name, got, want)
		}
	}
}

func TestCommentsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadComments(dir)
	if err != nil || c.Len() != 0 {
		t.Fatalf("missing file: %v %d", err, c.Len())
	}
	c.Set("a.txt", "plain")
	c.Set("with space.txt", "quoted")
	c.Set("multi.txt", "one\ntwo")
	c.Set("ü.txt", "unicode")
	c.Rename("a.txt", "b.txt")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, CommentFileName))
	if string(raw[:3]) != "\xEF\xBB\xBF" {
		t.Error("non-ASCII content should get a BOM")
	}
	d, err := LoadComments(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"b.txt": "plain", "a.txt": "", "with space.txt": "quoted", "multi.txt": "one\ntwo", "Ü.TXT": "unicode"} {
		if got := d.Get(name); got != want {
			t.Errorf("after reload Get(%q) = %q want %q", name, got, want)
		}
	}
	if d.Get("ü.txt") != "unicode" {
		t.Error("unicode name lost")
	}
	for _, n := range []string{"b.txt", "with space.txt", "multi.txt", "ü.txt"} {
		d.Set(n, "")
	}
	if err := d.Save(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, CommentFileName)); !os.IsNotExist(err) {
		t.Error("empty table should remove the file")
	}
	if err := d.Save(dir); err != nil {
		t.Errorf("removing an absent file: %v", err)
	}
}

func TestApplyComments(t *testing.T) {
	c := NewComments()
	c.Set("x", "hello")
	items := []Item{UpItem("/"), {Name: "X"}, {Name: "y"}}
	ApplyComments(items, c)
	if items[1].Comment != "hello" || items[2].Comment != "" || items[0].Comment != "" {
		t.Errorf("%+v", items)
	}
}
