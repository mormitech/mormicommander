// Tests for PrepareButton (buttons and start-menu entries), the iconic
// window-state mapping and the start-menu tree conversion.
package params

import (
	"os"
	"reflect"
	"testing"
)

func TestPrepareButton(t *testing.T) {
	ctx := winCtx(t)
	l, err := PrepareButton(Button{Command: `*"%EDITOR%"`, Params: "?-n %P%N", StartPath: "%T", Iconic: -1}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if l.Program != `C:\vim\vim.exe` || !l.Elevate || l.Internal || !l.Prompt || l.Show != ShowMaximized {
		t.Errorf("launch = %+v", l)
	}
	if l.Dir != `D:\tgt\` {
		t.Errorf("dir = %q", l.Dir)
	}
	if !reflect.DeepEqual(l.Args, []string{"-n", `C:\Src Dir\my file.txt`}) {
		t.Errorf("args = %q (line %s)", l.Args, l.Line)
	}
	if err := l.SetLine(`-x "a b"`); err != nil || !reflect.DeepEqual(l.Args, []string{"-x", "a b"}) {
		t.Errorf("SetLine: %q %v", l.Args, err)
	}

	l, err = PrepareButton(Button{Command: "cm_SrcThumbs"}, ctx)
	if err != nil || !l.Internal || l.Dir != ctx.Source.Path || l.Show != ShowNormal || len(l.Args) != 0 {
		t.Errorf("internal: %+v %v", l, err)
	}

	l, err = PrepareButton(Button{Command: "7z", Params: "a %L", Iconic: 1}, ctx)
	if err != nil || len(l.ListFiles) != 1 || l.Show != ShowMinimized {
		t.Fatalf("list: %+v %v", l, err)
	}
	f := l.ListFiles[0]
	if l.Args[1] != f {
		t.Errorf("list arg %q", l.Args)
	}
	if err := l.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("list file kept")
	}

	ctx.Ask = func(string, string) (string, bool) { return "", false }
	l, err = PrepareButton(Button{Command: "x", Params: "%L %[go?]"}, ctx)
	if err != nil || !l.Canceled || len(l.ListFiles) != 0 {
		t.Errorf("cancel: %+v %v", l, err)
	}

	pctx := posixCtx(t)
	if _, err := PrepareButton(Button{Command: "x", Params: "%Q%N"}, pctx); err == nil {
		t.Error("unterminated quote after %Q should fail to split")
	}
}

func TestIconic(t *testing.T) {
	for n, m := range map[int]ShowMode{0: ShowNormal, 1: ShowMinimized, 2: ShowMinimized, -1: ShowMaximized} {
		if ShowModeFromIconic(n) != m {
			t.Errorf("iconic %d", n)
		}
	}
	for _, m := range []ShowMode{ShowNormal, ShowMinimized, ShowMaximized} {
		if ShowModeFromIconic(m.Iconic()) != m {
			t.Errorf("round trip %d", m)
		}
	}
	if !IsInternalCommand("CM_Copy") || !IsInternalCommand("em_x") || IsInternalCommand("cm_") || IsInternalCommand("cmd") {
		t.Error("IsInternalCommand")
	}
}

func TestMenuTree(t *testing.T) {
	flat := []MenuEntry{
		{Title: "Notepad", Command: "notepad.exe", Params: "%N", Key: "C+A+N"},
		{Title: "-"},
		{Title: "-Tools"},
		{Title: "Pack", Command: "7z", Params: "a %L", Iconic: 1},
		{Title: "-Deeper"},
		{Title: "Deep", Command: "cm_About"},
		{Title: "--"},
		{Title: "--"},
		{Title: "-Empty"},
		{Title: "--"},
		{Title: "-Open"},
		{Title: "Last", Command: "x"},
	}
	tree, err := BuildMenu(flat)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 5 || !tree[1].Separator || !tree[2].IsSubmenu() || tree[2].Title != "Tools" ||
		len(tree[2].Children) != 2 || tree[2].Children[1].Children[0].Button.Command != "cm_About" ||
		!tree[3].IsSubmenu() || len(tree[3].Children) != 0 || tree[4].Children[0].Title != "Last" ||
		tree[0].Key != "C+A+N" || tree[2].Children[0].Button.Iconic != 1 {
		t.Fatalf("tree = %+v", tree)
	}
	back := FlattenMenu(tree)
	want := append(append([]MenuEntry{}, flat...), MenuEntry{Title: "--"})
	if !reflect.DeepEqual(back, want) {
		t.Errorf("flatten =\n%+v\nwant\n%+v", back, want)
	}
	if _, err := BuildMenu([]MenuEntry{{Title: "--"}}); err == nil {
		t.Error("stray -- should fail")
	}
}
