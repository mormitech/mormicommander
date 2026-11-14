// Tests of the Marta favourites reader: the loose Marco parser, the several shapes a favourites list may have, "~" and
// file:// folders, the "favorites" block of conf.marco, and that Marta's files are only read, never changed. Every
// file is written to a temporary folder; nothing of a real Marta installation is touched.
package marta

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseMarco(t *testing.T) {
	v := Parse(`
-- a comment
behavior {
    theme "Classic"   # another comment
    showHidden true
}
keyBindings {
    "Cmd+Shift+N" "core.new.folder"
}
list [1, 2, "three"]
`)
	if v.Kind != Object {
		t.Fatalf("kind %v", v.Kind)
	}
	b := v.Get("behavior")
	if b == nil || b.Get("theme") == nil || b.Get("theme").Text != "Classic" || b.Get("showHidden").Text != "true" {
		t.Fatalf("behavior: %+v", b)
	}
	if k := v.Get("keyBindings"); k == nil || k.Get("Cmd+Shift+N").Text != "core.new.folder" {
		t.Fatalf("keyBindings: %+v", k)
	}
	if l := v.Get("list"); l == nil || l.Kind != List || len(l.Items) != 3 || l.Items[2].Text != "three" || !l.Items[2].Quoted {
		t.Fatalf("list: %+v", l)
	}
	// broken input never panics and keeps what it can
	for _, src := range []string{"", "}", "[", `a "unclosed`, "{ a { b ] c", "key"} {
		Parse(src)
	}
	if v := Parse(`a "x\"y\\z"`); v.Get("a").Text != `x"y\z` {
		t.Fatalf("escapes: %q", v.Get("a").Text)
	}
}

func TestCollectShapes(t *testing.T) {
	home := "/Users/me"
	cases := []struct {
		name string
		src  string
		want []Favorite
	}{
		{"list of blocks", `[ { name "Projects" path "~/Code" } { title "Logs" url "file:///var/log/" } ]`,
			[]Favorite{{"Projects", "/Users/me/Code"}, {"Logs", "/var/log"}}},
		{"list of folders", `["/tmp", "~", "~/Downloads/"]`,
			[]Favorite{{"", "/tmp"}, {"", "/Users/me"}, {"", "/Users/me/Downloads"}}},
		{"name and folder pairs", `favorites { "Work Stuff" "/srv/work" home "~" }`,
			[]Favorite{{"Work Stuff", "/srv/work"}, {"home", "/Users/me"}}},
		{"no folders", `[ "word", 3, "https://example.com/x", "relative/path", "file://otherhost/x" ]`, nil},
		{"duplicates once", `[ "/a" "/a/" { path "/a" } ]`, []Favorite{{"", "/a"}}},
	}
	for _, c := range cases {
		got := Collect(Parse(c.src), home)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	// without a home folder "~" can't be expanded and is left out
	if got := Collect(Parse(`["~/x", "/y"]`), ""); !reflect.DeepEqual(got, []Favorite{{"", "/y"}}) {
		t.Errorf("no home: %+v", got)
	}
}

func TestFavoritesFromFolder(t *testing.T) {
	home := t.TempDir()
	dir := Dir(home)
	if Found(dir) {
		t.Fatal("no Marta folder yet")
	}
	if got, err := Favorites(dir, home); err != nil || len(got) != 0 {
		t.Fatalf("missing folder: %+v %v", got, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !Found(dir) {
		t.Fatal("the Marta folder should be found")
	}
	fav := `[ { name "Code" path "~/Code" } "/opt" ]`
	conf := `behavior { theme "Classic" startPath "/not/a/favorite" }
favorites [ "/opt" "/srv" ]`
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		os.Chtimes(p, old, old)
		return p
	}
	fp, cp := write(FavoritesFile, fav), write(ConfigFile, conf)
	before := map[string]os.FileInfo{}
	for _, p := range []string{fp, cp} {
		before[p], _ = os.Stat(p)
	}

	got, err := Favorites(dir, home)
	if err != nil {
		t.Fatal(err)
	}
	want := []Favorite{{"Code", filepath.Join(home, "Code")}, {"", "/opt"}, {"", "/srv"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	// Marta's files are unchanged: same content, same size, same time
	for p, st := range before {
		now, err := os.Stat(p)
		if err != nil || now.Size() != st.Size() || !now.ModTime().Equal(st.ModTime()) {
			t.Errorf("%s changed", p)
		}
	}
	if b, _ := os.ReadFile(fp); string(b) != fav {
		t.Error("favorites.marco was changed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("files were added to Marta's folder: %d", len(entries))
	}
}
