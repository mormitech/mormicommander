package fileops

// The queue is TC's background transfer manager: it owns every job, starts background jobs at once and queued jobs
// one after another in a single lane, and is the place the UI goes to pause, resume, cancel, re-limit or answer a job.
// Finished jobs stay listed (so the manager can show their statistics) until Forget drops them.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Queue runs jobs. The zero value is not usable; use NewQueue.
type Queue struct {
	Emit     func(event string, data any) // never nil after NewQueue
	Interval time.Duration                // how often running jobs report progress
	// AskFunc, when set, answers questions synchronously on the job's goroutine instead of waiting for Answer. The
	// EventAsk event is still emitted. It is meant for scripted use and tests; the UI uses events and Answer.
	AskFunc func(p Progress) Answer
	// Log, when set, receives one Entry per thing done or failed (see FileLogger for TC's log file).
	Log func(e Entry)
	// Lanes, when set, is how many queued jobs may run at the same time (1 when nil or below 1); AutoStart, when set
	// and false, leaves queued jobs waiting until RunNow starts them. Both are asked whenever a queued job could start.
	Lanes     func() int
	AutoStart func() bool

	mu   sync.Mutex
	next int
	jobs map[int]*job
	lane []*job // queued jobs in order; lane[0] runs

	beforeFile func(path string) // test hook, called before each file is worked on
}

// NewQueue makes a queue that reports through emit (nil: nothing is reported).
func NewQueue(emit func(event string, data any)) *Queue {
	if emit == nil {
		emit = func(string, any) {}
	}
	return &Queue{Emit: emit, Interval: 150 * time.Millisecond, jobs: map[int]*job{}}
}

// Start checks the request and starts the job (or puts it in the queue lane when Request.Queued). The id names it in
// the other methods and in the events.
func (q *Queue) Start(r Request) (int, error) {
	if err := validate(r); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{q: q, req: r, ctx: ctx, cancel: cancel, answers: make(chan Answer, 1), done: make(chan struct{}),
		policy: r.Overwrite, roPolicy: r.ReadOnly, errPolicy: r.OnError, resume: make(chan struct{})}
	j.filter = ParseFilter(r.Filter)
	j.limit.Store(max(r.SpeedLimit, 0))
	q.mu.Lock()
	q.next++
	j.p = Progress{ID: q.next, Kind: r.Kind, State: StateRunning, Target: r.Target, ETA: -1, Queued: r.Queued,
		SpeedLimit: j.limit.Load()}
	q.jobs[j.p.ID] = j
	run := true
	if r.Queued {
		q.lane = append(q.lane, j)
		if !q.laneFree() {
			run = false
			j.p.State = StateQueued
		}
	}
	if run {
		j.started.Store(true)
	}
	q.mu.Unlock()
	if run {
		go j.run()
	} else {
		q.Emit(EventProgress, j.snapshot())
	}
	return j.p.ID, nil
}

func validate(r Request) error {
	if len(r.Sources) == 0 {
		return errors.New("nothing is selected")
	}
	// Every path must be a full one: filepath.Clean turns "" into ".", the process's working folder, so an empty or
	// relative source would silently delete, trash, wipe or move whatever folder the program happens to run in.
	for _, s := range r.Sources {
		if s == "" || !filepath.IsAbs(s) {
			return fmt.Errorf("%q is not a full path", s)
		}
	}
	switch r.Kind {
	case Copy, Move:
		if r.Target == "" {
			return errors.New("no target folder given")
		}
		if !filepath.IsAbs(r.Target) {
			return fmt.Errorf("the target %q is not a full path", r.Target)
		}
		if r.Rename != "" && len(r.Sources) != 1 && !strings.ContainsAny(r.Rename, "*?") {
			return errors.New("a new name can only be given for one file; use a mask like *.bak for several")
		}
		if strings.ContainsAny(r.Rename, `/\`) {
			return errors.New("the new name must not contain a folder")
		}
		if r.Rename != "" && !strings.ContainsAny(r.Rename, "*?") {
			if err := checkName(r.Rename); err != nil {
				return fmt.Errorf("the new name %q is not allowed: %w", r.Rename, err)
			}
		}
	case Delete, Trash, Wipe:
	default:
		return fmt.Errorf("unknown kind of job %q", r.Kind)
	}
	switch r.Overwrite {
	case Ask, Overwrite, Skip, Older, AutoRenameSource, AutoRenameTarget, Larger, Smaller:
	default:
		return fmt.Errorf("unknown overwrite rule %q", r.Overwrite)
	}
	switch r.ReadOnly {
	case Ask, Overwrite, Skip:
	default:
		return fmt.Errorf("unknown read-only rule %q", r.ReadOnly)
	}
	switch r.Links {
	case LinksCopy, LinksFollow, LinksSkip:
	default:
		return fmt.Errorf("unknown link rule %q", r.Links)
	}
	switch r.OnError {
	case ErrorsSkip, ErrorsAsk, ErrorsStop:
	default:
		return fmt.Errorf("unknown error rule %q", r.OnError)
	}
	if r.SpeedLimit < 0 || r.BufferSize < 0 || r.Retries < 0 || r.WipePasses < 0 {
		return errors.New("negative numbers are not allowed in a request")
	}
	return nil
}

func (q *Queue) get(id int) *job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.jobs[id]
}

// laneDone removes a finished or cancelled job from the lane and starts the next one. The next job is marked started
// while the lock is held, so Cancel and RunNow never race with it.
func (q *Queue) laneDone(j *job) {
	q.mu.Lock()
	var next []*job
	for i, x := range q.lane {
		if x == j {
			q.lane = append(q.lane[:i], q.lane[i+1:]...)
			break
		}
	}
	for _, x := range q.lane {
		if !q.laneFree() {
			break
		}
		if x.started.CompareAndSwap(false, true) {
			next = append(next, x)
		}
	}
	q.mu.Unlock()
	for _, x := range next {
		go x.run()
	}
}

// laneFree says whether one more queued job may start now (q.mu held): fewer lane jobs are running than Lanes allows,
// and AutoStart does not hold them back. The job being added is already in the lane, not yet started.
func (q *Queue) laneFree() bool {
	if q.AutoStart != nil && !q.AutoStart() {
		return false
	}
	lanes := 1
	if q.Lanes != nil {
		lanes = max(1, q.Lanes())
	}
	running := 0
	for _, x := range q.lane {
		if x.started.Load() {
			running++
		}
	}
	return running < lanes
}

// Cancel stops a job; files done so far stay done, and a half-written file is removed. It returns false for an
// unknown or finished job.
func (q *Queue) Cancel(id int) bool {
	j := q.get(id)
	if j == nil || j.finished() {
		return false
	}
	j.cancel()
	q.mu.Lock()
	never := j.started.CompareAndSwap(false, true) // a queued job that never ran ends here, without a goroutine
	if never {
		for i, x := range q.lane {
			if x == j {
				q.lane = append(q.lane[:i], q.lane[i+1:]...)
				break
			}
		}
	}
	q.mu.Unlock()
	if never {
		j.finish()
	}
	return true
}

// Pause holds a job before its next block of bytes; Resume lets it go on. Both return false for an unknown or
// finished job. A queued job that is paused waits when its turn comes.
func (q *Queue) Pause(id int) bool {
	j := q.get(id)
	if j == nil || j.finished() {
		return false
	}
	j.setPaused(true)
	return true
}

// Resume undoes Pause.
func (q *Queue) Resume(id int) bool {
	j := q.get(id)
	if j == nil || j.finished() {
		return false
	}
	j.setPaused(false)
	return true
}

// SetSpeedLimit changes the speed limit of a running job, in bytes per second (0: no limit).
func (q *Queue) SetSpeedLimit(id int, bytesPerSecond int64) bool {
	j := q.get(id)
	if j == nil || j.finished() || bytesPerSecond < 0 {
		return false
	}
	j.limit.Store(bytesPerSecond)
	j.update(func(p *Progress) { p.SpeedLimit = bytesPerSecond })
	j.tick(true)
	return true
}

// RunNow takes a waiting job out of the queue lane and starts it at once, beside the others.
func (q *Queue) RunNow(id int) bool {
	j := q.get(id)
	if j == nil || j.finished() {
		return false
	}
	q.mu.Lock()
	if !j.started.CompareAndSwap(false, true) {
		q.mu.Unlock()
		return false
	}
	for i, x := range q.lane {
		if x == j {
			q.lane = append(q.lane[:i], q.lane[i+1:]...)
			break
		}
	}
	q.mu.Unlock()
	j.update(func(p *Progress) { p.Queued = false; p.State = StateRunning })
	go j.run()
	return true
}

// Answer answers the open question of a job.
func (q *Queue) Answer(id int, a Answer) error {
	j := q.get(id)
	if j == nil {
		return fmt.Errorf("there is no job %d", id)
	}
	j.mu.Lock()
	asking := j.p.Ask != nil
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
		if j.finished() {
			delete(q.jobs, id)
		}
	}
}

// CancelAll cancels every job that hasn't finished (used when the app quits).
func (q *Queue) CancelAll() {
	for _, p := range q.List() {
		q.Cancel(p.ID)
	}
}

// SkipErrors tells a running job to stop asking Retry / Skip when a file fails: from the next error on, the error is
// noted (Progress.Failed) and the job goes on with the rest. The app calls it when a job is sent to the background
// with "Skip errors in background" on. A question already on screen stays until it is answered. It returns false
// when there is no such job, or it has ended.
func (q *Queue) SkipErrors(id int) bool {
	j := q.get(id)
	if j == nil || j.finished() {
		return false
	}
	j.skipErrs.Store(true)
	return true
}
