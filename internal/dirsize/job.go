// A Job runs a calculation in the background, which is how the UI uses this package: Space on a folder starts a job
// and the panel keeps reacting to keys, the Ctrl+L dialog polls Progress for its counters, and Esc or the Cancel
// button calls Cancel. Wait returns the same Summary and error that CalculateAll would. A job owns its own context,
// derived from the one given to Start, so cancelling one job never disturbs another one running on the other panel.
package dirsize

import (
	"context"
	"sync"
)

// Job is a calculation running in the background.
type Job struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu   sync.Mutex
	last Progress
	sum  Summary
	err  error
}

// Start begins calculating paths in the background with the same rules as CalculateAll. The callbacks in opts are
// still called (from the job's goroutines); Progress can also be polled with Job.Progress.
func Start(ctx context.Context, paths []string, opts Options) *Job {
	ctx, cancel := context.WithCancel(ctx)
	j := &Job{cancel: cancel, done: make(chan struct{})}
	j.last = Progress{ItemsTotal: len(paths)}
	user := opts.Progress
	opts.Progress = func(p Progress) {
		j.mu.Lock()
		j.last = p
		j.mu.Unlock()
		if user != nil {
			user(p)
		}
	}
	go func() {
		defer close(j.done)
		defer cancel()
		s, err := CalculateAll(ctx, paths, opts)
		j.mu.Lock()
		j.sum, j.err = s, err
		j.mu.Unlock()
	}()
	return j
}

// Cancel stops the job; Wait then returns the partial result and an error wrapping ErrCanceled. Calling it after the
// job finished does nothing.
func (j *Job) Cancel() { j.cancel() }

// Done is closed when the job has finished, successfully or not.
func (j *Job) Done() <-chan struct{} { return j.done }

// Wait blocks until the job has finished and returns its result.
func (j *Job) Wait() (Summary, error) {
	<-j.done
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.sum, j.err
}

// Progress returns the latest progress snapshot (updated every Options.ProgressInterval and at the end).
func (j *Job) Progress() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.last
}
