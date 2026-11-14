package main

// The undo journal of file operations (board issue #13, F-NEW-2) as the page sees it: a thin layer over internal/undo.
// Renames in place (Shift+F6), new folders (F7) and copy and move jobs (F5, F6, drag and drop, paste) on this
// computer's disks are written to undo.json in the settings folder, so Cmd+Z (macOS) takes the newest one back, then
// the one before it, also after a restart. Operations on servers and in archives are not recorded, and neither are
// deletes (the trash has its own way back) nor the undo's own work.
//
// Copy and move jobs run in internal/fileops, which reports every file it handles through Queue.Log. Before a job
// starts, startTracked notes which of its targets were already there (those can't be undone: they were replaced or
// merged); while it runs, observe picks out the entries of its top-level items; when the job is done the entry goes
// into the journal with what really arrived where (a name the copy got to avoid a clash included). The setting
// undoJournal (on by default) false stops the recording; the journal that is there stays until it is undone.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"mormicommander/internal/fileops"
	"mormicommander/internal/undo"
)

// Undo is what the page calls for the undo journal (window.go.main.Undo).
type Undo struct {
	app *App
	j   *undo.Journal

	mu   sync.Mutex
	jobs map[int]*undoPending // copy and move jobs being watched, by job number
}

// undoPending is a copy or move job being watched.
type undoPending struct {
	kind    undo.Kind
	sources map[string]int  // cleaned source path → its index
	dir     []bool          // per source: it is a folder
	pre     map[string]bool // the targets that were already there before the job
	dst     []string        // per source: where it really arrived ("" = it did not)
}

func init() {
	registerFeature("undo", func(a *App) []exposed {
		path := ""
		if dir := configDir(); dir != "" {
			path = filepath.Join(dir, "undo.json")
		}
		u := newUndo(a, undo.Open(path, 0))
		a.undo = u
		return []exposed{expose(u)}
	})
	registerControl("undo-list", func(a *App, m map[string]any) (map[string]any, error) {
		if a.undo == nil {
			return nil, errors.New("no undo journal")
		}
		return map[string]any{"ok": true, "entries": a.undo.j.List()}, nil
	})
}

func newUndo(a *App, j *undo.Journal) *Undo {
	return &Undo{app: a, j: j, jobs: map[int]*undoPending{}}
}

// recording says whether operations are written to the journal now (setting undoJournal, on by default).
func (u *Undo) recording() bool {
	if u == nil || u.j == nil || u.app == nil || u.app.mock != nil {
		return false
	}
	if u.app.settings != nil {
		if on, ok := u.app.settings.Get("undoJournal").(bool); ok && !on {
			return false
		}
	}
	return true
}

// undoLocal says whether a path is on this computer's disks (not a server address).
func undoLocal(p string) bool { return p != "" && !strings.Contains(p, "://") && filepath.IsAbs(p) }

// record adds an instant operation (rename, new folder) to the journal.
func (u *Undo) record(kind undo.Kind, items ...undo.Item) {
	if !u.recording() {
		return
	}
	for _, it := range items {
		if !undoLocal(it.From) || !undoLocal(it.To) {
			return
		}
	}
	u.j.Add(undo.Entry{Kind: kind, Items: items})
}

// startTracked starts a file job through start and, for a copy or move on local disks, watches it for the journal.
// The lock is held while the job starts, so its first log entries wait until it is known here.
func (u *Undo) startTracked(r fileops.Request, start func(fileops.Request) (int, error)) (int, error) {
	if !u.recording() || r.NoJournal || (r.Kind != fileops.Copy && r.Kind != fileops.Move) || !undoLocal(r.Target) {
		return start(r)
	}
	p := &undoPending{kind: undo.Copy, sources: map[string]int{}, pre: map[string]bool{}}
	if r.Kind == fileops.Move {
		p.kind = undo.Move
	}
	target := filepath.Clean(r.Target)
	for i, s := range r.Sources {
		if !undoLocal(s) {
			return start(r)
		}
		src := filepath.Clean(s)
		p.sources[src] = i
		fi, err := os.Lstat(src)
		p.dir = append(p.dir, err == nil && fi.IsDir())
		dst := filepath.Join(target, fileops.ApplyMask(r.Rename, filepath.Base(src)))
		if _, err := os.Lstat(dst); err == nil && dst != src {
			p.pre[dst] = true
		}
	}
	p.dst = make([]string, len(r.Sources))
	u.mu.Lock()
	defer u.mu.Unlock()
	id, err := start(r)
	if err == nil {
		u.jobs[id] = p
	}
	return id, err
}

// observe gets every log entry of the job queue (Queue.Log): the arrival of a top-level item of a watched job, and the
// job's end, which writes its entry to the journal.
func (u *Undo) observe(e fileops.Entry) {
	if u == nil {
		return
	}
	u.mu.Lock()
	p := u.jobs[e.Job]
	if p == nil {
		u.mu.Unlock()
		return
	}
	switch e.Action {
	case "copy", "move", "mkdir", "rename":
		if i, ok := p.sources[e.Source]; ok && p.dst[i] == "" && e.Target != "" {
			p.dst[i] = e.Target
		}
	case "done":
		// written before the job is forgotten, so whoever sees it gone finds its entry in the journal
		u.j.Add(p.entry())
		delete(u.jobs, e.Job)
	}
	u.mu.Unlock()
}

// entry is the journal entry of a finished job: the items that arrived somewhere new, and the count of those that
// replaced or merged into something that was there.
func (p *undoPending) entry() undo.Entry {
	e := undo.Entry{Kind: p.kind}
	src := make([]string, len(p.dst))
	for s, i := range p.sources {
		src[i] = s
	}
	for i, dst := range p.dst {
		if dst == "" {
			continue
		}
		if p.pre[dst] {
			e.Lost++
			continue
		}
		it := undo.Item{From: src[i], To: dst, Dir: p.dir[i]}
		if p.kind == undo.Copy && !it.Dir {
			if fi, err := os.Lstat(dst); err == nil {
				it.Size, it.MTime = fi.Size(), fi.ModTime().UnixMilli()
			}
		}
		e.Items = append(e.Items, it)
	}
	return e
}

// UndoPreview is what the page shows before an undo: the question and what will be left alone.
type UndoPreview struct {
	ID       int64    `json:"id"`       // the entry; 0 = the journal is empty
	What     string   `json:"what"`     // "the copy of report.txt to /b"
	Question string   `json:"question"` // the whole question for the confirm dialog
	Problems []string `json:"problems"` // items that will be left alone
	Steps    int      `json:"steps"`    // how many items the undo will put back or remove
	Count    int      `json:"count"`    // entries in the journal
}

// UndoJob is a file job the page runs to finish an undo (with its progress box and questions).
type UndoJob struct {
	Request fileops.Request `json:"request"`
	Title   string          `json:"title"`
}

// UndoResult is what an undo did.
type UndoResult struct {
	What     string    `json:"what"`
	Done     int       `json:"done"`     // items put back at once
	Jobs     []UndoJob `json:"jobs"`     // file jobs still to run
	Problems []string  `json:"problems"` // items left alone
}

// trashWord is the name of the trash on this system.
func trashWord() string {
	if runtime.GOOS == "windows" {
		return "Recycle Bin"
	}
	return "Trash"
}

// Preview says what undoing the newest operation (id 0) or the operation id would do now.
func (u *Undo) Preview(id int64) (UndoPreview, error) {
	e, ok := u.entry(id)
	pv := UndoPreview{Problems: []string{}, Count: len(u.j.List())}
	if !ok {
		if id != 0 {
			return pv, errors.New("That operation is not in the undo list any more.")
		}
		return pv, nil
	}
	plan := undo.PlanUndo(e, fileops.SameVolume)
	pv.ID, pv.What, pv.Problems, pv.Steps = e.ID, undo.Describe(e), plan.Problems, len(plan.Steps)
	q := fmt.Sprintf("Undo %s?", pv.What)
	if e.Kind == undo.Copy && len(plan.Steps) > 0 {
		if fileops.TrashSupported() {
			q += fmt.Sprintf("\n\nThe copies go to the %s.", trashWord())
		} else {
			q += fmt.Sprintf("\n\nThere is no %s here, so the copies will be deleted permanently.", trashWord())
		}
	}
	if len(plan.Steps) == 0 {
		q = fmt.Sprintf("%s can't be undone any more.", undoCapital(pv.What))
	}
	if len(plan.Problems) > 0 {
		q += "\n\n" + strings.Join(plan.Problems, "\n")
	}
	pv.Question = q
	return pv, nil
}

// Run undoes the operation id (0: the newest): instant steps are done here, the rest comes back as file jobs for the
// page. The operation leaves the journal either way, so the next undo goes one further back.
func (u *Undo) Run(id int64) (UndoResult, error) {
	if u.app != nil && u.app.mock != nil {
		return UndoResult{}, errors.New("This is the demo: its files are not real, so nothing can be undone.")
	}
	e, ok := u.entry(id)
	if !ok {
		return UndoResult{}, errors.New("There is nothing to undo.")
	}
	plan := undo.PlanUndo(e, fileops.SameVolume)
	jobs, done, problems := undo.Apply(plan.Steps)
	u.j.Remove(e.ID)
	res := UndoResult{What: undo.Describe(e), Done: done, Jobs: undoJobs(jobs), Problems: append(plan.Problems, problems...)}
	return res, nil
}

// UndoListItem is one line of the undo list.
type UndoListItem struct {
	ID    int64     `json:"id"`
	Kind  undo.Kind `json:"kind"`
	What  string    `json:"what"` // "the copy of report.txt to /b"
	Time  int64     `json:"time"` // Unix milliseconds
	Items int       `json:"items"`
}

// List returns the journal, newest first.
func (u *Undo) List() []UndoListItem {
	l := u.j.List()
	out := make([]UndoListItem, 0, len(l))
	for i := len(l) - 1; i >= 0; i-- {
		e := l[i]
		out = append(out, UndoListItem{ID: e.ID, Kind: e.Kind, What: undo.Describe(e), Time: e.Time.UnixMilli(), Items: len(e.Items) + e.Lost})
	}
	return out
}

// Clear empties the journal.
func (u *Undo) Clear() { u.j.Clear() }

func (u *Undo) entry(id int64) (undo.Entry, bool) {
	if id == 0 {
		return u.j.Last()
	}
	return u.j.Get(id)
}

// undoJobs turns the steps that need real work into file jobs: the copies of a copy go to the trash (one job), items
// moved across volumes go back one job per item (each may need its old name back). The jobs never overwrite and are
// not recorded in the journal themselves.
func undoJobs(steps []undo.Step) []UndoJob {
	out := []UndoJob{}
	var remove []string
	for _, s := range steps {
		switch s.Op {
		case undo.OpRemove:
			remove = append(remove, s.To)
		case undo.OpMove:
			r := fileops.Request{Kind: fileops.Move, Sources: []string{s.To}, Target: filepath.Dir(s.From), Overwrite: fileops.Skip, NoJournal: true}
			if filepath.Base(s.From) != filepath.Base(s.To) {
				r.Rename = filepath.Base(s.From)
			}
			out = append(out, UndoJob{Request: r, Title: "Moving back"})
		}
	}
	if len(remove) > 0 {
		r := fileops.Request{Kind: fileops.Trash, Sources: remove, NoJournal: true}
		if !fileops.TrashSupported() {
			r.Kind = fileops.Delete
		}
		out = append(out, UndoJob{Request: r, Title: "Removing the copies"})
	}
	return out
}

func undoCapital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
