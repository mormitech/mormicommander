package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Start menu icons (F-450): an image file is scaled into a 16x16 PNG for the native menu; anything else gives none.
func TestMenuIconFile(t *testing.T) {
	dir := t.TempDir()
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			src.Set(x, y, color.RGBA{200, 10, 10, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "wide.png")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out := menuIconFile(p)
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Fatalf("size %v", b)
	}
	// a 2:1 picture keeps its proportions: the middle is drawn, the top row stays clear
	if _, _, _, a := img.At(8, 8).RGBA(); a == 0 {
		t.Error("middle not drawn")
	}
	if _, _, _, a := img.At(8, 0).RGBA(); a != 0 {
		t.Error("top row should stay clear")
	}
	if again := menuIconFile(p); !bytes.Equal(again, out) {
		t.Error("cached icon differs")
	}
	bad := filepath.Join(dir, "bad.png")
	_ = os.WriteFile(bad, []byte("not an image"), 0o600)
	for _, f := range []string{bad, filepath.Join(dir, "missing.png"), filepath.Join(dir, "x.txt"), dir} {
		if menuIconFile(f) != nil {
			t.Errorf("%s gave an icon", f)
		}
	}
	if menuIcon("") != nil || menuIcon("mc:find") != nil {
		t.Error("empty or built-in icon names give no file icon")
	}
	// a menu item with an icon builds like any other
	m := baseMenu(&App{}, []MenuItem{{Label: "Start", Items: []MenuItem{{Label: "Notes", Cmd: "cm_StartMenu_1", Icon: p}}}})
	if len(menuItems(m)) == 0 {
		t.Error("menu not built")
	}
}

// The Start menu's flat list goes back into a tree with the entries' icons, keys and submenus (F-448 to F-450).
func TestConfigStartTree(t *testing.T) {
	tree, err := configStartTree([]configStartItem{
		{Kind: "item", Title: "&Notes", Cmd: "open", Params: "-a TextEdit %P%N", Key: "Ctrl+Shift+N", Icon: " notes.png "},
		{Kind: "submenu", Title: "Tools"},
		{Kind: "item", Depth: 1, Title: "Reread", Cmd: "cm_RereadSource"},
		{Kind: "sep", Depth: 1},
		{Kind: "item", Depth: 5, Title: "Deep", Cmd: "cm_Edit"},
		{Kind: "item", Title: "Top", Cmd: "cm_List"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 || tree[0].Field("icon") != "notes.png" || tree[0].Field("key") == "" {
		t.Fatalf("tree %+v", tree)
	}
	if kids := tree[1].Children; len(kids) != 3 || kids[2].Title != "Deep" || kids[2].Field("icon") != "" {
		t.Fatalf("submenu %+v", kids)
	}
	if _, err := configStartTree([]configStartItem{{Kind: "submenu", Title: " "}}); err == nil {
		t.Error("a submenu without a name")
	}
	if _, err := configStartTree([]configStartItem{{Kind: "item", Title: "-x"}}); err == nil {
		t.Error("a title starting with -")
	}
	if _, err := configStartTree([]configStartItem{{Kind: "item", Title: "a", Key: "nonsense+"}}); err == nil {
		t.Error("a bad key")
	}
}
