package main

// Tests of the tabs and hotlist feature's Go side: what the page stores comes back after a restart, in TC's format.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func thTest(t *testing.T) (*TabsHotlist, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "folders.ini")
	return thOpen(&App{}, path), path
}

func TestTHHotlistRoundTrip(t *testing.T) {
	th, path := thTest(t)
	folder := filepath.Join(t.TempDir(), "my docs")
	if err := th.AddFolder("", folder, ""); err != nil {
		t.Fatal(err)
	}
	if !th.Contains(folder) {
		t.Fatal("the added folder is not in the hotlist")
	}
	items := th.Hotlist()
	items = append(items, THItem{Kind: "sep"}, THItem{Kind: "menu", Title: "&Work", Items: []THItem{{Kind: "cmd", Title: "&Src", Cmd: "cd ~/src", Target: "$HOME/build"}}})
	if err := th.SetHotlist(items); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "[DirMenu]") || !strings.Contains(string(b), "menu3=-&Work") {
		t.Fatalf("folders.ini is not in TC's [DirMenu] form:\n%s", b)
	}
	again := thOpen(&App{}, path).Hotlist()
	if len(again) != 3 || again[0].Label != "my docs" || again[2].Kind != "menu" || again[2].Key != "w" || again[2].Items[0].Label != "Src" {
		t.Fatalf("hotlist after a restart: %+v", again)
	}
	home, _ := os.UserHomeDir()
	a := th.Resolve(again[2].Items[0].Cmd, again[2].Items[0].Target)
	if a.Kind != "cd" || a.Path != filepath.Join(home, "src") || a.Target != filepath.Join(home, "build") {
		t.Fatalf("resolve: %+v", a)
	}
	if ok, err := th.RemoveFolder(folder); !ok || err != nil {
		t.Fatalf("remove: %v %v", ok, err)
	}
	if th.Contains(folder) {
		t.Fatal("the folder is still in the hotlist")
	}
	if m := th.Filter("src"); len(m) != 1 || m[0].Breadcrumb != "Work" {
		t.Fatalf("filter: %+v", m)
	}
}

func TestTHResolveKinds(t *testing.T) {
	th, _ := thTest(t)
	for cmd, kind := range map[string]string{"cd /a/b.tab": "tabs", "cm_SrcThumbs": "internal", "em_backup now": "user", "notes.txt": "program", "cd /tmp/*.txt": "cd", "": ""} {
		if a := th.Resolve(cmd, ""); a.Kind != kind {
			t.Errorf("%q: kind %q, want %q", cmd, a.Kind, kind)
		}
	}
	if a := th.Resolve("cd /tmp/*.txt", ""); a.Path != filepath.FromSlash("/tmp") || a.Filter != "*.txt" {
		t.Errorf("filter entry: %+v", a)
	}
	if got := th.MatchMask("*.txt | b*", []string{"a.txt", "b.txt", "c.md"}, nil); !got[0] || got[1] || got[2] {
		t.Errorf("mask: %v", got)
	}
}

func TestTHTabsAndFavorites(t *testing.T) {
	th, path := thTest(t)
	s := THTabs{
		Left:        THTabSet{Tabs: []THTab{{Path: "/a", Sort: "size", Desc: true}, {Path: "/b", Caption: "Bee", Lock: 1}}, Active: 1},
		Right:       THTabSet{Tabs: []THTab{{Path: "/c", Lock: 2, Sort: "ext"}}},
		LeftHistory: []string{"/x", "/y", "/x", "/z"}, HistoryMax: 30,
	}
	if err := th.SaveTabs(s); err != nil {
		t.Fatal(err)
	}
	th.flush()
	got := thOpen(&App{}, path).Tabs()
	if got.Left.Active != 1 || len(got.Left.Tabs) != 2 || got.Left.Tabs[0].Sort != "size" || !got.Left.Tabs[0].Desc || got.Left.Tabs[1].Caption != "Bee" || got.Left.Tabs[1].Lock != 1 {
		t.Fatalf("left tabs after a restart: %+v", got.Left)
	}
	if got.Right.Tabs[0].Lock != 2 || got.Right.Tabs[0].Sort != "ext" {
		t.Fatalf("right tabs: %+v", got.Right)
	}
	if strings.Join(got.LeftHistory, ",") != "/x,/y,/z" {
		t.Fatalf("history: %v", got.LeftHistory)
	}

	tab := filepath.Join(t.TempDir(), "sub", "set.tab")
	if err := th.SaveTabFile(tab, THTabFile{Active: s.Left, Inactive: s.Right}); err != nil {
		t.Fatal(err)
	}
	tf, err := th.LoadTabFile(tab)
	if err != nil || len(tf.Active.Tabs) != 2 || len(tf.Inactive.Tabs) != 1 {
		t.Fatalf("tab file: %+v %v", tf, err)
	}
	if _, err := th.LoadTabFile(filepath.Join(t.TempDir(), "none.tab")); err == nil {
		t.Fatal("a missing tab file loaded")
	}

	if err := th.SaveFavorite(THFavorite{Name: "Work", Left: s.Left, Right: s.Right}); err != nil {
		t.Fatal(err)
	}
	if err := th.SaveFavorite(THFavorite{Name: " "}); err == nil {
		t.Fatal("a nameless favourite was saved")
	}
	if err := th.RenameFavorite("Work", "Home"); err != nil {
		t.Fatal(err)
	}
	favs := thOpen(&App{}, path).Favorites()
	if len(favs) != 1 || favs[0].Name != "Home" || len(favs[0].Left.Tabs) != 2 {
		t.Fatalf("favourites: %+v", favs)
	}
	if err := th.DeleteFavorite("Home"); err != nil || len(th.Favorites()) != 0 {
		t.Fatalf("delete favourite: %v", err)
	}
}

func TestTHFrequentAndImport(t *testing.T) {
	th, path := thTest(t)
	for _, f := range []string{"/a", "/b", "/a", "/a", "/c"} {
		th.Visit(f)
	}
	th.flush()
	if top := thOpen(&App{}, path).Frequent(2); len(top) != 2 || top[0] != "/a" {
		t.Fatalf("frequent: %v", top)
	}
	if err := th.ClearFrequent(); err != nil || len(th.Frequent(5)) != 0 {
		t.Fatalf("clear: %v", err)
	}
	if got := th.TrimHistory([]string{"/1", "/2", "/1", "/3", "/4", "/5", "/6"}, 5); len(got) != 5 || got[0] != "/1" {
		t.Fatalf("trim: %v", got)
	}

	ini := filepath.Join(t.TempDir(), "wincmd.ini")
	os.WriteFile(ini, []byte("[DirMenu]\r\nmenu1=&Desktop\r\ncmd1=cd %USERPROFILE%\\Desktop\r\nmenu2=-Sub\r\nmenu3=X\r\ncmd3=cd c:\\x\r\nmenu4=--\r\n"), 0o644)
	items, err := th.ReadTC(ini)
	if err != nil || len(items) != 2 || items[0].Key != "d" || len(items[1].Items) != 1 {
		t.Fatalf("import: %+v %v", items, err)
	}
	os.WriteFile(ini, []byte("[Configuration]\n"), 0o644)
	if _, err := th.ReadTC(ini); err == nil {
		t.Fatal("a file without [DirMenu] imported")
	}
}

// Issue #128: the view of each tab comes back after a restart; a tab saved without one has none.
func TestTHTabsView(t *testing.T) {
	th, path := thTest(t)
	s := THTabs{Left: THTabSet{Tabs: []THTab{{Path: "/a", Sort: "size", View: "brief"}, {Path: "/b", View: "custom", Set: "Media"}, {Path: "/c"}}}}
	if err := th.SaveTabs(s); err != nil {
		t.Fatal(err)
	}
	th.flush()
	got := thOpen(&App{}, path).Tabs().Left.Tabs
	if len(got) != 3 || got[0].View != "brief" || got[0].Sort != "size" || got[1].View != "custom" || got[1].Set != "Media" || got[2].View != "" || got[2].Set != "" {
		t.Fatalf("tab views after a restart: %+v", got)
	}
}

// Board issue #12: after a restart every tab of both sides comes back with its folder, caption, lock, the folder a
// "folder changes allowed" tab was locked to, and which tab was active.
func TestTHTabsRestoreAll(t *testing.T) {
	th, path := thTest(t)
	s := THTabs{
		Left:  THTabSet{Tabs: []THTab{{Path: "/a"}, {Path: "/a/sub", Lock: 2, LockPath: "/a", Caption: "Work"}, {Path: "/b", Lock: 1}}, Active: 2},
		Right: THTabSet{Tabs: []THTab{{Path: "/r1"}, {Path: "/r2", LockPath: "/ignored"}}, Active: 1},
	}
	if err := th.SaveTabs(s); err != nil {
		t.Fatal(err)
	}
	th.flush()
	got := thOpen(&App{}, path).Tabs()
	l, r := got.Left, got.Right
	if len(l.Tabs) != 3 || l.Active != 2 || l.Tabs[1].Lock != 2 || l.Tabs[1].LockPath != "/a" || l.Tabs[1].Caption != "Work" || l.Tabs[2].Lock != 1 || l.Tabs[2].LockPath != "" {
		t.Fatalf("left tabs after a restart: %+v", l)
	}
	if len(r.Tabs) != 2 || r.Active != 1 || r.Tabs[1].Path != "/r2" || r.Tabs[1].LockPath != "" {
		t.Fatalf("right tabs after a restart: %+v", r)
	}
}

// Board issue #112: the folder history of every tab survives a restart, not only the active tab's. The active tab's
// stays in [LeftHistory]; the others are trimmed to the history size and lose repeats.
func TestTHTabsHistoryPerTab(t *testing.T) {
	th, path := thTest(t)
	s := THTabs{
		Left: THTabSet{Tabs: []THTab{
			{Path: "/a", History: []string{"/a1", "/a2", "/a1", "/a3", "/a4"}},
			{Path: "/b", History: []string{"/ignored"}},
			{Path: "/c"},
		}, Active: 1},
		Right:       THTabSet{Tabs: []THTab{{Path: "/r"}, {Path: "/s", History: []string{"/s1"}}}},
		LeftHistory: []string{"/b1"}, HistoryMax: 3,
	}
	if err := th.SaveTabs(s); err != nil {
		t.Fatal(err)
	}
	th.flush()
	got := thOpen(&App{}, path).Tabs()
	if h := strings.Join(got.Left.Tabs[0].History, ","); h != "/a1,/a2,/a3" {
		t.Fatalf("inactive tab history: %q", h)
	}
	if got.Left.Tabs[1].History != nil || got.Left.Tabs[2].History != nil {
		t.Fatalf("active or empty tab got a history: %+v", got.Left.Tabs)
	}
	if strings.Join(got.LeftHistory, ",") != "/b1" || strings.Join(got.Right.Tabs[1].History, ",") != "/s1" {
		t.Fatalf("other histories: %v %+v", got.LeftHistory, got.Right)
	}
}
