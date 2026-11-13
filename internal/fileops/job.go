package fileops

// A job is one request being carried out. This file holds what every kind of job shares: the progress record and how
// often it is reported, the speed and time-left estimate, pausing, the speed limit, asking the user, the error policy
// (retry / skip / ask / stop) and the final statistics. The work itself is in copy.go and delete.go.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type job struct {
	q       *Queue
	req     Request
	ctx     context.Context
	cancel  context.CancelFunc
	answers chan Answer
	done    chan struct{}
	started atomic.Bool
	filter  Filter

	mu        sync.Mutex // guards p and the fields below it
	p         Progress
	last      time.Time // last progress event
	samples   []sample  // recent (time, bytes) pairs for the speed
	paused    bool
	resume    chan struct{} // closed by Resume
	workStart time.Time     // when the current stretch of work (between pauses) began
	worked    time.Duration // work before that stretch

	// decisions that "… all" answers make for the rest of the job; only the job's goroutine touches them
	policy     Policy
	roPolicy   Policy
	errPolicy  ErrorPolicy
	delDirsAll bool // AskDeleteDir answered All
	delROAll   int  // AskDeleteReadOnly answered: 0 ask, 1 all yes, -1 all skip
	delDirSkip bool // AskDeleteDir answered Skip all

	limit    atomic.Int64 // bytes per second, 0 = none
	limStart time.Time    // speed limit bookkeeping: start of the current measuring window
	limBytes int64        // bytes written since limStart
	limRate  int64        // the limit the window was started with
}

// errorsSkipQuiet is the error policy after "Skip all": failed files are counted as skipped without asking.
const errorsSkipQuiet ErrorPolicy = "skip_quiet"

type sample struct {
	t time.Time
	b int64
}

func (j *job) finished() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

func (j *job) run() {
	j.update(func(p *Progress) {
		if p.State == StateQueued {
			p.State = StateRunning
		}
		if j.paused {
			p.State = StatePaused
		}
		j.workStart = time.Now()
	})
	j.tick(true)
	if j.gate() {
		switch j.req.Kind {
		case Copy:
			j.copyAll(false)
		case Move:
			j.copyAll(true)
		case Delete:
			j.deleteAll(false)
		case Wipe:
			j.deleteAll(true)
		case Trash:
			j.trashAll()
		}
	}
	if j.req.Queued {
		j.q.laneDone(j)
	}
	j.finish()
}

// finish sets the final state, emits EventDone and releases Wait.
func (j *job) finish() {
	j.update(func(p *Progress) {
		p.Ask, p.Current, p.CurrentSize, p.CurrentDone = nil, "", 0, 0
		if !j.workStart.IsZero() && !j.paused {
			j.worked += time.Since(j.workStart)
		}
		p.Elapsed = j.worked.Milliseconds()
		p.ETA = 0
		switch {
		case j.ctx.Err() != nil:
			p.State = StateCancelled
		case len(p.Errors) > 0:
			p.State = StateFailed
		default:
			p.State = StateDone
		}
		p.Summary = summary(*p)
	})
	close(j.done)
	j.log("done", "", "", nil)
	j.q.Emit(EventDone, j.snapshot())
}

func summary(p Progress) string {
	verb := map[Kind]string{Copy: "Copied", Move: "Moved", Delete: "Deleted", Trash: "Moved to the trash", Wipe: "Wiped"}[p.Kind]
	s := fmt.Sprintf("%s %d of %d files", verb, p.FilesDone, p.Files)
	if p.Kind == Copy || p.Kind == Move || p.Kind == Wipe {
		s += fmt.Sprintf(" (%s)", formatBytes(p.BytesDone))
	}
	s += fmt.Sprintf(" in %s", (time.Duration(p.Elapsed) * time.Millisecond).Round(100*time.Millisecond))
	if p.Skipped > 0 {
		s += fmt.Sprintf(", %d skipped", p.Skipped)
	}
	if p.Verified > 0 {
		s += fmt.Sprintf(", %d verified", p.Verified)
	}
	if n := len(p.Errors); n > 0 {
		s += fmt.Sprintf(", %d errors", n)
	}
	if p.State == StateCancelled {
		s += ", cancelled"
	}
	return s + "."
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	f, i := float64(n), 0
	for f >= unit && i < 4 {
		f /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", f, []string{"", "KB", "MB", "GB", "TB"}[i])
}

func (j *job) snapshot() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	p := j.p
	p.Errors = append([]string(nil), j.p.Errors...)
	if j.p.Ask != nil {
		c := *j.p.Ask
		c.Choices = append([]Choice(nil), c.Choices...)
		p.Ask = &c
	}
	return p
}

// tick refreshes speed and time left and reports progress, at most every Interval (force: now).
func (j *job) tick(force bool) {
	j.mu.Lock()
	now := time.Now()
	if !force && now.Sub(j.last) < j.q.Interval {
		j.mu.Unlock()
		return
	}
	j.last = now
	j.measure(now)
	j.mu.Unlock()
	j.q.Emit(EventProgress, j.snapshot())
}

// measure updates Speed, ETA and Elapsed from a window of the last three seconds. j.mu must be held.
func (j *job) measure(now time.Time) {
	elapsed := j.worked
	if !j.paused && !j.workStart.IsZero() {
		elapsed += now.Sub(j.workStart)
	}
	j.p.Elapsed = elapsed.Milliseconds()
	if j.paused {
		j.p.Speed = 0
		j.p.ETA = -1
		return
	}
	j.samples = append(j.samples, sample{now, j.p.BytesDone})
	cut := 0
	for cut < len(j.samples)-2 && now.Sub(j.samples[cut].t) > 3*time.Second {
		cut++
	}
	j.samples = j.samples[cut:]
	first := j.samples[0]
	if dt := now.Sub(first.t); dt > 50*time.Millisecond {
		j.p.Speed = int64(float64(j.p.BytesDone-first.b) / dt.Seconds())
	}
	switch {
	case j.p.Bytes <= j.p.BytesDone:
		j.p.ETA = 0
	case j.p.Speed > 0:
		j.p.ETA = (j.p.Bytes - j.p.BytesDone + j.p.Speed - 1) / j.p.Speed
	default:
		j.p.ETA = -1
	}
}

func (j *job) update(f func(p *Progress)) {
	j.mu.Lock()
	f(&j.p)
	j.mu.Unlock()
}

// addBytes counts n bytes as done, for the job and for the current file.
func (j *job) addBytes(n int64) {
	j.mu.Lock()
	j.p.BytesDone += n
	j.p.CurrentDone += n
	j.mu.Unlock()
}

// fail notes an error for the UI (at most maxErrors of them) and the log.
func (j *job) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	j.mu.Lock()
	if len(j.p.Errors) < maxErrors {
		j.p.Errors = append(j.p.Errors, msg)
	}
	j.mu.Unlock()
	j.logMsg("error", msg)
}

func (j *job) skipped() { j.update(func(p *Progress) { p.Skipped++ }) }

// setPaused pauses or resumes; the job notices at its next gate.
func (j *job) setPaused(on bool) {
	j.mu.Lock()
	if j.paused == on {
		j.mu.Unlock()
		return
	}
	j.paused = on
	now := time.Now()
	if on {
		if !j.workStart.IsZero() {
			j.worked += now.Sub(j.workStart)
		}
		if j.p.State == StateRunning {
			j.p.State = StatePaused
		}
	} else {
		close(j.resume)
		j.resume = make(chan struct{})
		if !j.workStart.IsZero() {
			j.workStart = now
		}
		if j.p.State == StatePaused {
			j.p.State = StateRunning
		}
		j.samples = nil // the speed starts fresh after a pause
	}
	j.mu.Unlock()
	if !on {
		j.limReset()
	}
	j.tick(true)
}

// gate waits while the job is paused. It returns false when the job was cancelled.
func (j *job) gate() bool {
	for {
		j.mu.Lock()
		paused, ch := j.paused, j.resume
		j.mu.Unlock()
		if !paused {
			return j.ctx.Err() == nil
		}
		select {
		case <-ch:
		case <-j.ctx.Done():
			return false
		}
	}
}

func (j *job) limReset() { j.limStart, j.limBytes, j.limRate = time.Time{}, 0, 0 }

// throttle sleeps as long as needed to keep the job under its speed limit after n more bytes. The window restarts
// whenever the limit changes, so a new limit takes effect at once.
func (j *job) throttle(n int) {
	rate := j.limit.Load()
	if rate <= 0 {
		j.limReset()
		return
	}
	now := time.Now()
	if rate != j.limRate || j.limStart.IsZero() || now.Sub(j.limStart) > 10*time.Second {
		j.limStart, j.limBytes, j.limRate = now, 0, rate
	}
	j.limBytes += int64(n)
	due := j.limStart.Add(time.Duration(float64(j.limBytes) / float64(rate) * float64(time.Second)))
	if wait := due.Sub(now); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-j.ctx.Done():
		}
	}
}

// chunk is the size of one read/write: the buffer, made smaller under a speed limit so progress stays smooth.
func (j *job) chunk(buf int) int {
	if rate := j.limit.Load(); rate > 0 {
		c := int(min(rate/8, int64(buf)))
		return max(c, 4096)
	}
	return buf
}

// ask puts a question to the user and waits for the answer (or a cancel).
func (j *job) ask(c Conflict) Answer {
	var prev string
	j.update(func(p *Progress) { prev, p.State, p.Ask = p.State, StateAsking, &c })
	snap := j.snapshot()
	j.q.Emit(EventAsk, snap)
	var a Answer
	if f := j.q.AskFunc; f != nil {
		a = f(snap)
		if j.ctx.Err() != nil {
			a = Answer{Choice: ChoiceCancel}
		}
	} else {
		select {
		case a = <-j.answers:
		case <-j.ctx.Done():
			a = Answer{Choice: ChoiceCancel}
		}
	}
	j.update(func(p *Progress) {
		p.Ask = nil
		if prev == StateAsking || prev == "" {
			prev = StateRunning
		}
		p.State = prev
		if j.paused {
			p.State = StatePaused
		}
	})
	if a.Choice == ChoiceCancel {
		j.cancel()
	}
	j.tick(true)
	return a
}

// try runs op until it works, applying the automatic retries and then the error policy. what is the sentence used in
// the error ("Can't copy %s"); extra are additional answers offered when the user is asked (ChoiceDelete for the
// trash). It returns the error left (nil: op worked) and, when the user picked an extra answer, that answer.
func (j *job) try(what, src, dst string, extra []Choice, op func() error) (error, Choice) {
	retries := j.req.Retries
	for {
		err := op()
		if err == nil {
			return nil, ""
		}
		if j.ctx.Err() != nil {
			return err, ChoiceCancel
		}
		if retries > 0 {
			retries--
			time.Sleep(200 * time.Millisecond)
			continue
		}
		switch j.errPolicy {
		case ErrorsAsk:
			choices := append([]Choice{ChoiceRetry, ChoiceSkip, ChoiceSkipAll}, extra...)
			a := j.ask(Conflict{Type: AskError, Source: src, Target: dst, Error: fmt.Sprintf("%s: %s.", what, plainErr(err)),
				Choices: append(choices, ChoiceCancel)})
			switch a.Choice {
			case ChoiceRetry:
				retries = j.req.Retries
				continue
			case ChoiceSkipAll:
				j.errPolicy = errorsSkipQuiet
			case ChoiceCancel:
				return err, ChoiceCancel
			default:
				for _, x := range extra {
					if a.Choice == x {
						return err, x
					}
				}
			}
			j.skipped()
			j.log("skip", src, dst, err)
			return err, ChoiceSkip
		case errorsSkipQuiet:
			j.skipped()
			j.log("skip", src, dst, err)
			return err, ChoiceSkip
		case ErrorsStop:
			j.fail("%s: %s.", what, plainErr(err))
			j.cancel()
			return err, ChoiceCancel
		default:
			j.fail("%s: %s.", what, plainErr(err))
			return err, ""
		}
	}
}

func (j *job) log(action, src, dst string, err error) {
	if j.q.Log == nil {
		return
	}
	e := Entry{Time: time.Now(), Job: j.p.ID, Kind: j.req.Kind, Action: action, Source: src, Target: dst}
	if err != nil {
		e.Err = plainErr(err)
	}
	if action == "done" {
		e.Err = ""
		e.Source = j.snapshot().Summary
	}
	j.q.Log(e)
}

func (j *job) logMsg(action, msg string) {
	if j.q.Log != nil {
		j.q.Log(Entry{Time: time.Now(), Job: j.p.ID, Kind: j.req.Kind, Action: action, Err: msg})
	}
}

// at marks the start of work on one file: the hook for tests, the gate for pauses and the current-file fields.
func (j *job) at(path string, size int64) bool {
	if !j.gate() {
		return false
	}
	if j.q.beforeFile != nil {
		j.q.beforeFile(path)
	}
	if j.ctx.Err() != nil {
		return false
	}
	j.update(func(p *Progress) { p.Current, p.CurrentSize, p.CurrentDone = path, size, 0 })
	return true
}
