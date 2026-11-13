// These tests check the multi-volume name helpers against every naming scheme
// (RAR new and old style, 7z/generic splits, spanned zip), including letter
// case, zero padding, the r99 -> s00 rollover and files that are not volumes.

package sevenrar

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseVolumeName(t *testing.T) {
	cases := []struct {
		in    string
		kind  volKind
		stem  string
		index int
	}{
		{"a.part1.rar", volRarNew, "a", 0},
		{"a.part01.rar", volRarNew, "a", 0},
		{"a.Part003.RAR", volRarNew, "a", 2},
		{"my.photos.part12.rar", volRarNew, "my.photos", 11},
		{"setup.part1.exe", volRarNew, "setup", 0},
		{"a.rar", volRarOld, "a", 0},
		{"a.r00", volRarOld, "a", 1},
		{"a.R05", volRarOld, "a", 6},
		{"a.r99", volRarOld, "a", 100},
		{"a.s00", volRarOld, "a", 101},
		{"a.7z.001", volSplit, "a.7z", 0},
		{"a.7z.010", volSplit, "a.7z", 9},
		{"movie.mkv.002", volSplit, "movie.mkv", 1},
		{"a.z01", volZipSplit, "a", 0},
		{"a.Z12", volZipSplit, "a", 11},
		{"a.7z", volNone, "", 0},
		{"a.zip", volNone, "", 0},
		{"report.2024", volNone, "", 0},
		{"a.part0.rar", volRarOld, "a.part0", 0},
		{"a.000", volNone, "", 0},
		{"readme", volNone, "", 0},
	}
	for _, c := range cases {
		v := parseVolumeName(c.in)
		if v.kind != c.kind || v.kind != volNone && (v.stem != c.stem || v.index != c.index) {
			t.Errorf("parseVolumeName(%q) = %+v, want kind %d stem %q index %d", c.in, v, c.kind, c.stem, c.index)
		}
	}
}

func TestNextVolumeName(t *testing.T) {
	cases := []struct {
		in   string
		idx  int
		want string
	}{
		{"a.part1.rar", 1, "a.part2.rar"},
		{"a.part01.rar", 9, "a.part10.rar"},
		{"a.Part001.RAR", 0, "a.Part001.RAR"},
		{"a.rar", 1, "a.r00"},
		{"a.RAR", 2, "a.R01"},
		{"a.r05", 0, "a.rar"},
		{"a.rar", 101, "a.s00"},
		{"a.7z.003", 0, "a.7z.001"},
		{"a.7z.001", 11, "a.7z.012"},
		{"a.z01", 1, "a.z02"},
	}
	for _, c := range cases {
		vn := parseVolumeName(c.in)
		if got := filepath.Base(nextVolumeName(filepath.Join("d", c.in), vn, c.idx)); got != c.want {
			t.Errorf("nextVolumeName(%s, %d) = %s, want %s", c.in, c.idx, got, c.want)
		}
	}
}

func TestFirstAndFindVolumes(t *testing.T) {
	dir := t.TempDir()
	touch := func(names ...string) {
		for _, n := range names {
			writeFile(t, filepath.Join(dir, n), []byte("x"))
		}
	}
	touch("n.part1.rar", "n.part2.rar", "n.part3.rar")
	touch("o.rar", "o.r00", "o.r01")
	touch("U.RAR", "U.R00")
	touch("s.7z.001", "s.7z.002", "s.7z.004") // .003 missing: set ends at .002
	touch("z.z01", "z.z02", "z.zip")
	touch("plain.7z")
	touch("gap.part2.rar")

	j := func(names ...string) []string {
		out := make([]string, len(names))
		for i, n := range names {
			out[i] = filepath.Join(dir, n)
		}
		return out
	}
	cases := []struct {
		in    string
		first string
		vols  []string
	}{
		{"n.part2.rar", "n.part1.rar", j("n.part1.rar", "n.part2.rar", "n.part3.rar")},
		{"n.part1.rar", "n.part1.rar", j("n.part1.rar", "n.part2.rar", "n.part3.rar")},
		{"o.r01", "o.rar", j("o.rar", "o.r00", "o.r01")},
		{"o.rar", "o.rar", j("o.rar", "o.r00", "o.r01")},
		{"U.R00", "U.RAR", j("U.RAR", "U.R00")},
		{"s.7z.002", "s.7z.001", j("s.7z.001", "s.7z.002")},
		{"z.z02", "z.zip", j("z.z01", "z.z02", "z.zip")},
		{"plain.7z", "plain.7z", j("plain.7z")},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.in)
		if got := FirstVolume(p); got != filepath.Join(dir, c.first) {
			t.Errorf("FirstVolume(%s) = %s, want %s", c.in, got, c.first)
		}
		got, err := FindVolumes(p)
		if err != nil || !reflect.DeepEqual(got, c.vols) {
			t.Errorf("FindVolumes(%s) = %v, %v; want %v", c.in, got, err, c.vols)
		}
	}
	_, err := FindVolumes(filepath.Join(dir, "gap.part2.rar"))
	var mv *MissingVolumeError
	if !errors.As(err, &mv) || filepath.Base(mv.Path) != "gap.part1.rar" {
		t.Errorf("gap: err = %v", err)
	}
	if _, err := FindVolumes(filepath.Join(dir, "nothing.7z")); err == nil {
		t.Error("missing single file: no error")
	}
}

func TestArchiveStem(t *testing.T) {
	cases := map[string]string{
		"photos.part01.rar": "photos",
		"photos.rar":        "photos",
		"photos.r03":        "photos",
		"backup.7z.001":     "backup",
		"backup.7z":         "backup",
		"src.tar.gz":        "src",
		"src.TGZ":           "src",
		"data.z01":          "data",
		"movie.mkv.001":     "movie.mkv",
		"noext":             "noext",
		".7z":               ".7z",
		"my.archive.zip":    "my.archive",
		"setup.exe":         "setup",
	}
	for in, want := range cases {
		if got := ArchiveStem(filepath.Join("some", "dir", in)); got != want {
			t.Errorf("ArchiveStem(%s) = %s, want %s", in, got, want)
		}
	}
}
