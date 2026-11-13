// This file computes the panel status line, TC's "12 k / 3,456 k in 2 / 40 file(s), 0 / 5 dir(s)". It counts
// selected and total files, folders and bytes; folder bytes are counted only once their size has been calculated
// (Space on a folder in TC), because before that the size is unknown. The ".." entry never counts.

package listing

import "strconv"

// Totals are the counts behind the status line.
type Totals struct {
	Files, Dirs       int
	Bytes             int64
	SelFiles, SelDirs int
	SelBytes          int64
	UnsizedDirs       int // folders whose size is unknown, so Bytes may be too low
	SelUnsizedDirs    int
}

// Count totals items; selected reports whether item i is marked (nil means nothing is).
func Count(items []Item, selected func(i int) bool) Totals {
	var t Totals
	for i := range items {
		it := &items[i]
		if it.IsUp() {
			continue
		}
		sel := selected != nil && selected(i)
		if it.IsDir() {
			t.Dirs++
			if sel {
				t.SelDirs++
			}
			if it.DirSize >= 0 {
				t.Bytes += it.DirSize
				if sel {
					t.SelBytes += it.DirSize
				}
			} else {
				t.UnsizedDirs++
				if sel {
					t.SelUnsizedDirs++
				}
			}
			continue
		}
		t.Files++
		t.Bytes += it.Size
		if sel {
			t.SelFiles++
			t.SelBytes += it.Size
		}
	}
	return t
}

// Add merges another total into t, for branch views and multi-folder selections.
func (t Totals) Add(o Totals) Totals {
	t.Files += o.Files
	t.Dirs += o.Dirs
	t.Bytes += o.Bytes
	t.SelFiles += o.SelFiles
	t.SelDirs += o.SelDirs
	t.SelBytes += o.SelBytes
	t.UnsizedDirs += o.UnsizedDirs
	t.SelUnsizedDirs += o.SelUnsizedDirs
	return t
}

// StatusText renders TC's status line in the given size unit (TC uses kilobytes; SizeDynamic reads better on large
// disks).
func StatusText(t Totals, unit SizeUnit, nf NumberFormat) string {
	if nf == (NumberFormat{}) {
		nf = DefaultNumberFormat
	}
	return FormatSize(t.SelBytes, unit, nf) + " / " + FormatSize(t.Bytes, unit, nf) +
		" in " + strconv.Itoa(t.SelFiles) + " / " + strconv.Itoa(t.Files) + " file(s), " +
		strconv.Itoa(t.SelDirs) + " / " + strconv.Itoa(t.Dirs) + " dir(s)"
}

// ItemText is the status line TC shows for the item under the cursor when nothing is selected: name, size and date.
func ItemText(it *Item, f *Format) string {
	if it.IsUp() {
		return ".."
	}
	return it.Name + "  " + f.SizeText(it) + "  " + FormatDate(it.ModTime, f.Date, f.Location) + "  " + f.AttrText(it)
}
