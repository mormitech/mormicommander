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
