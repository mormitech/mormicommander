// Recursive operations that follow links (TC's copy with "follow links", find
// files with "follow symlinks", Ctrl+L space calculation) can loop forever
// when a link points at one of its own parent folders, and can count the same
// data twice when two links point at the same folder. TC guards against both
// (F-722); this file gives every walker in the app the same guard. The
// LoopDetector remembers the identity (device + inode, or volume + file index
// on Windows) of each folder on the current descent path, so entering a
// folder that is already an ancestor is reported as a loop no matter which
// name was used to reach it. Optionally it also remembers every folder seen,
// so duplicates reached through a second link are skipped instead of being
// counted again. Walk is a ready-made recursive walker built on it for
// callers that do not have their own.

package links

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrAlreadyVisited is returned by LoopDetector.Enter (with duplicate
// skipping on) for a folder already walked through another path.
var ErrAlreadyVisited = errors.New("folder already visited through another path")

type walkKey struct {
	id   FileID
	path string // used when the platform has no FileID
}

// LoopDetector tracks folders during a recursive walk that follows links.
// It is not safe for concurrent use; give each walker its own.
type LoopDetector struct {
	ancestors map[walkKey]bool
	visited   map[walkKey]bool
}

// NewLoopDetector returns a detector. With skipDuplicates, Enter also refuses
// folders that were entered before through any other path.
func NewLoopDetector(skipDuplicates bool) *LoopDetector {
	d := &LoopDetector{ancestors: map[walkKey]bool{}}
	if skipDuplicates {
		d.visited = map[walkKey]bool{}
	}
	return d
}

func keyFor(path string) walkKey {
	if id, ok := IdentityOf(path); ok {
		return walkKey{id: id}
	}
	p := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		p = r
	}
	if a, err := filepath.Abs(p); err == nil {
		p = a
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return walkKey{path: p}
}

// Enter is called before descending into the folder at path. It returns a
// leave function to call when the folder is finished. When descending would
// loop, it returns an error wrapping ErrLoop (a *LoopError) and no leave
// function; with duplicate skipping, an already walked folder returns
// ErrAlreadyVisited.
func (d *LoopDetector) Enter(path string) (leave func(), err error) {
	k := keyFor(path)
	if d.ancestors[k] {
		return nil, &LoopError{Path: path}
	}
	if d.visited != nil {
		if d.visited[k] {
			return nil, fmt.Errorf("%s: %w", path, ErrAlreadyVisited)
		}
		d.visited[k] = true
	}
	d.ancestors[k] = true
	return func() { delete(d.ancestors, k) }, nil
}

// WalkOptions controls Walk.
type WalkOptions struct {
	// FollowLinks descends into folders reached through symlinks and
	// junctions, and reports the target's info for linked files.
	FollowLinks bool
	// SkipDuplicates skips folders already walked through another path, so
	// sizes are not counted twice.
	SkipDuplicates bool
}

// WalkFunc is called for every entry. info is the Lstat info of the entry,
// or the target's info when a link is followed. err is non-nil for entries
// that could not be read, for broken links (wrapping ErrBroken) when
// following links, and for loops (wrapping ErrLoop) or duplicates
// (ErrAlreadyVisited), which are reported and then not descended into.
// Returning filepath.SkipDir skips a folder; filepath.SkipAll stops the walk.
type WalkFunc func(path string, info fs.FileInfo, err error) error

// Walk walks the tree at root in lexical order, never looping. It stops when
// ctx is cancelled and returns ctx.Err().
func Walk(ctx context.Context, root string, opt WalkOptions, fn WalkFunc) error {
	var info fs.FileInfo
	var err error
	if opt.FollowLinks {
		info, err = os.Stat(root)
	} else {
		info, err = os.Lstat(root)
	}
	if err != nil {
		return ignoreSkip(fn(root, nil, err))
	}
	d := NewLoopDetector(opt.SkipDuplicates)
	return ignoreSkip(walk(ctx, root, info, opt, d, fn))
}

func ignoreSkip(err error) error {
	if errors.Is(err, filepath.SkipDir) || errors.Is(err, filepath.SkipAll) {
		return nil
	}
	return err
}

func walk(ctx context.Context, path string, info fs.FileInfo, opt WalkOptions, d *LoopDetector, fn WalkFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !info.IsDir() {
		return fn(path, info, nil)
	}
	leave, lerr := d.Enter(path)
	if lerr != nil {
		return fn(path, info, lerr)
	}
	defer leave()
	if err := fn(path, info, nil); err != nil {
		if errors.Is(err, filepath.SkipDir) {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if err := fn(path, info, err); err != nil && !errors.Is(err, filepath.SkipDir) {
			return err
		}
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := filepath.Join(path, e.Name())
		li, err := e.Info()
		if err != nil {
			if err := fn(p, nil, err); err != nil && !errors.Is(err, filepath.SkipDir) {
				return err
			}
			continue
		}
		child := li
		if isFSLink(p, li) {
			if !opt.FollowLinks {
				if err := fn(p, li, nil); err != nil && !errors.Is(err, filepath.SkipDir) {
					return err
				}
				continue
			}
			si, serr := os.Stat(p)
			if serr != nil {
				var cbErr error
				if isLoopErr(serr) || HasLoop(p) {
					cbErr = fn(p, li, &LoopError{Path: p})
				} else {
					cbErr = fn(p, li, fmt.Errorf("%s: %w: %w", p, ErrBroken, unwrapPath(serr)))
				}
				if cbErr != nil && !errors.Is(cbErr, filepath.SkipDir) {
					return cbErr
				}
				continue
			}
			child = si
		}
		if err := walk(ctx, p, child, opt, d, fn); err != nil {
			return err
		}
	}
	return nil
}

// sameName compares two real absolute paths the way the platform's default
// file system does (case-insensitively on Windows and macOS).
func sameName(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
