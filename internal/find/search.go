// This file is the heart of the Find Files engine (section 24 of FEATURES.md). Compile
// checks an Options value once and turns it into a Query; Query.Run walks the start
// folders and hands every hit to a callback as soon as it is found, so the results
// list fills while the search is still running (F-538), and stops promptly when the
// context is cancelled (the Stop button). Errors on single folders or files, such as
// "permission denied", are reported through a separate callback and never end the
// search, because one unreadable folder must not hide everything else.
//
// The walk lists a folder, judges each entry against the cheap filters first (kind,
// name, size, date, attributes) and reads file contents last, then descends into the
// subfolders in name order. Folder exclusions are checked before entering, the depth
// limit counts levels below each start folder, and when links are followed each
// folder is compared with its ancestors so a link loop cannot run forever.

package find

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrStop can be returned by a result callback to end the search early without an
// error (for example "find the first match").
var ErrStop = errors.New("find: stop")

// Result is one hit.
type Result struct {
	Path    string      `json:"path"`              // full path (for archive members: archive path + inner path)
	Name    string      `json:"name"`              // base name
	Root    string      `json:"root,omitempty"`    // the start folder it was found under
	Rel     string      `json:"rel"`               // path relative to Root, with forward slashes
	Size    int64       `json:"size"`              // bytes (files) or 0 (folders)
	ModTime time.Time   `json:"modTime"`           // last modification
	Mode    fs.FileMode `json:"mode"`              // type and permission bits
	IsDir   bool        `json:"isDir,omitempty"`   // a folder
	Attr    Attr        `json:"attr,omitempty"`    // normalised attributes
	Archive string      `json:"archive,omitempty"` // archive file this member is in
	Inner   string      `json:"inner,omitempty"`   // member path inside Archive (forward slashes)
	// Duplicate search only: the group number (1, 2, …; members of one group are
	// duplicates of each other) and, when hashing was used, the hex digest.
	DupGroup int    `json:"dupGroup,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// Progress is a snapshot of a running search, for the status line.
type Progress struct {
	Dir     string `json:"dir"`     // folder being searched
	Dirs    int64  `json:"dirs"`    // folders listed so far
	Files   int64  `json:"files"`   // entries examined so far
	Matches int64  `json:"matches"` // results so far
	Errors  int64  `json:"errors"`  // errors reported so far
}

// Stats is the summary returned by Run.
type Stats struct {
	Progress
	Elapsed time.Duration `json:"elapsed"`
}

// Handler receives what a search produces. Only Result is required. Callbacks are
// called from the searching goroutine, one at a time.
type Handler struct {
	Result   func(Result) error         // a hit; return ErrStop or another error to end the search
	Error    func(path string, e error) // a folder or file that could not be read
	Progress func(Progress)             // called whenever a new folder is entered
}

// Query is a compiled search, safe to Run more than once (not concurrently).
type Query struct {
	opts     Options
	fs       FS
	mask     *Mask
	nameRe   *regexp.Regexp
	exclDirs []Pattern
	content  *contentMatcher
	date     dateMatcher
	dup      *dupConfig
}

// Compile validates options and prepares a Query.
func Compile(o Options) (*Query, error) {
	q := &Query{opts: o, fs: o.FS}
	if q.fs == nil {
		q.fs = OSFS{}
	}
	var err error
	if o.NameRegex {
		if strings.TrimSpace(o.SearchFor) != "" {
			if q.nameRe, err = compileNameRegex(o.SearchFor, o.NameRegexCase); err != nil {
				return nil, err
			}
		}
	} else if q.mask, err = ParseMask(o.SearchFor); err != nil {
		return nil, err
	}
	for _, d := range o.ExcludeDirs {
		if d = strings.TrimRight(strings.TrimSpace(d), `/\`); d != "" {
			q.exclDirs = append(q.exclDirs, newPattern(d, false))
		}
	}
	if o.Content.Active() {
		if q.content, err = compileContent(o.Content); err != nil {
			return nil, err
		}
	}
	if o.Size.Active() {
		if _, err := o.Size.Unit.factor(); err != nil {
			return nil, err
		}
		switch o.Size.Op {
		case SizeEqual, SizeGreater, SizeLess:
		default:
			return nil, fmt.Errorf("find: unknown size comparison %q", o.Size.Op)
		}
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	if q.date, err = compileDate(o.Date, now()); err != nil {
		return nil, err
	}
	if o.Duplicates.Enabled {
		if q.dup, err = compileDup(o.Duplicates); err != nil {
			return nil, err
		}
	}
	if len(o.SearchIn) == 0 && len(o.Within) == 0 {
		return nil, errors.New("find: no folder to search in")
	}
	return q, nil
}

// Options returns the options the query was compiled from.
func (q *Query) Options() Options { return q.opts }

// entry is what the filters look at: an on-disk file or an archive member.
type entry struct {
	path, name, root, rel string
	size                  int64
	mod                   time.Time
	mode                  fs.FileMode
	isDir                 bool
	attr                  Attr
	archive, inner        string
	open                  func() (io.ReadCloser, error)
}

func (e *entry) result() Result {
	r := Result{Path: e.path, Name: e.name, Root: e.root, Rel: e.rel, Size: e.size, ModTime: e.mod,
		Mode: e.mode, IsDir: e.isDir, Attr: e.attr, Archive: e.archive, Inner: e.inner}
	if r.IsDir {
		r.Size = 0
	}
	return r
}

// run holds the state of one Run call.
type run struct {
	q        *Query
	ctx      context.Context
	h        Handler
	prog     Progress
	dupFound []*entry
}

// Run performs the search. It returns when the walk is finished, the context is
// cancelled (returning ctx.Err()), or a Result callback returned an error (returned
// as is, except ErrStop which ends the search without error).
func (q *Query) Run(ctx context.Context, h Handler) (Stats, error) {
	start := time.Now()
	if h.Result == nil {
		h.Result = func(Result) error { return nil }
	}
	r := &run{q: q, ctx: ctx, h: h}
	err := r.walkAll()
	if err == nil && q.dup != nil {
		err = r.emitDuplicates()
	}
	if errors.Is(err, ErrStop) {
		err = nil
	}
	return Stats{Progress: r.prog, Elapsed: time.Since(start)}, err
}

// Search is a convenience that collects all results in a slice.
func Search(ctx context.Context, o Options) ([]Result, Stats, error) {
	q, err := Compile(o)
	if err != nil {
		return nil, Stats{}, err
	}
	var out []Result
	st, err := q.Run(ctx, Handler{Result: func(r Result) error { out = append(out, r); return nil }})
	return out, st, err
}

func (r *run) fail(path string, err error) {
	r.prog.Errors++
	if r.h.Error != nil {
		r.h.Error(path, err)
	}
}

func (r *run) walkAll() error {
	q := r.q
	if len(q.opts.Within) > 0 {
		for _, p := range q.opts.Within {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			info, isLink, err := r.stat(p)
			if err != nil {
				r.fail(p, err)
				continue
			}
			e := r.makeEntry(p, q.fs.Base(p), "", q.fs.Base(p), info, isLink)
			if err := r.consider(e); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range q.opts.SearchIn {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		info, err := q.fs.Stat(root)
		if err != nil {
			r.fail(root, err)
			continue
		}
		if !info.IsDir() {
			// A file given as start path is tested by itself.
			lin, isLink, err := r.stat(root)
			if err != nil {
				r.fail(root, err)
				continue
			}
			e := r.makeEntry(root, q.fs.Base(root), root, q.fs.Base(root), lin, isLink)
			if err := r.consider(e); err != nil {
				return err
			}
			if err := r.maybeArchive(e, lin); err != nil {
				return err
			}
			continue
		}
		if err := r.walkDir(root, root, "", 0, []fs.FileInfo{info}); err != nil {
			return err
		}
	}
	return nil
}

// stat describes a path given directly (Within, file roots), following the link if
// FollowLinks is set.
func (r *run) stat(p string) (fs.FileInfo, bool, error) {
	info, err := r.q.fs.Lstat(p)
	if err != nil {
		return nil, false, err
	}
	isLink := info.Mode()&fs.ModeSymlink != 0
	if isLink && r.q.opts.FollowLinks {
		if t, err := r.q.fs.Stat(p); err == nil {
			info = t
		}
	}
	return info, isLink, nil
}

func (r *run) makeEntry(p, name, root, rel string, info fs.FileInfo, isLink bool) *entry {
	e := &entry{path: p, name: name, root: root, rel: rel, size: info.Size(), mod: info.ModTime(),
		mode: info.Mode(), isDir: info.IsDir(), attr: attrsOf(r.q.fs, p, info)}
	if isLink {
		e.attr |= AttrSymlink
	}
	f := r.q.fs
	e.open = func() (io.ReadCloser, error) { return f.Open(p) }
	return e
}

func (r *run) progress(dir string) {
	r.prog.Dir = dir
	r.prog.Dirs++
	if r.h.Progress != nil {
		r.h.Progress(r.prog)
	}
}

// subdir is a folder queued for descent after its parent's entries are processed.
type subdir struct {
	path, rel string
	info      fs.FileInfo
}

func (r *run) walkDir(dir, root, rel string, depth int, ancestors []fs.FileInfo) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	q := r.q
	r.progress(dir)
	list, err := q.fs.ReadDir(dir)
	if err != nil {
		r.fail(dir, err)
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	var subs []subdir
	descend := q.opts.Levels < 0 || depth+1 <= q.opts.Levels
	for _, de := range list {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		name := de.Name()
		full := q.fs.Join(dir, name)
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		info, err := de.Info()
		if err != nil {
			r.fail(full, err)
			continue
		}
		isLink := info.Mode()&fs.ModeSymlink != 0
		if isLink && q.opts.FollowLinks {
			if t, err := q.fs.Stat(full); err == nil {
				info = t
			}
		}
		e := r.makeEntry(full, name, root, childRel, info, isLink)
		if e.isDir && r.skipDir(name, childRel) {
			continue
		}
		if q.opts.SkipHidden && e.attr&AttrHidden != 0 {
			continue
		}
		if err := r.consider(e); err != nil {
			return err
		}
		if e.isDir {
			if descend && (!isLink || q.opts.FollowLinks) {
				subs = append(subs, subdir{full, childRel, info})
			}
			continue
		}
		if err := r.maybeArchive(e, info); err != nil {
			return err
		}
	}
	for _, s := range subs {
		if q.opts.FollowLinks && loops(s.info, ancestors) {
			continue
		}
		if err := r.walkDir(s.path, root, s.rel, depth+1, append(ancestors, s.info)); err != nil {
			return err
		}
	}
	return nil
}

// loops reports whether a folder is one of its own ancestors (reached through a link).
func loops(info fs.FileInfo, ancestors []fs.FileInfo) bool {
	for _, a := range ancestors {
		if os.SameFile(a, info) {
			return true
		}
	}
	return false
}

// skipDir applies the folder exclusions from the mask and from ExcludeDirs.
func (r *run) skipDir(name, rel string) bool {
	if r.q.mask.SkipDir(name, rel) {
		return true
	}
	for _, p := range r.q.exclDirs {
		if p.Match(name, rel) {
			return true
		}
	}
	return false
}

// consider runs the filters on one entry and emits it if it matches.
func (r *run) consider(e *entry) error {
	r.prog.Files++
	ok, err := r.q.match(r.ctx, e)
	if err != nil {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		r.fail(e.path, err)
		return nil
	}
	if !ok {
		return nil
	}
	if r.q.dup != nil {
		if !e.isDir {
			r.dupFound = append(r.dupFound, e)
		}
		return nil
	}
	r.prog.Matches++
	return r.h.Result(e.result())
}

// match is the filter chain, cheapest first.
func (q *Query) match(ctx context.Context, e *entry) (bool, error) {
	o := &q.opts
	switch o.Kind {
	case KindFiles:
		if e.isDir {
			return false, nil
		}
	case KindDirs:
		if !e.isDir {
			return false, nil
		}
	}
	if q.nameRe != nil {
		if !q.nameRe.MatchString(e.name) {
			return false, nil
		}
	} else if !q.mask.MatchName(e.name, e.rel) {
		return false, nil
	}
	if o.Size.Active() && (e.isDir || !o.Size.Match(e.size)) {
		return false, nil
	}
	if !q.date.match(e.mod) {
		return false, nil
	}
	if e.attr&o.AttrSet != o.AttrSet || e.attr&o.AttrClear != 0 {
		return false, nil
	}
	if o.PermMask != 0 && e.mode.Perm()&o.PermMask != o.PermValue&o.PermMask {
		return false, nil
	}
	if q.content != nil {
		if e.isDir {
			return false, nil
		}
		rc, err := e.open()
		if err != nil {
			return false, err
		}
		found, err := q.content.matchReader(ctx, rc)
		rc.Close()
		if err != nil {
			return false, err
		}
		if found == o.Content.NotContaining {
			return false, nil
		}
	}
	return true, nil
}

// Match tests one path against every filter except the folder walk itself, for
// using a saved search as a selection filter ("select group", custom filters).
func (q *Query) Match(ctx context.Context, p string) (bool, error) {
	info, err := q.fs.Lstat(p)
	if err != nil {
		return false, err
	}
	isLink := info.Mode()&fs.ModeSymlink != 0
	if isLink && q.opts.FollowLinks {
		if t, err := q.fs.Stat(p); err == nil {
			info = t
		}
	}
	r := &run{q: q, ctx: ctx}
	return q.match(ctx, r.makeEntry(p, q.fs.Base(p), "", q.fs.Base(p), info, isLink))
}

// maybeArchive searches inside an archive file if archive search is on.
func (r *run) maybeArchive(e *entry, info fs.FileInfo) error {
	q := r.q
	if !q.opts.SearchArchives || q.opts.Archives == nil || e.isDir || !q.opts.Archives.IsArchive(e.path, info) {
		return nil
	}
	r.progress(e.path)
	err := q.opts.Archives.Walk(r.ctx, q.fs, e.path, func(ae ArchiveEntry, open func() (io.ReadCloser, error)) error {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		inner := cleanInner(ae.Name)
		if inner == "" {
			return nil
		}
		parts := strings.Split(inner, "/")
		// Folder exclusions apply to every folder on the member's path.
		for i := range parts {
			last := i == len(parts)-1
			if last && !ae.IsDir {
				break
			}
			if r.skipDir(parts[i], e.rel+"/"+strings.Join(parts[:i+1], "/")) {
				return nil
			}
		}
		me := &entry{
			path: q.fs.Join(append([]string{e.path}, parts...)...), name: parts[len(parts)-1],
			root: e.root, rel: e.rel + "/" + inner, size: ae.Size, mod: ae.ModTime, isDir: ae.IsDir,
			mode: ae.Mode, archive: e.path, inner: inner, open: open,
		}
		if ae.IsDir {
			me.mode |= fs.ModeDir
			me.attr |= AttrDirectory
		}
		if me.mode.Perm() != 0 && me.mode.Perm()&0o222 == 0 {
			me.attr |= AttrReadOnly
		}
		if q.opts.SkipHidden && strings.HasPrefix(me.name, ".") {
			return nil
		}
		if err := r.consider(me); err != nil {
			return &callbackErr{err}
		}
		return nil
	})
	if err != nil {
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		var stop *callbackErr
		if errors.As(err, &stop) {
			return stop.err
		}
		r.fail(e.path, err)
	}
	return nil
}

// callbackErr marks an error that came from the Result callback, so it is not
// mistaken for an archive read error when it bubbles up through Walk.
type callbackErr struct{ err error }

func (c *callbackErr) Error() string { return c.err.Error() }
func (c *callbackErr) Unwrap() error { return c.err }
