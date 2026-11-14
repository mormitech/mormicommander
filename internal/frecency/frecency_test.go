// Tests of the frecency ranking: zoxide's binary and text databases are read (and damaged ones refused without a
// crash), zoxide's recency factors are applied, the database is looked for where zoxide keeps it, and the app's own
// folders and zoxide's merge into one list ranked best first.
package frecency

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// zoxideFile builds a db.zo the way zoxide writes it (bincode, fixed-width little-endian integers).
func zoxideFile(entries []Entry) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w(uint32(3))
	w(uint64(len(entries)))
	for _, e := range entries {
		w(uint64(len(e.Path)))
		b.WriteString(e.Path)
		w(e.Rank)
		w(uint64(e.LastAccessed))
	}
	return b.Bytes()
}

func TestParseZoxideBinary(t *testing.T) {
	in := []Entry{{"/home/a/src", 12.5, 1700000000}, {"/home/a/my docs|x", 1, 1700000100}, {"", 3, 1}, {"/bad", 0, 1}}
	got, err := ParseZoxide(zoxideFile(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Fatalf("entries: %+v", got)
	}
	if got, err := ParseZoxide(nil); err != nil || got != nil {
		t.Fatalf("an empty file: %v %v", got, err)
	}
}

func TestParseZoxideDamaged(t *testing.T) {
	full := zoxideFile([]Entry{{"/home/a/src", 2, 1}})
	for n := 5; n < len(full); n++ {
		if _, err := ParseZoxide(full[:n]); err == nil {
			t.Fatalf("a file cut at %d bytes was read without an error", n)
		}
	}
	// a count far too big must not allocate it
	huge := append([]byte{3, 0, 0, 0}, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f)
	if _, err := ParseZoxide(huge); err == nil {
		t.Fatal("a huge count was accepted")
	}
	if _, err := ParseZoxide([]byte{0, 1, 2, 3, 4, 0, 9}); err == nil {
		t.Fatal("binary junk was accepted")
	}
}

func TestParseZoxideText(t *testing.T) {
	text := "/home/a/src|10|1700000000\r\n\n/home/a/odd|name|2.5|1700000001\nbroken line\n/x|nan|1\n"
	got, err := ParseZoxide([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "/home/a/src" || got[0].Rank != 10 || got[1].Path != "/home/a/odd|name" || got[1].LastAccessed != 1700000001 {
		t.Fatalf("entries: %+v", got)
	}
	if _, err := ParseZoxide([]byte("nothing here\n")); err == nil {
		t.Fatal("a text file with no entries at all was accepted")
	}
}

func TestScore(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for age, want := range map[int64]float64{10: 40, 7200: 20, 3 * 86400: 5, 30 * 86400: 2.5} {
		if s := Score(Entry{Rank: 10, LastAccessed: now.Unix() - age}, now); s != want {
			t.Errorf("age %d: score %v, want %v", age, s, want)
		}
	}
}

func TestZoxideDBPath(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		goos string
		env  map[string]string
		home string
		want string
	}{
		{"darwin", nil, "/Users/a", "/Users/a/Library/Application Support/zoxide/db.zo"},
		{"darwin", map[string]string{"_ZO_DATA_DIR": "/data"}, "/Users/a", "/data/db.zo"},
		{"linux", nil, "/home/a", "/home/a/.local/share/zoxide/db.zo"},
		{"linux", map[string]string{"XDG_DATA_HOME": "/xdg"}, "/home/a", "/xdg/zoxide/db.zo"},
		{"linux", map[string]string{"XDG_DATA_HOME": "relative"}, "/home/a", "/home/a/.local/share/zoxide/db.zo"},
		{"windows", map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`}, "", `C:\Users\a\AppData\Local\zoxide\db.zo`},
		{"windows", nil, `C:\Users\a`, ""},
		{"linux", nil, "", ""},
	}
	for _, c := range cases {
		if got := ZoxideDBPath(c.goos, env(c.env), c.home); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.goos, c.env, got, c.want)
		}
	}
}

func TestMerge(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	own := map[string]float64{"/Work/Src": 3, "/home/a/notes": 1, "/zero": 0}
	zox := []Entry{
		{"/work/src/", 1, now.Unix() - 10},          // same folder by key, recent: +4
		{"/home/a/music", 10, now.Unix() - 40*86400}, // old: 2.5
		{"/home/a/photos", 1, now.Unix()},            // 4
	}
	key := func(p string) string { return strings.ToLower(strings.TrimRight(p, "/")) }
	got := Merge(own, zox, now, key)
	want := []Item{{"/Work/Src", 7, "both"}, {"/home/a/photos", 4, "zoxide"}, {"/home/a/music", 2.5, "zoxide"}, {"/home/a/notes", 1, "app"}}
	if len(got) != len(want) {
		t.Fatalf("merged: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged[%d] = %+v, want %+v (all: %+v)", i, got[i], want[i], got)
		}
	}
	if got := Merge(nil, nil, now, key); len(got) != 0 {
		t.Fatalf("nothing in, something out: %+v", got)
	}
}
