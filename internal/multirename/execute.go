// This file performs the renames shown in the preview and can undo them.
// Three things make this harder than calling os.Rename in a loop. First,
// names can form chains or cycles (a to b, b to a), so when any target is the
// current name of another item in the same folder we move the whole group to
// unique temporary names first and then to the final names. Second, with
// "include subdirectories" a folder and the files inside it may both be
// renamed, so we work from the deepest level upwards: children are renamed
// while their folder still has its old name, and the folder goes last.
// Third, a failure halfway must not leave a half-renamed mess, so every
// completed step is remembered and reversed in the opposite order if a later
// step fails. Before each rename we check that the target does not exist,
// because os.Rename silently replaces files on Unix. A successful run returns
// an UndoLog that records the final location of every item; Undo replays it
// backwards with the same machinery, and the log can be saved as JSON so
// "Undo" (F-514) still works after the dialog was closed.

package multirename

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExecOptions tunes Execute and Undo.
type ExecOptions struct {
	FS FS // nil means OSFS
	// Progress, when set, is called after each item is renamed.
	Progress func(done, total int)
}

// UndoOp records one completed rename: the item now called NewName in the
// folder Dir was called OldName. Dir is the folder's path after the whole
// batch, so an item inside a renamed folder is recorded under the new
// folder path.
type UndoOp struct {
	Dir     string `json:"dir"`
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
	IsDir   bool   `json:"isDir"`
}

// UndoLog is the record of one rename batch.
type UndoLog struct {
	Version int       `json:"version"`
	Time    time.Time `json:"time"`
	Ops     []UndoOp  `json:"ops"`
	// FoldCase records whether names were compared without regard to case,
	// so Undo uses the same rule as the original run.
	FoldCase bool `json:"foldCase"`
}

// ExecError describes a failed batch. Done steps were rolled back; any step
// that could not be rolled back is listed in RollbackErrs, and those files
// keep their temporary or new names.
type ExecError struct {
	From, To     string
	Err          error
	RollbackErrs []error
}

func (e *ExecError) Error() string {
	msg := fmt.Sprintf("multirename: renaming %q to %q failed: %v", e.From, e.To, e.Err)
	if len(e.RollbackErrs) > 0 {
		msg += fmt.Sprintf(" (and %d step(s) could not be rolled back)", len(e.RollbackErrs))
	} else {
		msg += " (all completed steps were rolled back)"
	}
	return msg
}

func (e *ExecError) Unwrap() error { return e.Err }

// ErrTargetExists is wrapped in an ExecError when a target appeared on disk
// after the preview was made.
var ErrTargetExists = errors.New("target already exists")

// op is a rename of one name to another inside the folder dir.
type op struct {
	dir      string
	from, to string
	isDir    bool
}

// step is one raw rename that was carried out, kept for rollback.
type step struct{ from, to string }

// Execute renames all changed entries of a preview. It refuses to run while
// the preview has errors.
func Execute(res *Result, o ExecOptions) (*UndoLog, error) {
	if res.HasErrors() {
		return nil, ErrHasConflicts
	}
	var ops []op
	for _, e := range res.Entries {
		if e.Changed() {
			ops = append(ops, op{dir: absDir(e.Item.Dir), from: e.Item.Name, to: e.NewName, isDir: e.Item.IsDir})
		}
	}
	fsys := o.FS
	if fsys == nil {
		fsys = OSFS{}
	}
	if err := runOps(ops, fsys, res.fold, o.Progress); err != nil {
		return nil, err
	}
	log := buildUndoLog(ops)
	log.FoldCase = res.fold
	return log, nil
}

// Undo reverses a batch recorded in log.
func Undo(log *UndoLog, o ExecOptions) error {
	if log == nil {
		return errors.New("multirename: nothing to undo")
	}
	ops := make([]op, len(log.Ops))
	for i, u := range log.Ops {
		ops[i] = op{dir: u.Dir, from: u.NewName, to: u.OldName, isDir: u.IsDir}
	}
	fsys := o.FS
	if fsys == nil {
		fsys = OSFS{}
	}
	return runOps(ops, fsys, log.FoldCase, o.Progress)
}

func absDir(d string) string {
	if a, err := filepath.Abs(d); err == nil {
		return a
	}
	return filepath.Clean(d)
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

// runOps executes ops level by level, deepest first, rolling everything back
// on the first failure.
func runOps(ops []op, fsys FS, fold bool, progress func(int, int)) error {
	sorted := append([]op(nil), ops...)
	sort.SliceStable(sorted, func(i, j int) bool { return depth(sorted[i].dir) > depth(sorted[j].dir) })
	key := func(dir, name string) string {
		if fold {
			name = strings.ToLower(name)
		}
		return filepath.Clean(dir) + "\x00" + name
	}
	var done []step
	fail := func(from, to string, err error) error {
		return &ExecError{From: from, To: to, Err: err, RollbackErrs: rollback(fsys, done)}
	}
	doRename := func(from, to string) error {
		if err := checkTarget(fsys, from, to); err != nil {
			return err
		}
		if err := fsys.Rename(from, to); err != nil {
			return err
		}
		done = append(done, step{from, to})
		return nil
	}
	finished := 0
	for start := 0; start < len(sorted); {
		end := start
		for end < len(sorted) && depth(sorted[end].dir) == depth(sorted[start].dir) {
			end++
		}
		level := sorted[start:end]
		sources := map[string]bool{}
		for _, p := range level {
			sources[key(p.dir, p.from)] = true
		}
		needTemp := false
		for _, p := range level {
			if k := key(p.dir, p.to); k != key(p.dir, p.from) && sources[k] {
				needTemp = true
				break
			}
		}
		if !needTemp {
			for _, p := range level {
				from, to := filepath.Join(p.dir, p.from), filepath.Join(p.dir, p.to)
				if err := doRename(from, to); err != nil {
					return fail(from, to, err)
				}
				finished++
				if progress != nil {
					progress(finished, len(sorted))
				}
			}
		} else {
			temps := make([]string, len(level))
			for i, p := range level {
				from := filepath.Join(p.dir, p.from)
				tmp, err := tempName(fsys, p.dir, i)
				if err != nil {
					return fail(from, filepath.Join(p.dir, p.to), err)
				}
				if err := doRename(from, tmp); err != nil {
					return fail(from, tmp, err)
				}
				temps[i] = tmp
			}
			for i, p := range level {
				to := filepath.Join(p.dir, p.to)
				if err := doRename(temps[i], to); err != nil {
					return fail(temps[i], to, err)
				}
				finished++
				if progress != nil {
					progress(finished, len(sorted))
				}
			}
		}
		start = end
	}
	return nil
}

// checkTarget fails when to exists and is not the same file as from (which
// happens for a case-only rename on a case-insensitive disk).
func checkTarget(fsys FS, from, to string) error {
	fi, err := fsys.Lstat(to)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if src, err := fsys.Lstat(from); err == nil && os.SameFile(fi, src) {
		return nil
	}
	return ErrTargetExists
}

// tempName finds an unused temporary name in dir.
func tempName(fsys FS, dir string, i int) (string, error) {
	for try := 0; try < 1000; try++ {
		p := filepath.Join(dir, fmt.Sprintf(".mrt~%d~%d~%d", os.Getpid(), i, try))
		if _, err := fsys.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
	}
	return "", errors.New("no free temporary name")
}

// rollback reverses done steps in the opposite order and returns the errors
// of the steps that could not be reversed.
func rollback(fsys FS, done []step) []error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if err := fsys.Rename(done[i].to, done[i].from); err != nil {
			errs = append(errs, fmt.Errorf("restore %q to %q: %w", done[i].to, done[i].from, err))
		}
	}
	return errs
}

// buildUndoLog records each op with its folder's final path. Folder renames
// are applied deepest first: a deeper rename only changes a path component
// below a shallower folder, so the shallower folder's old path still matches
// as a prefix afterwards.
func buildUndoLog(ops []op) *UndoLog {
	type dirRename struct{ old, new string }
	var dirs []dirRename
	for _, p := range ops {
		if p.isDir {
			dirs = append(dirs, dirRename{filepath.Join(p.dir, p.from), filepath.Join(p.dir, p.to)})
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return depth(dirs[i].old) > depth(dirs[j].old) })
	final := func(d string) string {
		for _, r := range dirs {
			if d == r.old {
				d = r.new
			} else if strings.HasPrefix(d, r.old+string(filepath.Separator)) {
				d = r.new + d[len(r.old):]
			}
		}
		return d
	}
	log := &UndoLog{Version: 1, Time: time.Now()}
	for _, p := range ops {
		log.Ops = append(log.Ops, UndoOp{Dir: final(p.dir), OldName: p.from, NewName: p.to, IsDir: p.isDir})
	}
	return log
}

// SaveUndoLog writes the log as JSON, replacing the file atomically.
func SaveUndoLog(path string, log *UndoLog) error {
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// LoadUndoLog reads a log written by SaveUndoLog.
func LoadUndoLog(path string) (*UndoLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var log UndoLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("multirename: bad undo log %s: %w", path, err)
	}
	if log.Version != 1 {
		return nil, fmt.Errorf("multirename: unsupported undo log version %d", log.Version)
	}
	return &log, nil
}

// writeFileAtomic writes data to a temporary file next to path and renames
// it into place, so a crash never leaves a truncated file.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
