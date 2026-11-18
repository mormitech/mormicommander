package main

// Tests of the configuration feature's Go side.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An imported Total Commander hotlist lands in folders.ini, where the hotlist feature reads it, not in
// mormicommander.ini.
func TestConfigImportTCHotlistGoesToFolders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", dir)
	old := thInstance
	defer func() { thInstance = old }()
	thInstance = thOpen(&App{}, filepath.Join(dir, "folders.ini"))
	b := filepath.Join(t.TempDir(), "B")
	tc := filepath.Join(t.TempDir(), "wincmd.ini")
	if err := os.WriteFile(tc, []byte("[DirMenu]\r\nmenu1=Folder B\r\ncmd1=cd "+b+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Configuration{app: &App{}}
	rep, err := c.ImportTC(tc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep["imported"].([]string), "\n"), "hotlist: 1") {
		t.Fatalf("report: %+v", rep)
	}
	hot := thInstance.Hotlist()
	if len(hot) != 1 || hot[0].Kind != "menu" || len(hot[0].Items) != 1 || hot[0].Items[0].Cmd != "cd "+b {
		t.Fatalf("hotlist after the import: %+v", hot)
	}
	again := thOpen(&App{}, filepath.Join(dir, "folders.ini")).Hotlist()
	if len(again) != 1 || len(again[0].Items) != 1 {
		t.Fatalf("hotlist after a restart: %+v", again)
	}
	if ini, _ := os.ReadFile(filepath.Join(dir, "mormicommander.ini")); strings.Contains(string(ini), "[DirMenu]") {
		t.Fatalf("the hotlist was also written to mormicommander.ini:\n%s", ini)
	}
}

// The Options dialog learns from Schema which options have no effect yet (board issue #94), so it can mark them.
func TestConfigSchemaMarksOptionsWithoutEffect(t *testing.T) {
	c := &Configuration{app: &App{}}
	fields, _ := c.Schema()["fields"].([]configField)
	marks := map[string]bool{}
	for _, f := range fields {
		marks[f.Name] = f.NoEffect
	}
	if len(marks) == 0 {
		t.Fatal("Schema gave no fields")
	}
	// Every option works since 0.14.27, so none is marked; Layout.MenuBar was the last example of one that was.
	for _, n := range []string{"Layout.MenuBar", "Display.ShowHidden", "Refresh.AutoRefresh", "FolderTabs.Show", "Misc.HideNoEffect"} {
		if v, ok := marks[n]; !ok || v {
			t.Errorf("%s: present=%v marked=%v, want present and not marked", n, ok, v)
		}
	}
}

// TestConfigSearchWords checks the binding behind the "Whole words" box (board issue #109): Search is unchanged, and
// SearchWords with whole words leaves out options where the text is only part of a longer word.
func TestConfigSearchWords(t *testing.T) {
	c := &Configuration{app: &App{}}
	has := func(r map[string]any, name string) bool {
		for _, n := range r["fields"].([]string) {
			if n == name {
				return true
			}
		}
		return false
	}
	if r := c.Search("two"); !has(r, "Refresh.RefreshNetwork") || !has(r, "Layout.DriveBar2") {
		t.Errorf("Search(two) = %v, want network refresh and the drive bars", r["fields"])
	}
	if r := c.SearchWords("two", false); !has(r, "Refresh.RefreshNetwork") {
		t.Errorf("SearchWords(two, false) = %v, want network refresh", r["fields"])
	}
	if r := c.SearchWords("two", true); has(r, "Refresh.RefreshNetwork") || !has(r, "Layout.DriveBar2") {
		t.Errorf("SearchWords(two, true) = %v, want the drive bars only", r["fields"])
	}
}
