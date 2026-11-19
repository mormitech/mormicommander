package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mormicommander/internal/config"
)

// Options > Language (issue #113): both file formats, sections, "\=" and "\n", empty translations, and where the
// file comes from (settings folder, built-in by name, built-in by interface language, English by default).
func TestParseLanguageLng(t *testing.T) {
	src := "; comment\n# comment\n[N] Name=[N] Név\nFiles=Fájl\nTarget \\= source=Cél = forrás\nMake it?\\n\\nNow=Most?\\n\\nIgen\nEmpty=\n[de]\nMark=Markieren\n[hu]\nMark=Kijelölés\n"
	m, err := parseLanguage([]byte(src), "x.lng", "hu")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"[N] Name": "[N] Név", "Files": "Fájl", "Target = source": "Cél = forrás", "Make it?\n\nNow": "Most?\n\nIgen", "Mark": "Kijelölés"}
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

// optionTexts are the words of the Options dialog that come from Go (internal/config): page titles, option labels and
// the names of their choices, sorted. The frontend translates them with t() when it draws the dialog.
func optionTexts() []string {
	seen := map[string]bool{}
	add := func(s string) {
		if strings.TrimSpace(s) != "" {
			seen[s] = true
		}
	}
	for _, p := range config.Pages() {
		add(p.Title)
	}
	for _, f := range config.Fields() {
		add(f.Label)
		for _, o := range f.Options {
			add(o.Label)
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// frontend/test/options-texts.json lists optionTexts() for the frontend's translation test and template (issue #113):
// lang/hu.lng must cover them and `npm run lang:template` puts them in lang/template.lng. It must match the schema;
// MC_WRITE_LANG_TEMPLATE=1 go test -run TestOptionTextsFile . writes it again after a label changed.
func TestOptionTextsFile(t *testing.T) {
	want, _ := json.MarshalIndent(optionTexts(), "", "  ")
	want = append(want, '\n')
	file := filepath.Join("frontend", "test", "options-texts.json")
	if os.Getenv("MC_WRITE_LANG_TEMPLATE") != "" {
		if err := os.WriteFile(file, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(want) {
		t.Fatalf("%s is out of date with the Options labels: run MC_WRITE_LANG_TEMPLATE=1 go test -run TestOptionTextsFile . (and translate the new texts in lang/hu.lng)", file)
	}
}
