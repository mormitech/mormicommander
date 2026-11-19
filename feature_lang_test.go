package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Options > Language (issue #113): both file formats, sections, "\=" and "\n", empty translations, and where the
// file comes from (settings folder, built-in by name, built-in by interface language, English by default).
func TestParseLanguageLng(t *testing.T) {
	src := "; comment\n# comment\nFiles=Fájl\nTarget \\= source=Cél = forrás\nMake it?\\n\\nNow=Most?\\n\\nIgen\nEmpty=\n[de]\nMark=Markieren\n[hu]\nMark=Kijelölés\n"
	m, err := parseLanguage([]byte(src), "x.lng", "hu")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Files": "Fájl", "Target = source": "Cél = forrás", "Make it?\n\nNow": "Most?\n\nIgen", "Mark": "Kijelölés"}
	if len(m) != len(want) {
		t.Fatalf("got %v", m)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%q: got %q, want %q", k, m[k], v)
		}
	}
	if m, _ := parseLanguage([]byte(src), "x.lng", "en"); m["Mark"] != "" || m["Files"] != "Fájl" {
		t.Errorf("en: %v", m)
	}
}

func TestParseLanguageJSON(t *testing.T) {
	src := `{"Files": "Fájl", "Empty": "", "hu": {"Mark": "Kijelölés", "Files": "Fájlok"}, "de": {"Mark": "Markieren"}}`
	m, err := parseLanguage([]byte(src), "x.json", "hu")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m["Files"] != "Fájlok" || m["Mark"] != "Kijelölés" {
		t.Fatalf("got %v", m)
	}
	if _, err := parseLanguage([]byte(`{"a": `), "x.json", "hu"); err == nil {
		t.Fatal("broken JSON: no error")
	}
}

func TestLanguageSource(t *testing.T) {
	// default: no file, interface language en → English (nil data, no error)
	if d, _, err := languageSource("", "", "en"); d != nil || err != nil {
		t.Fatalf("default: %v %v", d != nil, err)
	}
	// no file, interface language hu → the built-in Hungarian file
	d, _, err := languageSource("", "", "hu")
	if err != nil || d == nil {
		t.Fatalf("hu: %v", err)
	}
	m, err := parseLanguage(d, "hu.lng", "hu")
	if err != nil || m["Files"] != "Fájl" || m["Copy {what} to:"] != "{what} másolása ide:" || m["Target = source"] != "Cél = forrás" {
		t.Fatalf("built-in hu: %d entries, Files=%q err=%v", len(m), m["Files"], err)
	}
	for k, v := range m {
		if strings.Count(k, "{") != strings.Count(v, "{") {
			t.Errorf("placeholders differ: %q → %q", k, v)
		}
	}
	// a file in the settings folder wins; a missing one that names a built-in file uses that; any other is an error
	dir := t.TempDir()
	own := filepath.Join(dir, "hu.lng")
	if err := os.WriteFile(own, []byte("Files=Saját\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := languageSource("hu.lng", own, "hu"); string(d) != "Files=Saját\n" {
		t.Errorf("own file: %q", d)
	}
	if d, _, err := languageSource("hu.lng", filepath.Join(dir, "none", "hu.lng"), "en"); d == nil || err != nil {
		t.Errorf("built-in by name: %v", err)
	}
	if _, _, err := languageSource("xx.lng", filepath.Join(dir, "xx.lng"), "en"); err == nil {
		t.Error("missing file: no error")
	}
	if bundledLanguage("template") != nil || bundledLanguage("../hu") != nil {
		t.Error("the template or a path counted as a built-in language")
	}
}
