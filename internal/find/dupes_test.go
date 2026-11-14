// Tests for "Find duplicate files": grouping by name, size, content and hash, files
// that share a 64 KiB prefix but differ later, and that groups come out numbered and
// in a stable order.

package find

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dupGroups runs a duplicate search and returns groups as "rel,rel" strings.
func dupGroups(t *testing.T, o Options) []string {
	t.Helper()
	res, _, err := Search(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	cur := 0
	for _, r := range res {
		if r.DupGroup == 0 {
			t.Fatalf("result without group: %+v", r)
		}
		if r.DupGroup != cur {
			out = append(out, r.Rel)
			cur = r.DupGroup
		} else {
			out[len(out)-1] += "," + r.Rel
		}
	}
	return out
}

func TestDuplicates(t *testing.T) {
	big := strings.Repeat("0123456789abcdef", 8<<10) // 128 KiB
	root := mkTree(t, map[string]string{
		"a/one.txt":    "same content",
		"b/one.txt":    "same content",
		"c/ONE.TXT":    "other conten", // same size, different bytes
		"d/two.txt":    "same content",
		"e/empty1":     "",
		"e/empty2":     "",
		"big/x.bin":    big + "A",
		"big/y.bin":    big + "B", // same size and same 64 KiB prefix
		"big/z.bin":    big + "A",
		"lonely/u.txt": "unique!",
		"dir/one.txt/": "", // a folder with a file's name never counts
	})
	base := DefaultOptions()
	base.SearchIn = []string{root}
	cases := []struct {
		name string
		d    DupOptions
		want []string
	}{
		{"default size+content", DupOptions{Enabled: true},
			[]string{"a/one.txt,b/one.txt,d/two.txt", "big/x.bin,big/z.bin", "e/empty1,e/empty2"}},
		{"name only", DupOptions{Enabled: true, Name: true},
			[]string{"a/one.txt,b/one.txt,c/ONE.TXT"}},
		{"name+size", DupOptions{Enabled: true, Name: true, Size: true},
			[]string{"a/one.txt,b/one.txt,c/ONE.TXT"}},
		{"name+content", DupOptions{Enabled: true, Name: true, Content: true},
			[]string{"a/one.txt,b/one.txt"}},
		{"size only", DupOptions{Enabled: true, Size: true},
			[]string{"a/one.txt,b/one.txt,c/ONE.TXT,d/two.txt", "big/x.bin,big/y.bin,big/z.bin", "e/empty1,e/empty2"}},
		{"hash md5", DupOptions{Enabled: true, Hash: "md5"},
			[]string{"a/one.txt,b/one.txt,d/two.txt", "big/x.bin,big/z.bin", "e/empty1,e/empty2"}},
		{"hash crc32", DupOptions{Enabled: true, Hash: "CRC32"},
			[]string{"a/one.txt,b/one.txt,d/two.txt", "big/x.bin,big/z.bin", "e/empty1,e/empty2"}},
	}
	for _, c := range cases {
		o := base
		o.Duplicates = c.d
		eq(t, c.name, dupGroups(t, o), c.want)
	}

	// The filters still apply before grouping.
	o := base
	o.SearchFor = "*.txt"
	o.Duplicates = DupOptions{Enabled: true}
	eq(t, "filtered", dupGroups(t, o), []string{"a/one.txt,b/one.txt,d/two.txt"})

	// Hash mode reports the digest.
	o.Duplicates = DupOptions{Enabled: true, Hash: "sha1"}
	res, _, err := Search(context.Background(), o)
	if err != nil || len(res) != 3 || len(res[0].Hash) != 40 || res[0].Hash != res[2].Hash {
		t.Errorf("sha1 results: %+v %v", res, err)
	}
}

func TestDuplicatesUnreadable(t *testing.T) {
	root := mkTree(t, map[string]string{"a": "xx", "b": "xx", "c": "xx"})
	if os.Geteuid() == 0 || filepath.Separator == '\\' {
		t.Skip("permission bits do not stop this user")
	}
	p := filepath.Join(root, "c")
	os.Chmod(p, 0)
	t.Cleanup(func() { os.Chmod(p, 0o644) })
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Duplicates = DupOptions{Enabled: true}
	q, _ := Compile(o)
	var errs int
	var got []string
	_, err := q.Run(context.Background(), Handler{
		Result: func(r Result) error { got = append(got, r.Rel); return nil },
		Error:  func(string, error) { errs++ },
	})
	if err != nil || errs != 1 {
		t.Fatalf("err %v errs %d", err, errs)
	}
	eq(t, "readable pair", got, []string{"a", "b"})
}

func TestDuplicatesArchiveMembersNameOnly(t *testing.T) {
	root := mkTree(t, map[string]string{"p.fak": "x.txt=abc", "x.txt": "abc"})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchFor = "x.txt"
	o.SearchArchives = true
	o.Archives = &fakeArchives{}
	o.Duplicates = DupOptions{Enabled: true, Name: true}
	eq(t, "archive by name", dupGroups(t, o), []string{"p.fak/x.txt,x.txt"})
	o.Duplicates = DupOptions{Enabled: true, Content: true}
	eq(t, "archive skipped for content", dupGroups(t, o), nil)
}

func TestDuplicatesCancel(t *testing.T) {
	root := mkTree(t, map[string]string{"a": "1", "b": "1"})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Duplicates = DupOptions{Enabled: true}
	ctx, cancel := context.WithCancel(context.Background())
	q, _ := Compile(o)
	cancel()
	if _, err := q.Run(ctx, Handler{}); err == nil {
		t.Error("want cancellation error")
	}
}

// The same file reached twice (overlapping start folders, or a followed link
// to a folder that is also searched) is not its own duplicate (issue #91).
// Real hard links are separate names and still count.
func TestDuplicatesSameFileTwice(t *testing.T) {
	root := mkTree(t, map[string]string{
		"a/b/one.txt": "same content",
		"c/other.txt": "unique text!",
	})
	o := DefaultOptions()
	o.SearchIn = []string{root, filepath.Join(root, "a")}
	o.Duplicates = DupOptions{Enabled: true}
	if got := dupGroups(t, o); len(got) != 0 {
		t.Errorf("overlapping folders: groups %v", got)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "c", "link")); err != nil {
		t.Fatal(err)
	}
	o.SearchIn = []string{root}
	o.FollowLinks = true
	if got := dupGroups(t, o); len(got) != 0 {
		t.Errorf("followed link: groups %v", got)
	}
	if err := os.Link(filepath.Join(root, "a", "b", "one.txt"), filepath.Join(root, "c", "hard.txt")); err != nil {
		t.Skip("no hard links:", err)
	}
	if got := dupGroups(t, o); len(got) != 1 {
		t.Errorf("hard link: groups %v, want one group", got)
	}
}
