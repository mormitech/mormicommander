package main

// Change attributes (cm_SetAttrib, F-315, F-316, F-338, F-683..F-685): what the dialog shows for a selection and the
// change itself, over internal/attrs. The dialog keeps Total Commander's layout and swaps the attribute group by
// platform: the Windows attributes on Windows, the rwx grid with the special bits, the macOS hidden flag and owner and
// group on macOS and Linux. Checkboxes are three-state; the page sends only the boxes the user changed, so pressing OK
// without touching a box never rewrites it (attrs.ChangedOnly, done on the page side).

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mormicommander/internal/attrs"
	"mormicommander/internal/find"
	lst "mormicommander/internal/listing"
)

// ToolsBit is one three-state checkbox of the attributes dialog.
type ToolsBit struct {
	Bit      uint32 `json:"bit"`      // Unix permission bit (0o400…, 0o4000 setuid) or Windows attribute bit
	Name     string `json:"name"`     // label
	State    string `json:"state"`    // "on", "off" or "mixed"
	Settable bool   `json:"settable"` // false for attributes that are only shown (compressed, encrypted)
}

// ToolsAttrState is what the attributes dialog starts from.
type ToolsAttrState struct {
	Platform      string     `json:"platform"`
	UnixPerms     bool       `json:"unixPerms"`
	OwnerChange   bool       `json:"ownerChange"`
	CreatedTime   bool       `json:"createdTime"`
	HiddenFlag    bool       `json:"hiddenFlag"`
	NativeAttrs   bool       `json:"nativeAttrs"`
	Count         int        `json:"count"`
	Files         int        `json:"files"`
	Folders       int        `json:"folders"`
	Links         int        `json:"links"`
	Perm          []ToolsBit `json:"perm"`
	Octal         string     `json:"octal"` // "" when some bit differs between the selected entries
	Attrs         []ToolsBit `json:"attrs"`
	Hidden        string     `json:"hidden"` // the macOS hidden flag
	Owner         string     `json:"owner"`
	Group         string     `json:"group"`
	OwnerMixed    bool       `json:"ownerMixed"`
	GroupMixed    bool       `json:"groupMixed"`
	Modified      int64      `json:"modified"`
	Accessed      int64      `json:"accessed"`
	Created       int64      `json:"created"`
	ModifiedMixed bool       `json:"modifiedMixed"`
	AccessedMixed bool       `json:"accessedMixed"`
	CreatedMixed  bool       `json:"createdMixed"`
	IsRoot        bool       `json:"isRoot"`
}

// ToolsTime is one date of the change: Mode "" leaves it, "set" sets it to At (Unix ms), "now" sets the moment of the
// change (the same instant for every file, TC's "Current"), "shift" moves it by By milliseconds.
type ToolsTime struct {
	Mode string `json:"mode"`
	At   int64  `json:"at"`
	By   int64  `json:"by"`
}

// ToolsAttrRequest is the change the dialog asks for.
type ToolsAttrRequest struct {
	Paths     []string  `json:"paths"`
	Recursive bool      `json:"recursive"`
	ApplyTo   string    `json:"applyTo"`  // "", "files" or "folders"
	Mask      string    `json:"mask"`     // TC mask for recursive changes ("*.sh | .git\"); "" = all
	Template  string    `json:"template"` // a saved search (Find Files > Load/Save) that also has to match; "" = none (F-685)
	PermSet   uint32    `json:"permSet"`  // Unix bits
	PermClear uint32    `json:"permClear"`
	AttrSet   uint32    `json:"attrSet"` // Windows attribute bits (the macOS hidden flag is AttrHidden)
	AttrClear uint32    `json:"attrClear"`
	Owner     string    `json:"owner"`
	Group     string    `json:"group"`
	Modified  ToolsTime `json:"modified"`
	Accessed  ToolsTime `json:"accessed"`
	Created   ToolsTime `json:"created"`
	// ModifiedFrom takes each file's modification date from its contents (F-340): "exif" the EXIF date of a photo,
	// "name" a date written in the file name. Files without one keep the Modified setting; "" is off.
	ModifiedFrom string `json:"modifiedFrom"`
}

// ToolsFailure is one file a tool could not handle.
type ToolsFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// ToolsAttrResult is what a change did.
type ToolsAttrResult struct {
	Planned  int            `json:"planned"` // entries that needed a change
	Changed  int            `json:"changed"`
	Failed   []ToolsFailure `json:"failed"`
	Notes    []string       `json:"notes"`
	Canceled bool           `json:"canceled"`
}

var toolsAttrNames = map[attrs.WinAttr]string{
	attrs.AttrReadOnly: "Read only", attrs.AttrArchive: "Archive", attrs.AttrHidden: "Hidden", attrs.AttrSystem: "System",
	attrs.AttrCompressed: "Compressed", attrs.AttrEncrypted: "Encrypted",
}

var toolsPermNames = []string{"Read", "Write", "Execute", "Read", "Write", "Execute", "Read", "Write", "Execute", "Set user ID", "Set group ID", "Sticky"}

// AttrInfo describes the selection for the attributes dialog: three-state boxes, common dates, owner and group.
func (t *Tools) AttrInfo(paths []string) (ToolsAttrState, error) {
	if err := t.toolsLocal(paths...); err != nil {
		return ToolsAttrState{}, err
	}
	infos := make([]attrs.Info, 0, len(paths))
	for _, p := range paths {
		in, err := attrs.Stat(p)
		if err != nil {
			return ToolsAttrState{}, toolsErr(err)
		}
		infos = append(infos, in)
	}
	c := attrs.Caps()
	s := attrs.Summarize(infos)
	st := ToolsAttrState{
		Platform: runtime.GOOS, UnixPerms: c.UnixPerms, OwnerChange: c.Owner, CreatedTime: c.CreatedTime, HiddenFlag: c.HiddenFlag,
		NativeAttrs: c.NativeAttrs, Count: s.Count, Files: s.Files, Folders: s.Folders, Links: s.Links,
		Hidden: s.HiddenFlag().String(), Owner: s.Owner, Group: s.Group, OwnerMixed: s.OwnerMixed, GroupMixed: s.GroupMixed,
		Modified: toolsMS(s.Modified), Accessed: toolsMS(s.Accessed), Created: toolsMS(s.Created),
		ModifiedMixed: s.ModifiedMixed, AccessedMixed: s.AccessedMixed, CreatedMixed: s.CreatedMixed,
		IsRoot: os.Geteuid() == 0,
	}
	if c.UnixPerms {
		for i, b := range attrs.PermBits {
			st.Perm = append(st.Perm, ToolsBit{Bit: attrs.ToUnix(b), Name: toolsPermNames[i], State: s.PermBit(b).String(), Settable: true})
		}
		if m, ok := s.CommonPerm(); ok {
			st.Octal = attrs.FormatOctal(m)
		}
	}
	// The Windows attribute group is shown on Windows only: on macOS and Linux "read only" is the write bits of the
	// grid and "hidden" is the macOS hidden flag (st.Hidden), so the same thing never has two boxes.
	for _, a := range attrs.DialogAttrs {
		if !c.NativeAttrs || (c.ReportedAttrs&a == 0 && c.SettableAttrs&a == 0) {
			continue
		}
		st.Attrs = append(st.Attrs, ToolsBit{Bit: uint32(a), Name: toolsAttrNames[a], State: s.Attr(a).String(), Settable: c.SettableAttrs&a != 0})
	}
	return st, nil
}

// AttrAccounts lists the users and groups for the owner and group boxes (empty where owners can't be changed).
func (t *Tools) AttrAccounts() map[string][]string {
	out := map[string][]string{"users": {}, "groups": {}}
	for _, a := range attrs.Users() {
		out["users"] = append(out["users"], a.Name)
	}
	for _, a := range attrs.Groups() {
		out["groups"] = append(out["groups"], a.Name)
	}
	return out
}

// toolsTimeValue turns the page's date choice into the engine's.
func toolsTimeValue(v ToolsTime) attrs.TimeValue {
	switch v.Mode {
	case "set":
		return attrs.SetTime(time.UnixMilli(v.At))
	case "now":
		return attrs.NowTime()
	case "shift":
		return attrs.ShiftTime(time.Duration(v.By) * time.Millisecond)
	}
	return attrs.TimeValue{}
}

// toolsDateFrom gives the per-file modification date for ModifiedFrom, or nil when it is off. Only regular files
// have one; folders and other entries keep the Modified setting.
func toolsDateFrom(from string) func(attrs.Info) (time.Time, bool) {
	var get func(attrs.Info) (time.Time, bool)
	switch from {
	case "exif":
		get = func(info attrs.Info) (time.Time, bool) {
			it := lst.Item{Name: filepath.Base(info.Path), Dir: filepath.Dir(info.Path), Size: info.Size, ModTime: info.Modified, Mode: info.Mode}
			lst.LoadImageFacts(&it)
			t, ok := it.Extra[lst.ExtraExifDate].(time.Time)
			return t, ok && !t.IsZero()
		}
	case "name":
		get = func(info attrs.Info) (time.Time, bool) { return attrs.DateFromName(filepath.Base(info.Path)) }
	default:
		return nil
	}
	return func(info attrs.Info) (time.Time, bool) {
		if info.Kind != attrs.KindFile {
			return time.Time{}, false
		}
		return get(info)
	}
}

// SetAttrs changes attributes, permissions, owner and dates of the selection (and, with Recursive, of everything in
// the selected folders). It goes on after single failures and reports them.
func (t *Tools) SetAttrs(id string, r ToolsAttrRequest) (ToolsAttrResult, error) {
	if err := t.toolsLocal(r.Paths...); err != nil {
		return ToolsAttrResult{}, err
	}
	req := attrs.Request{
		Paths:     r.Paths,
		Recursive: r.Recursive,
		Attrs:     attrs.WinAttrChange{Set: attrs.WinAttr(r.AttrSet), Clear: attrs.WinAttr(r.AttrClear)},
		Perm:      attrs.PermChange{Set: attrs.FromUnix(r.PermSet) & attrs.PermMask, Clear: attrs.FromUnix(r.PermClear) & attrs.PermMask},
		Owner:     attrs.OwnerChange{Owner: r.Owner, Group: r.Group},
		Times: attrs.TimeChange{
			Modified: toolsTimeValue(r.Modified), Accessed: toolsTimeValue(r.Accessed), Created: toolsTimeValue(r.Created),
		},
	}
	req.Times.Func = toolsDateFrom(r.ModifiedFrom)
	switch r.ApplyTo {
	case "files":
		req.ApplyTo = attrs.FilesOnly
	case "folders":
		req.ApplyTo = attrs.FoldersOnly
	}
	if r.Recursive && r.Mask != "" && r.Mask != "*" && r.Mask != "*.*" {
		req.Filter = attrs.ParseMask(r.Mask)
	}
	if r.Recursive && strings.TrimSpace(r.Template) != "" {
		q, err := toolsTemplateQuery(r.Template)
		if err != nil {
			return ToolsAttrResult{}, err
		}
		if req.Filter == nil {
			req.Filter = &attrs.Filter{}
		}
		req.Filter.Match = func(p string, _ attrs.Info) bool {
			ok, err := q.Match(context.Background(), p)
			return err == nil && ok
		}
	}
	if req.IsZero() {
		return ToolsAttrResult{}, errors.New("Nothing to change: no box was changed and no date was chosen.")
	}
	if err := attrs.Validate(req); err != nil {
		return ToolsAttrResult{}, toolsErr(err)
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	progress := t.toolsProgress(id)
	progress(map[string]any{"text": "Looking at the files…"})
	plan, err := attrs.MakePlan(ctx, req)
	if err != nil {
		return ToolsAttrResult{Canceled: ctx.Err() != nil}, toolsErr(err)
	}
	res := ToolsAttrResult{Planned: plan.ChangedCount(), Failed: []ToolsFailure{}, Notes: []string{}}
	for i := range plan.Items {
		it := &plan.Items[i]
		if it.Err != nil {
			res.Failed = append(res.Failed, ToolsFailure{Path: it.Path, Error: toolsErrText(it.Err)})
		}
		for _, n := range it.Notes {
			if len(res.Notes) < 20 {
				res.Notes = append(res.Notes, it.Path+": "+n)
			}
		}
	}
	ar := plan.Apply(ctx, func(done, total int, it *attrs.Item, err error) {
		cur := ""
		if it != nil {
			cur = it.Path
		}
		progress(map[string]any{"done": done, "total": total, "current": cur})
	})
	res.Changed = ar.Changed
	res.Canceled = ar.Canceled
	for _, f := range ar.Failed {
		res.Failed = append(res.Failed, ToolsFailure{Path: f.Path, Error: toolsAttrErrText(f.Err)})
	}
	return res, nil
}

// toolsAttrErrText explains a failed change; a permission error on a file of someone else is the usual case.
func toolsAttrErrText(err error) string {
	if errors.Is(err, fs.ErrPermission) {
		return "You don't have permission to change it (only its owner or an administrator can)."
	}
	return toolsErrText(err)
}

// toolsTemplateQuery compiles a saved search as a filter for a recursive attribute change (TC's "Load from search
// template", F-685). Only the per-file tests count (name, size, date, attributes, permissions, text); the template's
// own folders are not walked and a duplicate search is not a filter.
func toolsTemplateQuery(name string) (*find.Query, error) {
	dir := configDir()
	if dir == "" {
		return nil, errors.New("Saved searches need a settings folder, and there is none.")
	}
	st, err := find.OpenStore(filepath.Join(dir, "searches.json"))
	if err != nil {
		return nil, errors.New(findErrText(err))
	}
	return toolsTemplateFrom(st, name)
}

func toolsTemplateFrom(st *find.Store, name string) (*find.Query, error) {
	o, ok := st.Get(name)
	if !ok {
		return nil, fmt.Errorf("There is no saved search named %q.", name)
	}
	o.SearchIn, o.Within = []string{"."}, nil // (Compile wants a start folder; Match never walks it)
	o.Duplicates = find.DupOptions{}
	o.SearchArchives = false
	q, err := find.Compile(o)
	if err != nil {
		return nil, errors.New(findErrText(err))
	}
	return q, nil
}
