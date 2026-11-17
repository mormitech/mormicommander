// Tests of the list of options with no effect yet (noeffect.go, board issue #94): every name on it is a real option,
// the field metadata carries the mark, and the mark changes nothing about loading and saving the option.

package config

import "testing"

func TestNoEffectNamesAreOptions(t *testing.T) {
	for name := range noEffect {
		if _, ok := LookupSetting(name); !ok {
			t.Errorf("noeffect.go names %q, which is not an option", name)
		}
	}
}

func TestNoEffectMarkOnFields(t *testing.T) {
	marked := 0
	for _, f := range Fields() {
		if f.NoEffect != noEffect[f.Name] {
			t.Errorf("%s: NoEffect = %v, want %v", f.Name, f.NoEffect, noEffect[f.Name])
		}
		if f.NoEffect {
			marked++
		}
	}
	if marked != len(noEffect) {
		t.Errorf("%d fields marked, %d names listed", marked, len(noEffect))
	}
	for name := range noEffect {
		if !HasNoEffect(name) {
			t.Errorf("HasNoEffect(%q) = false for a listed option", name)
		}
	}
	if HasNoEffect("Display.ShowHidden") || HasNoEffect("No.Such") {
		t.Error("HasNoEffect gives the wrong answer")
	}
	if HasNoEffect("Misc.HideNoEffect") {
		t.Error("the option that hides the others must not be marked itself")
	}
}

func TestNoEffectOptionsStillLoadAndSave(t *testing.T) {
	f := NewFile()
	f.Set("Configuration", "ShowSystemFiles", "1") // Display.ShowSystem (marked until 0.14.21; loading and saving never cared)
	f.Set("Configuration", "HideOptionsWithoutEffect", "1")
	s, warn := LoadSettings(f)
	if len(warn) > 0 {
		t.Fatalf("warnings: %v", warn)
	}
	if !s.Display.ShowSystem || !s.Misc.HideNoEffect {
		t.Errorf("loaded ShowSystem=%v HideNoEffect=%v, want true true", s.Display.ShowSystem, s.Misc.HideNoEffect)
	}
	out := NewFile()
	s.Save(out)
	if v, _ := out.Get("Configuration", "ShowSystemFiles"); v != "1" {
		t.Errorf("saved ShowSystemFiles = %q, want 1", v)
	}
	if Defaults().Misc.HideNoEffect {
		t.Error("HideNoEffect must be off by default (the dialog shows every option, as before)")
	}
	d, fresh := Defaults(), NewFile()
	d.Save(fresh)
	if fresh.Has("Configuration", "HideOptionsWithoutEffect") {
		t.Error("a default HideNoEffect must not be written to a new file")
	}
}
