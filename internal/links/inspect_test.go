// Tests for Inspect and FollowLink, the calls the panel makes: every link
// and shortcut kind is described correctly (target, broken, loop, hard link
// count), and following produces the TC action — open the folder, focus the
// file in its folder, open the URL, or run the .desktop program — or a clear
// error for ordinary files, broken links and loops.

package links

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInspectAndFollow(t *testing.T) {
	dir := realTemp(t)
	folder := filepath.Join(dir, "folder")
	file := filepath.Join(folder, "file.txt")
	writeFile(t, file, "x")
	plain := filepath.Join(dir, "plain.txt")
	writeFile(t, plain, "p")

	symDir := filepath.Join(dir, "symdir")
	symlinkOrSkip(t, folder, symDir)
	symFile := filepath.Join(dir, "symfile")
	symlinkOrSkip(t, filepath.Join("folder", "file.txt"), symFile)
	broken := filepath.Join(dir, "broken")
	symlinkOrSkip(t, filepath.Join(dir, "nothing"), broken)
	loop := filepath.Join(dir, "loop")
	symlinkOrSkip(t, "loop", loop)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	webURL := filepath.Join(dir, "site.url")
	must(CreateShortcut(KindURL, "https://example.com/", webURL, false))
	weblocFile := filepath.Join(dir, "file.webloc")
	must(CreateShortcut(KindWebloc, file, weblocFile, false))
	desktopDir := filepath.Join(dir, "folder.desktop")
	must(CreateShortcut(KindDesktop, folder, desktopDir, false))
	app := filepath.Join(dir, "app.desktop")
	writeFile(t, app, "[Desktop Entry]\nType=Application\nName=App\nExec=/bin/echo hi %u\nPath=/tmp\n")
	lnkForeign := filepath.Join(dir, "win.lnk")
	writeFile(t, lnkForeign, string(buildLnk(lnkSpec{localBase: `Z:\nowhere\x`, name: "desc"})))
	lnkRel := filepath.Join(dir, "rel.lnk")
	writeFile(t, lnkRel, string(buildLnk(lnkSpec{rel: `.\folder\file.txt`})))
	alias := filepath.Join(dir, "an alias")
	writeFile(t, alias, string(buildBookmark(splitPath(folder), true)))
	badURL := filepath.Join(dir, "bad.url")
	writeFile(t, badURL, "not an ini")
	// Z:\nowhere\x is a foreign path on macOS and Linux (existence unknown, so not marked broken), but a real local
	// path on Windows, where it does not exist and the shortcut is broken.
	foreignBroken := runtime.GOOS == "windows"

	tests := []struct {
		name      string
		path      string
		kind      Kind
		broken    bool
		loop      bool
		action    ActionKind
		actDir    string
		actFocus  string
		actURL    string
		followErr error
	}{
		{"plain", plain, KindNone, false, false, 0, "", "", "", ErrNotLink},
		{"symlink to dir", symDir, KindSymlink, false, false, ActionOpenDir, folder, "", "", nil},
		{"symlink to file", symFile, KindSymlink, false, false, ActionFocusFile, folder, "file.txt", "", nil},
		{"broken", broken, KindSymlink, true, false, 0, "", "", "", ErrBroken},
		{"loop", loop, KindSymlink, false, true, 0, "", "", "", ErrLoop},
		{"url", webURL, KindURL, false, false, ActionOpenURL, "", "", "https://example.com/", nil},
		{"webloc to file", weblocFile, KindWebloc, false, false, ActionFocusFile, folder, "file.txt", "", nil},
		{"desktop to dir", desktopDir, KindDesktop, false, false, ActionOpenDir, folder, "", "", nil},
		{"desktop app", app, KindDesktop, false, false, ActionRun, "/tmp", "", "", nil},
		{"lnk foreign path", lnkForeign, KindLnk, foreignBroken, false, 0, "", "", "", ErrBroken},
		{"lnk relative", lnkRel, KindLnk, false, false, ActionFocusFile, folder, "file.txt", "", nil},
		{"finder alias", alias, KindAlias, false, false, ActionOpenDir, folder, "", "", nil},
		{"unparsable url file", badURL, KindNone, false, false, 0, "", "", "", ErrNotLink},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind == KindAlias && runtime.GOOS == "windows" {
				t.Skip("Finder aliases store POSIX paths")
			}
			info, err := Inspect(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Kind != tc.kind || info.Broken != tc.broken || info.Loop != tc.loop {
				t.Errorf("Inspect = kind %v broken %v loop %v, want %v %v %v (%+v)", info.Kind, info.Broken, info.Loop,
					tc.kind, tc.broken, tc.loop, info)
			}
			a, err := FollowLink(tc.path)
			if tc.followErr != nil {
				if !errors.Is(err, tc.followErr) {
					t.Errorf("FollowLink err = %v, want %v", err, tc.followErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.Kind != tc.action || a.Dir != tc.actDir || a.Focus != tc.actFocus || a.URL != tc.actURL {
				t.Errorf("action = %+v", a)
			}
		})
	}

	// Details shown in Properties.
	info, _ := Inspect(symFile)
	if info.Target != filepath.Join("folder", "file.txt") || info.Resolved != file || len(info.Chain) != 2 || info.HardLinks != 1 {
		t.Errorf("symlink details: %+v", info)
	}
	info, _ = Inspect(lnkForeign)
	if info.Target != `Z:\nowhere\x` || info.Name != "desc" || info.Resolved != "" && !filepath.IsAbs(`Z:\nowhere\x`) {
		t.Errorf("lnk details: %+v", info)
	}
	a, _ := FollowLink(app)
	if len(a.Args) != 2 || a.Args[0] != "/bin/echo" || a.Args[1] != "hi" {
		t.Errorf("desktop args: %q", a.Args)
	}
	if _, err := Inspect(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: %v", err)
	}
	// A folder is inspected without error and is not a link.
	if info, err := Inspect(folder); err != nil || info.Kind != KindNone {
		t.Errorf("folder: %+v %v", info, err)
	}
}

func TestCreateShortcutRules(t *testing.T) {
	dir := realTemp(t)
	p := filepath.Join(dir, "x.url")
	if err := CreateShortcut(KindURL, "https://a/", p, false); err != nil {
		t.Fatal(err)
	}
	if err := CreateShortcut(KindURL, "https://b/", p, false); !errors.Is(err, fs.ErrExist) {
		t.Errorf("no overwrite: %v", err)
	}
	if err := CreateShortcut(KindURL, "https://b/", p, true); err != nil {
		t.Fatal(err)
	}
	if info, _ := Inspect(p); info.URL != "https://b/" {
		t.Errorf("after overwrite: %+v", info)
	}
	if err := CreateShortcut(KindLnk, "https://b/", filepath.Join(dir, "y.lnk"), false); !errors.Is(err, ErrUnsupported) {
		t.Errorf("lnk: %v", err)
	}
	if err := CreateShortcut(KindURL, "https://b/", dir, true); !errors.Is(err, fs.ErrExist) {
		t.Errorf("over folder: %v", err)
	}
	if err := CreateShortcut(KindURL, "", p, true); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("empty target: %v", err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("leftover files: %v", ents)
	}
	if ShortcutKindByName("A.LNK") != KindLnk || ShortcutKindByName("a.txt") != KindNone || ShortcutExt(KindSymlink) != "" {
		t.Error("ShortcutKindByName / ShortcutExt")
	}
}

func splitPath(p string) []string {
	var out []string
	for p != "" && p != string(filepath.Separator) && p != filepath.VolumeName(p)+string(filepath.Separator) {
		out = append([]string{filepath.Base(p)}, out...)
		p = filepath.Dir(p)
	}
	return out
}
