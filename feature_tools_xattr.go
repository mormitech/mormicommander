package main

// Extended attributes (board #29, F-NEW-13): the names the Properties dialog lists, "Remove quarantine flag"
// (cm_RemoveQuarantine) that clears com.apple.quarantine from the selection and everything inside the selected
// folders, and the check behind the one-time note when a copy goes onto a file system that drops extended attributes
// (FAT, exFAT). The work is done by the UI-free internal/xattr; nothing here changes how a copy runs.

import (
	"context"
	"errors"
	"path/filepath"

	"mormicommander/internal/dirsize"
	"mormicommander/internal/xattr"
)

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
	v, err := toolsVolumeOfTarget(target)
	if err != nil {
		return out, nil
	}
	out.FileSystem = v.FileSystem
	if !xattr.DropsXattrs(v.FileSystem) {
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
