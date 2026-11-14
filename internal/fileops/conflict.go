package fileops

// When a file is copied onto a name that is taken, Total Commander asks (or follows the overwrite mode chosen in the
// dialog): overwrite, outSkip, overwrite only older, rename the copy, auto-rename all copies, rename the existing file
// instead, copy only larger or smaller files, append, resume a broken-off copy, or compare the two first. The "… all"
// answers become the job's policy for every later conflict. A read-only target gets its own second question, because
// the read-only flag usually means someone wanted that file kept. resolve turns all of that into one decision.

import (
	"io/fs"
	"path/filepath"
)

// outcome of resolve.
type outcome int

const (
	outWrite  outcome = iota // write to the returned path
	outSkip                  // leave this file out (counted as skipped)
	outFailed                // can't be done (the error is already noted)
)

// resolve decides what to do when the target of a file exists: where to write and how, or to skip it.
func (j *job) resolve(it item) (string, writeMode, outcome) {
	dst := it.dst
	var same *bool
	srcRegular := it.info.Mode().IsRegular()
	for {
		di, err := lstat(dst)
		if err != nil {
			return dst, wmCreate, outWrite // nothing in the way
		}
		if di.IsDir() {
			j.fail("Can't overwrite the folder %s with a file.", dst)
			return dst, wmCreate, outFailed
		}
		if sameFile(it.info, di) {
			j.fail("Can't copy %s onto itself.", filepath.Base(it.src))
			return dst, wmCreate, outFailed
		}
		srcT, dstT := it.info.ModTime(), di.ModTime()
		var take bool
		decided := true
		switch j.policy {
		case Overwrite:
			take = true
		case Skip:
			take = false
		case Older:
			take = dstT.Before(srcT)
		case Larger:
			take = it.info.Size() > di.Size()
		case Smaller:
			take = it.info.Size() < di.Size()
		case AutoRenameSource:
			return filepath.Join(filepath.Dir(dst), UniqueName(filepath.Dir(dst), filepath.Base(dst))), wmCreate, outWrite
		case AutoRenameTarget:
			if !j.moveAside(dst) {
				return dst, wmCreate, outFailed
			}
			return dst, wmCreate, outWrite
		default:
			decided = false
		}
		if decided {
			if !take {
				return dst, wmCreate, outSkip
			}
			return dst, wmCreate, j.readOnlyCheck(it, dst, di)
		}
		c := Conflict{Source: it.src, Target: dst, SrcSize: it.info.Size(), DstSize: di.Size(),
			SrcTime: srcT.UnixMilli(), DstTime: dstT.UnixMilli(), Same: same,
			Choices: []Choice{ChoiceOverwrite, ChoiceOverwriteAll, ChoiceSkip, ChoiceSkipAll, ChoiceOlderAll,
				ChoiceRename, ChoiceAutoRename, ChoiceAutoRenameAll, ChoiceRenameTarget, ChoiceRenameTargetAll,
				ChoiceLargerAll, ChoiceSmallerAll}}
		if srcRegular && di.Mode().IsRegular() {
			c.Choices = append(c.Choices, ChoiceAppend)
			if di.Size() < it.info.Size() {
				c.Choices = append(c.Choices, ChoiceResume)
			}
			c.Choices = append(c.Choices, ChoiceCompare)
		}
		c.Choices = append(c.Choices, ChoiceCancel)
		a := j.ask(c)
		switch a.Choice {
		case ChoiceOverwrite:
			return dst, wmCreate, j.readOnlyCheck(it, dst, di)
		case ChoiceOverwriteAll:
			j.policy = Overwrite
		case ChoiceSkipAll:
			j.policy = Skip
		case ChoiceOlderAll:
			j.policy = Older
		case ChoiceLargerAll:
			j.policy = Larger
		case ChoiceSmallerAll:
			j.policy = Smaller
		case ChoiceAutoRenameAll:
			j.policy = AutoRenameSource
		case ChoiceRenameTargetAll:
			j.policy = AutoRenameTarget
		case ChoiceAutoRename:
			return filepath.Join(filepath.Dir(dst), UniqueName(filepath.Dir(dst), filepath.Base(dst))), wmCreate, outWrite
		case ChoiceRenameTarget:
			if !j.moveAside(dst) {
				return dst, wmCreate, outFailed
			}
			return dst, wmCreate, outWrite
		case ChoiceRename:
			name := filepath.Base(a.NewName) // a name, never a path somewhere else
			if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
				return dst, wmCreate, outSkip
			}
			dst = filepath.Join(filepath.Dir(dst), name)
			same = nil
		case ChoiceAppend, ChoiceResume:
			if !srcRegular || !di.Mode().IsRegular() {
				return dst, wmCreate, outSkip
			}
			wm := wmAppend
			if a.Choice == ChoiceResume {
				if di.Size() > it.info.Size() {
					return dst, wmCreate, outSkip
				}
				wm = wmResume
			}
			return dst, wm, j.readOnlyCheck(it, dst, di)
		case ChoiceCompare:
			eq, err := sameContent(it.src, dst)
			if err != nil {
				j.fail("Can't compare %s: %s.", filepath.Base(it.src), plainErr(err))
				eq = false
			}
			same = &eq
		case ChoiceCancel:
			return dst, wmCreate, outSkip
		default: // skip, or an answer we don't know
			return dst, wmCreate, outSkip
		}
		// loop: an "… all" answer now applies through the policy, a new name may be taken too, and a compare asks again
	}
}

// readOnlyCheck asks (or follows Request.ReadOnly) before a read-only target is overwritten, and makes it writable
// when the answer is yes.
func (j *job) readOnlyCheck(it item, dst string, di fs.FileInfo) outcome {
	if !readOnly(di) {
		return outWrite
	}
	take := false
	switch j.roPolicy {
	case Overwrite:
		take = true
	case Skip:
	default:
		a := j.ask(Conflict{Type: AskReadOnly, Source: it.src, Target: dst, SrcSize: it.info.Size(), DstSize: di.Size(),
			SrcTime: it.info.ModTime().UnixMilli(), DstTime: di.ModTime().UnixMilli(),
			Choices: []Choice{ChoiceOverwrite, ChoiceOverwriteAll, ChoiceSkip, ChoiceSkipAll, ChoiceCancel}})
		switch a.Choice {
		case ChoiceOverwrite:
			take = true
		case ChoiceOverwriteAll:
			j.roPolicy, take = Overwrite, true
		case ChoiceSkipAll:
			j.roPolicy = Skip
		}
	}
	if !take {
		return outSkip
	}
	if err := makeWritable(dst, di); err != nil {
		j.failAt(it.src, dst, err, "Can't make %s writable: %s.", dst, plainErr(err))
		return outFailed
	}
	return outWrite
}

// moveAside renames an existing target to a free name ("file(2).txt") so the copy can take its place.
func (j *job) moveAside(dst string) bool {
	dir := filepath.Dir(dst)
	aside := filepath.Join(dir, UniqueName(dir, filepath.Base(dst)))
	if err := rename(dst, aside); err != nil {
		j.fail("Can't rename the existing %s: %s.", filepath.Base(dst), plainErr(err))
		return false
	}
	j.log("rename", dst, aside, nil)
	return true
}
