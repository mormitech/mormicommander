// Tests for saving and loading named sync settings (F-554).
package syncdirs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProfilesRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg", "sync.json")
	ps, err := LoadProfiles(file)
	if err != nil || ps != nil {
		t.Fatalf("missing file: %v %v", ps, err)
	}
	o := DefaultOptions()
	o.ByContent = true
	o.IgnoreHours = 1
	o.Case = CaseInsensitive
	o.Filter = "*.txt | tmp\\"
	o.Selected = []string{"not saved"}
	ps = PutProfile(nil, Profile{Name: "photos", Left: "/a", Right: "/b", Options: o, Show: DefaultShow()})
	ps = PutProfile(ps, Profile{Name: "backup", Left: "/c", Right: "/d", Options: DefaultOptions(), Show: ShowAll()})
	ps = PutProfile(ps, Profile{Name: "photos", Left: "/a2", Right: "/b", Options: o, Show: DefaultShow()})
	if len(ps) != 2 || ps[0].Left != "/a2" {
		t.Fatalf("PutProfile: %+v", ps)
	}
	if err := SaveProfiles(file, ps); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProfiles(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "backup" || got[1].Name != "photos" {
		t.Fatalf("loaded %+v", got)
	}
	want := o
	want.Selected = nil
	if !reflect.DeepEqual(got[1].Options, want) || got[1].Show != DefaultShow() {
		t.Errorf("options = %+v, want %+v", got[1].Options, want)
	}
	got = DeleteProfile(got, "backup")
	if len(got) != 1 || got[0].Name != "photos" {
		t.Errorf("DeleteProfile: %+v", got)
	}
	ents, _ := os.ReadDir(filepath.Dir(file))
	if len(ents) != 1 {
		t.Errorf("temp files left: %v", ents)
	}
}

func TestProfilesErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "p.json")
	cases := [][]Profile{
		{{Name: ""}},
		{{Name: "a"}, {Name: "a"}},
	}
	for _, ps := range cases {
		if err := SaveProfiles(file, ps); err == nil {
			t.Errorf("SaveProfiles(%v) succeeded", ps)
		}
	}
	if err := os.WriteFile(file, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfiles(file); err == nil {
		t.Errorf("broken file loaded")
	}
}
