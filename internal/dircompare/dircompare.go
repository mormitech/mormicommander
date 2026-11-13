// Package dircompare is the engine behind Total Commander's "Compare directories" command (Shift+F2, Mark > Compare
// directories, cm_CompareDirs and cm_CompareDirsWithSubdirs). The user has two folders open side by side and wants to
// know at a glance which files are missing on the other side and which copy is the newer one. TC answers by selecting
// files in both panels: a file that exists only on one side is selected there, a file that is newer than its namesake
// on the other side is selected on its own side, and a pair with the same date but a different size is selected on
// both sides because neither can be called newer. Equal files stay unselected, and an optional "hide equal" view
// leaves only the differing files visible.
//
// The engine compares by name, date and size, with the tolerances people need in practice: FAT and many network
// shares store times with two-second resolution, and a daylight saving switch or a time zone mismatch shifts every
// time by exactly one hour, so both can be ignored. It can also open the files and compare their bytes, so that two
// copies with different dates but identical content count as equal. Subfolders can be left out, marked only when
// they are missing on the other side (TC's "with subdirs" variant), or walked recursively so that a folder is marked
// when anything inside it differs. The result is a pair of selection sets, one per panel, plus the full per-name
// verdicts for a UI that wants to colour rows or show arrows. Nothing here touches the UI: the panels feed in two
// Listers and get plain data back.
package dircompare

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// Status is the verdict for one name that appears in at least one of the two folders.
type Status int

const (
	// Equal means both sides have it and, under the chosen options, they are the same.
	Equal Status = iota
	// LeftOnly means only the left folder has this name; it is selected in the left panel.
	LeftOnly
	// RightOnly means only the right folder has this name; it is selected in the right panel.
	RightOnly
	// LeftNewer means both sides have the file and the left copy is newer; it is selected in the left panel.
	LeftNewer
	// RightNewer means both sides have the file and the right copy is newer; it is selected in the right panel.
	RightNewer
	// Different means both sides have it, they differ, and neither is newer (same time, other size or content). TC
	// selects both copies in that case.
	Different
	// TypeMismatch means one side has a file and the other a folder of the same name. Both are selected.
	TypeMismatch
	// Error means the entry could not be compared (a folder or file could not be read); see Item.Err. Both present
	// sides are selected so the user looks at them.
	Error
)

// String gives a short English word for logs and tests.
func (s Status) String() string {
	switch s {
	case Equal:
		return "equal"
	case LeftOnly:
		return "left-only"
	case RightOnly:
		return "right-only"
	case LeftNewer:
		return "left-newer"
	case RightNewer:
		return "right-newer"
	case Different:
		return "different"
	case TypeMismatch:
		return "type-mismatch"
	case Error:
		return "error"
	}
	return "unknown"
}

// Symbol is the arrow or sign the synchronize-style result list shows for a status: "=" equal, "->" left copy wins,
// "<-" right copy wins, "!=" different with no winner, "x" type mismatch or error.
func (s Status) Symbol() string {
	switch s {
	case Equal:
		return "="
	case LeftOnly, LeftNewer:
		return "->"
	case RightOnly, RightNewer:
		return "<-"
	case Different:
		return "!="
	}
	return "x"
}

// MarksLeft reports whether this status selects the left copy.
func (s Status) MarksLeft() bool {
	switch s {
	case LeftOnly, LeftNewer, Different, TypeMismatch, Error:
		return true
	}
	return false
}

// MarksRight reports whether this status selects the right copy.
func (s Status) MarksRight() bool {
	switch s {
	case RightOnly, RightNewer, Different, TypeMismatch, Error:
		return true
	}
	return false
}

// SubdirMode says what happens to folders.
type SubdirMode int

const (
	// SubdirsIgnore leaves folders out entirely, like plain cm_CompareDirs: only files are compared and selected.
	SubdirsIgnore SubdirMode = iota
	// SubdirsMissing selects a folder when it exists on one side only, like cm_CompareDirsWithSubdirs. Folders on
	// both sides are not opened and stay unselected.
	SubdirsMissing
	// SubdirsRecursive also walks folders present on both sides and selects such a folder on a side when something
	// inside it is selected on that side. Item.Children holds the verdicts inside.
	SubdirsRecursive
)

// CaseMode says how names are matched between the two folders.
type CaseMode int

const (
	// CaseAuto matches names without regard to case on Windows and macOS (whose usual file systems ignore case) and
	// exactly elsewhere.
	CaseAuto CaseMode = iota
	// CaseSensitive matches names exactly: "a.txt" and "A.txt" are two different files.
	CaseSensitive
	// CaseInsensitive treats "a.txt" and "A.txt" as the same file.
	CaseInsensitive
)

// Options tune the comparison. The zero value is TC's default Shift+F2: files only, exact times, no content check.
type Options struct {
	// Subdirs says what happens to folders; see SubdirMode.
	Subdirs SubdirMode
	// Case says how names are matched; see CaseMode.
	Case CaseMode
	// TimeTolerance is the largest time difference still counted as "same time". Use 2*time.Second for FAT drives and
	// many network shares, time.Second to ignore sub-second precision lost by a copy. Zero means only exactly equal
	// times (after truncating to whole milliseconds) are the same.
	TimeTolerance time.Duration
	// IgnoreHourOffset also counts times that differ by exactly one hour (within TimeTolerance) as the same, for files
	// copied across a daylight saving switch or between file systems that store local time.
	IgnoreHourOffset bool
	// IgnoreDate compares by size (and content, when ByContent is set) only: a pair with equal size is equal whatever
	// its dates, and a pair with different sizes is Different and selected on both sides.
	IgnoreDate bool
	// ByContent opens files of equal size and compares their bytes. Identical content makes the pair Equal even when
	// the dates differ; different content falls back to the dates to decide which side is newer (or Different when
	// the times are the same).
	ByContent bool
	// IncludeHidden lets entries with Hidden set take part, matching the panel's "show hidden files" switch. When it
	// is false they are skipped, so nothing invisible ends up selected.
	IncludeHidden bool
	// Filter, when set, decides which entries take part; it gets the path relative to the compared folders (with "/"
	// separators) and the entry. Entries it rejects are neither compared nor selected, like TC's file type filter.
	Filter func(rel string, e Entry) bool
	// Progress, when set, is called after each compared name and during long content comparisons. It must be quick.
	Progress func(Progress)
}

// Progress is a snapshot handed to Options.Progress.
type Progress struct {
	Current      string // relative path being compared
	Items        int    // names compared so far
	ContentBytes int64  // bytes read so far by content comparison (both sides counted)
}

// Item is the verdict for one name.
type Item struct {
	Name     string // the name as found on the left side, or on the right side when the left lacks it
	Rel      string // path relative to the compared folders with "/" separators; equals Name at the top level
	Left     *Entry // nil when the left side lacks it
	Right    *Entry // nil when the right side lacks it
	Status   Status
	Children []Item // verdicts inside a folder present on both sides, only with SubdirsRecursive
	Err      error  // why Status is Error
}

// IsDir reports whether this item is a folder on the sides that have it (a mismatch counts as not a folder).
func (it Item) IsDir() bool {
	if it.Left != nil && !it.Left.IsDir {
		return false
	}
	if it.Right != nil && !it.Right.IsDir {
		return false
	}
	return it.Left != nil || it.Right != nil
}

// Result is what a comparison produces.
type Result struct {
	// Items are the verdicts for the top-level names, sorted by name (folders first).
	Items []Item
	// LeftSelect and RightSelect are the names to select in each panel (top-level names as each panel spells them),
	// sorted. This is what Shift+F2 does to the panels.
	LeftSelect  []string
	RightSelect []string
	// LeftEqual and RightEqual are the names that compared equal, for the "hide equal files" view (F-352).
	LeftEqual  []string
	RightEqual []string
	// Counts per status over all items, including those inside recursed folders.
	Counts map[Status]int
	// Errors collects every item error, so the UI can list them after the run.
	Errors []error
}

// Differ reports whether anything was selected on either side.
func (r *Result) Differ() bool { return len(r.LeftSelect) > 0 || len(r.RightSelect) > 0 }

// Flatten returns every item, depth first, including those inside recursed folders.
func (r *Result) Flatten() []Item {
	var out []Item
	var walk func([]Item)
	walk = func(items []Item) {
		for _, it := range items {
			out = append(out, it)
			walk(it.Children)
		}
	}
	walk(r.Items)
	return out
}

// ErrNilLister is returned when Compare is given a nil Lister.
var ErrNilLister = errors.New("dircompare: nil lister")

// Compare compares folder leftDir of left with folder rightDir of right. The two Listers may differ (an archive on
// one side, the local disk on the other). Errors reading the two top folders are returned; errors further down are
// recorded in the items and in Result.Errors. Compare honours ctx and returns ctx.Err() when it is cancelled.
func Compare(ctx context.Context, left Lister, leftDir string, right Lister, rightDir string, opt Options) (*Result, error) {
	if left == nil || right == nil {
		return nil, ErrNilLister
	}
	lents, err := left.ReadDir(ctx, leftDir)
	if err != nil {
		return nil, err
	}
	rents, err := right.ReadDir(ctx, rightDir)
	if err != nil {
		return nil, err
	}
	c := &comparer{ctx: ctx, l: left, r: right, opt: opt, res: &Result{Counts: map[Status]int{}},
		ancL: map[string]bool{}, ancR: map[string]bool{}}
	c.enter(left, c.ancL, leftDir)
	c.enter(right, c.ancR, rightDir)
	items, err := c.compareLists(lents, rents, leftDir, rightDir, "")
	if err != nil {
		return nil, err
	}
	res := c.res
	res.Items = items
	for _, it := range items {
		if it.Status.MarksLeft() && it.Left != nil {
			res.LeftSelect = append(res.LeftSelect, it.Left.Name)
		}
		if it.Status.MarksRight() && it.Right != nil {
			res.RightSelect = append(res.RightSelect, it.Right.Name)
		}
		if it.Status == Equal {
			res.LeftEqual = append(res.LeftEqual, it.Left.Name)
			res.RightEqual = append(res.RightEqual, it.Right.Name)
		}
	}
	for _, s := range [][]string{res.LeftSelect, res.RightSelect, res.LeftEqual, res.RightEqual} {
		sort.Strings(s)
	}
	return res, nil
}

// CompareEntries compares two listings a panel already has in memory, without opening files or folders. ByContent
// and SubdirsRecursive are ignored here (they need a Lister); use Compare for those. It is what a panel uses for an
// instant Shift+F2 on two plain folder listings.
func CompareEntries(left, right []Entry, opt Options) []Item {
	opt.ByContent = false
	if opt.Subdirs == SubdirsRecursive {
		opt.Subdirs = SubdirsMissing
	}
	c := &comparer{ctx: context.Background(), opt: opt, res: &Result{Counts: map[Status]int{}}}
	items, _ := c.compareLists(left, right, "", "", "")
	return items
}

// comparer carries the state of one Compare run.
type comparer struct {
	ctx   context.Context
	l, r  Lister
	opt   Options
	res   *Result
	items int
	bytes int64
	// ancL and ancR hold the real paths of the folders being walked on each side (the roots and every folder on the
	// way down), so a link back to an ancestor is recognised as a loop. depth guards Listers without RealPather.
	ancL, ancR map[string]bool
	depth      int
}

// maxDepth limits recursion when a Lister cannot resolve real paths, as a last guard against link loops.
const maxDepth = 128

// ErrSymlinkLoop is recorded (inside an ItemError) for a folder that leads back to one of its own ancestors.
var ErrSymlinkLoop = errors.New("symbolic link loop: folder not entered")

// ErrTooDeep is recorded for a folder nested deeper than maxDepth on a Lister that cannot detect loops itself.
var ErrTooDeep = errors.New("folder nesting too deep (symbolic link loop?): folder not entered")

func isRealPather(l Lister) bool {
	_, ok := l.(RealPather)
	return ok
}

// enter records a folder as an ancestor on one side. It returns the key to release later, and false when the folder
// is already an ancestor (a loop). Without a RealPather it always succeeds.
func (c *comparer) enter(ls Lister, anc map[string]bool, dir string) (string, bool) {
	rp, ok := ls.(RealPather)
	if !ok {
		return "", true
	}
	real, err := rp.RealPath(c.ctx, dir)
	if err != nil {
		return "", true
	}
	if anc[real] {
		return "", false
	}
	anc[real] = true
	return real, true
}

func (c *comparer) caseInsensitive() bool {
	switch c.opt.Case {
	case CaseSensitive:
		return false
	case CaseInsensitive:
		return true
	}
	return defaultCaseInsensitive
}

// key is the matching key of a name under the chosen case mode.
func (c *comparer) key(name string) string {
	if c.caseInsensitive() {
		return strings.ToLower(strings.ToUpper(name))
	}
	return name
}

func joinRel(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

// keep applies the folder mode, the hidden switch and the user filter to one entry.
func (c *comparer) keep(rel string, e Entry) bool {
	if e.IsDir && c.opt.Subdirs == SubdirsIgnore {
		return false
	}
	if e.Name == "." || e.Name == ".." {
		return false
	}
	if e.Hidden && !c.opt.IncludeHidden {
		return false
	}
	if c.opt.Filter != nil && !c.opt.Filter(rel, e) {
		return false
	}
	return true
}

// compareLists matches two listings by name and compares each pair. ldir and rdir are the folders the listings came
// from (used to open files and subfolders), base is their path relative to the compared roots.
func (c *comparer) compareLists(lents, rents []Entry, ldir, rdir, base string) ([]Item, error) {
	type pair struct{ l, r *Entry }
	pairs := map[string]*pair{}
	var order []string
	add := func(ents []Entry, left bool) {
		// Pair in name order, not in the order the Lister happened to return (archives and other Listers need not
		// sort), so that when two names on one side fold together the same one always wins. A sorted copy keeps the
		// caller's slice untouched; the items point into the copy.
		ents = append([]Entry(nil), ents...)
		sort.SliceStable(ents, func(i, j int) bool { return ents[i].Name < ents[j].Name })
		for i := range ents {
			e := &ents[i]
			if !c.keep(joinRel(base, e.Name), *e) {
				continue
			}
			k := c.key(e.Name)
			p := pairs[k]
			if p == nil {
				p = &pair{}
				pairs[k] = p
				order = append(order, k)
			}
			// When a case-sensitive side has two names that fold together, the first one wins and the others are
			// compared as lone entries, so nothing is silently dropped.
			if (left && p.l != nil) || (!left && p.r != nil) {
				k2 := "\x00R" + e.Name
				if left {
					k2 = "\x00L" + e.Name
				}
				p = &pair{}
				pairs[k2] = p
				order = append(order, k2)
			}
			if left {
				p.l = e
			} else {
				p.r = e
			}
		}
	}
	add(lents, true)
	add(rents, false)

	items := make([]Item, 0, len(order))
	for _, k := range order {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		p := pairs[k]
		it := Item{Left: p.l, Right: p.r}
		if p.l != nil {
			it.Name = p.l.Name
		} else {
			it.Name = p.r.Name
		}
		it.Rel = joinRel(base, it.Name)
		if err := c.judge(&it, ldir, rdir); err != nil {
			return nil, err
		}
		c.res.Counts[it.Status]++
		if it.Err != nil {
			c.res.Errors = append(c.res.Errors, it.Err)
		}
		c.items++
		c.progress(it.Rel)
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		di, dj := items[i].IsDir(), items[j].IsDir()
		if di != dj {
			return di
		}
		ki, kj := c.key(items[i].Name), c.key(items[j].Name)
		if ki != kj {
			return ki < kj
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (c *comparer) progress(cur string) {
	if c.opt.Progress != nil {
		c.opt.Progress(Progress{Current: cur, Items: c.items, ContentBytes: c.bytes})
	}
}

// judge fills in it.Status (and Children, Err). It returns an error only for cancellation.
func (c *comparer) judge(it *Item, ldir, rdir string) error {
	l, r := it.Left, it.Right
	switch {
	case l == nil:
		it.Status = RightOnly
		return nil
	case r == nil:
		it.Status = LeftOnly
		return nil
	case l.IsDir != r.IsDir:
		it.Status = TypeMismatch
		return nil
	case l.IsDir:
		it.Status = Equal
		if c.opt.Subdirs != SubdirsRecursive || c.l == nil {
			return nil
		}
		return c.recurse(it, ldir, rdir)
	}
	return c.judgeFiles(it, ldir, rdir)
}

// recurse compares the insides of a folder present on both sides.
func (c *comparer) recurse(it *Item, ldir, rdir string) error {
	lsub, rsub := c.l.Join(ldir, it.Left.Name), c.r.Join(rdir, it.Right.Name)
	if c.depth >= maxDepth && !(isRealPather(c.l) && isRealPather(c.r)) {
		it.Status = Error
		it.Err = &ItemError{Rel: it.Rel, Err: ErrTooDeep}
		return nil
	}
	keyL, okL := c.enter(c.l, c.ancL, lsub)
	keyR, okR := c.enter(c.r, c.ancR, rsub)
	defer func() {
		if keyL != "" {
			delete(c.ancL, keyL)
		}
		if keyR != "" {
			delete(c.ancR, keyR)
		}
	}()
	if !okL || !okR {
		it.Status = Error
		it.Err = &ItemError{Rel: it.Rel, Err: ErrSymlinkLoop}
		return nil
	}
	c.depth++
	defer func() { c.depth-- }()
	lents, lerr := c.l.ReadDir(c.ctx, lsub)
	rents, rerr := c.r.ReadDir(c.ctx, rsub)
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if lerr != nil || rerr != nil {
		it.Status = Error
		it.Err = &ItemError{Rel: it.Rel, Err: errors.Join(lerr, rerr)}
		return nil
	}
	children, err := c.compareLists(lents, rents, lsub, rsub, it.Rel)
	if err != nil {
		return err
	}
	it.Children = children
	ml, mr := false, false
	for _, ch := range children {
		ml = ml || (ch.Status.MarksLeft() && ch.Left != nil)
		mr = mr || (ch.Status.MarksRight() && ch.Right != nil)
	}
	switch {
	case ml && mr:
		it.Status = Different
	case ml:
		it.Status = LeftNewer
	case mr:
		it.Status = RightNewer
	default:
		it.Status = Equal
	}
	return nil
}

// judgeFiles decides between two files of the same name.
func (c *comparer) judgeFiles(it *Item, ldir, rdir string) error {
	l, r := it.Left, it.Right
	timeCmp := c.compareTimes(l.ModTime, r.ModTime) // >0 left newer, <0 right newer, 0 same
	if c.opt.IgnoreDate {
		timeCmp = 0
	}
	if l.Size != r.Size {
		it.Status = byTime(timeCmp, Different)
		return nil
	}
	if !c.opt.ByContent || l.Special || r.Special {
		// Pipes, sockets and devices have no bytes to compare; reading one could block forever.
		it.Status = byTime(timeCmp, Equal)
		return nil
	}
	same, err := c.sameContent(c.l.Join(ldir, l.Name), c.r.Join(rdir, r.Name), it.Rel)
	if cerr := c.ctx.Err(); cerr != nil {
		return cerr
	}
	if err != nil {
		it.Status = Error
		it.Err = &ItemError{Rel: it.Rel, Err: err}
		return nil
	}
	if same {
		it.Status = Equal
		return nil
	}
	it.Status = byTime(timeCmp, Different)
	return nil
}

func byTime(cmp int, same Status) Status {
	switch {
	case cmp > 0:
		return LeftNewer
	case cmp < 0:
		return RightNewer
	}
	return same
}

// compareTimes compares two modification times under the tolerance options: 1 when a is newer, -1 when b is newer, 0
// when they count as the same.
func (c *comparer) compareTimes(a, b time.Time) int {
	return CompareTimes(a, b, c.opt.TimeTolerance, c.opt.IgnoreHourOffset)
}

// CompareTimes compares two modification times the way Compare does: 1 when a is newer, -1 when b is newer, 0 when
// they count as the same. Times are first truncated to whole milliseconds, since many file systems and archive formats
// keep no more. tol is the largest difference still counted as the same; hourOffset also accepts a difference of one
// hour plus or minus tol (daylight saving or time zone shifts).
func CompareTimes(a, b time.Time, tol time.Duration, hourOffset bool) int {
	a, b = a.Truncate(time.Millisecond), b.Truncate(time.Millisecond)
	d := a.Sub(b)
	abs := d
	if abs < 0 {
		abs = -abs
	}
	if tol < 0 {
		tol = -tol
	}
	if abs <= tol {
		return 0
	}
	if hourOffset && abs >= time.Hour-tol && abs <= time.Hour+tol {
		return 0
	}
	if d > 0 {
		return 1
	}
	return -1
}

// ItemError tells which relative path could not be compared.
type ItemError struct {
	Rel string
	Err error
}

func (e *ItemError) Error() string { return "compare " + e.Rel + ": " + e.Err.Error() }
func (e *ItemError) Unwrap() error { return e.Err }
