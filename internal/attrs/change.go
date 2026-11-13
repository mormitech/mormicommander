// Planning and applying an attribute change. The dialog collects a Request
// (which files, recursive or not, a filter, and what to change: attributes,
// permissions, owner, dates), MakePlan walks the files and works out the
// before and after state of every entry without touching anything, and the UI
// shows that plan as a preview ("rw-r--r-- -> rwxr-xr-x", "modified 2024-01-02
// -> 2025-06-01"). Only when the user confirms does Plan.Apply make the
// changes. Splitting it this way gives TC parity (the same dialog semantics,
// including grey "leave alone" checkboxes for mixed selections and the
// "Current" date button that stamps every file with the same instant) plus a
// safe dry run that TC itself lacks.
//
// Apply works through the plan from the deepest entries upwards, so taking
// away a folder's execute or read permission never locks us out of the files
// still to be changed inside it. Errors on single entries are collected and
// the run continues, the way TC reports failures at the end of the operation.
package attrs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ApplyTo restricts a change to files, folders or both.
type ApplyTo int

const (
	FilesAndFolders ApplyTo = iota
	FilesOnly
	FoldersOnly
)

// TimeMode says what to do with one of the three time stamps.
type TimeMode int

const (
	TimeKeep  TimeMode = iota // leave it alone
	TimeSet                   // set it to At
	TimeNow                   // set it to the moment the plan was made
	TimeShift                 // move the existing value by By (may be negative)
	TimeCopy                  // copy from another time stamp of the same file (From)
)

// TimeField names a time stamp, used by TimeCopy.
type TimeField int

const (
	FieldModified TimeField = iota
	FieldAccessed
	FieldCreated
)

// TimeValue is the dialog's setting for one time stamp.
type TimeValue struct {
	Mode TimeMode
	At   time.Time
	By   time.Duration
	From TimeField
}

// SetTime, NowTime, ShiftTime and CopyTime are shorthands for building
// TimeValues.
func SetTime(t time.Time) TimeValue       { return TimeValue{Mode: TimeSet, At: t} }
func NowTime() TimeValue                  { return TimeValue{Mode: TimeNow} }
func ShiftTime(d time.Duration) TimeValue { return TimeValue{Mode: TimeShift, By: d} }
func CopyTime(f TimeField) TimeValue      { return TimeValue{Mode: TimeCopy, From: f} }

// TimeChange holds the settings for the three time stamps. Func, when set,
// may supply a modification time per file (for example from EXIF data or a
// date in the file name); returning ok=false keeps the Modified setting.
type TimeChange struct {
	Modified TimeValue
	Accessed TimeValue
	Created  TimeValue
	Func     func(info Info) (t time.Time, ok bool)
}

// IsZero reports whether no time stamp changes.
func (c TimeChange) IsZero() bool {
	return c.Modified.Mode == TimeKeep && c.Accessed.Mode == TimeKeep && c.Created.Mode == TimeKeep && c.Func == nil
}

// OwnerChange holds a new owner and/or group, by name or number. An empty
// string keeps the current value.
type OwnerChange struct {
	Owner string
	Group string
}

// IsZero reports whether ownership stays as it is.
func (c OwnerChange) IsZero() bool { return c.Owner == "" && c.Group == "" }

// Request is everything the attributes dialog collected.
type Request struct {
	Paths     []string
	Recursive bool // TC's "Process subfolders"
	ApplyTo   ApplyTo
	Filter    *Filter
	// FollowLinks changes what symbolic links point to instead of the links
	// themselves. A recursive walk never descends through links either way,
	// which avoids cycles and surprises, as in TC.
	FollowLinks bool

	// Attrs changes Windows attributes. On Unix read-only maps to the write
	// bits and, on macOS, hidden maps to the Finder hidden flag.
	Attrs WinAttrChange
	Perm  PermChange
	Owner OwnerChange
	Times TimeChange
}

// IsZero reports whether the request changes nothing at all.
func (r Request) IsZero() bool {
	return r.Attrs.IsZero() && r.Perm.IsZero() && r.Owner.IsZero() && r.Times.IsZero()
}

// Validate checks a request against the platform's capabilities before any
// file is looked at, so the dialog can show a clear message.
func Validate(r Request) error {
	c := Caps()
	var problems []string
	if len(r.Paths) == 0 {
		problems = append(problems, "nothing selected")
	}
	if r.Attrs.Set&r.Attrs.Clear != 0 {
		problems = append(problems, fmt.Sprintf("attributes %s are both set and cleared", (r.Attrs.Set&r.Attrs.Clear).String()))
	}
	if bad := (r.Attrs.Set | r.Attrs.Clear) &^ c.SettableAttrs; bad != 0 {
		problems = append(problems, fmt.Sprintf("attribute %s cannot be changed on this platform", strings.Join(bad.Names(), ", ")))
	}
	if !r.Perm.IsZero() {
		if !c.UnixPerms {
			problems = append(problems, "permissions cannot be changed on this platform")
		} else if err := r.Perm.Validate(); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if !r.Owner.IsZero() && !c.Owner {
		problems = append(problems, "owner and group cannot be changed on this platform")
	}
	if r.Times.Created.Mode != TimeKeep && !c.CreatedTime {
		problems = append(problems, "the creation date cannot be changed on this platform")
	}
	for _, tv := range []TimeValue{r.Times.Modified, r.Times.Accessed, r.Times.Created} {
		if tv.Mode == TimeSet && tv.At.IsZero() {
			problems = append(problems, "a date to set is empty")
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// State is the part of an entry an attribute change can touch.
type State struct {
	Perm       fs.FileMode
	Win        WinAttr
	HiddenFlag bool
	UID, GID   int
	Owner      string
	Group      string
	Modified   time.Time
	Accessed   time.Time
	Created    time.Time
}

func stateOf(info Info) State {
	return State{
		Perm: info.Perm(), Win: info.Win, HiddenFlag: info.HiddenFlag,
		UID: info.UID, GID: info.GID, Owner: info.Owner, Group: info.Group,
		Modified: info.Modified, Accessed: info.Accessed, Created: info.Created,
	}
}

// Item is one entry of a plan.
type Item struct {
	Path   string
	Depth  int // 0 for selected entries, 1 for their children and so on
	Info   Info
	Before State
	After  State
	// Changes lists the differences in plain English, ready for the preview.
	Changes []string
	// Notes explains parts of the request that do not apply to this entry
	// (for example permissions on a symbolic link).
	Notes []string
	// Err is set when the entry could not be read while planning.
	Err error

	setPerm, setWin, setHidden, setOwner bool
	setMtime, setAtime, setCtime         bool
}

// Changed reports whether applying the plan will touch this entry.
func (it *Item) Changed() bool { return it.Err == nil && len(it.Changes) > 0 }

// Summary is a one-line description for the preview list.
func (it *Item) Summary() string {
	switch {
	case it.Err != nil:
		return it.Path + ": " + it.Err.Error()
	case len(it.Changes) == 0:
		return it.Path + ": no change"
	default:
		return it.Path + ": " + strings.Join(it.Changes, "; ")
	}
}

// Plan is the result of MakePlan: the request, the instant used for "now"
// and one Item per visited entry in walk order (parents before children).
type Plan struct {
	Request Request
	Now     time.Time
	Items   []Item

	uid, gid int             // resolved owner and group, -1 to keep
	seen     map[string]bool // paths already planned, so overlaps count once
}

// ChangedCount returns how many entries will be touched.
func (p *Plan) ChangedCount() int {
	n := 0
	for i := range p.Items {
		if p.Items[i].Changed() {
			n++
		}
	}
	return n
}

// Errors returns the entries that could not be planned.
func (p *Plan) Errors() []*Item {
	var out []*Item
	for i := range p.Items {
		if p.Items[i].Err != nil {
			out = append(out, &p.Items[i])
		}
	}
	return out
}

// timeFormat is how dates appear in the preview.
const timeFormat = "2006-01-02 15:04:05"

// MakePlan validates the request, walks the selection and computes the
// changes. It does not modify anything. Entries that a filter rejects are
// left out of the plan entirely; entries that pass but need no change are
// included with an empty Changes list so the preview can show them.
func MakePlan(ctx context.Context, r Request) (*Plan, error) {
	return makePlanAt(ctx, r, time.Now())
}

func makePlanAt(ctx context.Context, r Request, now time.Time) (*Plan, error) {
	if err := Validate(r); err != nil {
		return nil, err
	}
	p := &Plan{Request: r, Now: now, uid: -1, gid: -1, seen: map[string]bool{}}
	if r.Owner.Owner != "" {
		uid, err := lookupUID(r.Owner.Owner)
		if err != nil {
			return nil, fmt.Errorf("owner %q: %w", r.Owner.Owner, err)
		}
		p.uid = uid
	}
	if r.Owner.Group != "" {
		gid, err := lookupGID(r.Owner.Group)
		if err != nil {
			return nil, fmt.Errorf("group %q: %w", r.Owner.Group, err)
		}
		p.gid = gid
	}
	for _, path := range r.Paths {
		if err := p.visit(ctx, path, 0); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// visit plans one entry and, for folders in a recursive request, its
// children in name order.
func (p *Plan) visit(ctx context.Context, path string, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := p.Request
	key := filepath.Clean(path)
	if p.seen[key] {
		return nil
	}
	p.seen[key] = true
	info, err := Stat(path)
	if err != nil {
		p.Items = append(p.Items, Item{Path: path, Depth: depth, Err: err})
		return nil
	}
	target := info
	if r.FollowLinks && info.Kind == KindSymlink {
		if t, err := StatFollow(path); err == nil {
			target = t
		}
	}
	if p.wants(target) {
		p.Items = append(p.Items, p.planItem(path, depth, target))
	}
	if !r.Recursive || info.Kind != KindDir {
		return nil
	}
	if depth > 0 && !r.Filter.Descend(info.Name) {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		p.Items = append(p.Items, Item{Path: path, Depth: depth, Info: info,
			Err: fmt.Errorf("cannot read folder: %w", err)})
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if err := p.visit(ctx, filepath.Join(path, e.Name()), depth+1); err != nil {
			return err
		}
	}
	return nil
}

// wants applies ApplyTo and the filter.
func (p *Plan) wants(info Info) bool {
	switch p.Request.ApplyTo {
	case FilesOnly:
		if info.IsDir() {
			return false
		}
	case FoldersOnly:
		if !info.IsDir() {
			return false
		}
	}
	return p.Request.Filter.Matches(info)
}

// planItem works out the after state of one entry.
func (p *Plan) planItem(path string, depth int, info Info) Item {
	r := p.Request
	caps := Caps()
	it := Item{Path: path, Depth: depth, Info: info, Before: stateOf(info)}
	it.After = it.Before
	isLink := info.Kind == KindSymlink

	// Permissions, with read-only folded in on Unix.
	if caps.UnixPerms {
		newPerm := it.Before.Perm
		permTouched := false
		if !r.Perm.IsZero() {
			if np, err := r.Perm.Apply(newPerm, info.IsDir()); err == nil {
				newPerm, permTouched = np, true
			} else {
				it.Err = err
				return it
			}
		}
		if !caps.NativeAttrs {
			if r.Attrs.Set&AttrReadOnly != 0 {
				newPerm &^= 0o222
				permTouched = true
			}
			if r.Attrs.Clear&AttrReadOnly != 0 && newPerm&0o200 == 0 {
				newPerm |= 0o200
				permTouched = true
			}
		}
		if permTouched {
			if isLink {
				it.Notes = append(it.Notes, "permissions of a symbolic link itself are not used, left unchanged")
			} else if newPerm != it.Before.Perm {
				it.After.Perm = newPerm
				it.setPerm = true
				it.Changes = append(it.Changes, fmt.Sprintf("permissions %s (%s) → %s (%s)",
					FormatPerm(it.Before.Perm), FormatOctal(it.Before.Perm), FormatPerm(newPerm), FormatOctal(newPerm)))
			}
		}
	}

	// Windows attributes, or the macOS hidden flag.
	if caps.NativeAttrs {
		if !r.Attrs.IsZero() {
			na := r.Attrs.Apply(it.Before.Win)
			if na != it.Before.Win {
				it.After.Win = na
				it.setWin = true
				it.Changes = append(it.Changes, fmt.Sprintf("attributes %s → %s", it.Before.Win.String(), na.String()))
			}
		}
	} else {
		hidden := it.Before.HiddenFlag
		if caps.HiddenFlag && (r.Attrs.Set|r.Attrs.Clear)&AttrHidden != 0 {
			hidden = r.Attrs.Set&AttrHidden != 0
			if hidden != it.Before.HiddenFlag {
				it.After.HiddenFlag = hidden
				it.setHidden = true
				it.Changes = append(it.Changes, fmt.Sprintf("hidden flag %s → %s", onOff(it.Before.HiddenFlag), onOff(hidden)))
			}
			if !hidden && isDotName(info.Name) {
				it.Notes = append(it.Notes, "the name starts with a dot, so the entry stays hidden")
			}
		}
		it.After.Win = unixWinAttr(info.Kind, it.After.Perm, isDotName(info.Name) || it.After.HiddenFlag)
	}

	// Owner and group.
	if p.uid >= 0 && p.uid != it.Before.UID {
		it.After.UID = p.uid
		it.After.Owner = ownerLabel(p.uid)
		it.setOwner = true
		it.Changes = append(it.Changes, fmt.Sprintf("owner %s → %s", it.Before.Owner, it.After.Owner))
	}
	if p.gid >= 0 && p.gid != it.Before.GID {
		it.After.GID = p.gid
		it.After.Group = groupName(p.gid)
		it.setOwner = true
		it.Changes = append(it.Changes, fmt.Sprintf("group %s → %s", it.Before.Group, it.After.Group))
	}

	// Dates.
	mod := r.Times.Modified
	if r.Times.Func != nil {
		if t, ok := r.Times.Func(info); ok {
			mod = SetTime(t)
		}
	}
	if t, ok := p.resolveTime(mod, it.Before, it.Before.Modified); ok && !t.Equal(it.Before.Modified) {
		it.After.Modified, it.setMtime = t, true
		it.Changes = append(it.Changes, "modified "+fmtTime(it.Before.Modified)+" → "+fmtTime(t))
	}
	if t, ok := p.resolveTime(r.Times.Accessed, it.Before, it.Before.Accessed); ok && !t.Equal(it.Before.Accessed) {
		it.After.Accessed, it.setAtime = t, true
		it.Changes = append(it.Changes, "accessed "+fmtTime(it.Before.Accessed)+" → "+fmtTime(t))
	}
	if t, ok := p.resolveTime(r.Times.Created, it.Before, it.Before.Created); ok && !t.Equal(it.Before.Created) {
		it.After.Created, it.setCtime = t, true
		it.Changes = append(it.Changes, "created "+fmtTime(it.Before.Created)+" → "+fmtTime(t))
	}
	return it
}

// resolveTime turns a TimeValue into a concrete time for one entry.
func (p *Plan) resolveTime(v TimeValue, before State, cur time.Time) (time.Time, bool) {
	switch v.Mode {
	case TimeSet:
		return v.At, true
	case TimeNow:
		return p.Now, true
	case TimeShift:
		if cur.IsZero() {
			return time.Time{}, false
		}
		return cur.Add(v.By), true
	case TimeCopy:
		var src time.Time
		switch v.From {
		case FieldModified:
			src = before.Modified
		case FieldAccessed:
			src = before.Accessed
		case FieldCreated:
			src = before.Created
		}
		if src.IsZero() {
			return time.Time{}, false
		}
		return src, true
	}
	return time.Time{}, false
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format(timeFormat)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Progress is called after each entry is processed during Apply.
type Progress func(done, total int, item *Item, err error)

// Failure records an entry that could not be changed.
type Failure struct {
	Path string
	Err  error
}

// Result summarises an Apply run.
type Result struct {
	Changed  int
	Failed   []Failure
	Canceled bool
}

// Apply makes the planned changes, deepest entries first. It keeps going
// after single failures and stops early only when ctx is canceled. progress
// may be nil.
func (p *Plan) Apply(ctx context.Context, progress Progress) Result {
	var res Result
	var todo []*Item
	for i := range p.Items {
		if p.Items[i].Changed() {
			todo = append(todo, &p.Items[i])
		}
	}
	// Deepest first, so a folder is always handled after everything below
	// it; within one depth the walk order is kept.
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].Depth > todo[j].Depth })
	follow := p.Request.FollowLinks
	for n, it := range todo {
		if ctx.Err() != nil {
			res.Canceled = true
			break
		}
		err := applyItem(it, follow)
		if err != nil {
			res.Failed = append(res.Failed, Failure{Path: it.Path, Err: err})
		} else {
			res.Changed++
		}
		if progress != nil {
			progress(n+1, len(todo), it, err)
		}
	}
	return res
}

// applyItem performs one entry's changes in a safe order: owner first
// (chown may clear setuid/setgid, so permissions must come after), then
// permissions, then dates and attributes. On Windows a read-only file cannot
// get new dates, so when read-only is being cleared the attributes go first
// and when it is being set they go last.
func applyItem(it *Item, follow bool) error {
	path := it.Path
	var errs []error
	if it.setOwner {
		uid, gid := -1, -1
		if it.After.UID != it.Before.UID {
			uid = it.After.UID
		}
		if it.After.GID != it.Before.GID {
			gid = it.After.GID
		}
		if err := chownPath(path, uid, gid, follow); err != nil {
			errs = append(errs, err)
		}
	}
	if it.setPerm {
		if err := chmodPath(path, it.After.Perm); err != nil {
			errs = append(errs, err)
		}
	}
	attrsFirst := it.setWin && it.Before.Win&AttrReadOnly != 0 && it.After.Win&AttrReadOnly == 0
	if attrsFirst {
		if err := setWinAttrs(path, it.After.Win, follow); err != nil {
			errs = append(errs, err)
		}
	}
	if it.setMtime || it.setAtime || it.setCtime {
		var m, a, c *time.Time
		if it.setMtime {
			m = &it.After.Modified
		}
		if it.setAtime {
			a = &it.After.Accessed
		}
		if it.setCtime {
			c = &it.After.Created
		}
		if err := setTimes(path, m, a, c, follow); err != nil {
			errs = append(errs, err)
		}
	}
	if it.setWin && !attrsFirst {
		if err := setWinAttrs(path, it.After.Win, follow); err != nil {
			errs = append(errs, err)
		}
	}
	if it.setHidden {
		if err := setHiddenFlag(path, it.After.HiddenFlag, follow); err != nil {
			errs = append(errs, &os.PathError{Op: "set hidden flag", Path: path, Err: err})
		}
	}
	return errors.Join(errs...)
}
