// Tests for the ignore list: name and path entries, folder-only entries, comments, toggling and file loading.

package masks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIgnoreList(t *testing.T) {
	text := "\ufeff# notes\r\n*.bak\r\n\r\n.git\\\r\ndesktop.ini\n/srv/data/*.log\nC:\\Windows\\*.dll\n/\n"
	l, err := ParseIgnoreList(strings.NewReader(text), CaseSensitive)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		dir, name string
		isDir     bool
		want      bool
	}{
		{"/x", "a.bak", false, true},
		{"/x", "a.txt", false, false},
		{"/x", ".git", true, true},
		{"/x", ".git", false, false},
		{"", "desktop.ini", false, true},
		{"/srv/data", "app.log", false, true},
		{"/srv/data/", "app.log", false, true},
		{"/srv/data/old", "app.log", false, false},
		{"/srv", "app.log", false, false},
		{"", "app.log", false, false},
		{"C:/Windows", "k.dll", false, true},
		{"/x", "..", true, false},
	}
	for _, tt := range tests {
		if got := l.Ignored(tt.dir, tt.name, tt.isDir); got != tt.want {
			t.Errorf("Ignored(%q, %q, %v) = %v, want %v", tt.dir, tt.name, tt.isDir, got, tt.want)
		}
	}
	want := []string{"*.bak", `.git\`, "desktop.ini", "/srv/data/*.log", `C:\Windows\*.dll`, "/"}
	if !reflect.DeepEqual(l.Lines(), want) {
		t.Errorf("Lines = %q", l.Lines())
	}
	entries := []Entry{{Name: "a.bak"}, {Name: "keep"}}
	if got := l.Filter("/x", entries); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("Filter = %v", got)
	}
	if l.Toggle() || l.Ignored("/x", "a.bak", false) {
		t.Error("disabled list must hide nothing")
	}
	if !l.Toggle() {
		t.Error("toggle back on")
	}
	var nilL *IgnoreList
	if nilL.Ignored("/", "a.bak", false) {
		t.Error("nil list")
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	l, err := LoadIgnoreFile(filepath.Join(dir, "missing.txt"), CaseDefault)
	if err != nil || !l.Enabled || l.Ignored("/", "x", false) {
		t.Fatalf("missing file: %v %+v", err, l)
	}
	p := filepath.Join(dir, "ignore.txt")
	if err := os.WriteFile(p, []byte("Thumbs.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err = LoadIgnoreFile(p, CaseInsensitive)
	if err != nil || !l.Ignored("", "thumbs.DB", false) {
		t.Fatalf("loaded file: %v", err)
	}
}
