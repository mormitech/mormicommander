// The Split file dialog offers a drop-down of part sizes named after the
// media they fit on, as Total Commander does: floppy disks, CDs, DVDs and so
// on, plus "Automatic" (fill whatever space the target has) and any custom
// size the user types, such as "100M" or "1.5 GB". This file holds those
// presets with their exact byte counts (the usable capacity of the medium,
// not the marketing number) and the parser for typed sizes.

package fileutil

import (
	"fmt"
	"strconv"
	"strings"
)

// SizeAuto as SplitOptions.PartSize means "as big as the free space on the
// target allows", re-checked before every part, which is TC's Automatic
// setting for writing straight to removable disks.
const SizeAuto int64 = -1

// Exact byte counts of the presets.
const (
	Size360K  int64 = 362496      // 5.25" DD floppy, usable space
	Size720K  int64 = 730112      // 3.5" DD floppy
	Size1_2M  int64 = 1213952     // 5.25" HD floppy
	Size1_44M int64 = 1457664     // 3.5" HD floppy, TC's 1.44 MB value
	SizeCD650 int64 = 681984000   // 74 min CD-R: 333,000 sectors of 2048 bytes
	SizeCD700 int64 = 737280000   // 80 min CD-R: 360,000 sectors
	SizeDVD   int64 = 4700372992  // single layer DVD±R: 2,295,104 sectors
	SizeDVDDL int64 = 8547991552  // dual layer DVD+R: 4,173,824 sectors
	SizeBD25  int64 = 25025314816 // single layer BD-R
	SizeFAT32 int64 = 4294967295  // largest file a FAT32 drive can hold
)

// SizePreset is one entry of the part size drop-down.
type SizePreset struct {
	Name string
	Size int64
}

// SizePresets lists the drop-down entries in display order.
var SizePresets = []SizePreset{
	{"Automatic", SizeAuto},
	{"1.44 MB", Size1_44M},
	{"1.2 MB", Size1_2M},
	{"720 KB", Size720K},
	{"360 KB", Size360K},
	{"650 MB CD", SizeCD650},
	{"700 MB CD", SizeCD700},
	{"4.7 GB DVD", SizeDVD},
	{"8.5 GB DVD DL", SizeDVDDL},
	{"25 GB BD", SizeBD25},
	{"4 GB FAT32", SizeFAT32},
}

// ParseSize parses a part size as typed in the dialog. It accepts a preset
// name ("1.44 MB", "700 MB CD", "Automatic", "auto"), a plain byte count, or
// a number with a unit: B, K/KB/KiB, M/MB/MiB, G/GB/GiB, T/TB/TiB. Units are
// binary (1 KB = 1024 bytes), matching the way TC and the file panels show
// sizes. Spaces, case and a decimal comma are ignored, so "1,5 gb" works.
func ParseSize(s string) (int64, error) {
	key := strings.ToLower(strings.Join(strings.Fields(s), ""))
	if key == "" {
		return 0, fmt.Errorf("no size given")
	}
	if key == "auto" || key == "automatic" {
		return SizeAuto, nil
	}
	for _, p := range SizePresets {
		if key == strings.ToLower(strings.Join(strings.Fields(p.Name), "")) {
			return p.Size, nil
		}
	}
	key = strings.ReplaceAll(key, ",", ".")
	i := 0
	for i < len(key) && (key[i] >= '0' && key[i] <= '9' || key[i] == '.') {
		i++
	}
	num, unit := key[:i], key[i:]
	if num == "" {
		return 0, fmt.Errorf("%q is not a size", s)
	}
	mult := map[string]float64{
		"": 1, "b": 1, "byte": 1, "bytes": 1,
		"k": 1 << 10, "kb": 1 << 10, "kib": 1 << 10,
		"m": 1 << 20, "mb": 1 << 20, "mib": 1 << 20,
		"g": 1 << 30, "gb": 1 << 30, "gib": 1 << 30,
		"t": 1 << 40, "tb": 1 << 40, "tib": 1 << 40,
	}[unit]
	if mult == 0 {
		return 0, fmt.Errorf("%q has an unknown unit %q", s, unit)
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size", s)
	}
	n := int64(f * mult)
	if n <= 0 {
		return 0, fmt.Errorf("the part size must be larger than zero")
	}
	return n, nil
}
