// Tests for importing a real Total Commander setup. The fixture is a
// wincmd.ini written the way TC writes it (Windows-1252 bytes, CRLF line
// ends, Windows paths, %COMMANDER_PATH% references), together with a
// DEFAULT.BAR that opens a sub-bar and a usercmd.ini, all copied into a temp
// folder. The import must bring hotlist, hotkeys, colours, column sets,
// Start menu, histories, options, bars and user commands over, warn about
// Windows drive paths, and leave our own settings file alone until Save.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const tcWincmd = `[Configuration]
InstallDir=C:\totalcmd
ShowHiddenSystem=1
DirBrackets=0
TitleBarText=Total Commander
IgnoreListFile=C:\totalcmd\ignore.txt
[Buttonbar]
Buttonbar=%COMMANDER_PATH%\DEFAULT.BAR
Buttonheight=32
SmallIcons=1
[DirMenu]
menu1=&Dokumente Äpfel
cmd1=cd C:\Users\peter\Documents
menu2=-Projects
menu3=&Go
cmd3=cd \\server\share\go
path3=d:\target
menu4=--
menu5=-
menu6=Thumbnails
cmd6=cm_SrcThumbs
[Shortcuts]
C+Y=cm_Edit
CS+F1=em_backup
[ShortcutsWin]
E=cm_Exit
[Colors]
ForeColor=0
BackColor=16777215
ColorFilter1=*.exe *.bat
ColorFilter1Color=255
ColorFilter2=*.txt
ColorFilter2Color=32768
[CustomFields]
Titles=Sizes
Widths1=150,-60,100
Headers1=Size\nDate
Contents1=[=tc.size]\n[=tc.writedate]
Options1=-1|0
[user]
menu1=Notepad
cmd1=notepad.exe
param1=%P%N
key1=CS+N
menu2=-Tools
menu3=Calc
cmd3=calc.exe
menu4=--
[LeftHistory]
0=C:\
1=D:\Data
[SearchName]
0=*.txt
[Command line history]
0=dir /s
`

const tcDefaultBar = `[Buttonbar]
Buttoncount=3
button1=%COMMANDER_PATH%\WCMICONS.DLL,1
cmd1=cm_RereadSource
menu1=Reread
button2=
button3=wcmicons.dll,5
cmd3=C:\totalcmd\Tools.bar
menu3=Tools
`

const tcToolsBar = `[Buttonbar]
Buttoncount=1
button1=notepad.exe
cmd1=notepad.exe
menu1=Notepad
`

const tcUserCmd = `[em_backup]
cmd=rsync
param=-a %P %T
menu=Backup
`

func writeCP1252(t *testing.T, path, text string) {
	t.Helper()
	text = strings.ReplaceAll(text, "\n", "\r\n")
	data, err := Encode(text, EncCP1252)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportTC(t *testing.T) {
	tcDir := t.TempDir()
	iniPath := filepath.Join(tcDir, "wincmd.ini")
	writeCP1252(t, iniPath, tcWincmd)
	writeCP1252(t, filepath.Join(tcDir, "DEFAULT.BAR"), tcDefaultBar)
	writeCP1252(t, filepath.Join(tcDir, "tools.bar"), tcToolsBar)
	writeCP1252(t, filepath.Join(tcDir, "usercmd.ini"), tcUserCmd)

	c := openTemp(t, "")
	c.Hotlist.AddFolder("Mine", "/mine", "")
	rep, err := ImportTC(c, iniPath, AllImport())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("imported: %v", rep.Imported)
	t.Logf("warnings: %v", rep.Warnings)

	// Hotlist: ours stays, TC's goes into a submenu, accents decoded.
	if len(c.Hotlist.Items) != 2 || c.Hotlist.Items[1].Kind != MenuSubmenu || c.Hotlist.Items[1].Title != "Total Commander" {
		t.Fatalf("hotlist top level = %+v", c.Hotlist.Items)
	}
	tcItems := c.Hotlist.Items[1].Children
	if tcItems[0].Title != "&Dokumente Äpfel" {
		t.Errorf("title = %q", tcItems[0].Title)
	}
	if tcItems[1].Kind != MenuSubmenu || len(tcItems[1].Children) != 1 || tcItems[1].Children[0].Field("path") != `d:\target` {
		t.Errorf("submenu = %+v", tcItems[1])
	}
	if tcItems[2].Kind != MenuSeparator || tcItems[3].Field("cmd") != "cm_SrcThumbs" {
		t.Errorf("rest = %+v %+v", tcItems[2], tcItems[3])
	}

	// Hotkeys.
	if b, ok := c.Keymap.Lookup(Hotkey{Ctrl: true, Key: "Y"}); !ok || b.Command != "cm_Edit" {
		t.Errorf("C+Y = %+v %v", b, ok)
	}
	if b, ok := c.Keymap.Lookup(Hotkey{Win: true, Key: "E"}); !ok || b.Command != "cm_Exit" {
		t.Errorf("W+E = %+v %v", b, ok)
	}

	// Colours.
	if c.Colors.Fore != RGB(0, 0, 0) || c.Colors.Back != RGB(255, 255, 255) || len(c.Colors.Filters) != 2 {
		t.Errorf("colours = %+v", c.Colors)
	}
	if c.Colors.FileColor("x.BAT", "", false) != RGB(255, 0, 0) {
		t.Error("filter colour")
	}

	// Column sets (old "Titles" key, right-aligned size).
	if len(c.Columns) != 1 || c.Columns[0].Name != "Sizes" || c.Columns[0].NameWidth != 150 ||
		c.Columns[0].Columns[0].Align != AlignRight || c.Columns[0].Columns[1].Content != "[=tc.writedate]" {
		t.Errorf("columns = %+v", c.Columns)
	}

	// Start menu with submenu and hotkey.
	if len(c.StartMenu.Items) != 2 || c.StartMenu.Items[1].Kind != MenuSubmenu {
		t.Errorf("start menu = %+v", c.StartMenu.Items)
	}
	hk, bad := c.StartMenu.Hotkeys()
	if len(hk) != 1 || len(bad) != 0 || hk[0].Item.Field("cmd") != "notepad.exe" {
		t.Errorf("start hotkeys = %+v %v", hk, bad)
	}

	// Histories.
	if got := c.LeftHistory.Items(); len(got) != 2 || got[0] != `C:\` {
		t.Errorf("left history = %v", got)
	}
	if got := c.Inputs[SectionSearchName].Items(); len(got) != 1 || got[0] != "*.txt" {
		t.Errorf("search history = %v", got)
	}
	if c.CmdHistory.Items()[0] != "dir /s" {
		t.Error("command history")
	}

	// Options with TC key names; Windows paths are not taken on Unix.
	if !c.Settings.Display.ShowHidden || c.Settings.Display.DirBrackets || c.Settings.Misc.WindowTitle != "Total Commander" {
		t.Errorf("options not imported: %+v", c.Settings.Display)
	}
	if runtime.GOOS != "windows" {
		if strings.HasPrefix(c.Settings.IgnoreList.File, "C:") {
			t.Error("Windows path option must not be imported on Unix")
		}
		if !containsSub(rep.Warnings, "Windows drives") {
			t.Errorf("no foreign path warning: %v", rep.Warnings)
		}
	}

	// Button bar copied, sub-bar rewritten to the copy.
	barPath := c.ButtonBarPath()
	if filepath.Base(barPath) != "tc_default.bar" {
		t.Fatalf("bar path = %s", barPath)
	}
	bar, _, err := LoadBarFile(barPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(bar.Buttons) != 3 || !bar.Buttons[1].IsSeparator() || !bar.Buttons[2].IsSubBar() {
		t.Fatalf("bar = %+v", bar.Buttons)
	}
	head, _ := SplitCommand(bar.Buttons[2].Command)
	sub := c.Expand(UnquoteArg(head))
	if _, err := os.Stat(sub); err != nil {
		t.Errorf("sub-bar copy %s: %v", sub, err)
	}
	if v, _ := c.File.Get(SectionButtonbar, "Buttonheight"); v != "32" {
		t.Errorf("Buttonheight = %q", v)
	}

	// usercmd.ini written next to our settings.
	cmds, _, err := c.LoadUserCommands()
	if err != nil || len(cmds) != 1 || cmds[0].Name != "em_backup" || cmds[0].Params != "-a %P %T" {
		t.Errorf("user commands = %+v %v", cmds, err)
	}

	// Our settings file is only written on Save.
	if _, err := os.Stat(c.Loc.Ini); !os.IsNotExist(err) {
		t.Error("ImportTC must not save the settings file")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := Open(c.Loc)
	if err != nil {
		t.Fatal(err)
	}
	if countCommands(c2.Hotlist.Items) != 4 || len(c2.Columns) != 1 || len(c2.Keymap.Bindings()) != 3 {
		t.Errorf("after save: hotlist %d, columns %d, keys %d",
			countCommands(c2.Hotlist.Items), len(c2.Columns), len(c2.Keymap.Bindings()))
	}
}

func TestImportTCSelective(t *testing.T) {
	tcDir := t.TempDir()
	iniPath := filepath.Join(tcDir, "wincmd.ini")
	writeCP1252(t, iniPath, tcWincmd)
	c := openTemp(t, "")
	c.Hotlist.AddFolder("Mine", "/mine", "")
	rep, err := ImportTC(c, iniPath, ImportOptions{Hotlist: true, ReplaceHotlist: true, ButtonBar: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Hotlist.Items) != 4 || c.Hotlist.Items[0].Title != "&Dokumente Äpfel" {
		t.Errorf("replace hotlist: %+v", c.Hotlist.Items)
	}
	if len(c.Keymap.Bindings()) != 0 || len(c.Columns) != 0 || c.Settings.Display.ShowHidden {
		t.Error("unselected parts were imported")
	}
	if !containsSub(rep.Warnings, "button bar") {
		t.Errorf("missing bar must be reported: %v", rep.Warnings)
	}
	if _, err := ImportTC(c, filepath.Join(tcDir, "nope.ini"), AllImport()); err == nil {
		t.Error("missing ini must fail")
	}
}

func TestImportTCDuplicates(t *testing.T) {
	tcDir := t.TempDir()
	iniPath := filepath.Join(tcDir, "wincmd.ini")
	writeCP1252(t, iniPath, tcWincmd)
	c := openTemp(t, "")
	c.Columns = []ColumnSet{{Name: "sizes"}}
	rep, _ := ImportTC(c, iniPath, ImportOptions{Columns: true})
	if len(c.Columns) != 1 || !containsSub(rep.Warnings, "already exists") {
		t.Errorf("duplicate column set: %v %v", c.Columns, rep.Warnings)
	}
}

func TestLooksLikeForeignPath(t *testing.T) {
	cases := map[string]bool{`C:\x`: true, "d:/y": true, "/usr": false, `\\srv\x`: false, "c:": false, "": false}
	for in, want := range cases {
		if runtime.GOOS == "windows" {
			want = false
		}
		if got := looksLikeForeignPath(in); got != want {
			t.Errorf("%q = %v", in, got)
		}
	}
}

func containsSub(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
