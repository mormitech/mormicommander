package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/disk"
	"mormicommander/internal/vfs"
)

// F-716: drives:// lists the drive list as folders and opens a volume's real top; the real disks are never asked.
func TestDrivesFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := drivesVolumes
	defer func() { drivesVolumes = old }()
	drivesVolumes = func() []disk.Volume {
		return []disk.Volume{
			{Name: "Disk", Path: dir, Kind: "fixed", Free: 42},
			{Name: "Disk", Path: "/elsewhere", Kind: "removable"},
			{Name: "a/b", Path: "/x", Kind: "network"},
		}
	}
	fsys := vfs.For("drives://", nil)
	if fsys == nil {
		t.Fatal("drives:// is not registered")
	}
	l, err := fsys.List(context.Background(), "drives://")
	if err != nil {
		t.Fatal(err)
	}
	if l.Path != drivesRoot || len(l.Entries) != 3 {
		t.Fatalf("listing = %+v", l)
	}
	want := []string{"Disk", "Disk (2)", "a∕b"}
	for i, e := range l.Entries {
		if e.Name != want[i] || e.Flags&vfs.FlagDir == 0 {
			t.Fatalf("entry %d = %+v, want folder %q", i, e, want[i])
		}
	}
	if l.Entries[0].Size != 42 {
		t.Fatalf("free space not in the size column: %+v", l.Entries[0])
	}
	in, err := fsys.List(context.Background(), fsys.Join("drives://", "disk"))
	if err != nil {
		t.Fatal(err)
	}
	if in.Path != dir || len(in.Entries) != 1 || in.Entries[0].Name != "a.txt" {
		t.Fatalf("entering a drive gave %+v", in)
	}
	if _, err := fsys.List(context.Background(), "drives://nope"); err == nil {
		t.Fatal("an unknown drive listed")
	}
	if fsys.Parent("drives://Disk") != drivesRoot || fsys.Parent("drives://") != "" || fsys.Clean("drives:") != drivesRoot {
		t.Fatal("parent/clean wrong")
	}
	f := &FS{app: &App{}, router: vfs.NewRouter()}
	if info := f.PathInfo("drives://"); info.Kind != "drives" || !info.ReadOnly || info.ReadOnlyWhy == "" {
		t.Fatalf("PathInfo = %+v", info)
	}
}
