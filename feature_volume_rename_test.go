package main

import (
	"runtime"
	"strings"
	"testing"

	"mormicommander/internal/disk"
)

// F-713: the label is checked, only listed disks are renamed, and the mount point goes as an argument of its own.
// No disk is ever renamed here: the volume list and the program runner are fakes.
func TestVolumeRenameRules(t *testing.T) {
	for name, ok := range map[string]bool{"Photos": true, "  ": false, "a/b": false, "a:b": false, strings.Repeat("x", 33): false, "tab\there": false} {
		if (checkVolumeLabel(name) == "") != ok {
			t.Errorf("checkVolumeLabel(%q) = %q", name, checkVolumeLabel(name))
		}
	}
	vols := []disk.Volume{
		{Name: "Macintosh HD", Path: "/", Kind: "fixed"},
		{Name: "USB", Path: "/Volumes/USB", Kind: "removable"},
		{Name: "share", Path: "/Volumes/share", Kind: "network"},
		{Name: "Home", Path: "/Users/me", Kind: "home"},
		{Name: "Dead", Path: "/Volumes/Dead", Kind: "removable", Stale: true},
	}
	if _, err := renamableVolume("darwin", "/Volumes/USB/", vols); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/", "/Volumes/share", "/Users/me", "/Volumes/Dead", "/Volumes/Other"} {
		if _, err := renamableVolume("darwin", p, vols); err == nil {
			t.Errorf("%s would be renamed", p)
		}
	}
	if _, err := renamableVolume("linux", "/Volumes/USB", vols); err == nil {
		t.Error("Linux renames")
	}
	win := []disk.Volume{{Name: "C:", Path: `C:\`, Kind: "fixed"}}
	if _, err := renamableVolume("windows", `c:\`, win); err != nil {
		t.Error(err)
	}
	prog, args := renameVolumeCommand("darwin", "/Volumes/USB", "My stick; rm -rf")
	if prog != "diskutil" || len(args) != 3 || args[0] != "rename" || args[1] != "/Volumes/USB" || args[2] != "My stick; rm -rf" {
		t.Fatalf("command = %s %q", prog, args)
	}
	if p, _ := renameVolumeCommand("windows", `C:\`, "x"); p != "" {
		t.Fatal("Windows renames through a program")
	}
}

func TestVolumeRenameRunsOnlyTheFake(t *testing.T) {
	oldV, oldR := renameVolumeVolumes, renameVolumeRun
	defer func() { renameVolumeVolumes, renameVolumeRun = oldV, oldR }()
	renameVolumeVolumes = func() []disk.Volume {
		return []disk.Volume{{Name: "USB", Path: "/Volumes/USB", Kind: "removable"}, {Name: "C:", Path: `C:\`, Kind: "removable"}}
	}
	var ran []string
	renameVolumeRun = func(name string, args []string) (string, error) {
		ran = append(ran, name+" "+strings.Join(args, "|"))
		return "", nil
	}
	f := &SystemFiles{app: &App{}}
	if err := f.RenameVolume("/Volumes/USB", "  "); err == nil {
		t.Fatal("empty name accepted")
	}
	if len(ran) != 0 {
		t.Fatal("ran a program for a bad name")
	}
	err := f.RenameVolume("/Volumes/USB", " Stick ")
	switch {
	case runtime.GOOS == "darwin":
		if err != nil || len(ran) != 1 || ran[0] != "diskutil rename|/Volumes/USB|Stick" {
			t.Fatalf("err %v, ran %q", err, ran)
		}
	case runtime.GOOS == "linux":
		if err == nil || len(ran) != 0 {
			t.Fatalf("Linux: err %v, ran %q", err, ran)
		}
	}
}
