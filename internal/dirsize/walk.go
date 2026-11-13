// This file holds the folder walker behind every size calculation. Folders are read in parallel by a bounded number
// of goroutines: when a worker slot is free a subfolder is handed to a new goroutine, otherwise the current goroutine
// reads it itself, so deep trees never create unbounded goroutines and a busy pool never stalls the walk. Counters are
// atomic so the progress reporter can read them at any time without locks. Hard links, mount points and symbolic link
// loops are tracked in small maps guarded by one mutex; they only see files with more than one link and folders that
// need a check, so the lock is rarely contended. Entries that vanish while counting are ignored, other read errors are
// recorded and the walk goes on, because one unreadable folder should not hide the size of everything else.
package dirsize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// fileID identifies the data of a file independent of its name: device (or volume serial) plus inode (or file index).
type fileID struct{ dev, ino uint64 }

// entryStat is what the platform layer reports about one file.
type entryStat struct {
	size  int64  // logical size
	alloc int64  // size on disk
	id    fileID // valid when hasID
	links uint64 // number of hard links; 0 or 1 means "not shared"
	hasID bool
}

// linkInfo remembers the sizes of a multi-link file so totals over several paths can drop shared data again.
type linkInfo struct{ size, alloc int64 }

// counters are the running totals of one walk.
type counters struct {
	bytes, alloc, files, dirs, symlinks, others, dupes, dupeBytes, errors, skipped atomic.Int64
}

type walker struct {
	ctx  context.Context
	opts Options
	env  *statEnv
	sem  chan struct{}
	wg   sync.WaitGroup
	c    counters

	checkDev bool
	rootDev  uint64

	mu      sync.Mutex
	links   map[fileID]linkInfo
	visited map[fileID]bool
	errs    []PathError
	skipped []string

	current atomic.Pointer[string]
}

func newWalker(ctx context.Context, opts Options) *walker {
	return &walker{
		ctx:     ctx,
		opts:    opts,
		sem:     make(chan struct{}, opts.workers()),
		links:   map[fileID]linkInfo{},
		visited: map[fileID]bool{},
	}
}

// run calculates one path. The root is resolved with Stat, so a link to a folder counts that folder.
func (w *walker) run(path string) Result {
	start := time.Now()
	r := Result{Path: path}
	w.current.Store(&path)
	fi, err := os.Stat(path)
	if err != nil {
		w.fail(path, err, true)
		r = w.result(r)
		r.rootErr = err
		r.Complete = w.ctx.Err() == nil
		r.Duration = time.Since(start)
		return r
	}
	w.env = newStatEnv(path, w.opts.Quick)
	if !fi.IsDir() {
		if fi.Mode().IsRegular() {
			w.file(path, fi, true) // the root was found with Stat, so a link is followed to its target
		} else {
			w.c.others.Add(1)
		}
	} else {
		r.IsDir = true
		if w.opts.SameVolume {
			w.rootDev, w.checkDev = dirDev(path, fi, w.env)
		}
		if w.opts.FollowSymlinks {
			if id, ok := dirID(path, fi, w.env); ok {
				w.visited[id] = true
			}
		}
		w.dir(path)
		w.wg.Wait()
	}
	r = w.result(r)
	r.Complete = w.ctx.Err() == nil
	r.Duration = time.Since(start)
	return r
}

// result copies the counters and lists into r.
func (w *walker) result(r Result) Result {
	r.Bytes = w.c.bytes.Load()
	r.Allocated = w.c.alloc.Load()
	r.Files = w.c.files.Load()
	r.Dirs = w.c.dirs.Load()
	r.Symlinks = w.c.symlinks.Load()
	r.Others = w.c.others.Load()
	r.HardlinkDupes = w.c.dupes.Load()
	r.HardlinkBytes = w.c.dupeBytes.Load()
	r.ErrorCount = w.c.errors.Load()
	r.SkippedMountCount = w.c.skipped.Load()
	w.mu.Lock()
	r.Errors = append([]PathError(nil), w.errs...)
	r.SkippedMounts = append([]string(nil), w.skipped...)
	w.mu.Unlock()
	return r
}

// dir reads one folder, counts its entries and then walks its subfolders, in parallel where a worker slot is free.
func (w *walker) dir(path string) {
	if w.ctx.Err() != nil {
		return
	}
	w.current.Store(&path)
	subs, ok := w.readDir(path)
	if !ok {
		return
	}
	for _, s := range subs {
		if w.ctx.Err() != nil {
			return
		}
		select {
		case w.sem <- struct{}{}:
			w.wg.Add(1)
			go func(s string) {
				defer w.wg.Done()
				defer func() { <-w.sem }()
				w.dir(s)
			}(s)
		default:
			w.dir(s)
		}
	}
}

// readDir counts the entries of one folder in batches (so a cancel is noticed quickly even in a folder with a
// million files) and returns the subfolders to enter. The folder is closed before its subfolders are walked, so deep
// trees do not hold one open handle per level.
func (w *walker) readDir(path string) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		w.fail(path, err, false)
		return nil, false
	}
	defer f.Close()
	var subs []string
	for {
		ents, err := f.ReadDir(512)
		for _, e := range ents {
			if sub, ok := w.entry(path, e); ok {
				subs = append(subs, sub)
			}
		}
		if err != nil {
			if err != io.EOF {
				w.fail(path, err, false)
			}
			break
		}
		if w.ctx.Err() != nil {
			return nil, false
		}
	}
	return subs, true
}

// entry counts one folder entry and reports whether it is a folder to enter.
func (w *walker) entry(parent string, e fs.DirEntry) (string, bool) {
	p := filepath.Join(parent, e.Name())
	t := e.Type()
	switch {
	case t.IsDir():
		var fi fs.FileInfo
		if w.checkDev || w.opts.FollowSymlinks {
			info, err := e.Info()
			if err != nil {
				w.fail(p, err, false)
				return "", false
			}
			fi = info
		}
		return w.enterDir(p, fi, false)
	case t&fs.ModeSymlink != 0 || t&fs.ModeIrregular != 0:
		// Symbolic links, and on Windows junctions and volume mount points (ModeIrregular), are counted as links
		// unless the user asked to follow them.
		if !w.opts.FollowSymlinks {
			w.c.symlinks.Add(1)
			return "", false
		}
		fi, err := os.Stat(p)
		if err != nil { // broken link
			w.c.symlinks.Add(1)
			return "", false
		}
		switch {
		case fi.IsDir():
			return w.enterDir(p, fi, true)
		case fi.Mode().IsRegular():
			w.file(p, fi, true)
		default:
			w.c.others.Add(1)
		}
	case t.IsRegular():
		fi, err := e.Info()
		if err != nil {
			w.fail(p, err, false)
			return "", false
		}
		if !fi.Mode().IsRegular() { // changed type between ReadDir and Lstat
			w.c.others.Add(1)
			return "", false
		}
		w.file(p, fi, false)
	default:
		w.c.others.Add(1)
	}
	return "", false
}

// enterDir decides whether a folder is walked: it is skipped when it is on another volume (SameVolume) or when it was
// already counted through another link (FollowSymlinks).
func (w *walker) enterDir(p string, fi fs.FileInfo, viaLink bool) (string, bool) {
	if w.opts.FollowSymlinks && fi != nil {
		if id, ok := dirID(p, fi, w.env); ok {
			w.mu.Lock()
			seen := w.visited[id]
			w.visited[id] = true
			w.mu.Unlock()
			if seen {
				if viaLink {
					w.c.symlinks.Add(1)
					w.skip(p)
				} else {
					w.c.dirs.Add(1) // the real folder, already counted through a link earlier
				}
				return "", false
			}
		}
	}
	w.c.dirs.Add(1)
	if w.checkDev && fi != nil {
		if dev, ok := dirDev(p, fi, w.env); ok && dev != w.rootDev {
			w.skip(p)
			return "", false
		}
	}
	return p, true
}

// file adds one regular file, counting the data of a multi-link file only the first time it is seen. follow is set
// when p is a symbolic link being followed (fi then describes its target).
func (w *walker) file(p string, fi fs.FileInfo, follow bool) {
	dedupe := !w.opts.CountHardlinksTwice
	st := fileStat(p, fi, w.env, dedupe, dedupe && w.opts.FollowSymlinks, follow)
	w.c.files.Add(1)
	// With links followed, the same file can be reached by a link and by its real path although it has only one
	// name, so then every file identity is remembered, not only those of multi-link files.
	if dedupe && st.hasID && (st.links > 1 || w.opts.FollowSymlinks) {
		w.mu.Lock()
		_, seen := w.links[st.id]
		if !seen {
			w.links[st.id] = linkInfo{st.size, st.alloc}
		}
		w.mu.Unlock()
		if seen {
			w.c.dupes.Add(1)
			w.c.dupeBytes.Add(st.size)
			return
		}
	}
	w.c.bytes.Add(st.size)
	w.c.alloc.Add(st.alloc)
}

// fail records a read error. Entries that disappeared while counting are not errors (the root is: the user pointed
// at something that is gone), and nothing is recorded after a cancel, because the errors would only be its echo.
func (w *walker) fail(p string, err error, root bool) {
	if w.ctx.Err() != nil || (!root && errors.Is(err, fs.ErrNotExist)) {
		return
	}
	w.c.errors.Add(1)
	msg := err.Error()
	var pe *fs.PathError
	if errors.As(err, &pe) {
		msg = pe.Err.Error()
	}
	w.mu.Lock()
	if len(w.errs) < maxListed {
		w.errs = append(w.errs, PathError{Path: p, Err: msg})
	}
	w.mu.Unlock()
}

func (w *walker) skip(p string) {
	w.c.skipped.Add(1)
	w.mu.Lock()
	if len(w.skipped) < maxListed {
		w.skipped = append(w.skipped, p)
	}
	w.mu.Unlock()
}

// Calculate counts one file or folder. The error is non-nil when the path cannot be read at all or when ctx is
// cancelled (it then wraps ErrCanceled and the partial result is returned).
func Calculate(ctx context.Context, path string, opts Options) (Result, error) {
	s, err := CalculateAll(ctx, []string{path}, opts)
	r := s.Items[0]
	if err == nil && r.rootErr != nil {
		err = r.rootErr
	}
	return r, err
}

// CalculateAll counts several files and folders one after another (each with parallel reads inside), which is what
// Alt+Shift+Enter (every folder of the panel) and Ctrl+L (the selection) need. Options.ItemDone reports each path as
// soon as it is finished. Paths that cannot be read get an error in their Result; the returned error is only set on
// cancel and wraps ErrCanceled, with the paths not reached left at zero and Complete false.
func CalculateAll(ctx context.Context, paths []string, opts Options) (Summary, error) {
	start := time.Now()
	s := Summary{Items: make([]Result, len(paths)), Total: Result{IsDir: true, Complete: true}}
	for i, p := range paths {
		s.Items[i].Path = p
	}

	pr := &progressState{total: len(paths), start: start}
	stop := make(chan struct{})
	var tick sync.WaitGroup
	if opts.Progress != nil {
		tick.Add(1)
		go func() {
			defer tick.Done()
			t := time.NewTicker(opts.interval())
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					opts.Progress(pr.snapshot(false))
				}
			}
		}()
	}

	global := map[fileID]linkInfo{}
	total := &s.Total
	for i, p := range paths {
		if ctx.Err() != nil {
			break
		}
		w := newWalker(ctx, opts)
		pr.begin(i, p, w)
		r := w.run(p)
		s.Items[i] = r
		mergeTotal(total, r, w.links, global)
		pr.finish(r)
		if opts.ItemDone != nil {
			opts.ItemDone(i, r)
		}
	}
	close(stop)
	tick.Wait()
	total.Duration = time.Since(start)
	if ctx.Err() != nil {
		total.Complete = false
	}
	if opts.Progress != nil {
		opts.Progress(pr.snapshot(true))
	}
	if err := ctx.Err(); err != nil {
		return s, fmt.Errorf("%w (%w)", ErrCanceled, err)
	}
	return s, nil
}

// mergeTotal adds one item to the total and removes data that a hard link in an earlier item already counted.
func mergeTotal(t *Result, r Result, links, global map[fileID]linkInfo) {
	t.Bytes += r.Bytes
	t.Allocated += r.Allocated
	t.Files += r.Files
	t.Dirs += r.Dirs
	if r.IsDir {
		t.Dirs++
	}
	t.Symlinks += r.Symlinks
	t.Others += r.Others
	t.HardlinkDupes += r.HardlinkDupes
	t.HardlinkBytes += r.HardlinkBytes
	t.ErrorCount += r.ErrorCount
	t.SkippedMountCount += r.SkippedMountCount
	for _, e := range r.Errors {
		if len(t.Errors) < maxListed {
			t.Errors = append(t.Errors, e)
		}
	}
	for _, m := range r.SkippedMounts {
		if len(t.SkippedMounts) < maxListed {
			t.SkippedMounts = append(t.SkippedMounts, m)
		}
	}
	if !r.Complete {
		t.Complete = false
	}
	for id, li := range links {
		if _, seen := global[id]; seen {
			t.Bytes -= li.size
			t.Allocated -= li.alloc
			t.HardlinkDupes++
			t.HardlinkBytes += li.size
			continue
		}
		global[id] = li
	}
}

// progressState combines the totals of finished paths with the live counters of the path being walked.
type progressState struct {
	mu          sync.Mutex
	total       int
	start       time.Time
	item        int
	itemPath    string
	done        int
	active      *walker
	bytes       int64
	alloc       int64
	files       int64
	dirs        int64
	errors      int64
	lastCurrent string
}

func (p *progressState) begin(i int, path string, w *walker) {
	p.mu.Lock()
	p.item, p.itemPath, p.active = i, path, w
	p.mu.Unlock()
}

func (p *progressState) finish(r Result) {
	p.mu.Lock()
	p.active = nil
	p.done++
	p.bytes += r.Bytes
	p.alloc += r.Allocated
	p.files += r.Files
	p.dirs += r.Dirs
	if r.IsDir {
		p.dirs++
	}
	p.errors += r.ErrorCount
	p.mu.Unlock()
}

func (p *progressState) snapshot(done bool) Progress {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := Progress{
		Item: p.item, ItemPath: p.itemPath, ItemsDone: p.done, ItemsTotal: p.total,
		Bytes: p.bytes, Allocated: p.alloc, Files: p.files, Dirs: p.dirs, Errors: p.errors,
		Elapsed: time.Since(p.start), Done: done, Current: p.lastCurrent,
	}
	if w := p.active; w != nil {
		s.Bytes += w.c.bytes.Load()
		s.Allocated += w.c.alloc.Load()
		s.Files += w.c.files.Load()
		s.Dirs += w.c.dirs.Load()
		s.Errors += w.c.errors.Load()
		if c := w.current.Load(); c != nil {
			s.Current = *c
			p.lastCurrent = *c
		}
	}
	return s
}
