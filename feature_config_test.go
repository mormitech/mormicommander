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
