// This file describes the volume a folder lives on, for the drive bar buttons, the drive popup (Alt+F1 / Alt+F2) and
// the drive info line above each panel, which in Total Commander reads "[label]  12,345,678 k of 99,999,996 k free".
// VolumeOf answers for any path, not only for a drive root, because the panel asks "where am I?" after every change
// of folder; the platform files find the mount point, label and file system type. The formatting helpers live here so
// that every platform shows the same wording, and so the size column, the Ctrl+L dialog and the status line can share
// one way of turning byte counts into text.
package dirsize

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// VolumeInfo describes the volume that holds a path.
type VolumeInfo struct {
	Path       string `json:"path"`       // the path that was asked about
	Root       string `json:"root"`       // its mount point or drive root: "/", "/Volumes/USB", `C:\`
	Label      string `json:"label"`      // the volume name: "Macintosh HD", "DATA"; "" when it has none
	FileSystem string `json:"fileSystem"` // "apfs", "ext4", "NTFS", "exfat", "smbfs" …
	Device     string `json:"device"`     // "/dev/disk3s1", "/dev/sda2", `\\?\Volume{…}\`, "//server/share"
	Serial     uint32 `json:"serial"`     // Windows volume serial number; 0 elsewhere
	Total      uint64 `json:"total"`      // size in bytes
	Free       uint64 `json:"free"`       // free bytes, including space reserved for the system
	Available  uint64 `json:"available"`  // free bytes this user can write (what the drive line shows)
	BlockSize  uint64 `json:"blockSize"`  // allocation unit (cluster size), for "size on disk"
	ReadOnly   bool   `json:"readOnly"`
	Remote     bool   `json:"remote"`    // network share
	Removable  bool   `json:"removable"` // USB stick, card, optical disc (best effort on macOS)
}

// Used is the number of bytes in use.
func (v VolumeInfo) Used() uint64 {
	if v.Free > v.Total {
		return 0
	}
	return v.Total - v.Free
}

// UsedFraction is Used/Total in 0..1, for the free space bar on the drive buttons; 0 for an empty drive slot.
func (v VolumeInfo) UsedFraction() float64 {
	if v.Total == 0 {
		return 0
	}
	return float64(v.Used()) / float64(v.Total)
}

// DisplayName is what the drive popup shows: the label, else the last element of the root, else the root itself.
func (v VolumeInfo) DisplayName() string {
	if v.Label != "" {
		return v.Label
	}
	base := filepath.Base(v.Root)
	if base == "" || base == "." || base == string(filepath.Separator) || strings.HasSuffix(v.Root, `:\`) {
		return v.Root
	}
	return base
}

// Unit selects how byte counts are written, matching Total Commander's "file size format" option (F-093).
type Unit int

const (
	UnitBytes   Unit = iota // 1,234,567
	UnitKB                  // 1,206 k
	UnitMB                  // 1.18 M
	UnitGB                  // 0.00 G
	UnitDynamic             // the largest unit that keeps the number at or above 1: 1.18 M
)

// FormatSize writes n in the given unit with thousands separators: "1,234,567", "1,206 k", "1.18 M", "3.5 G".
// Sizes are binary (1 k = 1024 bytes), as Total Commander uses. KB values are rounded up, like TC, so a 1-byte file
// shows "1 k" and never "0 k".
func FormatSize(n int64, u Unit) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var s string
	switch u {
	case UnitBytes:
		s = Group(uint64(n))
	case UnitKB:
		s = Group(uint64((n+1023)/1024)) + " k"
	case UnitMB:
		s = decimal(float64(n)/(1<<20)) + " M"
	case UnitGB:
		s = decimal(float64(n)/(1<<30)) + " G"
	default:
		switch {
		case n < 1024:
			s = strconv.FormatInt(n, 10) + " b"
		case n < 1<<20:
			s = decimal(float64(n)/(1<<10)) + " k"
		case n < 1<<30:
			s = decimal(float64(n)/(1<<20)) + " M"
		case n < 1<<40:
			s = decimal(float64(n)/(1<<30)) + " G"
		default:
			s = decimal(float64(n)/(1<<40)) + " T"
		}
	}
	if neg {
		return "-" + s
	}
	return s
}

// decimal writes up to three significant digits: 1.18, 11.8, 118, 1,180.
func decimal(f float64) string {
	switch {
	case f < 10:
		return strings.TrimSuffix(strings.TrimRight(strconv.FormatFloat(f, 'f', 2, 64), "0"), ".")
	case f < 100:
		return strings.TrimSuffix(strings.TrimRight(strconv.FormatFloat(f, 'f', 1, 64), "0"), ".")
	default:
		return Group(uint64(f + 0.5))
	}
}

// Group writes n with commas between groups of three digits: 1234567 → "1,234,567". The UI may replace the commas
// with the locale's separator.
func Group(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// DriveLine is the drive info line above a panel, in Total Commander's wording: "[DATA]  1,234 k of 5,678 k free".
// With UnitDynamic it reads "[DATA]  1.2 G of 5.5 G free". The brackets hold the label (or the display name when the
// volume has no label).
func DriveLine(v VolumeInfo, u Unit) string {
	if u == UnitBytes {
		u = UnitKB
	}
	return fmt.Sprintf("[%s]  %s of %s free", v.DisplayName(), FormatSize(int64(v.Available), u), FormatSize(int64(v.Total), u))
}

// SpaceLine is the Ctrl+L / Space status text for a result: "1,234,567 bytes (1.18 M) in 12 files, 3 folders",
// with ", 1.2 M on disk" appended when the size on disk differs.
func SpaceLine(r Result) string {
	s := fmt.Sprintf("%s bytes (%s) in %s files, %s folders", Group(uint64(max(r.Bytes, 0))), FormatSize(r.Bytes, UnitDynamic),
		Group(uint64(r.Files)), Group(uint64(r.Dirs)))
	if r.Allocated != r.Bytes {
		s += ", " + FormatSize(r.Allocated, UnitDynamic) + " on disk"
	}
	return s
}
