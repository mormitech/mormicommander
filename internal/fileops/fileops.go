// Package fileops is the engine behind copy, move and delete: a queue of jobs that run in the background, report
// progress as events, can be cancelled, and stop to ask when a file is in the way ("overwrite?"). It has no UI
// dependencies: the app gives it an Emit function (Wails events in the app, a recorder in the tests) and answers the
// questions through Answer.
//
// The rules follow Total Commander and the brief of the mormi apps:
//   - a folder is never copied into itself, and links are copied as links, so a link loop can't make a job run forever;
//   - a move on the same volume is a rename (instant); across volumes it is copy, then the source is removed, and only
//     the files that were copied without error are removed;
//   - "delete" is permanent and "trash" uses the system's trash; the UI picks which one (F8 / Shift+F8);
//   - an error on one file doesn't stop the job: it is noted, and the job goes on with the next file.
package fileops

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Kind is what a job does.
type Kind string

const (
	Copy   Kind = "copy"
	Move   Kind = "move"
	Delete Kind = "delete" // permanent
	Trash  Kind = "trash"  // to the Trash / Recycle Bin
)

// Policy says what happens when a target file exists.
type Policy string

const (
	Ask       Policy = ""          // ask each time (the default)
	Overwrite Policy = "overwrite" // overwrite all
	Skip      Policy = "skip"      // skip all
	Older     Policy = "older"     // overwrite only targets that are older than the source
)

// Request is a job to start.
type Request struct {
	Kind      Kind     `json:"kind"`
	Sources   []string `json:"sources"`   // full paths
	Target    string   `json:"target"`    // the destination folder (copy and move)
	Rename    string   `json:"rename"`    // a new name for a single source (copy "a.txt" as "b.txt"); "" keeps names
	Overwrite Policy   `json:"overwrite"` // see Policy
}

// Conflict is the question asked when a target exists.
type Conflict struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	SrcSize int64  `json:"srcSize"`
	DstSize int64  `json:"dstSize"`
	SrcTime int64  `json:"srcTime"` // Unix milliseconds
	DstTime int64  `json:"dstTime"`
	DstDir  bool   `json:"dstDir"` // the target is a folder (a file can't overwrite it)
}

// Choice is the answer to a Conflict.
type Choice string

const (
	ChoiceOverwrite    Choice = "overwrite"
	ChoiceSkip         Choice = "skip"
	ChoiceOverwriteAll Choice = "overwrite_all"
	ChoiceSkipAll      Choice = "skip_all"
	ChoiceOlderAll     Choice = "older_all" // overwrite all older targets, skip the rest
	ChoiceRename       Choice = "rename"    // copy under Answer.NewName instead
	ChoiceCancel       Choice = "cancel"
)

// Answer is what the user said to a Conflict.
type Answer struct {
	Choice  Choice `json:"choice"`
	NewName string `json:"newName"` // for ChoiceRename: the name (not a path) to use in the target folder
}

// State of a job.
const (
	StateRunning   = "running"
	StateAsking    = "asking"
	StateDone      = "done"
	StateFailed    = "failed" // finished, but some files had errors
	StateCancelled = "cancelled"
)

// Progress is the state of a job as the UI shows it.
type Progress struct {
	ID        int       `json:"id"`
	Kind      Kind      `json:"kind"`
	State     string    `json:"state"`
	Files     int       `json:"files"` // files (and links) the job works on, found before it starts
	FilesDone int       `json:"filesDone"`
	Bytes     int64     `json:"bytes"`
	BytesDone int64     `json:"bytesDone"`
	Current   string    `json:"current"` // the file being worked on
	Errors    []string  `json:"errors"`  // plain sentences, at most maxErrors
	Skipped   int       `json:"skipped"`
	Ask       *Conflict `json:"ask,omitempty"` // the open question while State is "asking"
}

const maxErrors = 100

// Event names given to Emit.
const (
	EventProgress = "job"      // Progress, at most every Interval
	EventAsk      = "job-ask"  // Progress with Ask set
	EventDone     = "job-done" // the final Progress
)

// Queue runs jobs. The zero value is not usable; use NewQueue.
type Queue struct {
	Emit     func(event string, data any) // never nil after NewQueue
	Interval time.Duration                // how often running jobs report progress

	mu   sync.Mutex
	next int
	jobs map[int]*job

	beforeFile func(path string) // test hook, called before each file is worked on
}

// NewQueue makes a queue that reports through emit (nil: nothing is reported).
func NewQueue(emit func(event string, data any)) *Queue {
	if emit == nil {
		emit = func(string, any) {}
	}
	return &Queue{Emit: emit, Interval: 150 * time.Millisecond, jobs: map[int]*job{}}
}

type job struct {
	q       *Queue
	req     Request
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	p       Progress
	answers chan Answer
	policy  Policy
	done    chan struct{}
	last    time.Time
}

// Start checks the request and starts the job. The id names it in Cancel, Answer and the events.
func (q *Queue) Start(r Request) (int, error) {
	if err := validate(r); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.mu.Lock()
	q.next++
	id := q.next
	j := &job{q: q, req: r, ctx: ctx, cancel: cancel, answers: make(chan Answer, 1), policy: r.Overwrite, done: make(chan struct{})}
	j.p = Progress{ID: id, Kind: r.Kind, State: StateRunning}
	q.jobs[id] = j
	q.mu.Unlock()
	go j.run()
	return id, nil
}

func validate(r Request) error {
	if len(r.Sources) == 0 {
		return errors.New("nothing is selected")
	}
	switch r.Kind {
	case Copy, Move:
		if r.Target == "" {
			return errors.New("no target folder given")
		}
		if r.Rename != "" && len(r.Sources) != 1 {
			return errors.New("a new name can only be given for one file")
		}
	case Delete, Trash:
	default:
		return fmt.Errorf("unknown kind of job %q", r.Kind)
	}
	switch r.Overwrite {
	case Ask, Overwrite, Skip, Older:
	default:
		return fmt.Errorf("unknown overwrite rule %q", r.Overwrite)
	}
	return nil
}

func (q *Queue) get(id int) *job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.jobs[id]
}

// Cancel stops a job; files done so far stay done. It returns false for an unknown or finished job.
func (q *Queue) Cancel(id int) bool {
	j := q.get(id)
	if j == nil {
		return false
	}
	select {
	case <-j.done:
		return false
	default:
	}
	j.cancel()
	return true
}

// Answer answers the open question of a job.
func (q *Queue) Answer(id int, a Answer) error {
	j := q.get(id)
	if j == nil {
		return fmt.Errorf("there is no job %d", id)
	}
	j.mu.Lock()
	asking := j.p.State == StateAsking
	j.mu.Unlock()
	if !asking {
		return fmt.Errorf("job %d asks nothing now", id)
	}
	select {
	case j.answers <- a:
		return nil
	default:
		return fmt.Errorf("job %d already has an answer", id)
	}
}

// Wait blocks until the job ends and returns its final state.
func (q *Queue) Wait(id int) Progress {
	j := q.get(id)
	if j == nil {
		return Progress{ID: id}
	}
	<-j.done
	return j.snapshot()
}

// Get is the current state of a job.
func (q *Queue) Get(id int) (Progress, bool) {
	j := q.get(id)
	if j == nil {
		return Progress{}, false
	}
	return j.snapshot(), true
}

// List is the state of all jobs, oldest first. Finished jobs stay listed until Forget.
func (q *Queue) List() []Progress {
	q.mu.Lock()
	js := make([]*job, 0, len(q.jobs))
	for _, j := range q.jobs {
		js = append(js, j)
	}
	q.mu.Unlock()
	out := make([]Progress, len(js))
	for i, j := range js {
		out[i] = j.snapshot()
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

// Forget drops finished jobs from List.
func (q *Queue) Forget() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, j := range q.jobs {
		select {
		case <-j.done:
			delete(q.jobs, id)
		default:
		}
	}
}

func (j *job) snapshot() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	p := j.p
	p.Errors = append([]string(nil), j.p.Errors...)
	if j.p.Ask != nil {
		c := *j.p.Ask
		p.Ask = &c
	}
	return p
}

// tick reports progress, at most every Interval (force: now).
func (j *job) tick(force bool) {
	j.mu.Lock()
	if !force && time.Since(j.last) < j.q.Interval {
		j.mu.Unlock()
		return
	}
	j.last = time.Now()
	j.mu.Unlock()
	j.q.Emit(EventProgress, j.snapshot())
}

func (j *job) fail(format string, args ...any) {
	j.mu.Lock()
	if len(j.p.Errors) < maxErrors {
		j.p.Errors = append(j.p.Errors, fmt.Sprintf(format, args...))
	}
	j.mu.Unlock()
}

func (j *job) update(f func(p *Progress)) {
	j.mu.Lock()
	f(&j.p)
	j.mu.Unlock()
}

func (j *job) run() {
	defer close(j.done)
	j.tick(true)
	switch j.req.Kind {
	case Copy:
		j.copyAll(false)
	case Move:
		j.copyAll(true)
	case Delete:
		j.deleteAll()
	case Trash:
		j.trashAll()
	}
	j.update(func(p *Progress) {
		p.Ask, p.Current = nil, ""
		switch {
		case j.ctx.Err() != nil:
			p.State = StateCancelled
		case len(p.Errors) > 0:
			p.State = StateFailed
		default:
			p.State = StateDone
		}
	})
	j.q.Emit(EventDone, j.snapshot())
}

// ask puts a conflict to the user and waits for the answer (or a cancel).
func (j *job) ask(c Conflict) Answer {
	j.update(func(p *Progress) { p.State, p.Ask = StateAsking, &c })
	j.q.Emit(EventAsk, j.snapshot())
	var a Answer
	select {
	case a = <-j.answers:
	case <-j.ctx.Done():
		a = Answer{Choice: ChoiceCancel}
	}
	j.update(func(p *Progress) { p.State, p.Ask = StateRunning, nil })
	return a
}
