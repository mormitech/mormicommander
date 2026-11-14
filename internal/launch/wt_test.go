package launch

import "testing"

// TestWindowsTerminalSemicolonInFolder: Windows Terminal splits its own command line at every ";" (even inside a
// quoted argument) into separate subcommands, and the text after it is a command line it runs in a new tab. A folder
// named "x; calc" therefore opened a tab in C:\x and started calc. The semicolon is written as "\;", which wt.exe
// turns back into a plain ";" of the folder name.
func TestWindowsTerminalSemicolonInFolder(t *testing.T) {
	l, _ := fakeLauncher(t, "windows", nil, "wt.exe")
	got, err := l.OpenTerminalCommand(`C:\x; calc`)
	if err != nil {
		t.Fatal(err)
	}
	sameCmd(t, got, Command{Path: "wt.exe", Params: `-d "C:\x\; calc"`, Dir: `C:\x; calc`})
	got, _ = l.OpenTerminalCommand(`C:\a;b`)
	sameCmd(t, got, Command{Path: "wt.exe", Params: `-d C:\a\;b`, Dir: `C:\a;b`})
}
