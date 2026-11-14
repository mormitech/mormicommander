// These tests cover the helpers around the engine: name lists from files and
// the clipboard in various encodings, presets, natural sorting and the
// extension splitter.

package multirename

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseNames(t *testing.T) {
	utf16le := []byte{0xFF, 0xFE, 'a', 0, '\r', 0, '\n', 0, 0x51, 0x01, 0x0A, 0}
	utf16be := []byte{0xFE, 0xFF, 0, 'x', 0, '\n', 0, 'y'}
	cases := []struct {
		name string
		in   []byte
		want []string
	}{
		{"lf", []byte("a\nb\nc\n"), []string{"a", "b", "c"}},
		{"crlf", []byte("a\r\nb\r\n"), []string{"a", "b"}},
		{"cr only", []byte("a\rb"), []string{"a", "b"}},
		{"no final newline", []byte("a\nb"), []string{"a", "b"}},
		{"inner empty kept", []byte("a\n\nb\n"), []string{"a", "", "b"}},
		{"bom", []byte("\xEF\xBB\xBFő.txt\n"), []string{"ő.txt"}},
		{"utf16le", utf16le, []string{"a", "ő"}},
		{"utf16be", utf16be, []string{"x", "y"}},
		{"latin1", []byte("caf\xE9\n"), []string{"café"}},
		{"empty", []byte(""), []string{}},
		{"just newline", []byte("\n"), []string{}},
	}
	for _, c := range cases {
		if got := ParseNames(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFormatNamesRoundTrip(t *testing.T) {
	names := []string{"első.txt", "második.txt", ""}
	for _, crlf := range []bool{false, true} {
		data := FormatNames(names, crlf)
		if crlf != strings.Contains(string(data), "\r\n") {
			t.Errorf("crlf=%v: %q", crlf, data)
		}
		// The trailing empty name is lost, which is the price of ignoring a
		// final newline; real names are never empty.
		if got := ParseNames(data); !reflect.DeepEqual(got, names[:2]) && !reflect.DeepEqual(got, names) {
			t.Errorf("round trip crlf=%v: %q", crlf, got)
		}
	}
	if FormatNames(nil, false) != nil {
		t.Error("no names should give no bytes")
	}
	// Editor round trip through a real file (F-512).
	p := filepath.Join(t.TempDir(), "names.txt")
	os.WriteFile(p, FormatNames([]string{"a.txt", "b.txt"}, false), 0o644)
	data, _ := os.ReadFile(p)
	if got := ParseNames(data); !reflect.DeepEqual(got, []string{"a.txt", "b.txt"}) {
		t.Errorf("file round trip %q", got)
	}
}

func TestPresets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "presets")
	if names, err := ListPresets(dir); err != nil || len(names) != 0 {
		t.Fatalf("missing dir: %v %v", names, err)
	}
	s := DefaultSettings()
	s.NameMask, s.Search, s.Replace, s.RegEx = "[Y]-[C]", `(\d)`, "$1", true
	s.Case, s.CounterDigits, s.RemoveAccents = CaseEachWord, AutoDigits, true
	presetNames := []string{"Photos", "photos", "a/b: c?", "Zeta", "  spaced  "}
	for _, n := range presetNames {
		if err := SavePreset(dir, n, s); err != nil {
			t.Fatalf("save %q: %v", n, err)
		}
	}
	got, err := ListPresets(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"  spaced  ", "a/b: c?", "Photos", "photos", "Zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("list = %q, want %q", got, want)
	}
	for _, n := range presetNames {
		back, err := LoadPreset(dir, n)
		if err != nil || !reflect.DeepEqual(back, s) {
			t.Errorf("load %q: %v %+v", n, err, back)
		}
	}
	// Overwrite and delete.
	s2 := DefaultSettings()
	s2.NameMask = "other"
	SavePreset(dir, "Photos", s2)
	if back, _ := LoadPreset(dir, "Photos"); back.NameMask != "other" {
		t.Errorf("overwrite failed: %+v", back)
	}
	if back, _ := LoadPreset(dir, "photos"); back.NameMask != s.NameMask {
		t.Errorf("case-distinct preset was clobbered: %+v", back)
	}
	if err := DeletePreset(dir, "Zeta"); err != nil {
		t.Fatal(err)
	}
	if err := DeletePreset(dir, "Zeta"); err != nil {
		t.Errorf("second delete: %v", err)
	}
	if _, err := LoadPreset(dir, "Zeta"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("load deleted: %v", err)
	}
	if err := SavePreset(dir, "  ", s); !errors.Is(err, ErrPresetName) {
		t.Errorf("blank name: %v", err)
	}
	// Older presets without some fields get the defaults for them, and
	// junk files in the folder are ignored by the listing.
	os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"name":"old","settings":{"nameMask":"[N]x"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "junk.json"), []byte(`not json`), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`x`), 0o644)
	old, err := LoadPreset(dir, "old")
	if err != nil || old.NameMask != "[N]x" || old.ExtMask != "[E]" || old.CounterStep != 1 {
		t.Errorf("old preset: %v %+v", err, old)
	}
	got, _ = ListPresets(dir)
	if len(got) != 5 {
		t.Errorf("list with junk = %q", got)
	}
	if _, err := decodePreset([]byte(`{"name":"x","settings":5}`)); err == nil {
		t.Error("bad settings type should fail")
	}
}

func TestNaturalCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"img2", "img10", -1},
		{"img10", "img2", 1},
		{"a", "B", -1},
		{"B", "a", 1},
		{"file007", "file7", -1}, // equal numbers fall back to plain order
		{"x", "x", 0},
		{"abc", "abcd", -1},
		{"1.10", "1.9", 1},
		{"Á", "á", -1},
		{"z9", "z09a", -1},
	}
	for _, c := range cases {
		if got := NaturalCompare(c.a, c.b); got != c.want {
			t.Errorf("NaturalCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSortItems(t *testing.T) {
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	base := []Item{
		{Dir: "/b", Name: "img10.png", Size: 30, ModTime: t0.Add(2 * time.Hour)},
		{Dir: "/a", Name: "img2.jpg", Size: 10, ModTime: t0.Add(3 * time.Hour)},
		{Dir: "/a", Name: "IMG1.gif", Size: 20, ModTime: t0},
	}
	names := func(items []Item) string {
		var out []string
		for _, it := range items {
			out = append(out, it.Name)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		key  SortKey
		desc bool
		want string
	}{
		{SortByName, false, "IMG1.gif,img2.jpg,img10.png"},
		{SortByName, true, "img10.png,img2.jpg,IMG1.gif"},
		{SortByExt, false, "IMG1.gif,img2.jpg,img10.png"},
		{SortBySize, false, "img2.jpg,IMG1.gif,img10.png"},
		{SortByDate, false, "IMG1.gif,img10.png,img2.jpg"},
		{SortByDate, true, "img2.jpg,img10.png,IMG1.gif"},
		{SortByPath, false, "IMG1.gif,img2.jpg,img10.png"},
	}
	for _, c := range cases {
		items := append([]Item(nil), base...)
		SortItems(items, c.key, c.desc)
		if got := names(items); got != c.want {
			t.Errorf("%s desc=%v: %s, want %s", c.key, c.desc, got, c.want)
		}
	}
}

func TestSplitExt(t *testing.T) {
	cases := []struct{ in, base, ext string }{
		{"a.txt", "a", "txt"},
		{"a.tar.gz", "a.tar", "gz"},
		{".bashrc", ".bashrc", ""},
		{"noext", "noext", ""},
		{"trailing.", "trailing.", ""}, // the dot stays in the name part (#149)
		{"", "", ""},
	}
	for _, c := range cases {
		b, e := SplitExt(c.in)
		if b != c.base || e != c.ext {
			t.Errorf("SplitExt(%q) = %q %q", c.in, b, e)
		}
	}
	if joinExt("a", "") != "a" || joinExt("a", "b") != "a.b" {
		t.Error("joinExt")
	}
	it := Item{Dir: "/x", Name: "y"}
	if it.Path() != filepath.Join("/x", "y") {
		t.Error("Path")
	}
}
