// Tests for folder tabs, saved .tab files, favourite tab sets, panel state
// and the per-screen window position: TC's key layout is read and written,
// bad numbers fall back safely, and the window position section is found
// again whatever suffix TC put after the resolution.

package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTabSetRoundTrip(t *testing.T) {
	ts := TabSet{Active: 1, Tabs: []Tab{
		{Path: "/home/p", Caption: "Home"},
		{Path: "/tmp", Options: "1|0|0", Lock: TabLockedChangeOK},
	}}
	f := NewFile()
	SaveTabSet(f, SectionRightTabs, ts)
	s := f.String()
	for _, want := range []string{"0_path=/home/p", "0_caption=Home", "1_options=1|0|0", "1_locked=2", "activetab=1"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
	got := LoadTabSet(ParseString(s), SectionRightTabs)
	if got.Active != 1 || len(got.Tabs) != 2 || !reflect.DeepEqual(got.Tabs, ts.Tabs) {
		t.Errorf("got %+v", got)
	}
	if got.ActiveTab().Path != "/tmp" {
		t.Error("ActiveTab")
	}
}

func TestTabSetBadValues(t *testing.T) {
	got := LoadTabSet(ParseString("[lefttabs]\n0_path=/a\n0_locked=9\n2_path=/gap\nactivetab=5\n"), SectionLeftTabs)
	if len(got.Tabs) != 1 || got.Tabs[0].Lock != TabUnlocked || got.Active != 0 {
		t.Errorf("got %+v", got)
	}
	if !reflect.DeepEqual((TabSet{}).ActiveTab(), Tab{}) {
		t.Error("empty ActiveTab")
	}
}

func TestTabFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "work.tab")
	tf := TabFile{
		Active:   TabSet{Tabs: []Tab{{Path: "/a"}, {Path: "/b"}}, Active: 1},
		Inactive: TabSet{Tabs: []Tab{{Path: "/c"}}},
	}
	if err := SaveTabFile(p, tf); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTabFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Active.Tabs) != 2 || got.Active.Active != 1 || got.Inactive.Tabs[0].Path != "/c" {
		t.Errorf("got %+v", got)
	}
	// The hotlist opens .tab files with "cd".
	if a := ParseHotlistCommand(`cd "` + p + `"`); a.Kind != ActionTabSet || a.Path != p {
		t.Errorf("hotlist action = %+v", a)
	}
}

func TestFavoriteTabs(t *testing.T) {
	f := NewFile()
	SaveFavoriteTabs(f, FavoriteTabs{Name: "Work", Left: TabSet{Tabs: []Tab{{Path: "/w"}}}, Right: TabSet{Tabs: []Tab{{Path: "/x"}, {Path: "/y"}}, Active: 1}})
	SaveFavoriteTabs(f, FavoriteTabs{Name: "art", Left: TabSet{Tabs: []Tab{{Path: "/art"}}}})
	favs := LoadFavoriteTabs(ParseString(f.String()))
	if len(favs) != 2 || favs[0].Name != "art" || favs[1].Name != "Work" {
		t.Fatalf("favs = %+v", favs)
	}
	if favs[1].Right.Active != 1 || len(favs[1].Right.Tabs) != 2 || favs[1].Left.Tabs[0].Path != "/w" {
		t.Errorf("work = %+v", favs[1])
	}
	// Replace, then delete.
	SaveFavoriteTabs(f, FavoriteTabs{Name: "Work", Left: TabSet{Tabs: []Tab{{Path: "/new"}}}})
	if favs = LoadFavoriteTabs(f); favs[1].Left.Tabs[0].Path != "/new" || len(favs[1].Right.Tabs) != 0 {
		t.Errorf("replaced = %+v", favs[1])
	}
	if !DeleteFavoriteTabs(f, "Work") || len(LoadFavoriteTabs(f)) != 1 {
		t.Error("delete")
	}
}

func TestPanelState(t *testing.T) {
	f := ParseString("[right]\npath=/r\nShowTree=1\n")
	p := LoadPanelState(f, SectionRight)
	if p.Path != "/r" || p.View != "full" {
		t.Errorf("got %+v", p)
	}
	p.View, p.ColumnSet, p.SortColumn, p.SortDesc = "custom", "Dev", 3, true
	SavePanelState(f, SectionRight, p)
	if !f.Has(SectionRight, "ShowTree") {
		t.Error("unknown key lost")
	}
	if got := LoadPanelState(ParseString(f.String()), SectionRight); got != p {
		t.Errorf("round trip %+v", got)
	}
}

func TestWindowPos(t *testing.T) {
	f := ParseString("[1920x1080 (10x20)]\nx=10\ny=20\ndx=1000\ndy=700\nDivider=400\n[1280x800]\nx=1\n")
	p, ok := LoadWindowPos(f, 1920, 1080)
	if !ok || p.X != 10 || p.W != 1000 || p.Divider != 400 || p.Maximized {
		t.Errorf("got %+v %v", p, ok)
	}
	if _, ok := LoadWindowPos(f, 1280, 800); ok {
		t.Error("section without dx must not match")
	}
	if _, ok := LoadWindowPos(f, 19201, 1080); ok {
		t.Error("prefix of another size matched")
	}
	SaveWindowPos(f, 1920, 1080, WindowPos{X: 5, Y: 6, W: 7, H: 8, Divider: 500, Maximized: true})
	if f.HasSection(ScreenSection(1920, 1080)) {
		t.Error("existing section with another suffix must be reused")
	}
	SaveWindowPos(f, 2560, 1440, WindowPos{W: 1, H: 1})
	if !f.HasSection("2560x1440 (8x16)") {
		t.Error("new section name")
	}
	p, _ = LoadWindowPos(ParseString(f.String()), 1920, 1080)
	if p != (WindowPos{X: 5, Y: 6, W: 7, H: 8, Divider: 500, Maximized: true}) {
		t.Errorf("round trip %+v", p)
	}
}

// Issue #128: each tab keeps its own view, in keys of our own that TC ignores; tabs saved without them have none.
func TestTabSetView(t *testing.T) {
	ts := TabSet{Tabs: []Tab{
		{Path: "/a", View: "brief"},
		{Path: "/b", View: "custom", ColumnSet: "Media"},
		{Path: "/c"},
	}}
	f := NewFile()
	SaveTabSet(f, SectionLeftTabs, ts)
	s := f.String()
	for _, want := range []string{"0_view=brief", "1_view=custom", "1_columnset=Media"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "2_view") || strings.Contains(s, "0_columnset") {
		t.Errorf("empty view keys written:\n%s", s)
	}
	got := LoadTabSet(ParseString(s), SectionLeftTabs)
	for i := range ts.Tabs {
		if !reflect.DeepEqual(got.Tabs[i], ts.Tabs[i]) {
			t.Errorf("tab %d: got %+v, want %+v", i, got.Tabs[i], ts.Tabs[i])
		}
	}
	old := LoadTabSet(ParseString("[lefttabs]\n0_path=/a\n0_options=1|0\nactivetab=0\n"), SectionLeftTabs)
	if old.Tabs[0].View != "" || old.Tabs[0].ColumnSet != "" {
		t.Errorf("a tab saved by an older version got a view: %+v", old.Tabs[0])
	}
}

// Board issue #12: a locked tab that shows another folder than the one it was locked to keeps that folder in a key of
// our own; an unlocked tab, or one still in its locked folder, writes none, and a stray key on an unlocked tab is ignored.
func TestTabSetLockPath(t *testing.T) {
	ts := TabSet{Tabs: []Tab{
		{Path: "/a/sub", Lock: TabLockedChangeOK, LockPath: "/a"},
		{Path: "/b", Lock: TabLocked, LockPath: "/b"},
		{Path: "/c", LockPath: "/x"},
	}, Active: 1}
	f := NewFile()
	SaveTabSet(f, SectionRightTabs, ts)
	s := f.String()
	if !strings.Contains(s, "0_lockpath=/a") || strings.Contains(s, "1_lockpath") || strings.Contains(s, "2_lockpath") {
		t.Fatalf("lockpath keys:\n%s", s)
	}
	got := LoadTabSet(ParseString(s), SectionRightTabs)
	if got.Active != 1 || got.Tabs[0].LockPath != "/a" || got.Tabs[0].Lock != TabLockedChangeOK || got.Tabs[1].LockPath != "" || got.Tabs[2].LockPath != "" {
		t.Fatalf("read back: %+v", got)
	}
	stray := LoadTabSet(ParseString("[righttabs]\n0_path=/a\n0_lockpath=/x\nactivetab=0\n"), SectionRightTabs)
	if stray.Tabs[0].LockPath != "" {
		t.Errorf("an unlocked tab got a lock folder: %+v", stray.Tabs[0])
	}
}

// Board issue #112: each tab can keep its own folder history in "<n>_history<k>" keys, newest first; a tab without
// one writes no keys, and empty or multi-line entries are skipped without leaving gaps.
func TestTabSetHistory(t *testing.T) {
	ts := TabSet{Tabs: []Tab{
		{Path: "/a", History: []string{"/a/old", "", "/bad\nline", "/older"}},
		{Path: "/b"},
	}, Active: 1}
	f := NewFile()
	SaveTabSet(f, SectionLeftTabs, ts)
	s := f.String()
	if !strings.Contains(s, "0_history0=/a/old") || !strings.Contains(s, "0_history1=/older") || strings.Contains(s, "0_history2") || strings.Contains(s, "1_history") {
		t.Fatalf("history keys:\n%s", s)
	}
	got := LoadTabSet(ParseString(s), SectionLeftTabs)
	if strings.Join(got.Tabs[0].History, ",") != "/a/old,/older" || got.Tabs[1].History != nil {
		t.Fatalf("read back: %+v", got)
	}
}
