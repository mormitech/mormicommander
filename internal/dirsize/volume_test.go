package dirsize

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVolumeOf(t *testing.T) {
	dir := t.TempDir()
	v, err := VolumeOf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v.Total == 0 || v.Available > v.Total || v.Free > v.Total || v.Root == "" || v.BlockSize == 0 || v.FileSystem == "" {
		t.Fatalf("odd volume %+v", v)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if !strings.HasPrefix(strings.ToLower(real), strings.ToLower(v.Root)) && v.Root != "/" {
		t.Errorf("root %q does not hold %q", v.Root, real)
	}
	if f := v.UsedFraction(); f < 0 || f > 1 {
		t.Errorf("UsedFraction = %v", f)
	}
	if v.DisplayName() == "" || !strings.HasPrefix(DriveLine(v, UnitKB), "[") {
		t.Errorf("display %q line %q", v.DisplayName(), DriveLine(v, UnitKB))
	}
	t.Logf("%+v  %s", v, DriveLine(v, UnitDynamic))

	if _, err := VolumeOf(filepath.Join(dir, "missing", "x")); err == nil {
		t.Error("VolumeOf on a missing path did not fail")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		n    int64
		u    Unit
		want string
	}{
		{0, UnitBytes, "0"},
		{999, UnitBytes, "999"},
		{1234567, UnitBytes, "1,234,567"},
		{1, UnitKB, "1 k"},
		{0, UnitKB, "0 k"},
		{1024, UnitKB, "1 k"},
		{1025, UnitKB, "2 k"},
		{1234567, UnitKB, "1,206 k"},
		{1234567, UnitMB, "1.18 M"},
		{3 << 29, UnitGB, "1.5 G"},
		{512, UnitDynamic, "512 b"},
		{1536, UnitDynamic, "1.5 k"},
		{1234567, UnitDynamic, "1.18 M"},
		{50 << 20, UnitDynamic, "50 M"},
		{123 << 30, UnitDynamic, "123 G"},
		{2 << 40, UnitDynamic, "2 T"},
		{1500 << 40, UnitDynamic, "1,500 T"},
		{-2048, UnitDynamic, "-2 k"},
	}
	for _, tc := range tests {
		if got := FormatSize(tc.n, tc.u); got != tc.want {
			t.Errorf("FormatSize(%d, %d) = %q, want %q", tc.n, tc.u, got, tc.want)
		}
	}
}

func TestGroup(t *testing.T) {
	for n, want := range map[uint64]string{0: "0", 12: "12", 123: "123", 1234: "1,234", 123456: "123,456", 1234567890: "1,234,567,890"} {
		if got := Group(n); got != want {
			t.Errorf("Group(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestDriveLineAndNames(t *testing.T) {
	tests := []struct {
		v    VolumeInfo
		u    Unit
		name string
		line string
	}{
		{VolumeInfo{Root: `C:\`, Label: "System", Total: 100 << 30, Available: 25 << 30}, UnitKB, "System", "[System]  26,214,400 k of 104,857,600 k free"},
		{VolumeInfo{Root: `D:\`, Total: 1 << 30, Available: 1 << 29}, UnitDynamic, `D:\`, `[D:\]  512 M of 1 G free`},
		{VolumeInfo{Root: "/media/usb", Total: 2048, Available: 1024}, UnitBytes, "usb", "[usb]  1 k of 2 k free"},
		{VolumeInfo{Root: "/", Total: 0}, UnitKB, "/", "[/]  0 k of 0 k free"},
	}
	for _, tc := range tests {
		if got := tc.v.DisplayName(); got != tc.name {
			t.Errorf("DisplayName(%+v) = %q, want %q", tc.v, got, tc.name)
		}
		if got := DriveLine(tc.v, tc.u); got != tc.line {
			t.Errorf("DriveLine = %q, want %q", got, tc.line)
		}
	}
	v := VolumeInfo{Total: 100, Free: 25}
	if v.Used() != 75 || v.UsedFraction() != 0.75 {
		t.Errorf("Used %d fraction %v", v.Used(), v.UsedFraction())
	}
}

func TestSpaceLine(t *testing.T) {
	r := Result{Bytes: 1234567, Allocated: 1234567, Files: 1200, Dirs: 3}
	if got, want := SpaceLine(r), "1,234,567 bytes (1.18 M) in 1,200 files, 3 folders"; got != want {
		t.Errorf("SpaceLine = %q, want %q", got, want)
	}
	r.Allocated = 2 << 20
	if got := SpaceLine(r); !strings.HasSuffix(got, ", 2 M on disk") {
		t.Errorf("SpaceLine = %q", got)
	}
}
