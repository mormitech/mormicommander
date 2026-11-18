package main

// Real file drag-out of archives and servers (board issue #108, feature_system_dragout.go). Files inside an archive or
// on a server have no path Finder or Explorer could copy, so with the option "nativeDragOut" on, SystemDrag.StartFrom
// first unpacks or downloads the dragged files to a folder of their own (one per drag) below the temporary folder
// "mormicommander-dragout", and only then starts the native drag of those copies. The user keeps the mouse button down
// meanwhile ("Preparing…" on the page, with progress through "native-drag-prep" events): when the button is let go
// before the copies are ready, the copying stops, the folder is removed and no drag starts (the page says so).
//
// Only small drags are made this way: up to dragOutVfsLimit bytes and dragOutVfsMaxItems files and folders; larger
// ones are refused with a message (F5 copies them). The per-drag folders are removed an hour after their drag (the
// program that took the drop copies the files when the drop happens, but some read them a little later), and folders
// left over from an earlier run are removed at the next start once they are an hour old. Nothing but folders named
// "drag-…" directly inside "mormicommander-dragout" is ever removed. Nothing here goes online by itself: a server is
// only read because the user dragged its files.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/vfs"
)

const (
	dragOutVfsLimit    = 200 << 20 // bytes a drag out of an archive or server may copy
	dragOutVfsMaxItems = 5000      // files and folders it may copy
	dragTempKeep       = time.Hour // how long a per-drag folder is kept
	dragTempRootName   = "mormicommander-dragout"
	dragTempPrefix     = "drag-"
	dragPrepQuiet      = 300 * time.Millisecond // no progress is shown for copies quicker than this
	dragButtonPoll     = 100 * time.Millisecond
)

// VfsDragResult is the answer of StartFrom.
type VfsDragResult struct {
	// Status: "started" (the native drag of the copies is under way), "released" (the mouse button was let go before
	// the copies were ready: nothing is dragged), "none" (this system has no native drag)
	Status string   `json:"status"`
	Paths  []string `json:"paths,omitempty"` // the copies being dragged
}

// dragPrep is a "native-drag-prep" event: how far the copying of a drag out of an archive or server is.
type dragPrep struct {
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Name  string `json:"name"`
	End   bool   `json:"end,omitempty"` // the copying is over (the page hides its progress)
}

// errDragReleased: the mouse button was let go while the copies were made.
var errDragReleased = errors.New("the mouse button was let go")

// StartFrom begins a native drag of files inside an archive or on a server: they are copied to a temporary folder
// first, while the user holds the mouse button, then the copies are dragged. See the top of this file.
func (d *SystemDrag) StartFrom(paths []string) (VfsDragResult, error) {
	if d.app.mock != nil {
		return VfsDragResult{}, errors.New("This is the demo: its files are not real, so they can't be dragged to other programs.")
	}
	r := vfsRouter(d.app)
	if r == nil {
		return VfsDragResult{}, errors.New("Files inside archives or on servers can't be dragged out here.")
	}
	sources, err := dragVfsSources(r, paths)
	if err != nil {
		return VfsDragResult{}, err
	}
	if !nativeDragSupported {
		return VfsDragResult{Status: "none"}, nil
	}
	if !mouseButtonDown() {
		return VfsDragResult{Status: "released"}, nil
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go watchButton(ctx, cancel, mouseButtonDown)

	var mu sync.Mutex
	start, last := time.Now(), time.Time{}
	shown := false
	progress := func(p dragPrep) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if now.Sub(start) < dragPrepQuiet || now.Sub(last) < 150*time.Millisecond {
			return
		}
		shown, last = true, now
		d.app.emit("native-drag-prep", p)
	}
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		if shown {
			d.app.emit("native-drag-prep", dragPrep{End: true})
		}
	}()

	dir, copies, err := dragCopyOut(ctx, r, sources, dragTempRoot(), dragOutVfsLimit, dragOutVfsMaxItems, progress)
	if errors.Is(context.Cause(ctx), errDragReleased) {
		return VfsDragResult{Status: "released"}, nil
	}
	if err != nil {
		return VfsDragResult{}, err
	}
	cancel(nil)
	w := d.app.mainWindow()
	if w == nil {
		removeDragTemp(dragTempRoot(), dir)
		return VfsDragResult{Status: "released"}, nil
	}
	time.AfterFunc(dragTempKeep, func() { removeDragTemp(dragTempRoot(), dir) })
	setDragEnded(func(op int) { d.app.emit("native-drag-end", dragOpName(op)) })
	if !nativeFileDrag(w, copies) {
		setDragEnded(nil)
		removeDragTemp(dragTempRoot(), dir)
		return VfsDragResult{Status: "released"}, nil
	}
	return VfsDragResult{Status: "started", Paths: copies}, nil
}

// dragVfsSources checks the files of a drag out of an archive or server: at least one, each inside an archive or on
// a server (local files go through Start), none given twice.
func dragVfsSources(r *vfs.Router, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("No files to drag.")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, errors.New("No files to drag.")
		}
		switch r.Kind(p) {
		case vfs.KindArchive, vfs.KindFTP, vfs.KindSFTP, vfs.KindOther:
		default:
			return nil, errors.New("Only files inside an archive or on a server are copied out before a drag.")
		}
		c := r.Clean(p)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, nil
}

// watchButton cancels ctx (cause errDragReleased) as soon as the mouse button is no longer down.
func watchButton(ctx context.Context, cancel context.CancelCauseFunc, down func() bool) {
	t := time.NewTicker(dragButtonPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !down() {
				cancel(errDragReleased)
				return
			}
		}
	}
}

// dragItem is one file or folder to copy out for a drag.
type dragItem struct {
	src, dst string
	dir      bool
	size     int64
}

// dragTooBig is the sentence for a drag over the limits.
func dragTooBig(items, limit int64, maxItems int) error {
	if items > int64(maxItems) {
		return fmt.Errorf("Too many files to drag out of an archive or server as real files (more than %d). Copy them with F5 instead.", maxItems)
	}
	return fmt.Errorf("The files are too big to drag out of an archive or server as real files (the limit is %d MB). Copy them with F5 instead.",
		limit>>20)
}

// planDragOut lists what copying sources into dest means, file by file, and refuses when it is over the limits.
func planDragOut(ctx context.Context, r *vfs.Router, sources []string, dest string, limit int64, maxItems int) ([]dragItem, int64, error) {
	var items []dragItem
	var total int64
	taken := map[string]bool{}
	var walk func(src, dst string) error
	walk = func(src, dst string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		e, err := r.Stat(src)
		if err != nil {
			return errors.New(listError(src, err))
		}
		it := dragItem{src: src, dst: dst, dir: e.Flags&vfs.FlagDir != 0 && e.Flags&vfs.FlagLink == 0}
		if !it.dir {
			it.size = max(e.Size, 0)
		}
		total += it.size
		items = append(items, it)
		if total > limit || len(items) > maxItems {
			return dragTooBig(int64(len(items)), limit, maxItems)
		}
		if !it.dir {
			return nil
		}
		l, err := r.List(ctx, src)
		if err != nil {
			return errors.New(listError(src, err))
		}
		for _, c := range l.Entries {
			if err := walk(r.Join(src, c.Name), filepath.Join(dst, archive.SanitizeName(c.Name))); err != nil {
				return err
			}
		}
		return nil
	}
	for _, s := range sources {
		name := archive.SanitizeName(vfsBase(s))
		if name == "" || name == "." || name == ".." {
			name = "file"
		}
		// two sources of the same name (from different folders) get their own names
		name = vfsFreeName(name, func(n string) bool { return taken[strings.ToLower(n)] })
		taken[strings.ToLower(name)] = true
		if err := walk(s, filepath.Join(dest, name)); err != nil {
			return nil, total, err
		}
	}
	return items, total, nil
}

// dragCopyOut copies sources into a new per-drag folder below root and answers the folder and the copies of the
// sources (in their order). On any failure the folder is removed again.
func dragCopyOut(ctx context.Context, r *vfs.Router, sources []string, root string, limit int64, maxItems int, progress func(dragPrep)) (string, []string, error) {
	// the limits are checked before anything is copied (or a folder made)
	items, total, err := planDragOut(ctx, r, sources, "", limit, maxItems)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", nil, fmt.Errorf("Can't make a temporary folder: %v", err)
	}
	dir, err := os.MkdirTemp(root, dragTempPrefix)
	if err != nil {
		return "", nil, fmt.Errorf("Can't make a temporary folder: %v", err)
	}
	fail := func(err error) (string, []string, error) {
		removeDragTemp(root, dir)
		return "", nil, err
	}
	var done int64
	var copies []string
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		dst := filepath.Join(dir, it.dst)
		if filepath.Dir(it.dst) == "." {
			copies = append(copies, dst)
		}
		if progress != nil {
			progress(dragPrep{Done: done, Total: total, Name: vfsBase(it.src)})
		}
		if it.dir {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fail(fmt.Errorf("Can't copy %s: %v", vfsBase(it.src), err))
			}
			continue
		}
		base := done
		_, err := r.Download(ctx, it.src, dst, vfs.TransferOptions{PreserveTime: true, Progress: func(d, _ int64) {
			if progress != nil {
				progress(dragPrep{Done: base + d, Total: total, Name: vfsBase(it.src)})
			}
		}})
		if err != nil {
			if ctx.Err() != nil {
				return fail(ctx.Err())
			}
			return fail(fmt.Errorf("Can't copy %s: %s", vfsBase(it.src), vfsErrText(err)))
		}
		done += it.size
	}
	if progress != nil {
		progress(dragPrep{Done: total, Total: total})
	}
	return dir, copies, nil
}

// dragTempRoot is the folder the per-drag folders go into (below Options > Zip > Temporary folder when that is set).
func dragTempRoot() string {
	base := os.TempDir()
	if s, ok := currentSettings(); ok {
		if t := strings.TrimSpace(s.Zip.TempDir); t != "" {
			if fi, err := os.Stat(t); err == nil && fi.IsDir() {
				base = t
			}
		}
	}
	return filepath.Join(base, dragTempRootName)
}

// removeDragTemp removes the per-drag folder dir, but only when it really is one: a folder (not a link) named
// "drag-…" directly inside root.
func removeDragTemp(root, dir string) bool {
	if root == "" || dir == "" || filepath.Base(filepath.Clean(root)) != dragTempRootName {
		return false
	}
	if filepath.Dir(filepath.Clean(dir)) != filepath.Clean(root) || !strings.HasPrefix(filepath.Base(dir), dragTempPrefix) {
		return false
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	return os.RemoveAll(dir) == nil
}

// cleanDragTemp removes the per-drag folders in root that are older than keep (left over from an earlier run, or
// from a drag of this one that is long over); it answers how many it removed.
func cleanDragTemp(root string, keep time.Duration, now time.Time) int {
	es, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range es {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), dragTempPrefix) {
			continue
		}
		fi, err := e.Info()
		if err != nil || now.Sub(fi.ModTime()) < keep {
			continue
		}
		if removeDragTemp(root, filepath.Join(root, e.Name())) {
			n++
		}
	}
	return n
}

var dragTempCleanOnce sync.Once

func init() {
	onStartup(func(a *App) {
		if a.mock != nil {
			return
		}
		dragTempCleanOnce.Do(func() { go cleanDragTemp(dragTempRoot(), dragTempKeep, time.Now()) })
	})
}
