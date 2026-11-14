// Tests for the typed Configuration dialog model: every field has valid
// metadata, defaults load, values from a hand-written ini are read with the
// right types (TC's "yes"/"true" spellings, clamping of out-of-range numbers),
// saving only writes keys that exist or differ from the default, and the
// name-based Get/Set/Diff/Search helpers used by the dialog and control mode
// behave.

package config

import (
	"strings"
	"testing"
)

func TestFieldsMetadata(t *testing.T) {
	seenName := map[string]bool{}
	seenKey := map[string]string{}
	for _, fi := range Fields() {
		if seenName[strings.ToLower(fi.Name)] {
			t.Errorf("duplicate option name %s", fi.Name)
		}
		seenName[strings.ToLower(fi.Name)] = true
		k := strings.ToLower(fi.Section + ":" + fi.Key)
		if other, dup := seenKey[k]; dup {
			t.Errorf("%s and %s share ini key %s", fi.Name, other, k)
		}
		seenKey[k] = fi.Name
		if fi.Label == "" {
			t.Errorf("%s has no label", fi.Name)
		}
		if fi.Section == "" || fi.Key == "" {
			t.Errorf("%s has no ini location", fi.Name)
		}
		if fi.Type == "int" && fi.Min != nil && fi.Max != nil && *fi.Min > *fi.Max {
			t.Errorf("%s: min > max", fi.Name)
		}
		for _, o := range fi.Options {
			if fi.Min != nil && o.Value < *fi.Min || fi.Max != nil && o.Value > *fi.Max {
				t.Errorf("%s: option %d outside range", fi.Name, o.Value)
			}
		}
	}
	if len(Pages()) < 30 {
		t.Errorf("only %d pages", len(Pages()))
	}
	// Every page from FEATURES.md section 17 must exist.
	want := []string{"Layout", "Display", "Icons", "Font", "Colors", "Language", "Operation",
		"ViewerEditor", "CopyDelete", "Refresh", "QuickSearch", "FTP", "Proxy", "Plugins",
		"Thumbnails", "Log", "IgnoreList", "Packer", "Zip", "Misc", "FolderTabs",
		"CustomColumns", "FavoriteTabs", "BackgroundQueue", "Hotlist", "ViewModes",
		"Tooltips", "Lister", "DragDrop", "Tree", "FindFiles"}
	have := map[string]bool{}
	for _, p := range Pages() {
		have[p.ID] = true
		if p.Title == "" {
			t.Errorf("page %s has no title", p.ID)
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing page %s", w)
		}
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	cases := []struct {
		name, want string
	}{
		{"Layout.ButtonBar", "1"},
		{"Colors.DarkMode", "2"},
		{"Operation.SavePath", "1"},
		{"Zip.Level", "6"},
		{"FindFiles.Depth", "-1"},
		{"Misc.WindowTitle", "mormiCommander"},
		{"IgnoreList.File", "%COMMANDER_PATH%/ignore.txt"},
	}
	for _, c := range cases {
		got, err := d.Get(c.name)
		if err != nil || got != c.want {
			t.Errorf("default %s = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestLoadSettingsValues(t *testing.T) {
	f := ParseString(`[Configuration]
DarkMode=7
Savepath=no
SaveHistory=true
HistorySize=0
TitleBarText=My TC
onlyonce=maybe
[Packer]
ZipLevel=9 ; best
[Layout]
ButtonBar=0
`)
	s, warn := LoadSettings(f)
	if s.Colors.DarkMode != 2 {
		t.Errorf("DarkMode clamped = %d, want 2", s.Colors.DarkMode)
	}
	if s.Operation.SavePath || !s.Operation.SaveHistory {
		t.Errorf("bool spellings: SavePath=%v SaveHistory=%v", s.Operation.SavePath, s.Operation.SaveHistory)
	}
	if s.Hotlist.HistorySize != 1 {
		t.Errorf("HistorySize clamped = %d, want 1", s.Hotlist.HistorySize)
	}
	if s.Misc.WindowTitle != "My TC" {
		t.Errorf("WindowTitle = %q", s.Misc.WindowTitle)
	}
	if s.Zip.Level != 9 {
		t.Errorf("ZipLevel = %d", s.Zip.Level)
	}
	if s.Layout.ButtonBar {
		t.Error("ButtonBar should be off")
	}
	if s.Operation.OnlyOnce {
		t.Error("bad bool must keep the default")
	}
	if len(warn) != 1 || !strings.Contains(warn[0], "onlyonce") && !strings.Contains(warn[0], "OnlyOnce") {
		t.Errorf("warnings = %v", warn)
	}
}

func TestSettingsSaveMinimal(t *testing.T) {
	f := ParseString("; my settings\n[Configuration]\nSavepath=1\n")
	s, _ := LoadSettings(f)
	s.Misc.ConfirmExit = true
	s.Save(f)
	out := f.String()
	if !strings.Contains(out, "; my settings") {
		t.Error("comment lost")
	}
	if !strings.Contains(out, "Savepath=1") {
		t.Error("existing key at default must stay")
	}
	if !strings.Contains(out, "ConfirmExit=1") {
		t.Error("changed key missing")
	}
	if strings.Contains(out, "ZipLevel") || strings.Contains(out, "[Lister]") {
		t.Errorf("defaults must not be written:\n%s", out)
	}
	// Round trip.
	s2, _ := LoadSettings(ParseString(out))
	if d := s.Diff(&s2); len(d) != 0 {
		t.Errorf("round trip diff %v", d)
	}
}

func TestSettingsGetSetDiff(t *testing.T) {
	a := Defaults()
	b := Defaults()
	cases := []struct {
		name, value, want string
		err               bool
	}{
		{"display.showhidden", "1", "1", false},
		{"Zip.Level", "42", "9", false},
		{"Zip.Level", "abc", "", true},
		{"Misc.WindowTitle", "x y", "x y", false},
		{"No.Such", "1", "", true},
	}
	for _, c := range cases {
		err := b.Set(c.name, c.value)
		if (err != nil) != c.err {
			t.Errorf("Set(%s,%s) err=%v", c.name, c.value, err)
			continue
		}
		if c.err {
			continue
		}
		got, _ := b.Get(c.name)
		if got != c.want {
			t.Errorf("Get(%s) = %q, want %q", c.name, got, c.want)
		}
	}
	d := a.Diff(&b)
	want := []string{"Display.ShowHidden", "Misc.WindowTitle", "Zip.Level"}
	if strings.Join(d, ",") != strings.Join(want, ",") {
		t.Errorf("Diff = %v, want %v", d, want)
	}
}

func TestSearchSettings(t *testing.T) {
	cases := []struct {
		q        string
		wantPage string
		wantMin  int
	}{
		{"hidden", "Display", 1},
		{"zip level", "Zip", 1},
		{"dark", "Colors", 1},
		{"follow system", "Colors", 1}, // matches an option label
	}
	for _, c := range cases {
		fs, pages := SearchSettings(c.q)
		if len(fs) < c.wantMin {
			t.Errorf("%q: %d results", c.q, len(fs))
		}
		found := false
		for _, p := range pages {
			if p == c.wantPage {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: pages %v lack %s", c.q, pages, c.wantPage)
		}
	}
	if fs, _ := SearchSettings("xyzzy-nothing"); len(fs) != 0 {
		t.Errorf("nonsense query matched %d", len(fs))
	}
	if fs, _ := SearchSettings(""); len(fs) != len(Fields()) {
		t.Error("empty query should list everything")
	}
}

// TestSearchSettingsWholeWords checks board issue #109: by default "two" also
// finds the network options (as the search always did); with whole words it
// finds only "Two drive button bars".
func TestSearchSettingsWholeWords(t *testing.T) {
	names := func(fs []FieldInfo) map[string]bool {
		m := map[string]bool{}
		for _, f := range fs {
			m[f.Name] = true
		}
		return m
	}
	part, _ := SearchSettings("two")
	if pm := names(part); !pm["Layout.DriveBar2"] || !pm["Refresh.RefreshNetwork"] {
		t.Errorf("part-word search for two = %v, want both the drive bars and network refresh", pm)
	}
	if p2, _ := SearchSettingsWords("two", false); len(p2) != len(part) {
		t.Errorf("SearchSettingsWords(false) found %d, SearchSettings %d", len(p2), len(part))
	}
	whole, pages := SearchSettingsWords("two", true)
	wm := names(whole)
	if !wm["Layout.DriveBar2"] || wm["Refresh.RefreshNetwork"] {
		t.Errorf("whole-word search for two = %v, want the drive bars and not network refresh", wm)
	}
	if len(pages) == 0 || pages[0] != "Layout" {
		t.Errorf("whole-word pages = %v, want Layout", pages)
	}
	if fs, _ := SearchSettingsWords("network", true); !names(fs)["Refresh.RefreshNetwork"] {
		t.Error("whole-word network should still find Refresh network folders (by its label)")
	}
	if fs, _ := SearchSettingsWords("", true); len(fs) != len(Fields()) {
		t.Error("an empty whole-word query should list everything")
	}
}

func TestContainsWord(t *testing.T) {
	cases := []struct {
		hay, w string
		whole  bool
		want   bool
	}{
		{"network folders", "two", false, true},
		{"network folders", "two", true, false},
		{"two drive bars", "two", true, true},
		{"bars two", "two", true, true},
		{"layout.two-panel", "two", true, true},
		{"network and two", "two", true, true}, // a later place still counts
		{"two2", "two", true, false},
		{"zwei über", "über", true, true},
		{"drüber", "über", true, false},
		{"anything", "", true, true},
	}
	for _, c := range cases {
		if got := ContainsWord(c.hay, c.w, c.whole); got != c.want {
			t.Errorf("ContainsWord(%q, %q, %v) = %v, want %v", c.hay, c.w, c.whole, got, c.want)
		}
	}
}

// TestHiddenFilesOptions checks the two options of board issue #4: the status-line marker is on by default, and Find
// files keeps its old way (hidden files as the last search had them) unless HiddenFromPanel is turned on; both are
// read from and written to their ini keys, and the marker's label keeps its quotes.
func TestHiddenFilesOptions(t *testing.T) {
	d := Defaults()
	for name, want := range map[string]string{"Layout.HiddenMarker": "1", "FindFiles.HiddenFromPanel": "0"} {
		if got, err := d.Get(name); err != nil || got != want {
			t.Errorf("default %s = %q, %v; want %q", name, got, err, want)
		}
	}
	s, _ := LoadSettings(ParseString("[Layout]\nHiddenMarker=0\n[SearchDefaults]\nHiddenFollowsPanel=1\n"))
	if s.Layout.HiddenMarker || !s.FindFiles.HiddenFromPanel {
		t.Errorf("loaded HiddenMarker=%v HiddenFromPanel=%v, want false true", s.Layout.HiddenMarker, s.FindFiles.HiddenFromPanel)
	}
	for _, fi := range Fields() {
		if fi.Name == "Layout.HiddenMarker" && !strings.Contains(fi.Label, `"Hidden: shown"`) {
			t.Errorf("marker label = %q", fi.Label)
		}
	}
}
