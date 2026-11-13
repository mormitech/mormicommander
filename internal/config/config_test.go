// Tests for locating, opening and saving the whole configuration: portable
// mode and command-line overrides pick the right file, a fresh start works
// without a file, a save keeps comments and unknown sections, the "remember
// history" options decide whether histories are written, hand edits come
// back with Reload, and Export/Restore make a usable backup. Everything runs
// in t.TempDir().

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocate(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "exe")
	user := filepath.Join(root, "user")
	other := filepath.Join(root, "other")
	for _, d := range []string{exe, user, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	portableExe := filepath.Join(root, "pexe")
	os.MkdirAll(portableExe, 0o755)
	os.WriteFile(filepath.Join(portableExe, IniName), []byte("[Configuration]\n"), 0o644)

	cases := []struct {
		name     string
		opt      LocateOptions
		wantIni  string
		wantFtp  string
		portable bool
		override bool
	}{
		{"user dir", LocateOptions{ExeDir: exe, UserConfigDir: user},
			filepath.Join(user, AppDirName, IniName), filepath.Join(user, AppDirName, FtpIniName), false, false},
		{"portable", LocateOptions{ExeDir: portableExe, UserConfigDir: user},
			filepath.Join(portableExe, IniName), filepath.Join(portableExe, FtpIniName), true, false},
		{"override file", LocateOptions{ExeDir: portableExe, UserConfigDir: user, IniOverride: filepath.Join(other, "x.ini")},
			filepath.Join(other, "x.ini"), filepath.Join(other, FtpIniName), false, true},
		{"override folder quoted", LocateOptions{ExeDir: exe, UserConfigDir: user, IniOverride: `"` + other + `"`, FtpOverride: filepath.Join(root, "f.ini")},
			filepath.Join(other, IniName), filepath.Join(root, "f.ini"), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loc, err := Locate(c.opt)
			if err != nil {
				t.Fatal(err)
			}
			if loc.Ini != c.wantIni || loc.FtpIni != c.wantFtp || loc.Portable != c.portable || loc.Override != c.override {
				t.Errorf("got %+v", loc)
			}
			if loc.Dir != filepath.Dir(loc.Ini) {
				t.Errorf("Dir = %s", loc.Dir)
			}
		})
	}
}

func TestParseConfigArgs(t *testing.T) {
	ini, ftp, rest := ParseConfigArgs([]string{"/I=c:\\a.ini", "-o", "--ftpini=/x/f.ini", "/L=c:\\", "-T"})
	if ini != `c:\a.ini` || ftp != "/x/f.ini" {
		t.Errorf("ini=%q ftp=%q", ini, ftp)
	}
	if strings.Join(rest, " ") != `-o /L=c:\ -T` {
		t.Errorf("rest = %v", rest)
	}
}

func openTemp(t *testing.T, content string) *Config {
	t.Helper()
	dir := t.TempDir()
	ini := filepath.Join(dir, IniName)
	if content != "" {
		if err := os.WriteFile(ini, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Open(Locations{Dir: dir, Ini: ini, FtpIni: filepath.Join(dir, FtpIniName)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOpenMissingAndSave(t *testing.T) {
	c := openTemp(t, "")
	if c.Settings != Defaults() {
		t.Error("fresh config must use defaults")
	}
	if len(c.Hotlist.Items) != 0 || c.Keymap == nil || c.CmdHistory == nil {
		t.Error("models not initialised")
	}
	c.Hotlist.AddFolder("", "/tmp/data", "")
	c.CmdHistory.Add("ls -la")
	c.Settings.Display.ShowHidden = true
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(c.Loc.Ini)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"[DirMenu]", "menu1=data", "[Command line history]", "0=ls -la"} {
		if !strings.Contains(s, want) {
			t.Errorf("saved file lacks %q:\n%s", want, s)
		}
	}
	c2, err := Open(c.Loc)
	if err != nil {
		t.Fatal(err)
	}
	if !c2.Settings.Display.ShowHidden || len(c2.Hotlist.Items) != 1 || c2.CmdHistory.Items()[0] != "ls -la" {
		t.Error("reopened config differs")
	}
}

const sampleIni = `; mormiCommander settings, edited by hand
[Configuration]
SaveHistory=1
SaveCommands=0
Savepath=1
; keep this comment
[SomeOtherTool]
foo=bar
[LeftHistory]
0=/a
1=/b
[Command line history]
0=make
[left]
path=/a
View=brief
sortorder=2
negative=1
[lefttabs]
0_path=/a
0_caption=Home
1_path=/b
1_locked=1
activetab=1
`

func TestSyncPreservesAndRespectsOptions(t *testing.T) {
	c := openTemp(t, sampleIni)
	if got := c.LeftHistory.Items(); len(got) != 2 || got[0] != "/a" {
		t.Errorf("left history = %v", got)
	}
	if c.Left.View != "brief" || c.Left.SortColumn != 2 || !c.Left.SortDesc {
		t.Errorf("left panel = %+v", c.Left)
	}
	if c.LeftTabs.Active != 1 || c.LeftTabs.Tabs[0].Caption != "Home" || c.LeftTabs.Tabs[1].Lock != TabLocked {
		t.Errorf("left tabs = %+v", c.LeftTabs)
	}
	c.LeftHistory.Add("/c")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(c.Loc.Ini)
	s := string(data)
	for _, want := range []string{"; keep this comment", "[SomeOtherTool]", "foo=bar", "0=/c"} {
		if !strings.Contains(s, want) {
			t.Errorf("lost %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "[Command line history]") {
		t.Error("SaveCommands=0 must drop the command line history")
	}
	if !strings.HasPrefix(s, "; mormiCommander settings") {
		t.Error("leading comment moved")
	}
}

func TestReloadExportRestore(t *testing.T) {
	c := openTemp(t, sampleIni)
	// Hand edit while running.
	data, _ := os.ReadFile(c.Loc.Ini)
	edited := strings.Replace(string(data), "[Configuration]", "[Configuration]\nConfirmExit=1", 1)
	os.WriteFile(c.Loc.Ini, []byte(edited), 0o644)
	if err := c.Reload(); err != nil {
		t.Fatal(err)
	}
	if !c.Settings.Misc.ConfirmExit {
		t.Error("Reload did not pick up the hand edit")
	}
	backup := filepath.Join(t.TempDir(), "backup.ini")
	c.Settings.Misc.WindowTitle = "Backup title"
	if err := c.Export(backup); err != nil {
		t.Fatal(err)
	}
	live, _ := os.ReadFile(c.Loc.Ini)
	if strings.Contains(string(live), "Backup title") {
		t.Error("Export must not touch the live file")
	}
	c.Settings.Misc.WindowTitle = "Changed"
	if err := c.Restore(backup); err != nil {
		t.Fatal(err)
	}
	if c.Settings.Misc.WindowTitle != "Backup title" {
		t.Errorf("restored title = %q", c.Settings.Misc.WindowTitle)
	}
	live, _ = os.ReadFile(c.Loc.Ini)
	if !strings.Contains(string(live), "TitleBarText=Backup title") {
		t.Error("Restore must write the live file")
	}
}

func TestConfigPathsAndBars(t *testing.T) {
	c := openTemp(t, "")
	if got := c.ButtonBarPath(); got != filepath.Join(c.Loc.Dir, "default.bar") {
		t.Errorf("default bar path = %s", got)
	}
	bar, err := c.LoadButtonBar()
	if err != nil || len(bar.Buttons) != len(DefaultBar().Buttons) {
		t.Fatalf("default bar: %v", err)
	}
	p := filepath.Join(c.Loc.Dir, "bars", "mine.bar")
	c.SetButtonBarPath(p)
	if v, _ := c.File.Get(SectionButtonbar, "Buttonbar"); !strings.Contains(v, "%COMMANDER_PATH%") {
		t.Errorf("stored bar path not contracted: %q", v)
	}
	if got := c.ButtonBarPath(); got != p {
		t.Errorf("bar path = %s, want %s", got, p)
	}
	if c.VerticalBarPath() != "" {
		t.Error("no vertical bar expected")
	}
	if got := c.Path("rel/x.txt"); got != filepath.Join(c.Loc.Dir, "rel", "x.txt") {
		t.Errorf("Path(rel) = %s", got)
	}
	if got := c.Path("%COMMANDER_PATH%/ignore.txt"); got != filepath.Join(c.Loc.Dir, "ignore.txt") {
		t.Errorf("Path(var) = %s", got)
	}
}

func TestIgnoreListViaConfig(t *testing.T) {
	c := openTemp(t, "")
	os.WriteFile(filepath.Join(c.Loc.Dir, "ignore.txt"), []byte("; hidden\n.DS_Store\n*.tmp\n"), 0o644)
	l, err := c.LoadIgnoreList()
	if err != nil || len(l.Masks) != 0 {
		t.Fatalf("disabled list must be empty: %v %v", l, err)
	}
	c.Settings.IgnoreList.Enabled = true
	l, err = c.LoadIgnoreList()
	if err != nil {
		t.Fatal(err)
	}
	if !l.Hidden(".DS_Store", "/x/.DS_Store", false) || !l.Hidden("a.tmp", "/x/a.tmp", false) || l.Hidden("a.txt", "/x/a.txt", false) {
		t.Error("ignore list matching wrong")
	}
}

func TestActiveColors(t *testing.T) {
	c := openTemp(t, "[Colors]\nForeColor=255\nColorFilter1=*.go\nColorFilter1Color=#00ff00\n")
	light := c.ActiveColors(false) // DarkMode default 2 = follow system
	if light.Fore != RGB(255, 0, 0) {
		t.Errorf("light fore = %s", light.Fore.Hex())
	}
	if light.Back.IsDefault() {
		t.Error("unset colours must come from the preset")
	}
	dark := c.ActiveColors(true)
	if dark.Fore == RGB(255, 0, 0) {
		t.Error("dark mode without [ColorsDark] must use the dark preset")
	}
	if len(dark.Filters) != 1 || dark.FileColor("a.go", "", false) != RGB(0, 255, 0) {
		t.Error("file type colours must carry over to dark mode")
	}
	c.Settings.Colors.DarkMode = 0
	if c.ActiveColors(true).Fore != RGB(255, 0, 0) {
		t.Error("DarkMode=0 must ignore the system")
	}
}

func TestDefaultBarCommands(t *testing.T) {
	want := []string{"cm_RereadSource", "cm_SrcShort", "cm_SrcLong", "cm_SrcThumbs", "cm_SrcTree",
		"cm_ExchangeSelection", "cm_GoToPrev", "cm_GoToNext", "cm_PackFiles", "cm_UnpackFiles",
		"cm_SearchFor", "cm_List", "cm_MultiRename", "cm_SyncDirs", "cm_FtpConnect",
		"cm_DirectoryHotlist", "cm_Edit", "cm_CompareFilesByContent"}
	have := map[string]bool{}
	for _, b := range DefaultBar().Buttons {
		if !b.IsSeparator() {
			have[b.Command] = true
			if b.Tooltip == "" {
				t.Errorf("%s has no tooltip", b.Command)
			}
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("default bar lacks %s", w)
		}
	}
}
