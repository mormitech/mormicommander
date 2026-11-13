package main

import (
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
	var labels []string
	for _, it := range m.Items {
		labels = append(labels, it.Label)
	}
	want := []string{"Files", "Show", "Help"}
	if runtime.GOOS == "darwin" {
		want = []string{"mormiCommander", "", "Files", "Show", "", "Help​"} // Edit and Window are system role menus, without a label of ours
	}
	if len(labels) != len(want) {
		t.Fatalf("menus %q, want %q", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("menus %q, want %q", labels, want)
		}
	}
	var files, show = m.Items[0], m.Items[1]
	if runtime.GOOS == "darwin" {
		files, show = m.Items[2], m.Items[3]
	}
	if n := len(files.SubMenu.Items); n != 3 || !files.SubMenu.Items[2].Disabled {
		t.Errorf("files menu %d items", n)
	}
	if hid := show.SubMenu.Items[0]; !hid.Checked {
		t.Error("check mark lost")
	}
	if sub := show.SubMenu.Items[1]; sub.SubMenu == nil || len(sub.SubMenu.Items) != 1 {
		t.Error("submenu lost")
	}
}
