package main

// Extended attributes (board #29, F-NEW-13): the names the Properties dialog lists, "Remove quarantine flag"
// (cm_RemoveQuarantine) that clears com.apple.quarantine from the selection and everything inside the selected
// folders, and the check behind the one-time note when a copy goes onto a file system that drops extended attributes
// (FAT, exFAT) or into a cloud-synced folder (iCloud Drive, ~/Library/CloudStorage), "Remove attribute…" in the
// Properties dialog (RemoveXattr: one attribute of one file, after the page has asked), and the custom column fields
// [=mc.xattrs], [=mc.xattrcount] and [=mc.quarantine] (opt-in: they are read only when a custom column set uses them,
// because reading attributes for every file costs time). The work is done by the UI-free internal/xattr; nothing here
// changes how a copy runs.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mormicommander/internal/dirsize"
	lst "mormicommander/internal/listing"
	"mormicommander/internal/xattr"
)

func init() {
	lst.RegisterField("mc", lst.Field{Name: "xattrs", Kind: lst.KindText, Help: "names of the extended attributes",
		Get: func(it *lst.Item, _ string, _ *lst.Format) lst.Value {
			return lst.TextValue(strings.Join(xattrNamesOf(it), ", "))
		}})
	lst.RegisterField("mc", lst.Field{Name: "xattrcount", Kind: lst.KindNumber, Help: "number of extended attributes",
		Get: func(it *lst.Item, _ string, _ *lst.Format) lst.Value {
			n := len(xattrNamesOf(it))
			if n == 0 {
				return lst.Value{Kind: lst.KindNumber, Empty: true}
			}
			return lst.NumValue(int64(n), strconv.Itoa(n))
		}})
	lst.RegisterField("mc", lst.Field{Name: "quarantine", Kind: lst.KindText, Help: "\"quarantined\" when macOS's downloaded-from-the-internet flag is set",
		Get: func(it *lst.Item, _ string, _ *lst.Format) lst.Value {
			if it.IsUp() || !tagsLocal(it.Dir) || !xattr.Has(filepath.Join(it.Dir, it.Name), xattr.Quarantine) {
				return lst.TextValue("")
			}
			return lst.TextValue("quarantined")
		}})
}

// xattrNamesOf lists the attribute names of a listed item on this computer's disks (never of servers or archives).
func xattrNamesOf(it *lst.Item) []string {
	if it.IsUp() || !tagsLocal(it.Dir) || !xattr.Supported() {
		return nil
	}
	l, err := xattr.List(filepath.Join(it.Dir, it.Name))
	if err != nil {
		return nil
	}
	names := make([]string, len(l))
	for i, a := range l {
		names[i] = a.Name
	}
	return names
}

// ToolsXattrResult is what "Remove quarantine flag" did.
type ToolsXattrResult struct {
	Checked  int            `json:"checked"` // files and folders looked at
	Removed  int            `json:"removed"` // of those, the ones that had the flag
	Failed   []ToolsFailure `json:"failed"`
	Canceled bool           `json:"canceled"`
}

// ToolsXattrLoss says whether copying some files to a folder loses extended attributes.
type ToolsXattrLoss struct {
	Loses      bool   `json:"loses"`      // the target drops them and a source has one worth keeping
	FileSystem string `json:"fileSystem"` // the target's file system ("msdos", "exfat" …)
	Example    string `json:"example"`    // a source path that has one
	Attribute  string `json:"attribute"`  // and that attribute's name
	Cloud      string `json:"cloud"`      // the target is in a cloud-synced folder: its name ("iCloud Drive", "Dropbox" …)
}

// toolsXattrs lists a path's extended attributes for the Properties dialog (nil when there are none or the system
// has none).
func toolsXattrs(path string) []xattr.Attr {
	l, err := xattr.List(path)
	if err != nil || len(l) == 0 {
		return nil
	}
	return l
}

// XattrSupported says whether this system has extended attributes the tools can read and remove.
func (t *Tools) XattrSupported() bool { return xattr.Supported() }

// RemoveQuarantine clears macOS's "downloaded from the internet" flag (com.apple.quarantine) from the paths and from
// everything inside the folders among them. Links are not followed. It sends "tools-progress" events while it runs.
func (t *Tools) RemoveQuarantine(id string, paths []string) (ToolsXattrResult, error) {
	if err := t.toolsLocal(paths...); err != nil {
		return ToolsXattrResult{}, err
	}
	if !xattr.Supported() {
		return ToolsXattrResult{}, errors.New("This system has no quarantine flag to remove.")
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	progress := t.toolsProgress(id)
	r, err := xattr.RemoveAll(ctx, paths, xattr.Quarantine, true, func(p string, r xattr.Result) {
		progress(map[string]any{"current": p, "text": filepath.Base(p), "done": r.Checked})
	})
	out := ToolsXattrResult{Checked: r.Checked, Removed: r.Removed, Failed: []ToolsFailure{}}
	for _, f := range r.Failed {
		out.Failed = append(out.Failed, ToolsFailure{Path: f.Path, Error: toolsErrText(f.Err)})
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			out.Canceled = true
			return out, nil
		}
		return out, toolsErr(err)
	}
	return out, nil
}

// XattrLossCheck tells whether copying sources into target loses extended attributes worth keeping: the target's
// file system keeps none (FAT, exFAT) and a source (or, in a folder, one of its first few thousand entries) has one.
// It only looks; it never changes or stops a copy.
func (t *Tools) XattrLossCheck(sources []string, target string) (ToolsXattrLoss, error) {
	var out ToolsXattrLoss
	if !xattr.Supported() || t.app.mock != nil || target == "" || len(sources) == 0 {
		return out, nil
	}
	home, _ := os.UserHomeDir()
	out.Cloud = cloudFolder(target, home)
	if v, err := toolsVolumeOfTarget(target); err == nil {
		out.FileSystem = v.FileSystem
	}
	if out.Cloud == "" && !xattr.DropsXattrs(out.FileSystem) {
		return out, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out.Example, out.Attribute = xattr.FindAny(ctx, sources, 2000)
	out.Loses = out.Example != ""
	return out, nil
}

// toolsVolumeOfTarget finds the volume of a copy target, which may not exist yet: it tries the folder and then each
// folder above it.
func toolsVolumeOfTarget(target string) (dirsize.VolumeInfo, error) {
	p := filepath.Clean(target)
	for {
		v, err := dirsize.VolumeOf(p)
		if err == nil {
			return v, nil
		}
		up := filepath.Dir(p)
		if up == p {
			return v, err
		}
		p = up
	}
}

// RemoveXattr removes one extended attribute from one file or folder (not from what is inside a folder, and never
// through a link). The page asks first and names the file and the attribute. It answers whether something was removed.
func (t *Tools) RemoveXattr(path, name string) (bool, error) {
	if err := t.toolsLocal(path); err != nil {
		return false, err
	}
	if !xattr.Supported() {
		return false, errors.New("This system has no extended attributes.")
	}
	if strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) {
		return false, errors.New("No attribute was named.")
	}
	ok, err := xattr.Remove(path, name)
	if err != nil {
		return false, toolsErr(err)
	}
	return ok, nil
}

// cloudFolder says whether a path is in a folder that a cloud service syncs, by where macOS puts them: iCloud Drive
// in ~/Library/Mobile Documents, the File Provider services (Dropbox, OneDrive, Google Drive, Box …) in
// ~/Library/CloudStorage/<Service>-<account>. The file system gives no reliable signal, so this goes by the path
// alone; the answer is the service's name, or "".
func cloudFolder(path, home string) string {
	if path == "" || home == "" {
		return ""
	}
	rel, err := filepath.Rel(filepath.Clean(home), filepath.Clean(path))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 || parts[0] != "Library" {
		return ""
	}
	switch parts[1] {
	case "Mobile Documents":
		return "iCloud Drive"
	case "CloudStorage":
		if len(parts) < 3 || parts[2] == "" {
			return ""
		}
		svc := parts[2]
		if i := strings.IndexByte(svc, '-'); i > 0 {
			svc = svc[:i]
		}
		if svc == "GoogleDrive" {
			svc = "Google Drive"
		}
		return svc
	}
	return ""
}
