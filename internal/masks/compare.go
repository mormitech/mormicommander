// This file implements "Compare directories" (Shift+F2, cm_CompareDirs) and its variant with subfolders
// (cm_CompareDirsWithSubdirs). Like Total Commander, it pairs the entries of the two panels by name and marks, in
// both panels at once, every file that is missing on the other side or newer than its partner, so a following F5
// copies exactly what is needed to bring the other side up to date. Dates within a small tolerance count as equal
// (two seconds by default, because FAT and many network shares store times with two-second precision), and an
// optional rule treats an exact one-hour difference as equal too, which hides daylight-saving and FAT/NTFS time-zone
// artefacts. Files with the same date but a different size cannot be ordered by age, so both are marked. With a
// content comparer, files of equal size are also compared byte by byte: equal content counts as the same file
// whatever the dates say, and different content with equal dates marks both sides. In the subfolder variant, folders
// missing on the other side are marked and folders present on both sides are marked when anything inside them
// differs, recursively; CompareTrees does that walk on any io/fs.FS, so the same code serves local disks, archives
// and remote file systems.

package masks

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"path"
	"time"
)

// CompareStatus says how an entry relates to its partner on the other side.
type CompareStatus uint8

const (
	StatusSame           CompareStatus = iota // equal, or not compared (excluded, or a folder without Subdirs)
	StatusOnlyHere                            // no entry of that name and type on the other side
	StatusNewer                               // newer than the partner
	StatusOlder                               // older than the partner
	StatusSizeDiffers                         // same date, different size
	StatusContentDiffers                      // same date and size, different content
	StatusDirDiffers                          // a folder on both sides whose contents differ (Subdirs only)
)

// Marked says whether the status gets a selection mark: everything except Same and Older.
func (c CompareStatus) Marked() bool { return c != StatusSame && c != StatusOlder }

// String gives a short English word for logs and tests.
func (c CompareStatus) String() string {
	return [...]string{"same", "only-here", "newer", "older", "size-differs", "content-differs", "dir-differs"}[c]
}

// DefaultTimeTolerance is the date difference that still counts as "same date".
const DefaultTimeTolerance = 2 * time.Second

// CompareOptions mirror the options of TC's compare dialog.
type CompareOptions struct {
	Case            CaseRule
	Subdirs         bool          // also compare folders (cm_CompareDirsWithSubdirs)
	IgnoreDate      bool          // mark only missing files and files that differ in size or content
	Tolerance       time.Duration // dates closer than this are equal; use DefaultTimeTolerance for TC behaviour
	IgnoreHourShift bool          // a difference of exactly one hour (within Tolerance) also counts as equal
	// Content, when set, compares two files of equal size and says whether their contents are equal. An error counts
	// as "different" and is reported in CompareResult.Err.
	Content func(left, right Entry) (bool, error)
	// Filter, when set, limits the compare to entries it accepts; folders it excludes are not descended into.
	Filter *List
}

// DefaultCompareOptions returns TC's defaults: platform case rule, two-second tolerance, no subfolders.
func DefaultCompareOptions() CompareOptions { return CompareOptions{Tolerance: DefaultTimeTolerance} }

// CompareResult holds one status per entry of each side, in the order the entries were given.
type CompareResult struct {
	Left, Right []CompareStatus
	Err         error // the first content-compare or read error, if any
}

// LeftMarks returns the marks for the left panel, ready for Selection.SetMarks.
func (r CompareResult) LeftMarks() []bool { return marks(r.Left) }

// RightMarks returns the marks for the right panel.
func (r CompareResult) RightMarks() []bool { return marks(r.Right) }

// marks turns statuses into selection marks.
func marks(st []CompareStatus) []bool {
	out := make([]bool, len(st))
	for i, s := range st {
		out[i] = s.Marked()
	}
	return out
}

// Differs says whether any entry on either side is anything but Same.
func (r CompareResult) Differs() bool {
	for _, s := range append(append([]CompareStatus(nil), r.Left...), r.Right...) {
		if s != StatusSame {
			return true
		}
	}
	return false
}

// CompareLists compares two folder listings that are already in memory. Folders on both sides are reported as Same;
// use CompareTrees to look inside them.
func CompareLists(left, right []Entry, opt CompareOptions) CompareResult {
	return compareLists(left, right, opt, nil)
}

// compareLists does the pairing; dirDiffers, when set, is asked about folders present on both sides.
func compareLists(left, right []Entry, opt CompareOptions, dirDiffers func(l, r Entry) (bool, error)) CompareResult {
	cs := opt.Case.Sensitive()
	res := CompareResult{Left: make([]CompareStatus, len(left)), Right: make([]CompareStatus, len(right))}
	keyOf := func(e Entry) string {
		k := e.Name
		if !cs {
			k = foldString(k)
		}
		if e.IsDir {
			return "d:" + k // a file and a folder with the same name are not partners
		}
		return "f:" + k
	}
	skip := func(e Entry) bool {
		if e.Name == ".." || e.Name == "." {
			return true
		}
		if e.IsDir && !opt.Subdirs {
			return true
		}
		if opt.Filter != nil && !opt.Filter.Match(e.Name, e.IsDir) {
			return true
		}
		return false
	}
	rightIdx := make(map[string]int, len(right))
	for i, e := range right {
		if !skip(e) {
			rightIdx[keyOf(e)] = i
		}
	}
	paired := make([]bool, len(right))
	setErr := func(err error) {
		if err != nil && res.Err == nil {
			res.Err = err
		}
	}
	for i, l := range left {
		if skip(l) {
			continue
		}
		j, ok := rightIdx[keyOf(l)]
		if !ok {
			res.Left[i] = StatusOnlyHere
			continue
		}
		paired[j] = true
		r := right[j]
		if l.IsDir {
			if dirDiffers != nil {
				d, err := dirDiffers(l, r)
				setErr(err)
				if d {
					res.Left[i], res.Right[j] = StatusDirDiffers, StatusDirDiffers
				}
			}
			continue
		}
		ls, rs, err := compareFiles(l, r, opt)
		setErr(err)
		res.Left[i], res.Right[j] = ls, rs
	}
	for j, r := range right {
		if !paired[j] && !skip(r) {
			res.Right[j] = StatusOnlyHere
		}
	}
	return res
}

// compareFiles decides the status of a pair of files with the same name.
func compareFiles(l, r Entry, opt CompareOptions) (CompareStatus, CompareStatus, error) {
	contentEqual := func() (bool, error) {
		if opt.Content == nil {
			return true, nil
		}
		eq, err := opt.Content(l, r)
		if err != nil {
			return false, err
		}
		return eq, nil
	}
	sameDate := opt.IgnoreDate || datesEqual(l.ModTime, r.ModTime, opt)
	if sameDate {
		if l.Size != r.Size {
			return StatusSizeDiffers, StatusSizeDiffers, nil
		}
		eq, err := contentEqual()
		if !eq {
			return StatusContentDiffers, StatusContentDiffers, err
		}
		return StatusSame, StatusSame, nil
	}
	if opt.Content != nil && l.Size == r.Size {
		eq, err := contentEqual()
		if err != nil {
			return StatusContentDiffers, StatusContentDiffers, err
		}
		if eq {
			return StatusSame, StatusSame, nil
		}
	}
	if l.ModTime.After(r.ModTime) {
		return StatusNewer, StatusOlder, nil
	}
	return StatusOlder, StatusNewer, nil
}

// datesEqual applies the tolerance and the optional one-hour rule.
func datesEqual(a, b time.Time, opt CompareOptions) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	if d <= opt.Tolerance {
		return true
	}
	if opt.IgnoreHourShift {
		h := d - time.Hour
		if h < 0 {
			h = -h
		}
		return h <= opt.Tolerance
	}
	return false
}

// EntriesFS lists folder dir of fsys as Entries. Path holds the slash-separated path inside fsys, which is what
// ContentEqualFS needs. Folder sizes are -1 (not calculated).
func EntriesFS(fsys fs.FS, dir string) ([]Entry, error) {
	des, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue // vanished between listing and stat; the next refresh will not show it either
		}
		e := Entry{Name: de.Name(), IsDir: de.IsDir(), Size: info.Size(), ModTime: info.ModTime(),
			Path: path.Join(dir, de.Name())}
		if e.IsDir {
			e.Size = -1
		}
		out = append(out, e)
	}
	return out, nil
}

// CompareTrees compares folder lroot of lfs with folder rroot of rfs and returns the two listings with their
// statuses. With opt.Subdirs, folders on both sides are walked recursively and marked when they differ.
func CompareTrees(lfs fs.FS, lroot string, rfs fs.FS, rroot string, opt CompareOptions) (left, right []Entry,
	res CompareResult, err error) {
	if left, err = EntriesFS(lfs, lroot); err != nil {
		return nil, nil, res, err
	}
	if right, err = EntriesFS(rfs, rroot); err != nil {
		return nil, nil, res, err
	}
	var differ func(l, r Entry) (bool, error)
	differ = func(l, r Entry) (bool, error) {
		if opt.Filter != nil && opt.Filter.ExcludesDir(l.Name) {
			return false, nil
		}
		le, err := EntriesFS(lfs, l.Path)
		if err != nil {
			return true, err
		}
		re, err := EntriesFS(rfs, r.Path)
		if err != nil {
			return true, err
		}
		sub := compareLists(le, re, opt, differ)
		return sub.Differs(), sub.Err
	}
	if !opt.Subdirs {
		differ = nil
	}
	return left, right, compareLists(left, right, opt, differ), nil
}

// ContentEqualFS returns a content comparer for CompareOptions.Content that reads both files from the given file
// systems by Entry.Path (as set by EntriesFS) and compares them byte by byte, stopping at the first difference.
func ContentEqualFS(lfs, rfs fs.FS) func(l, r Entry) (bool, error) {
	return func(l, r Entry) (bool, error) {
		if l.Path == "" || r.Path == "" {
			return false, errors.New("masks: content compare needs Entry.Path")
		}
		lf, err := lfs.Open(l.Path)
		if err != nil {
			return false, err
		}
		defer lf.Close()
		rf, err := rfs.Open(r.Path)
		if err != nil {
			return false, err
		}
		defer rf.Close()
		return readersEqual(lf, rf)
	}
}

// readersEqual compares two streams in 64 KiB blocks.
func readersEqual(a, b io.Reader) (bool, error) {
	const block = 64 * 1024
	ba, bb := make([]byte, block), make([]byte, block)
	for {
		na, ea := io.ReadFull(a, ba)
		nb, eb := io.ReadFull(b, bb)
		if !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		aEnd := ea == io.EOF || ea == io.ErrUnexpectedEOF
		bEnd := eb == io.EOF || eb == io.ErrUnexpectedEOF
		if ea != nil && !aEnd {
			return false, ea
		}
		if eb != nil && !bEnd {
			return false, eb
		}
		if aEnd || bEnd {
			return aEnd == bEnd, nil
		}
	}
}
