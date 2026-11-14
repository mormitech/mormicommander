package main

import (
	"reflect"
	"runtime"
	"testing"
)

func TestBaseMenu(t *testing.T) {
	on := true
	spec := []MenuItem{
		{Label: "Files", Items: []MenuItem{{Label: "Copy…", Cmd: "cm_Copy"}, {Sep: true}, {Label: "Quit", Cmd: "cm_Exit", Disabled: true}}},
		{Label: "Show", Items: []MenuItem{{Label: "Hidden files", Cmd: "cm_SwitchHidSys", Checked: &on}, {Label: "Sort", Items: []MenuItem{{Label: "By name", Cmd: "cm_SrcByName"}}}}},
		{Label: "Help", Items: []MenuItem{{Label: "About mormiCommander", Cmd: "cm_About"}}},
	}
	m := baseMenu(&App{}, spec)
	items := menuItems(m)
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label())
	}
	want := []string{"Files", "Show", "Help"}
	if runtime.GOOS == "darwin" {
		want = []string{"mormiCommander", "Edit", "Files", "Show", "Window", "Help​"} // Edit and Window are the system's role menus
	}
	if len(labels) != len(want) {
		t.Fatalf("menus %q, want %q", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("menus %q, want %q", labels, want)
		}
	}
	files, show := items[0], items[1]
	if runtime.GOOS == "darwin" {
		files, show = items[2], items[3]
	}
	if fi := menuItems(files.GetSubmenu()); len(fi) != 3 || fi[2].Enabled() || !fi[1].IsSeparator() {
		t.Errorf("files menu %d items", len(fi))
	}
	si := menuItems(show.GetSubmenu())
	if !si[0].IsCheckbox() || !si[0].Checked() {
		t.Error("check mark lost")
	}
	if sub := si[1].GetSubmenu(); sub == nil || len(menuItems(sub)) != 1 {
		t.Error("submenu lost")
	}
	if tree := nativeMenuTree(m); len(tree) != len(want) {
		t.Errorf("native menu tree has %d menus", len(tree))
	}
}

func TestServices(t *testing.T) {
	a := NewApp(startOpts{})
	seen := map[string]bool{}
	svcs := services(a)
	for _, s := range svcs {
		if s.Instance() == nil {
			t.Fatal("a service without its object")
		}
		// Every bound object must be a pointer with a unique type name, since the page calls it as main.<Type>.
		rt := reflect.TypeOf(s.Instance())
		if rt.Kind() != reflect.Pointer {
			t.Fatalf("bound object %v is not a pointer", rt)
		}
		name := rt.Elem().Name()
		if seen[name] {
			t.Fatalf("two bound objects named %s", name)
		}
		seen[name] = true
	}
	if svcs[0].Instance() != any(a) {
		t.Fatal("the App is not the first bound object")
	}
	for _, want := range []string{"App", "FS", "Settings", "Lister", "SystemClip"} {
		if !seen[want] {
			t.Errorf("%s is not bound", want)
		}
	}
}
