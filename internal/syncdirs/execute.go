// This file is the synchronize half of the tool (FEATURES.md F-551). A Plan
// turns the rows' actions into an ordered list of disk operations, filtered
// by the three checkboxes of Total Commander's Synchronize dialog (copy left
// to right, copy right to left, delete). Execute then carries the plan out
// with progress reports, cancellation, an overwrite confirmation hook and a
// Skip / Retry / Abort error hook.
//
// The order is chosen so the tree is always consistent: folders are created
// before the files that go into them, files are deleted before the folders
// that hold them, and copied folders get their dates last, after their
// contents stopped changing them. Copies are written to a temporary file in
// the target folder and renamed over the target only when complete, so a
// cancelled or failed copy never leaves a truncated file. Before touching
// anything the engine checks that both sides still look the way they did at
// compare time; if a file changed in between, that operation is skipped with
// ErrChanged instead of overwriting or deleting something the user never
// saw. Folders are only ever removed when empty, links are copied as links
// and deleted as links, and nothing is written through a symlinked folder.
package syncdirs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
)

// ErrChanged means a file changed on disk after the compare, so the
// operation was skipped. Compare again to see the new state.
var ErrChanged = errors.New("syncdirs: file changed since the compare")

// ErrAborted is returned by Execute when the error hook chose Abort.
var ErrAborted = errors.New("syncdirs: synchronization aborted")

// errSkipped marks an operation the user declined to overwrite.
var errSkipped = errors.New("syncdirs: skipped")

// OpKind is the kind of one disk operation.
type OpKind int

const (
	// OpMkdir creates a folder on the target side.
	OpMkdir OpKind = iota
	// OpCopyFile copies a file to the target side.
	OpCopyFile
	// OpCopyLink recreates a symbolic link on the target side.
	OpCopyLink
	// OpDelete deletes a file or link on the target side.
	OpDelete
	// OpRemoveDir removes an empty folder on the target side.
	OpRemoveDir
)

var opNames = [...]string{"mkdir", "copyFile", "copyLink", "delete", "removeDir"}

func (k OpKind) String() string {
	if k >= 0 && int(k) < len(opNames) {
		return opNames[k]
	}
	return fmt.Sprintf("OpKind(%d)", int(k))
}

// MarshalText writes the kind as its name, for JSON.
func (k OpKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// Op is one disk operation of a plan.
type Op struct {
	Kind OpKind `json:"kind"`
	// Side is the side that is changed (the copy target or the side
	// deleted from).
	Side Side `json:"side"`
	// Src is the path on the other side that is copied (copy ops only).
	Src string `json:"src,omitempty"`
	// Dst is the path that is created, replaced or deleted on Side.
	Dst  string `json:"dst"`
	Size int64  `json:"size"`
	Item *Item  `json:"-"`
}

// PlanFilter holds the checkboxes of the Synchronize dialog.
type PlanFilter struct {
	LeftToRight bool `json:"leftToRight"`
	RightToLeft bool `json:"rightToLeft"`
	Delete      bool `json:"delete"`
}

// AllOps has every checkbox on.
func AllOps() PlanFilter { return PlanFilter{true, true, true} }

// Plan is the ordered list of operations plus the totals the dialog shows.
type Plan struct {
	Ops             []Op  `json:"ops"`
	FilesLR         int   `json:"filesLeftToRight"`
	FilesRL         int   `json:"filesRightToLeft"`
	BytesLR         int64 `json:"bytesLeftToRight"`
	BytesRL         int64 `json:"bytesRightToLeft"`
	DeletesLeft     int   `json:"deletesLeft"`
	DeletesRight    int   `json:"deletesRight"`
	NeedsWriteLeft  bool  `json:"needsWriteLeft"`
	NeedsWriteRight bool  `json:"needsWriteRight"`
	result          *Result
}

// Plan builds the operations for the current actions.
func (r *Result) Plan(f PlanFilter) *Plan {
	p := &Plan{result: r}
	var copies, deletes []Op
	for _, it := range r.Items {
		switch it.Action {
		case CopyLeftToRight:
			if f.LeftToRight {
				copies = append(copies, copyOp(it, RightSide))
			}
		case CopyRightToLeft:
			if f.RightToLeft {
				copies = append(copies, copyOp(it, LeftSide))
			}
		case DeleteLeft, DeleteRight, DeleteBoth:
			if !f.Delete {
				continue
			}
			if it.Action != DeleteRight {
				deletes = append(deletes, deleteOp(it, LeftSide))
			}
			if it.Action != DeleteLeft {
				deletes = append(deletes, deleteOp(it, RightSide))
			}
		}
	}
	// Rows are in tree order (parents first), so copies already create
	// folders before their contents; deletes run in reverse so contents go
	// before their folders.
	for i, j := 0, len(deletes)-1; i < j; i, j = i+1, j-1 {
		deletes[i], deletes[j] = deletes[j], deletes[i]
	}
	p.Ops = append(copies, deletes...)
	for _, op := range p.Ops {
		if op.Side == LeftSide {
			p.NeedsWriteLeft = true
		} else {
			p.NeedsWriteRight = true
		}
		switch op.Kind {
		case OpCopyFile, OpCopyLink:
			if op.Side == RightSide {
				p.FilesLR++
				p.BytesLR += op.Size
			} else {
				p.FilesRL++
				p.BytesRL += op.Size
			}
		case OpDelete, OpRemoveDir:
			if op.Side == LeftSide {
				p.DeletesLeft++
			} else {
				p.DeletesRight++
			}
		}
	}
	return p
}

func copyOp(it *Item, to Side) Op {
	src, dst, e := it.LeftPath, it.RightPath, it.Left
	if to == LeftSide {
		src, dst, e = it.RightPath, it.LeftPath, it.Right
	}
	op := Op{Side: to, Src: src, Dst: dst, Item: it}
	switch e.Kind {
	case KindDir:
		op.Kind = OpMkdir
	case KindLink:
		op.Kind = OpCopyLink
	default:
		op.Kind = OpCopyFile
		op.Size = e.Size
	}
	return op
}

func deleteOp(it *Item, side Side) Op {
	p, e := it.LeftPath, it.Left
	if side == RightSide {
		p, e = it.RightPath, it.Right
	}
	op := Op{Kind: OpDelete, Side: side, Dst: p, Item: it}
	if e.Kind == KindDir {
		op.Kind = OpRemoveDir
	}
	return op
}

// ErrorDecision is the answer of the error hook.
type ErrorDecision int

const (
	// Skip records the error and continues with the next operation.
	Skip ErrorDecision = iota
	// Retry runs the failed operation again.
	Retry
	// Abort stops the synchronization; Execute returns ErrAborted.
	Abort
)

// OverwriteDecision is the answer of the overwrite confirmation hook.
type OverwriteDecision int

const (
	// Overwrite replaces this one file.
	Overwrite OverwriteDecision = iota
	// SkipFile keeps this one file.
	SkipFile
	// OverwriteAll replaces this and every later file without asking.
	OverwriteAll
	// SkipAll keeps this and every later file without asking.
	SkipAll
	// CancelSync stops the synchronization; Execute returns
	// context.Canceled.
	CancelSync
)

// Progress is reported during Execute.
type Progress struct {
	Op         *Op   `json:"op"`
	OpsDone    int   `json:"opsDone"`
	OpsTotal   int   `json:"opsTotal"`
	BytesDone  int64 `json:"bytesDone"`
	BytesTotal int64 `json:"bytesTotal"`
	FileDone   int64 `json:"fileDone"` // bytes of the current file
}

// ExecOptions are the hooks of Execute; all may be nil.
type ExecOptions struct {
	// Progress is called before each operation and while copying.
	Progress func(Progress)
	// OnError decides what happens after a failed operation (default Skip).
	OnError func(op *Op, err error) ErrorDecision
	// ConfirmOverwrite is asked before an existing file or link is
	// replaced (default Overwrite).
	ConfirmOverwrite func(op *Op) OverwriteDecision
	// Force skips the "unchanged since compare" checks.
	Force bool
}

// OpError is one failed operation.
type OpError struct {
	Op  Op    `json:"op"`
	Err error `json:"-"`
}

func (e OpError) Error() string {
	return fmt.Sprintf("%s %s %s: %v", e.Op.Kind, e.Op.Side, e.Op.Dst, e.Err)
}

// MarshalText lets the UI see the message.
func (e OpError) MarshalText() ([]byte, error) { return []byte(e.Error()), nil }

func (e OpError) Unwrap() error { return e.Err }

// Report is the outcome of Execute.
type Report struct {
	Done    int       `json:"done"`
	Skipped int       `json:"skipped"`
	Bytes   int64     `json:"bytes"`
	Errors  []OpError `json:"errors,omitempty"`
}

type executor struct {
	ctx       context.Context
	left      FS
	right     FS
	opts      ExecOptions
	prog      Progress
	overwrite OverwriteDecision // sticky OverwriteAll / SkipAll, else Overwrite
	sticky    bool
	dirTimes  []Op
}

// Execute runs the plan on the two sides, which must be the same file
// systems that were compared. It returns the report and, when the run
// stopped early, ctx.Err(), context.Canceled (overwrite dialog cancelled)
// or ErrAborted. ErrReadOnly is returned before anything is done when the
// plan needs to change a side that is not a WritableFS.
func Execute(ctx context.Context, p *Plan, left, right FS, opts ExecOptions) (*Report, error) {
	if p.NeedsWriteLeft {
		if _, ok := left.(WritableFS); !ok {
			return &Report{}, PathError{Side: LeftSide, Path: ".", Err: ErrReadOnly}
		}
	}
	if p.NeedsWriteRight {
		if _, ok := right.(WritableFS); !ok {
			return &Report{}, PathError{Side: RightSide, Path: ".", Err: ErrReadOnly}
		}
	}
	e := &executor{ctx: ctx, left: left, right: right, opts: opts}
	e.prog.OpsTotal = len(p.Ops)
	e.prog.BytesTotal = p.BytesLR + p.BytesRL
	rep := &Report{}
	var base int64 // bytes of the copy operations already finished
	for i := range p.Ops {
		op := &p.Ops[i]
		if err := ctx.Err(); err != nil {
			e.finishDirTimes()
			return rep, err
		}
		e.prog.Op = op
		e.report()
		for {
			// A retried or failed copy may have counted some bytes; start
			// each attempt from the finished total so the numbers stay
			// honest.
			e.prog.BytesDone = base
			e.prog.FileDone = 0
			err := e.run(op)
			if err == nil {
				rep.Done++
				if op.Kind == OpCopyFile {
					rep.Bytes += op.Size
				}
				break
			}
			if errors.Is(err, errSkipped) {
				rep.Skipped++
				break
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				e.finishDirTimes()
				return rep, err
			}
			dec := Skip
			if opts.OnError != nil {
				dec = opts.OnError(op, err)
			}
			if dec == Retry {
				continue
			}
			rep.Errors = append(rep.Errors, OpError{Op: *op, Err: err})
			if dec == Abort {
				e.finishDirTimes()
				return rep, ErrAborted
			}
			rep.Skipped++
			break
		}
		e.prog.OpsDone++
		if op.Kind == OpCopyFile {
			base += op.Size
		}
		e.prog.BytesDone = base
	}
	e.prog.Op = nil
	e.report()
	e.finishDirTimes()
	return rep, nil
}

func (e *executor) report() {
	if e.opts.Progress != nil {
		e.opts.Progress(e.prog)
	}
}

func (e *executor) sides(target Side) (src FS, dst WritableFS) {
	if target == RightSide {
		dst, _ = e.right.(WritableFS)
		return e.left, dst
	}
	dst, _ = e.left.(WritableFS)
	return e.right, dst
}

func entries(it *Item, target Side) (src, dst *Entry) {
	if target == RightSide {
		return it.Left, it.Right
	}
	return it.Right, it.Left
}

// sameAsCompared checks that p on f still looks like e did at compare time.
// A nil e means the path must not exist.
func sameAsCompared(f FS, p string, e *Entry) error {
	var fi fs.FileInfo
	var err error
	if e != nil && e.Followed {
		fi, err = f.Stat(p)
	} else {
		fi, err = f.Lstat(p)
	}
	if e == nil {
		if err == nil {
			return fmt.Errorf("%w: %s now exists", ErrChanged, p)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s is gone", ErrChanged, p)
		}
		return err
	}
	var now Entry
	entryFromInfo(&now, fi)
	if fi.Mode()&fs.ModeSymlink != 0 {
		now.Kind = KindLink
		now.Size = 0
	}
	if now.Kind != e.Kind {
		return fmt.Errorf("%w: %s changed type", ErrChanged, p)
	}
	if e.Kind == KindFile && (now.Size != e.Size || !now.ModTime.Equal(e.ModTime)) {
		return fmt.Errorf("%w: %s", ErrChanged, p)
	}
	if e.Kind == KindLink {
		if t, err := f.Readlink(p); err == nil && t != e.LinkTarget {
			return fmt.Errorf("%w: %s points elsewhere", ErrChanged, p)
		}
	}
	return nil
}

func (e *executor) run(op *Op) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	switch op.Kind {
	case OpMkdir:
		return e.mkdir(op)
	case OpCopyFile:
		return e.copyFile(op)
	case OpCopyLink:
		return e.copyLink(op)
	case OpDelete, OpRemoveDir:
		return e.remove(op)
	}
	return fmt.Errorf("syncdirs: unknown op %v", op.Kind)
}

// ensureParents creates the missing folders above p.
func ensureParents(dst WritableFS, p string) error {
	dir := path.Dir(p)
	if dir == "." || dir == "/" {
		return nil
	}
	fi, err := dst.Lstat(dir)
	if err == nil {
		if fi.Mode()&fs.ModeSymlink != 0 {
			return &fs.PathError{Op: "mkdir", Path: dir, Err: ErrSymlinkInPath}
		}
		if !fi.IsDir() {
			return &fs.PathError{Op: "mkdir", Path: dir, Err: fs.ErrExist}
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := ensureParents(dst, dir); err != nil {
		return err
	}
	if err := dst.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}

func (e *executor) mkdir(op *Op) error {
	_, dst := e.sides(op.Side)
	srcE, _ := entries(op.Item, op.Side)
	if err := ensureParents(dst, op.Dst); err != nil {
		return err
	}
	fi, err := dst.Lstat(op.Dst)
	switch {
	case err == nil && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0:
		// already there
	case err == nil:
		return fmt.Errorf("%w: %s exists and is not a folder", ErrChanged, op.Dst)
	case errors.Is(err, fs.ErrNotExist):
		perm := fs.FileMode(0o755)
		if srcE != nil && srcE.Mode.Perm() != 0 {
			perm = srcE.Mode.Perm() | 0o700
		}
		if err := dst.Mkdir(op.Dst, perm); err != nil {
			return err
		}
	default:
		return err
	}
	e.dirTimes = append(e.dirTimes, *op)
	return nil
}

// finishDirTimes gives created folders the source folder's date, deepest
// first, after everything inside them was written.
func (e *executor) finishDirTimes() {
	for i := len(e.dirTimes) - 1; i >= 0; i-- {
		op := e.dirTimes[i]
		_, dst := e.sides(op.Side)
		if srcE, _ := entries(op.Item, op.Side); srcE != nil {
			_ = dst.Chtimes(op.Dst, srcE.ModTime)
		}
	}
	e.dirTimes = nil
}

// confirm asks the overwrite hook, honouring the sticky "all" answers.
func (e *executor) confirm(op *Op) error {
	if e.opts.ConfirmOverwrite == nil {
		return nil
	}
	d := e.overwrite
	if !e.sticky {
		d = e.opts.ConfirmOverwrite(op)
	}
	switch d {
	case OverwriteAll:
		e.overwrite, e.sticky = OverwriteAll, true
		return nil
	case SkipAll:
		e.overwrite, e.sticky = SkipAll, true
		return errSkipped
	case SkipFile:
		return errSkipped
	case CancelSync:
		return context.Canceled
	}
	return nil
}

func (e *executor) checkBoth(op *Op, src FS, dst WritableFS) (exists bool, err error) {
	srcE, dstE := entries(op.Item, op.Side)
	if !e.opts.Force {
		if err := sameAsCompared(src, op.Src, srcE); err != nil {
			return false, err
		}
		if err := sameAsCompared(dst, op.Dst, dstE); err != nil {
			return false, err
		}
	}
	_, err = dst.Lstat(op.Dst)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (e *executor) copyFile(op *Op) error {
	src, dst := e.sides(op.Side)
	srcE, _ := entries(op.Item, op.Side)
	exists, err := e.checkBoth(op, src, dst)
	if err != nil {
		return err
	}
	if exists {
		if err := e.confirm(op); err != nil {
			return err
		}
	}
	if err := ensureParents(dst, op.Dst); err != nil {
		return err
	}
	in, err := src.Open(op.Src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, tmp, err := dst.CreateTemp(path.Dir(op.Dst))
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = dst.Remove(tmp)
		}
	}()
	buf := make([]byte, chunkSize)
	for {
		if err := e.ctx.Err(); err != nil {
			out.Close()
			return err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			e.prog.FileDone += int64(n)
			e.prog.BytesDone += int64(n)
			e.report()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if perm := srcE.Mode.Perm(); perm != 0 {
		_ = dst.Chmod(tmp, perm|0o200)
	}
	if err := dst.Chtimes(tmp, srcE.ModTime); err != nil {
		return err
	}
	if err := dst.Rename(tmp, op.Dst); err != nil {
		return err
	}
	ok = true
	// Restore a read-only source mode after the rename (the temp file had
	// to stay writable for the date change on some systems).
	if perm := srcE.Mode.Perm(); perm != 0 && perm&0o200 == 0 {
		_ = dst.Chmod(op.Dst, perm)
	}
	return nil
}

func (e *executor) copyLink(op *Op) error {
	src, dst := e.sides(op.Side)
	exists, err := e.checkBoth(op, src, dst)
	if err != nil {
		return err
	}
	target, err := src.Readlink(op.Src)
	if err != nil {
		return err
	}
	if exists {
		if err := e.confirm(op); err != nil {
			return err
		}
		fi, err := dst.Lstat(op.Dst)
		if err != nil {
			return err
		}
		if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
			return fmt.Errorf("%w: %s is a folder", ErrChanged, op.Dst)
		}
		if err := dst.Remove(op.Dst); err != nil {
			return err
		}
	}
	if err := ensureParents(dst, op.Dst); err != nil {
		return err
	}
	return dst.Symlink(target, op.Dst)
}

func (e *executor) remove(op *Op) error {
	var f FS = e.left
	ent := op.Item.Left
	if op.Side == RightSide {
		f, ent = e.right, op.Item.Right
	}
	dst, _ := f.(WritableFS)
	if !e.opts.Force {
		// Folders are only checked for their type (deleting their contents
		// changes their date). Something already gone counts as deleted.
		if err := sameAsCompared(f, op.Dst, ent); err != nil {
			if isGone(f, op.Dst) {
				return nil
			}
			return err
		}
	}
	err := dst.Remove(op.Dst)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func isGone(f FS, p string) bool {
	_, err := f.Lstat(p)
	return errors.Is(err, fs.ErrNotExist)
}
