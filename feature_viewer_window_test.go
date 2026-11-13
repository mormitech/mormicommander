// Tests of the parts of the Lister windows that need no real window: titles, the kept size, and the settings change
// that every window hears of. The windows themselves are checked in the real app through control mode (docs/control.md,
// "Lister windows").
package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestListerTitle(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/a.txt":        "Lister – a.txt",
		`C:\Users\x\b.png`:  "Lister – b.png",
		"/tmp/folder/":      "Lister – folder",
		"plain":             "Lister – plain",
		"ftp://host/d/c.md": "Lister – c.md",
	} {
		if got := listerTitle(in); got != want {
			t.Errorf("listerTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListerWindowSize(t *testing.T) {
	a := &App{settings: loadSettings(filepath.Join(t.TempDir(), "settings.json"))}
	lw := &ListerWindows{app: a, wins: map[string]*listerWin{}}
	if w, h := lw.size(); w != listerWinW || h != listerWinH {
		t.Fatalf("default size %dx%d", w, h)
	}
	a.settings.put("listerWindow", map[string]any{"w": 700, "h": 500})
	if w, h := lw.size(); w != 700 || h != 500 {
		t.Fatalf("kept size %dx%d", w, h)
	}
	a.settings.put("listerWindow", map[string]any{"w": 10, "h": 10}) // too small: the default
	if w, h := lw.size(); w != listerWinW || h != listerWinH {
		t.Fatalf("too small a size was taken: %dx%d", w, h)
	}
	if _, err := lw.Open(ListerRequest{Path: "/tmp/a.txt"}); err == nil {
		t.Fatal("Open without the app should fail")
	}
	if _, err := lw.pick(""); err == nil {
		t.Fatal("pick with no window open should fail")
	}
}

func TestSettingsChanged(t *testing.T) {
	s := loadSettings(filepath.Join(t.TempDir(), "settings.json"))
	var got []string
	s.changed = func(key string, value any, from string) { got = append(got, key+"@"+from) }
	s.put("a", 1)
	s.Set(context.Background(), "b", 2)
	if len(got) != 2 || got[0] != "a@" || got[1] != "b@" {
		t.Fatalf("changes told: %v", got)
	}
}
