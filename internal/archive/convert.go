// This file moves entries from one archive straight into another without a
// detour through a temporary folder. Total Commander supports copying files
// from an archive panel into an archive in the other panel, and converting an
// archive to another format (unpack plus pack, F-628). Both come down to
// turning archive entries into Packer AddItems whose Open callback reads from
// the source archive, which is what ItemsFrom does; Convert wraps it for the
// whole-archive case.
package archive

import (
	"context"
	"fmt"
	"io"
	"io/fs"
)

// ItemsFrom turns entries of a (as returned by Selection) into AddItems for a
// Packer. Entry paths are taken relative to base (the folder the source panel
// shows) and placed below destDir inside the target archive. Folders become
// folder items so empty folders survive; symbolic links are packed as the
// small files holding their target, like other archivers do without link
// support in the target format.
func ItemsFrom(a Archive, entries []Entry, base, destDir string) []AddItem {
	base, _ = CleanPath(base)
	destDir, _ = CleanPath(destDir)
	var out []AddItem
	for _, e := range entries {
		if e.Path == base && e.IsDir {
			continue
		}
		rel := relTo(e.Path, base)
		dest := rel
		if destDir != "" {
			dest = destDir + "/" + rel
		}
		it := AddItem{Dest: dest, Size: e.Size, ModTime: e.ModTime, Mode: e.Mode &^ fs.ModeSymlink}
		if e.IsDir {
			it.Mode = fs.ModeDir | e.Mode.Perm()
			out = append(out, it)
			continue
		}
		entry := e
		it.Open = func() (io.ReadCloser, error) { return a.Open(entry) }
		out = append(out, it)
	}
	return out
}

// Convert repacks every entry of src into a new archive of format at
// dstPath, for example zip to tar.gz. Encrypted source entries ask for their
// password through the source's OpenOptions.Password callback.
func Convert(ctx context.Context, src Archive, format string, dstPath string, opts PackOptions) (PackResult, error) {
	f, ok := Lookup(format)
	if !ok || f.Packer == nil {
		return PackResult{}, fmt.Errorf("%w: %s", ErrReadOnly, format)
	}
	items := ItemsFrom(src, Selection(src, nil, ""), "", "")
	ch := Changes{Add: items}
	if c := src.Comment(); c != "" && f.Caps.Comments {
		ch.Comment = &c
	}
	return f.Packer.Update(ctx, dstPath, ch, opts)
}
