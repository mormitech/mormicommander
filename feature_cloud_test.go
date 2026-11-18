package main

import (
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/config"
	lst "mormicommander/internal/listing"
	"mormicommander/internal/vfs"
)

// #30: ordinary files are kept for thumbnails whether or not the option is on (a real placeholder would need iCloud
// or OneDrive, and the detection itself is tested in internal/cloudfile with faked stat data).
func TestCloudKeepDownloaded(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	os.WriteFile(a, []byte("x"), 0o644)
	in := []string{a, filepath.Join(dir, "missing.jpg")}
	for _, skip := range []bool{false, true} {
		if got := cloudKeepDownloaded(append([]string(nil), in...), skip); len(got) != 2 {
			t.Errorf("skip=%v: %v", skip, got)
		}
	}
}

func TestCloudFieldAndOptionDefaults(t *testing.T) {
	f, ok := lst.LookupField("mc.cloud")
	if !ok {
		t.Fatal("[=mc.cloud] is not registered")
	}
	if v := f.Get(&lst.Item{Name: "a", Attr: lst.AttrCloud}, "", nil); v.Text != "online-only" {
		t.Errorf("placeholder: %+v", v)
	}
	if v := f.Get(&lst.Item{Name: "a"}, "", nil); v.Text != "" {
		t.Errorf("downloaded file: %+v", v)
	}
	// both options are off by default, so nothing changes until the user asks
	for _, name := range []string{"Display.CloudMark", "Operation.CloudNoDownload"} {
		s, ok := config.LookupSetting(name)
		if !ok || s.Default != "0" {
			t.Errorf("%s: missing or not off by default (%+v)", name, s)
		}
	}
	if lst.SkipCloudContent == nil {
		t.Error("the media fields are not wired to the option")
	}
}

// The custom columns build their items from listing entries, so the flag must reach [=mc.cloud] (and the media fields).
func TestCloudFlagReachesColumnItems(t *testing.T) {
	it := viewsItemFromEntry("/x", vfs.Entry{Name: "a.txt", Flags: vfs.FlagCloud}, 0)
	if it.Attr&lst.AttrCloud == 0 {
		t.Error("vfs.FlagCloud was not carried into the column item")
	}
}
