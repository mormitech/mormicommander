// This file builds the preview list: the old name, the new name and any
// problem for every item, which is what the Multi-Rename dialog shows in its
// lower half and refreshes on every keystroke. Besides the per-name checks of
// ValidateName it looks across the list for two entries that would end up
// with the same name in the same folder, and asks the file system whether a
// target name is already taken by a file that is not itself being renamed
// away. Renaming in a chain (a to b while b becomes c) or swapping two names
// is allowed, because Execute handles it with temporary names. When names
// were loaded from a file or the clipboard they replace the computed names
// line by line, and a line count that does not match the list produces a
// warning just as in Total Commander.

package multirename

import (
	"errors"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FS is the small part of the file system the engine needs. OSFS uses the
// real one; tests substitute a wrapper to simulate failures.
type FS interface {
	Rename(oldpath, newpath string) error
	Lstat(name string) (fs.FileInfo, error)
}

// OSFS is the real file system.
type OSFS struct{}

func (OSFS) Rename(o, n string) error               { return os.Rename(o, n) }
func (OSFS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }

// CaseRule decides whether names that differ only in case are the same.
type CaseRule int

const (
	CaseRuleAuto        CaseRule = iota // follow the operating system
	CaseRuleSensitive                   // "a" and "A" are different names
	CaseRuleInsensitive                 // "a" and "A" are the same name
)

func (c CaseRule) fold() bool {
	switch c {
	case CaseRuleSensitive:
		return false
	case CaseRuleInsensitive:
		return true
	}
	return defaultFoldCase
}

// PreviewOptions tunes how the preview is checked.
type PreviewOptions struct {
	// Names, when not nil, are new names loaded from a file or the
	// clipboard; line i replaces the computed name of item i.
	Names []string
	// TargetOS selects the naming rules (a runtime.GOOS value); empty means
	// the current system.
	TargetOS string
	CaseRule CaseRule
	// FS is used for the "already exists" check; nil means OSFS.
	FS FS
	// SkipDiskCheck turns off the "already exists" check, for previews of
	// items that are not on a local disk.
	SkipDiskCheck bool
}

// Entry is one line of the preview.
type Entry struct {
	Item     Item      `json:"item"`
	NewName  string    `json:"newName"`
	Problems []Problem `json:"problems,omitempty"`
	Warnings []string  `json:"warnings,omitempty"`
}

// Changed reports whether the entry would actually be renamed.
func (e Entry) Changed() bool { return e.NewName != e.Item.Name }

// OK reports whether the entry has no blocking problem.
func (e Entry) OK() bool { return len(e.Problems) == 0 }

// NewPath returns the full target path.
func (e Entry) NewPath() string { return filepath.Join(e.Item.Dir, e.NewName) }

// Result is the whole preview.
type Result struct {
	Entries  []Entry  `json:"entries"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   int      `json:"errors"`  // number of entries with problems
	Changes  int      `json:"changes"` // number of entries that change name
	// fold remembers the case rule used, so Execute applies the same one.
	fold bool
}

// HasErrors reports whether any entry has a blocking problem. It looks at the
// entries themselves rather than at the Errors count, because a caller may
// edit names and rerun CheckEntries after the count was taken; a stale count
// must never let a conflicting batch through to Execute. The count is
// refreshed on the way so messages built from it stay correct.
func (r *Result) HasErrors() bool {
	r.Recount()
	return r.Errors > 0
}

// Recount refreshes Errors and Changes from the entries, for callers that
// changed names or reran CheckEntries after the preview was built.
func (r *Result) Recount() {
	r.Errors, r.Changes = 0, 0
	for _, e := range r.Entries {
		if !e.OK() {
			r.Errors++
		}
		if e.Changed() {
			r.Changes++
		}
	}
}

// Preview compiles the settings and builds the preview in one call.
func Preview(items []Item, s Settings, o PreviewOptions) (*Result, error) {
	r, err := Compile(s)
	if err != nil {
		return nil, err
	}
	return r.Preview(items, o), nil
}

// Preview computes and checks the new names for items.
func (r *Renamer) Preview(items []Item, o PreviewOptions) *Result {
	names, warns := r.NewNames(items)
	res := &Result{fold: o.CaseRule.fold()}
	if o.Names != nil {
		if len(o.Names) != len(items) {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"the name list has %d lines but %d items are selected", len(o.Names), len(items)))
		}
		for i := range names {
			if i < len(o.Names) {
				names[i] = o.Names[i]
			}
		}
	}
	res.Entries = make([]Entry, len(items))
	for i := range items {
		res.Entries[i] = Entry{Item: items[i], NewName: names[i], Warnings: warns[i]}
	}
	CheckEntries(res.Entries, o)
	res.Recount()
	return res
}

// CheckEntries recomputes the Problems of every entry. The dialog calls it
// after the user edits a new name by hand; Preview calls it internally.
func CheckEntries(entries []Entry, o PreviewOptions) {
	goos := o.TargetOS
	if goos == "" {
		goos = runtime.GOOS
	}
	fold := o.CaseRule.fold()
	fsys := o.FS
	if fsys == nil {
		fsys = OSFS{}
	}
	key := func(dir, name string) string {
		if fold {
			// a folding file system (macOS, Windows) treats case, and on APFS the Unicode form, as one name
			name = strings.ToLower(norm.NFC.String(name))
		}
		return filepath.Clean(dir) + "\x00" + name
	}
	targets := map[string][]int{}
	sources := map[string]int{}
	for i := range entries {
		e := &entries[i]
		e.Problems = nil
		targets[key(e.Item.Dir, e.NewName)] = append(targets[key(e.Item.Dir, e.NewName)], i)
		sources[key(e.Item.Dir, e.Item.Name)] = i
	}
	for i := range entries {
		e := &entries[i]
		if !e.Changed() {
			// An unchanged entry can still collide with another entry's
			// new name, which is reported on that other entry.
			if len(targets[key(e.Item.Dir, e.NewName)]) > 1 {
				e.Problems = append(e.Problems, Problem{ProblemDuplicate, "another item is renamed to this name"})
			}
			continue
		}
		e.Problems = append(e.Problems, ValidateName(e.NewName, goos)...)
		k := key(e.Item.Dir, e.NewName)
		if dup := targets[k]; len(dup) > 1 {
			e.Problems = append(e.Problems, Problem{ProblemDuplicate,
				fmt.Sprintf("%d items would be named %q", len(dup), e.NewName)})
		}
		if o.SkipDiskCheck || len(e.Problems) > 0 {
			continue
		}
		if _, isSource := sources[k]; isSource {
			continue // taken by an item that is part of this rename
		}
		fi, err := fsys.Lstat(e.NewPath())
		if err != nil {
			continue
		}
		if caseOnlySameFile(fsys, e.Item.Path(), e.NewPath(), fi) {
			continue // a case-only change on a case-insensitive disk
		}
		e.Problems = append(e.Problems, Problem{ProblemExists, fmt.Sprintf("%q already exists", e.NewName)})
	}
}

// ErrHasConflicts is returned by Execute when the preview still has errors.
var ErrHasConflicts = errors.New("multirename: the preview has conflicts")
