package main

// Properties (cm_Properties, Alt+Enter / Cmd+I, F-184, F-263, F-324, F-681, F-682, F-691) and occupied space (Space on
// a folder, Alt+Shift+Enter, Ctrl+L: F-091, F-118, F-183, F-244, F-264, F-325, F-736, F-737, F-752, F-797). Properties
// reads one entry with internal/attrs and internal/links; sizes of folders and selections come from internal/dirsize,
// which walks in parallel, counts hard links once and can be cancelled.

import (
	"errors"
	"path/filepath"
	"strings"

	"mormicommander/internal/attrs"
	"mormicommander/internal/dirsize"
	"mormicommander/internal/fileutil"
	"mormicommander/internal/links"
)

// ToolsProps is everything the Properties dialog shows about one entry.
type ToolsProps struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Folder       string   `json:"folder"`
	Kind         string   `json:"kind"`
	Type         string   `json:"type"`
	MIME         string   `json:"mime"`
	Executable   bool     `json:"executable"`
	IsDir        bool     `json:"isDir"`
	Size         int64    `json:"size"`
	SizeOnDisk   int64    `json:"sizeOnDisk"` // -1 when unknown
	Modified     int64    `json:"modified"`
	Accessed     int64    `json:"accessed"`
	Changed      int64    `json:"changed"`
	Created      int64    `json:"created"`
	Perm         string   `json:"perm"` // rwxr-xr-x ("" on Windows)
	Octal        string   `json:"octal"`
	Mode         string   `json:"mode"`
	Attrs        string   `json:"attrs"` // Windows attributes in words
	Owner        string   `json:"owner"`
	Group        string   `json:"group"`
	UID          int      `json:"uid"`
	GID          int      `json:"gid"`
	HardLinks    uint64   `json:"hardLinks"`
	Inode        uint64   `json:"inode"`
	Hidden       bool     `json:"hidden"`
	LinkKind     string   `json:"linkKind"` // "", "symlink", "junction", "hardlink", "url", "webloc", "desktop", "lnk", "alias"
	LinkTarget   string   `json:"linkTarget"`
	LinkResolved string   `json:"linkResolved"`
	LinkBroken   bool     `json:"linkBroken"`
	LinkLoop     bool     `json:"linkLoop"`
	LinkChain    []string `json:"linkChain"`
	URL          string   `json:"url"`
	Arguments    string   `json:"arguments"`
	WorkingDir   string   `json:"workingDir"`
	Volume       string   `json:"volume"`
	FileSystem   string   `json:"fileSystem"`
	BlockSize    int64    `json:"blockSize"`
}

// Properties describes one file or folder for the Properties dialog.
func (t *Tools) Properties(path string) (ToolsProps, error) {
	if err := t.toolsLocal(path); err != nil {
		return ToolsProps{}, err
	}
	in, err := attrs.Stat(path)
	if err != nil {
		return ToolsProps{}, toolsErr(err)
	}
	ti := attrs.DescribeType(in)
	p := ToolsProps{
		Name: in.Name, Path: path, Folder: filepath.Dir(path), Kind: in.Kind.String(), Type: ti.Description, MIME: ti.MIME,
		Executable: ti.Executable, IsDir: in.IsDir(), Size: in.Size, SizeOnDisk: in.SizeOnDisk,
		Modified: toolsMS(in.Modified), Owner: in.Owner, Group: in.Group, UID: in.UID, GID: in.GID,
		HardLinks: in.Links, Inode: in.Inode, Hidden: in.Hidden,
		LinkTarget: in.LinkTarget, LinkResolved: in.LinkResolved, LinkBroken: in.LinkBroken,
	}
	if in.HasAccessed {
		p.Accessed = toolsMS(in.Accessed)
	}
	if in.HasChanged {
		p.Changed = toolsMS(in.Changed)
	}
	if in.HasCreated {
		p.Created = toolsMS(in.Created)
	}
	c := attrs.Caps()
	if c.UnixPerms {
		p.Perm = attrs.FormatPerm(in.Perm())
		p.Octal = attrs.FormatOctal(in.Perm())
		p.Mode = attrs.FormatMode(in.Mode)
	}
	if in.WinNative {
		p.Attrs = strings.Join(in.Win.Names(), ", ")
	}
	if li, err := links.Inspect(path); err == nil && li.Kind != links.KindNone {
		p.LinkKind = li.Kind.String()
		if li.Target != "" {
			p.LinkTarget = li.Target
		}
		if li.Resolved != "" {
			p.LinkResolved = li.Resolved
		}
		p.LinkBroken = li.Broken
		p.LinkLoop = li.Loop
		if len(li.Chain) > 2 {
			p.LinkChain = li.Chain
		}
		p.URL = li.URL
		p.Arguments = li.Arguments
		p.WorkingDir = li.WorkingDir
		if li.HardLinks > 1 {
			p.HardLinks = li.HardLinks
		}
	}
	if v, err := dirsize.VolumeOf(path); err == nil {
		p.Volume = v.DisplayName()
		p.FileSystem = v.FileSystem
		p.BlockSize = int64(v.BlockSize)
	}
	return p, nil
}

// OpenSystemProperties opens the system's own window for a file: Finder's Get Info, the Explorer property sheet, or
// the file manager's properties on Linux.
func (t *Tools) OpenSystemProperties(path string) error {
	if err := t.toolsLocal(path); err != nil {
		return err
	}
	if err := attrs.OpenSystemProperties(path); err != nil {
		return errors.New("The system's properties window could not be opened: " + toolsErrText(err))
	}
	return nil
}

// Checksums calculates checksums of one file with several algorithms in one read (the Properties dialog).
func (t *Tools) Checksums(id, path string, algorithms []string) (map[string]string, error) {
	if err := t.toolsLocal(path); err != nil {
		return nil, err
	}
	algs, err := toolsAlgorithms(algorithms)
	if err != nil {
		return nil, err
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	progress := t.toolsProgress(id)
	d, err := fileutil.HashFile(ctx, path, algs, func(p fileutil.Progress) { progress(toolsFileProgress(p)) })
	if err != nil {
		return nil, toolsErr(err)
	}
	out := map[string]string{}
	for a, v := range d {
		out[a.String()] = v
	}
	return out, nil
}

// MatchChecksum compares a file with a pasted checksum; the algorithm is picked by the checksum's length.
func (t *Tools) MatchChecksum(id, path, digest string) (map[string]any, error) {
	if err := t.toolsLocal(path); err != nil {
		return nil, err
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	progress := t.toolsProgress(id)
	alg, ok, err := fileutil.MatchChecksum(ctx, path, strings.TrimSpace(digest), func(p fileutil.Progress) { progress(toolsFileProgress(p)) })
	if err != nil {
		return nil, toolsErr(err)
	}
	name := ""
	if alg.Valid() {
		name = alg.String()
	}
	return map[string]any{"algorithm": name, "ok": ok}, nil
}

// ToolsSpaceItem is the size of one path.
type ToolsSpaceItem struct {
	Path      string `json:"path"`
	IsDir     bool   `json:"isDir"`
	Bytes     int64  `json:"bytes"`
	Allocated int64  `json:"allocated"`
	Files     int64  `json:"files"`
	Dirs      int64  `json:"dirs"`
	Links     int64  `json:"links"`
	Errors    int64  `json:"errors"`
	Complete  bool   `json:"complete"`
	Error     string `json:"error"` // the path could not be read at all
}

// ToolsSpace is the answer to a size calculation.
type ToolsSpace struct {
	Items     []ToolsSpaceItem `json:"items"`
	Total     ToolsSpaceItem   `json:"total"`
	BlockSize int64            `json:"blockSize"` // cluster size of the volume (0 when unknown)
	Skipped   int64            `json:"skipped"`   // folders not entered (other volumes, link loops)
	Canceled  bool             `json:"canceled"`
}

func toolsSpaceItem(r dirsize.Result) ToolsSpaceItem {
	it := ToolsSpaceItem{Path: r.Path, IsDir: r.IsDir, Bytes: r.Bytes, Allocated: r.Allocated, Files: r.Files, Dirs: r.Dirs,
		Links: r.Symlinks, Errors: r.ErrorCount, Complete: r.Complete}
	if r.ErrorCount > 0 && r.Bytes == 0 && r.Files == 0 && r.Dirs == 0 && len(r.Errors) > 0 {
		it.Error = r.Errors[0].Err
	}
	return it
}

// Space calculates the size of files and folders (Space on a folder, Alt+Shift+Enter, Ctrl+L). While it runs it
// sends "tools-progress" events with the running totals, and one event with itemDone (the index) and the size as
// soon as each path is finished, so the size column fills in folder by folder.
func (t *Tools) Space(id string, paths []string) (ToolsSpace, error) {
	if err := t.toolsLocal(paths...); err != nil {
		return ToolsSpace{}, err
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	progress := t.toolsProgress(id)
	opts := dirsize.Options{
		Progress: func(p dirsize.Progress) {
			progress(map[string]any{"current": p.Current, "item": p.Item, "itemsDone": p.ItemsDone, "itemsTotal": p.ItemsTotal,
				"bytes": p.Bytes, "allocated": p.Allocated, "files": p.Files, "dirs": p.Dirs, "errors": p.Errors})
		},
		ItemDone: func(i int, r dirsize.Result) {
			t.app.emit("tools-progress", map[string]any{"id": id, "itemDone": i, "path": r.Path, "bytes": r.Bytes, "complete": r.Complete})
		},
	}
	sum, err := dirsize.CalculateAll(ctx, paths, opts)
	out := ToolsSpace{Total: toolsSpaceItem(sum.Total), Items: make([]ToolsSpaceItem, 0, len(sum.Items))}
	for _, r := range sum.Items {
		it := toolsSpaceItem(r)
		out.Items = append(out.Items, it)
		out.Skipped += r.SkippedMountCount
	}
	if err != nil {
		if errors.Is(err, dirsize.ErrCanceled) || ctx.Err() != nil {
			out.Canceled = true
			return out, nil
		}
		return out, toolsErr(err)
	}
	if len(paths) > 0 {
		if v, err := dirsize.VolumeOf(paths[0]); err == nil {
			out.BlockSize = int64(v.BlockSize)
		}
	}
	return out, nil
}
