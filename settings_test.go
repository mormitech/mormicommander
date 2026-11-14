package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	s := loadSettings(p)
	if s.Get("keymap") != nil {
		t.Fatal("empty settings have a keymap")
	}
	if err := s.put("keymap", map[string]string{"Ctrl+K": "cm_Copy"}); err != nil {
		t.Fatal(err)
	}
	s.put("hidden", true)
	s2 := loadSettings(p)
	km, _ := s2.Get("keymap").(map[string]any)
	if km["Ctrl+K"] != "cm_Copy" || s2.Get("hidden") != true {
		t.Errorf("reloaded: %v", s2.All())
	}
	s2.put("hidden", nil)
	if _, ok := loadSettings(p).All()["hidden"]; ok {
		t.Error("nil did not remove the key")
	}
	os.WriteFile(p, []byte("{broken"), 0o644)
	if len(loadSettings(p).All()) != 0 {
		t.Error("broken file not ignored")
	}
}

// A staged value (the page's session, written a moment after each change) reaches the file when the app ends before
// the page wrote it, and a value the page did write wins over an older staged one (board issue #131).
func TestSettingsStageFlush(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	s := loadSettings(p)
	s.put("session", map[string]any{"active": 0.0})
	s.Stage("session", map[string]any{"active": 1.0})
	if m, _ := loadSettings(p).Get("session").(map[string]any); m["active"] != 0.0 {
		t.Fatalf("staging wrote the file: %v", m)
	}
	s.flushPending()
	if m, _ := loadSettings(p).Get("session").(map[string]any); m["active"] != 1.0 {
		t.Errorf("staged session not written on flush: %v", m)
	}

	s.Stage("session", map[string]any{"active": 0.0})
	s.put("session", map[string]any{"active": 1.0, "left": "x"})
	s.flushPending()
	if m, _ := loadSettings(p).Get("session").(map[string]any); m["left"] != "x" {
		t.Errorf("an older staged value replaced the written one: %v", m)
	}
}
