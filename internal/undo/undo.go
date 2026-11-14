// Package undo keeps the undo journal of file operations (board issue #13, F-NEW-2): after a rename, a move, a copy,
// a new folder or a delete to the trash, the journal holds what was done, so that the user can take it back later, one step at a time, newest
// first, also after the app was restarted (the journal is a small JSON file in the settings folder).
//
// The package has no UI and touches only the paths an entry names. Undoing never overwrites anything: before each step
// it checks that the item is still where the operation put it and that its old place is free again; an item that
// fails the check is left alone and named in the plan's problems. Steps that are instant (a rename back on the same
// volume, removing an empty folder that was made, bringing an item back from the trash) are carried out here by Apply; steps that are real work (moving a
// folder back across volumes, removing the copies of a copy) are handed back as file jobs, so the caller runs them
// with progress, the trash and the usual questions. Removing a copy always prefers the trash.
//
// A delete to the trash is recorded with the place each item got in the trash (internal/trash finds it), and its undo
// moves the item back from there (internal/trash.Restore, which also removes the trash's own record of it). An item
// whose place in the trash could not be found is only counted (Entry.Unknown): the undo says to put it back with the
// system's own tools. Permanent deletes are never recorded, because nothing can bring them back.
package undo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mormicommander/internal/trash"
)

// Kind is the operation an entry records.
type Kind string

const (
	Rename Kind = "rename" // a file or folder got a new name in its folder (Shift+F6)
	Move   Kind = "move"   // items went to another folder (F6)
	Copy   Kind = "copy"   // items were copied (F5); undo removes the copies
	MkDir  Kind = "mkdir"  // a folder was made (F7); undo removes it while it is still empty
	Trash  Kind = "trash"  // items went to the trash (F8); undo brings them back to where they were
)

// Item is one thing an operation changed.
type Item struct {
	// From is where the item was before (rename, move), the source of a copy, or for a new folder the first level that
	// was made (F7 "a/b/c" in a folder without "a" makes three levels; From is ".../a").
	From string `json:"from"`
	// To is where the item is now: the new name, the moved item, the copy, the deepest folder made, or the item in the
	// trash.
	To  string `json:"to"`
	Dir bool   `json:"dir,omitempty"`
	// Size and MTime describe a copied file as it was right after the copy (Unix milliseconds), so a copy the user
	// has changed since is not thrown away by an undo. Zero for folders and for the other kinds.
	Size  int64 `json:"size,omitempty"`
	MTime int64 `json:"mtime,omitempty"`
}

// Entry is one operation in the journal.
type Entry struct {
	ID    int64     `json:"id"`
	Time  time.Time `json:"time"`
	Kind  Kind      `json:"kind"`
	Items []Item    `json:"items"`
	// Lost counts the items of a copy or move that replaced or merged into something that was already there. Those
	// can't be undone (the old file is gone, or the merged folder holds older files too), so they have no Item.
	Lost int `json:"lost,omitempty"`
	// Unknown counts the items of a delete to the trash whose place in the trash could not be found afterwards (the app
	// may not be allowed to read the macOS Trash, a drive may have no Recycle Bin). They are in the trash, but only the
	// system's own tools can put them back, so they have no Item either.
	Unknown int `json:"unknown,omitempty"`
}

// Journal is the list of operations that can still be undone, oldest first. It is safe for use by several
// goroutines.
type Journal struct {
	path string
	max  int

	mu      sync.Mutex
	entries []Entry
	next    int64
}

// DefaultMax is how many operations the journal keeps; older ones fall off.
const DefaultMax = 100

// Open reads the journal at path (a missing or unreadable file gives an empty journal). path "" keeps the journal in
// memory only. max <= 0 means DefaultMax.
func Open(path string, max int) *Journal {
	if max <= 0 {
		max = DefaultMax
	}
	j := &Journal{path: path, max: max}
	if path == "" {
		return j
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return j
	}
	var f struct {
		Entries []Entry `json:"entries"`
	}
	if json.Unmarshal(b, &f) != nil {
		return j
	}
	for _, e := range f.Entries {
		if e.ID > j.next {
			j.next = e.ID
		}
	}
	j.entries = f.Entries
	j.trim()
	return j
}

// Add records an operation and returns it with its number and time. An entry with neither items nor lost or unknown
// items is not recorded (ok false).
func (j *Journal) Add(e Entry) (Entry, bool) {
	if len(e.Items) == 0 && e.Lost == 0 && e.Unknown == 0 {
		return e, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.next++
	e.ID = j.next
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	j.entries = append(j.entries, e)
	j.trim()
	j.save()
	return e, true
}

// List returns the entries, oldest first.
func (j *Journal) List() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]Entry(nil), j.entries...)
}

// Last returns the newest entry.
func (j *Journal) Last() (Entry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.entries) == 0 {
		return Entry{}, false
	}
	return j.entries[len(j.entries)-1], true
}

// Get returns the entry with the number id.
func (j *Journal) Get(id int64) (Entry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range j.entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Remove drops the entry with the number id; false when there is none.
func (j *Journal) Remove(id int64) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, e := range j.entries {
		if e.ID == id {
			j.entries = append(j.entries[:i], j.entries[i+1:]...)
			j.save()
			return true
		}
	}
	return false
}

// Clear empties the journal.
func (j *Journal) Clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.entries = nil
	j.save()
}

func (j *Journal) trim() {
	if n := len(j.entries) - j.max; n > 0 {
		j.entries = append([]Entry(nil), j.entries[n:]...)
	}
}

// save writes the journal through a temporary file, so a crash never leaves half a file. Errors are ignored: a
// journal that can't be saved still works for this session.
func (j *Journal) save() {
	if j.path == "" {
		return
	}
	b, err := json.MarshalIndent(struct {
		Entries []Entry `json:"entries"`
	}{j.entries}, "", " ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, j.path); err != nil {
		os.Remove(tmp)
	}
}

// ---- planning and carrying out an undo ----

// Op is what one step of an undo does.
type Op string

const (
	OpRename  Op = "rename"  // instant: To goes back to From (same volume)
	OpRmdir   Op = "rmdir"   // instant: the empty folders from To up to From are removed
	OpMove    Op = "move"    // a job: To is moved back to From across volumes
	OpRemove  Op = "remove"  // a job: the copy To goes to the trash (or is deleted where there is no trash)
	OpRestore Op = "restore" // instant: the item To comes back from the trash to From
)

// Step is one thing an undo does to one item.
type Step struct {
	Op   Op     `json:"op"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Plan is what undoing an entry would do now.
type Plan struct {
	Entry    Entry    `json:"entry"`
	Steps    []Step   `json:"steps"`
	Problems []string `json:"problems"` // items that will be left alone, in plain sentences
}

// SameVolumeFunc says whether two paths are on the same volume (fileops.SameVolume in the app).
type SameVolumeFunc func(a, b string) bool

// PlanUndo checks every item of e against the disk as it is now and says what an undo would do. Items are taken in
// reverse order, the way they would be put back by hand.
func PlanUndo(e Entry, sameVolume SameVolumeFunc) Plan {
	p := Plan{Entry: e, Steps: []Step{}, Problems: []string{}}
	if e.Lost > 0 {
		what := "item"
		if e.Lost > 1 {
			what = "items"
		}
		p.Problems = append(p.Problems, fmt.Sprintf("%d %s replaced or merged with what was already there, so %s can't be undone.",
			e.Lost, what, map[bool]string{true: "they", false: "it"}[e.Lost > 1]))
	}
	if e.Unknown > 0 {
		what, them := "1 item", "it"
		if e.Unknown > 1 {
			what, them = fmt.Sprintf("%d items", e.Unknown), "them"
		}
		p.Problems = append(p.Problems, fmt.Sprintf("%s went to the %s, but %s place there could not be found; put %s back with %s.",
			what, TrashWord(), map[bool]string{true: "their", false: "its"}[e.Unknown > 1], them, systemRestore()))
	}
	for i := len(e.Items) - 1; i >= 0; i-- {
		it := e.Items[i]
		if !filepath.IsAbs(it.From) || !filepath.IsAbs(it.To) {
			p.Problems = append(p.Problems, "An entry of the journal has no full path; it was left alone.")
			continue
		}
		switch e.Kind {
		case Rename, Move:
			ti, err := os.Lstat(it.To)
			if err != nil {
				p.Problems = append(p.Problems, fmt.Sprintf("%s is not there any more.", it.To))
				continue
			}
			if fi, err := os.Lstat(it.From); err == nil && !os.SameFile(fi, ti) {
				p.Problems = append(p.Problems, fmt.Sprintf("%s can't go back: %s is there again.", filepath.Base(it.To), it.From))
				continue
			}
			op := OpRename
			if e.Kind == Move && sameVolume != nil && !sameVolume(it.To, filepath.Dir(it.From)) {
				op = OpMove
			}
			p.Steps = append(p.Steps, Step{Op: op, From: it.From, To: it.To})
		case Copy:
			ti, err := os.Lstat(it.To)
			if err != nil {
				p.Problems = append(p.Problems, fmt.Sprintf("The copy %s is not there any more.", it.To))
				continue
			}
			if !it.Dir && (ti.IsDir() || ti.Size() != it.Size || (it.MTime != 0 && ti.ModTime().UnixMilli() != it.MTime)) {
				p.Problems = append(p.Problems, fmt.Sprintf("The copy %s was changed after the copy, so it was left alone.", it.To))
				continue
			}
			if it.Dir && !ti.IsDir() {
				p.Problems = append(p.Problems, fmt.Sprintf("%s is not the folder that was copied any more.", it.To))
				continue
			}
			p.Steps = append(p.Steps, Step{Op: OpRemove, From: it.From, To: it.To})
		case MkDir:
			if !within(it.To, it.From) {
				p.Problems = append(p.Problems, "An entry of the journal is not a folder that was made; it was left alone.")
				continue
			}
			ti, err := os.Lstat(it.To)
			if err != nil || !ti.IsDir() {
				p.Problems = append(p.Problems, fmt.Sprintf("The folder %s is not there any more.", it.To))
				continue
			}
			if !emptyChain(it.To, it.From) {
				p.Problems = append(p.Problems, fmt.Sprintf("The folder %s is not empty any more, so it was left alone.", it.From))
				continue
			}
			p.Steps = append(p.Steps, Step{Op: OpRmdir, From: it.From, To: it.To})
		case Trash:
			if _, err := os.Lstat(it.To); err != nil {
				p.Problems = append(p.Problems, fmt.Sprintf("%s is not in the %s any more.", filepath.Base(it.From), TrashWord()))
				continue
			}
			if _, err := os.Lstat(it.From); err == nil {
				p.Problems = append(p.Problems, fmt.Sprintf("%s can't come back from the %s: %s is there again.", filepath.Base(it.From), TrashWord(), it.From))
				continue
			}
			p.Steps = append(p.Steps, Step{Op: OpRestore, From: it.From, To: it.To})
		default:
			p.Problems = append(p.Problems, fmt.Sprintf("Undoing %q is not known to this version.", e.Kind))
			return p
		}
	}
	return p
}

// within says whether p is dir or below it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// emptyChain says whether the folders from deep up to top hold nothing but the next folder of the chain.
func emptyChain(deep, top string) bool {
	child := ""
	for p := deep; ; p = filepath.Dir(p) {
		list, err := os.ReadDir(p)
		if err != nil {
			return false
		}
		for _, d := range list {
			if d.Name() != child || !d.IsDir() {
				return false
			}
		}
		if p == top || filepath.Dir(p) == p {
			return p == top
		}
		child = filepath.Base(p)
	}
}

// Apply carries out the instant steps of a plan (rename back, remove empty folders) and returns the steps that are
// left for file jobs (OpMove, OpRemove), how many steps were done, and the problems met on the way.
func Apply(steps []Step) (jobs []Step, done int, problems []string) {
	jobs, problems = []Step{}, []string{}
	for _, s := range steps {
		switch s.Op {
		case OpRename:
			if err := renameBack(s); err != nil {
				problems = append(problems, fmt.Sprintf("Can't put %s back: %s.", filepath.Base(s.To), plain(err)))
				continue
			}
			done++
		case OpRmdir:
			if err := removeChain(s.To, s.From); err != nil {
				problems = append(problems, fmt.Sprintf("Can't remove the folder %s: %s.", s.From, plain(err)))
				continue
			}
			done++
		case OpRestore:
			if err := restoreTrashed(s.To, s.From); err != nil {
				problems = append(problems, fmt.Sprintf("Can't bring %s back from the %s: %s.", filepath.Base(s.From), TrashWord(), plain(err)))
				continue
			}
			done++
		case OpMove, OpRemove:
			jobs = append(jobs, s)
		}
	}
	return jobs, done, problems
}

// restoreTrashed brings an item back from the trash; the tests replace it, so they work with temporary folders and
// never with the real trash.
var restoreTrashed = trash.Restore

// TrashWord is the name of the trash on this system.
func TrashWord() string {
	if runtime.GOOS == "windows" {
		return "Recycle Bin"
	}
	return "Trash"
}

// systemRestore names the system's own way to take an item out of the trash.
func systemRestore() string {
	switch runtime.GOOS {
	case "darwin":
		return "Finder's Put Back"
	case "windows":
		return "Restore in the Recycle Bin"
	}
	return "your file manager's Restore"
}

// errTaken is returned when the old place of an item was taken between the plan and the step.
var errTaken = errors.New("something else has the old name now")

// renameBack moves s.To back to s.From on the same volume, never over another file.
func renameBack(s Step) error {
	ti, err := os.Lstat(s.To)
	if err != nil {
		return err
	}
	if fi, err := os.Lstat(s.From); err == nil {
		if !os.SameFile(fi, ti) {
			return errTaken
		}
		// the same file under another case (a case-only rename on a case-insensitive disk): two steps through a
		// free temporary name
		tmp := filepath.Join(filepath.Dir(s.To), fmt.Sprintf(".%s.mc-undo-%d", tempStem(filepath.Base(s.From)), time.Now().UnixNano()))
		if err := os.Rename(s.To, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, s.From); err != nil {
			os.Rename(tmp, s.To)
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.From), 0o777); err != nil {
		return err
	}
	return os.Rename(s.To, s.From)
}

// tempStem is the part of a name that goes into a temporary name next to it: the whole name plus the prefix and suffix
// would pass the 255-byte limit of file names for long names, so only the start of a long name is kept (cut between
// characters).
func tempStem(name string) string {
	const max = 64
	if len(name) <= max {
		return name
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}

// removeChain removes the empty folders from deep up to and including top.
func removeChain(deep, top string) error {
	if !within(deep, top) {
		return errors.New("the folder is not part of what was made")
	}
	for p := deep; ; p = filepath.Dir(p) {
		if err := os.Remove(p); err != nil {
			return err
		}
		if p == top || filepath.Dir(p) == p {
			return nil
		}
	}
}

// plain says what went wrong in a few words, without the Go error's path prefix.
func plain(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "it is not there any more"
	case errors.Is(err, fs.ErrExist):
		return "the name is taken"
	}
	return err.Error()
}

// Describe is a short phrase for an entry, for menus and the confirm question: "the copy of report.txt to /b",
// "the move of 3 items to /c", "the rename of a.txt to b.txt", "the new folder x", "the move of a.txt to the Trash".
func Describe(e Entry) string {
	n := len(e.Items) + e.Lost + e.Unknown
	what := fmt.Sprintf("%d items", n)
	if n == 1 {
		what = "1 item"
	}
	if len(e.Items) == 1 && e.Lost == 0 && e.Unknown == 0 {
		what = filepath.Base(e.Items[0].From)
	}
	where := ""
	if len(e.Items) > 0 {
		where = filepath.Dir(e.Items[0].To)
	}
	switch e.Kind {
	case Rename:
		if len(e.Items) == 1 {
			return fmt.Sprintf("the rename of %s to %s", filepath.Base(e.Items[0].From), filepath.Base(e.Items[0].To))
		}
		return "the rename of " + what
	case Move:
		if where == "" {
			return "the move of " + what
		}
		return fmt.Sprintf("the move of %s to %s", what, where)
	case Copy:
		if where == "" {
			return "the copy of " + what
		}
		return fmt.Sprintf("the copy of %s to %s", what, where)
	case MkDir:
		if len(e.Items) == 1 {
			return "the new folder " + filepath.Base(e.Items[0].From)
		}
		return "the new folders"
	case Trash:
		return fmt.Sprintf("the move of %s to the %s", what, TrashWord())
	}
	return string(e.Kind)
}
