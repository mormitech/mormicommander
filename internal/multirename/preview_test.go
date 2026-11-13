// These tests cover the per-OS name rules and the preview's conflict
// detection: duplicates inside the list, names already taken on disk,
// chains and swaps that are allowed, and names loaded from a list.

package multirename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func kinds(ps []Problem) string {
	var out []string
	for _, p := range ps {
		out = append(out, string(p.Kind))
	}
	return strings.Join(out, ",")
}

func TestValidateName(t *testing.T) {
	long := strings.Repeat("a", 256)
	longUTF8 := strings.Repeat("é", 128) // 256 bytes but only 128 UTF-16 units
	cases := []struct {
		name, goos, want string
	}{
		{"ok.txt", "windows", ""},
		{"ok.txt", "linux", ""},
		{"", "linux", "empty"},
		{".", "darwin", "dot-name"},
		{"..", "windows", "dot-name"},
		{"a:b", "windows", "invalid-chars"},
		{"a:b", "darwin", "invalid-chars"},
		{"a:b", "linux", ""},
		{"a/b", "linux", "invalid-chars"},
		{`a\b`, "windows", "invalid-chars"},
		{`a\b`, "linux", ""},
		{"a?b*c|d<e>f\"g", "windows", "invalid-chars"},
		{"a\tb", "windows", "invalid-chars"},
		{"a\tb", "linux", ""},
		{"a\x00b", "linux", "invalid-chars"},
		{"CON", "windows", "reserved"},
		{"con.txt", "windows", "reserved"},
		{"Lpt9.tar.gz", "windows", "reserved"},
		{"COM0", "windows", ""},
		{"CONSOLE", "windows", ""},
		{"con.txt", "linux", ""},
		{"name.", "windows", "trailing"},
		{"name ", "windows", "trailing"},
		{"name.", "linux", ""},
		{"nul .", "windows", "reserved,trailing"},
		{long, "linux", "too-long"},
		{long, "windows", "too-long"},
		{longUTF8, "windows", ""},
		{longUTF8, "linux", "too-long"},
	}
	for _, c := range cases {
		if got := kinds(ValidateName(c.name, c.goos)); got != c.want {
			t.Errorf("ValidateName(%q, %s) = %q, want %q", c.name, c.goos, got, c.want)
		}
	}
	if d := ValidateName("a<b<c", "windows")[0].Detail; strings.Count(d, "<") != 1 {
		t.Errorf("repeated chars should be listed once: %q", d)
	}
}

// makeFiles creates empty files in a new temp dir and returns the dir and
// the matching items.
func makeFiles(t *testing.T, names ...string) (string, []Item) {
	t.Helper()
	dir := t.TempDir()
	var items []Item
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			items = append(items, Item{Dir: dir, Name: strings.TrimSuffix(n, "/"), IsDir: true})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		items = append(items, Item{Dir: filepath.Dir(p), Name: filepath.Base(p), Size: int64(len(n))})
	}
	return dir, items
}

func TestPreviewConflicts(t *testing.T) {
	cases := []struct {
		name    string
		files   []string // files to create
		list    int      // how many of them are in the rename list
		mask    string
		fold    CaseRule
		want    []string // problem kinds per entry
		changes int
	}{
		{"all fine", []string{"a.txt", "b.txt"}, 2, "[N]_x", CaseRuleSensitive, []string{"", ""}, 2},
		{"duplicate", []string{"a.txt", "b.txt"}, 2, "same", CaseRuleSensitive, []string{"duplicate", "duplicate"}, 2},
		{"exists on disk", []string{"a.txt", "a_x.txt"}, 1, "[N]_x", CaseRuleSensitive, []string{"exists"}, 1},
		{"chain allowed", []string{"a.txt", "a_x.txt"}, 2, "[N]_x", CaseRuleSensitive, []string{"", ""}, 2},
		{"swap allowed", []string{"1.txt", "2.txt"}, 2, "[C2+-1]", CaseRuleSensitive, []string{"", ""}, 2},
		{"collides with unchanged", []string{"a.txt", "b.txt"}, 2, "b", CaseRuleSensitive, []string{"duplicate", "duplicate"}, 1},
		{"case fold duplicate", []string{"a.txt", "b.txt"}, 2, "[C]X", CaseRuleInsensitive, []string{"", ""}, 2},
		{"unchanged ok", []string{"a.txt"}, 1, "[N]", CaseRuleSensitive, []string{""}, 0},
		{"invalid name", []string{"a.txt"}, 1, "a/b", CaseRuleSensitive, []string{"invalid-chars"}, 1},
	}
	for _, c := range cases {
		_, items := makeFiles(t, c.files...)
		s := DefaultSettings()
		s.NameMask = c.mask
		res, err := Preview(items[:c.list], s, PreviewOptions{TargetOS: "linux", CaseRule: c.fold})
		if err != nil {
			t.Fatal(err)
		}
		for i, e := range res.Entries {
			if got := kinds(e.Problems); got != c.want[i] {
				t.Errorf("%s: entry %d (%s -> %s) problems %q, want %q", c.name, i, e.Item.Name, e.NewName, got, c.want[i])
			}
		}
		errs := 0
		for _, w := range c.want {
			if w != "" {
				errs++
			}
		}
		if res.Errors != errs || res.HasErrors() != (errs > 0) || res.Changes != c.changes {
			t.Errorf("%s: Errors=%d Changes=%d, want %d %d", c.name, res.Errors, res.Changes, errs, c.changes)
		}
	}
}

func TestPreviewCaseFold(t *testing.T) {
	items := []Item{{Dir: "/d", Name: "a.txt"}, {Dir: "/d", Name: "b.txt"}}
	s := DefaultSettings()
	s.NameMask = "x"
	s.Case = CaseUnchanged
	// "x.txt" twice is a duplicate under either rule.
	r, _ := Compile(s)
	res := r.Preview(items, PreviewOptions{TargetOS: "linux", SkipDiskCheck: true, Names: []string{"X.txt", "x.txt"}})
	if res.Errors != 0 && defaultFoldCase == false {
		t.Errorf("auto rule on case-sensitive system: %+v", res.Entries)
	}
	res = r.Preview(items, PreviewOptions{SkipDiskCheck: true, CaseRule: CaseRuleInsensitive, Names: []string{"X.txt", "x.txt"}})
	if res.Errors != 2 {
		t.Errorf("insensitive: want 2 errors, got %d", res.Errors)
	}
	res = r.Preview(items, PreviewOptions{SkipDiskCheck: true, CaseRule: CaseRuleSensitive, Names: []string{"X.txt", "x.txt"}})
	if res.Errors != 0 {
		t.Errorf("sensitive: want 0 errors, got %d", res.Errors)
	}
}

func TestPreviewCaseOnlyChange(t *testing.T) {
	_, items := makeFiles(t, "photo.JPG")
	s := DefaultSettings()
	s.Case = CaseLower
	res, err := Preview(items, s, PreviewOptions{CaseRule: CaseRuleInsensitive})
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors != 0 || res.Entries[0].NewName != "photo.jpg" {
		t.Errorf("case-only change flagged: %+v", res.Entries[0])
	}
}

func TestPreviewNameList(t *testing.T) {
	items := []Item{{Dir: "/d", Name: "a.txt"}, {Dir: "/d", Name: "b.txt"}, {Dir: "/d", Name: "c.txt"}}
	s := DefaultSettings()
	s.NameMask = "[N]_m"
	opts := PreviewOptions{SkipDiskCheck: true, TargetOS: "linux", Names: []string{"one.txt", "two.txt"}}
	res, err := Preview(items, s, opts)
	if err != nil {
		t.Fatal(err)
	}
	got := NewNamesOf(res)
	if strings.Join(got, ",") != "one.txt,two.txt,c_m.txt" {
		t.Errorf("names = %v", got)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "2 lines but 3 items") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	opts.Names = []string{"1", "2", "3"}
	res, _ = Preview(items, s, opts)
	if len(res.Warnings) != 0 {
		t.Errorf("matching count should not warn: %v", res.Warnings)
	}
}

func TestCheckEntriesAfterManualEdit(t *testing.T) {
	_, items := makeFiles(t, "a.txt", "b.txt")
	res, _ := Preview(items, DefaultSettings(), PreviewOptions{TargetOS: "windows"})
	if res.Changes != 0 {
		t.Fatalf("default settings should change nothing")
	}
	res.Entries[0].NewName = "b.txt"
	res.Entries[1].NewName = "a?.txt"
	CheckEntries(res.Entries, PreviewOptions{TargetOS: "windows", CaseRule: CaseRuleSensitive})
	if kinds(res.Entries[0].Problems) != "" {
		t.Errorf("a->b with b renamed away should be fine: %v", res.Entries[0].Problems)
	}
	if kinds(res.Entries[1].Problems) != "invalid-chars" {
		t.Errorf("got %v", res.Entries[1].Problems)
	}
}
