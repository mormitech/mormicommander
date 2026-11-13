// This file wraps a search in a background job for the UI (F-538, F-543). The Find
// Files dialog starts a search, shows results as they arrive, updates a status line
// and offers a Stop button; Total Commander can even keep several searches running in
// separate windows. Job runs Query.Run on its own goroutine, delivers results through
// a channel, keeps the latest progress and the per-path errors for display, and stops
// cleanly when Stop is called or its context ends. The channel is closed when the
// search is over, so a UI loop can simply range over it.

package find

import (
	"context"
	"sync"
)

// PathError is one error reported during a search.
type PathError struct {
	Path string `json:"path"`
	Err  string `json:"err"`
}

// maxJobErrors caps how many errors a Job keeps, so a search over a huge unreadable
// tree cannot eat memory; the count in Progress.Errors stays exact.
const maxJobErrors = 1000

// Job is a search running in the background.
type Job struct {
	results chan Result
	cancel  context.CancelFunc
	done    chan struct{}

	mu     sync.Mutex
	prog   Progress
	errs   []PathError
	stats  Stats
	runErr error
}

// Start compiles the options and starts searching. Results arrive on Results().
func Start(ctx context.Context, o Options) (*Job, error) {
	q, err := Compile(o)
	if err != nil {
		return nil, err
	}
	return q.Start(ctx), nil
}

// Start runs a compiled query in the background.
func (q *Query) Start(ctx context.Context) *Job {
	ctx, cancel := context.WithCancel(ctx)
	j := &Job{results: make(chan Result, 256), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(j.done)
		defer close(j.results)
		defer cancel()
		st, err := q.Run(ctx, Handler{
			Result: func(r Result) error {
				select {
				case j.results <- r:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
			Error: func(p string, e error) {
				j.mu.Lock()
				if len(j.errs) < maxJobErrors {
					j.errs = append(j.errs, PathError{p, e.Error()})
				}
				j.mu.Unlock()
			},
			Progress: func(p Progress) {
				j.mu.Lock()
				j.prog = p
				j.mu.Unlock()
			},
		})
		j.mu.Lock()
		j.stats, j.runErr = st, err
		j.prog = st.Progress
		j.mu.Unlock()
	}()
	return j
}

// Results delivers hits as they are found; it is closed when the search ends.
func (j *Job) Results() <-chan Result { return j.results }

// Stop cancels the search. Results already found stay valid. Safe to call twice.
func (j *Job) Stop() { j.cancel() }

// Done is closed when the search has ended.
func (j *Job) Done() <-chan struct{} { return j.done }

// Wait blocks until the search ends and returns its summary. A search ended by Stop
// returns context.Canceled. Results not read from the channel are discarded, so a
// caller that only wants the summary does not deadlock the search.
func (j *Job) Wait() (Stats, error) {
	for {
		select {
		case <-j.done:
			j.mu.Lock()
			defer j.mu.Unlock()
			return j.stats, j.runErr
		case _, ok := <-j.results:
			if !ok {
				<-j.done
			}
		}
	}
}

// Progress returns the latest progress snapshot.
func (j *Job) Progress() Progress {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.prog
}

// Errors returns the errors reported so far (at most the first 1000).
func (j *Job) Errors() []PathError {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]PathError(nil), j.errs...)
}
