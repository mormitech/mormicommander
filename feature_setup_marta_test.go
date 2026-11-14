package main

// Tests of "Import from Marta" (feature_setup_marta.go and TabsHotlist.ImportFolders): Marta is found only when its
// settings folder is there, its favourites land in a "Marta" submenu of the hotlist, folders the hotlist already has
// are skipped, a second import adds nothing, and Marta's files are left exactly as they were. The home folder, the
// Marta folder and folders.ini are all in temporary folders.

import (
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/config"
	"mormicommander/internal/marta"
)

// withMartaFakes points the setup at a temporary home folder and hotlist for one test.
func withMartaFakes(t *testing.T, goos string) (home string, th *TabsHotlist) {
	t.Helper()
	withSetupFakes(t, goos, "", nil)
	home = t.TempDir()
	th = thOpen(nil, filepath.Join(t.TempDir(), "folders.ini"))
	oldHome, oldHot := setupHome, setupHotlist
	setupHome = func() (string, error) { return home, nil }
	setupHotlist = func() *TabsHotlist { return th }
	t.Cleanup(func() { setupHome, setupHotlist = oldHome, oldHot })
	return home, th
}

func TestMartaNotFound(t *testing.T) {
	withMartaFakes(t, "darwin")
	s := &Setup{}
	if info := s.Marta(); info.Found || len(info.Favorites) != 0 {
		t.Fatalf("no Marta folder: %+v", info)
	}
	if _, err := s.ImportMarta(); err == nil {
		t.Fatal("importing without Marta should fail")
	}

	// not a Mac: never found, even with the folder there
	home, _ := withMartaFakes(t, "linux")
	os.MkdirAll(marta.Dir(home), 0o755)
	if info := s.Marta(); info.Found {
		t.Fatal("Marta is a Mac app; it should not be looked for elsewhere")
	}
}

func TestMartaImportIntoHotlist(t *testing.T) {
	home, th := withMartaFakes(t, "darwin")
	dir := marta.Dir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fav := `[ { name "Code" path "~/Code" } { name "Logs" path "/var/log" } "/opt" ]`
	fp := filepath.Join(dir, marta.FavoritesFile)
	if err := os.WriteFile(fp, []byte(fav), 0o600); err != nil {
		t.Fatal(err)
	}
	// the hotlist already has /var/log (at the top level): it is not added again
	if err := th.AddFolder("My logs", "/var/log", ""); err != nil {
		t.Fatal(err)
	}

	s := &Setup{}
	info := s.Marta()
	if !info.Found || info.Dir != dir || len(info.Favorites) != 3 || info.Error != "" {
		t.Fatalf("info: %+v", info)
	}
	n, err := s.ImportMarta()
	if err != nil || n != 2 {
		t.Fatalf("import: %d %v", n, err)
	}
	tree := th.Hotlist()
	if len(tree) != 2 || tree[0].Label != "My logs" || tree[1].Kind != "menu" || tree[1].Label != "Marta" {
		t.Fatalf("tree: %+v", tree)
	}
	got := tree[1].Items
	if len(got) != 2 || got[0].Label != "Code" || got[1].Label != "opt" {
		t.Fatalf("Marta submenu: %+v", got)
	}
	if a := config.ParseHotlistCommand(got[0].Cmd); a.Path != filepath.Join(home, "Code") {
		t.Fatalf("Code entry: %+v", got[0])
	}

	// saved to folders.ini
	again := thOpen(nil, th.path)
	if tr := again.Hotlist(); len(tr) != 2 || len(tr[1].Items) != 2 {
		t.Fatalf("saved tree: %+v", tr)
	}

	// a second import adds nothing and makes no second submenu
	if n, err := s.ImportMarta(); err != nil || n != 0 {
		t.Fatalf("second import: %d %v", n, err)
	}
	if tr := th.Hotlist(); len(tr) != 2 {
		t.Fatalf("second import changed the tree: %+v", tr)
	}

	// Marta's file is unchanged and nothing was added to its folder
	if b, _ := os.ReadFile(fp); string(b) != fav {
		t.Error("Marta's favourites file was changed")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("Marta's folder has %d files", len(entries))
	}
}

func TestImportFoldersReusesSubmenu(t *testing.T) {
	th := thOpen(nil, filepath.Join(t.TempDir(), "folders.ini"))
	if n, err := th.ImportFolders("Marta", []THFolder{{Folder: "/a"}}); err != nil || n != 1 {
		t.Fatalf("first: %d %v", n, err)
	}
	if n, err := th.ImportFolders("Marta", []THFolder{{Folder: "/a"}, {Title: "Bee", Folder: "/b"}, {Folder: " "}, {Folder: "/"}}); err != nil || n != 2 {
		t.Fatalf("second: %d %v", n, err)
	}
	tree := th.Hotlist()
	if len(tree) != 1 || len(tree[0].Items) != 3 || tree[0].Items[1].Label != "Bee" || tree[0].Items[2].Label != "/" {
		t.Fatalf("tree: %+v", tree)
	}
	// nothing to add: no empty submenu is made
	th2 := thOpen(nil, "")
	if n, _ := th2.ImportFolders("Marta", nil); n != 0 || len(th2.Hotlist()) != 0 {
		t.Fatal("an empty import should change nothing")
	}
}
