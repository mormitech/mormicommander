// This file is the background worker pool behind the thumbnails view. Total
// Commander shows a folder immediately with icons and fills in the previews
// as they are generated, starting with the ones on screen; when the user
// leaves the folder the remaining work is dropped. The Pool mirrors that: the
// panel submits a Batch with all paths of the folder, results arrive through
// a callback one by one, Prioritize moves the paths that just scrolled into
// view to the front of the queue, and Cancel (or cancelling the context)
// throws away whatever has not started yet. A fixed number of workers keeps
// CPU and disk use bounded no matter how many folders are opened.

package thumbs

import (
	"context"
	"runtime"
	"sync"
)

// Result is delivered once per submitted path that was not cancelled.
type Result struct {
	Index int    // position of Path in the submitted slice
	Path  string // the submitted path
	Thumb *Thumb // nil when Err is set
	Err   error
}

// Pool runs thumbnail jobs on a fixed set of goroutines.
type Pool struct {
	t *Thumbnailer

	mu     sync.Mutex
	cond   *sync.Cond
	queue  []*job
	closed bool
	wg     sync.WaitGroup
}

type job struct {
	b    *Batch
	idx  int
	path string
}

// NewPool starts workers goroutines (<= 0 means half the CPUs, at least 1)
// that generate thumbnails with t.
func NewPool(t *Thumbnailer, workers int) *Pool {
	if workers <= 0 {
		workers = max(1, runtime.NumCPU()/2)
	}
	p := &Pool{t: t}
	p.cond = sync.NewCond(&p.mu)
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

// Thumbnailer returns the Thumbnailer the pool uses.
func (p *Pool) Thumbnailer() *Thumbnailer { return p.t }

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		for len(p.queue) == 0 && !p.closed {
			p.cond.Wait()
		}
		if len(p.queue) == 0 {
			p.mu.Unlock()
			return
		}
		j := p.queue[0]
		p.queue[0] = nil
		p.queue = p.queue[1:]
		p.mu.Unlock()
		j.b.run(j)
	}
}

// Batch is one group of submitted paths, typically one folder listing.
type Batch struct {
	p      *Pool
	ctx    context.Context
	cancel context.CancelFunc
	fn     func(Result)
	wg     sync.WaitGroup
	done   chan struct{}
}

// Submit queues paths for thumbnail generation and returns at once. fn is
// called from worker goroutines, possibly concurrently, once per path in
// roughly submission order; it is not called for paths cancelled before or
// while they were being generated. Submitting to a closed pool returns a
// batch that is already done.
func (p *Pool) Submit(ctx context.Context, paths []string, fn func(Result)) *Batch {
	bctx, cancel := context.WithCancel(ctx)
	b := &Batch{p: p, ctx: bctx, cancel: cancel, fn: fn, done: make(chan struct{})}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		cancel()
		close(b.done)
		return b
	}
	b.wg.Add(len(paths))
	for i, path := range paths {
		p.queue = append(p.queue, &job{b: b, idx: i, path: path})
	}
	p.mu.Unlock()
	p.cond.Broadcast()

	go func() {
		<-bctx.Done()
		p.purge(b)
	}()
	go func() {
		b.wg.Wait()
		close(b.done)
		cancel() // releases the purge goroutine
	}()
	return b
}

func (b *Batch) run(j *job) {
	defer b.wg.Done()
	if b.ctx.Err() != nil {
		return
	}
	th, err := b.p.t.Thumbnail(b.ctx, j.path)
	if b.ctx.Err() != nil {
		return
	}
	if b.fn != nil {
		b.fn(Result{Index: j.idx, Path: j.path, Thumb: th, Err: err})
	}
}

// purge drops the batch's queued (not yet started) jobs.
func (p *Pool) purge(b *Batch) {
	p.mu.Lock()
	kept := p.queue[:0]
	var dropped int
	for _, j := range p.queue {
		if j.b == b {
			dropped++
			continue
		}
		kept = append(kept, j)
	}
	for i := len(kept); i < len(p.queue); i++ {
		p.queue[i] = nil
	}
	p.queue = kept
	p.mu.Unlock()
	for i := 0; i < dropped; i++ {
		b.wg.Done()
	}
}

// Cancel drops the batch's pending work; jobs already running finish but
// their results are not delivered. Cancel never blocks.
func (b *Batch) Cancel() { b.cancel() }

// Wait blocks until every job of the batch has finished or been dropped.
func (b *Batch) Wait() { <-b.done }

// Done is closed when the batch has finished.
func (b *Batch) Done() <-chan struct{} { return b.done }

// Prioritize moves the given paths of this batch, in the given order, to the
// front of the pool queue: call it with the items that just became visible.
// Paths not queued any more (finished, running, or unknown) are ignored.
func (b *Batch) Prioritize(paths ...string) {
	if len(paths) == 0 {
		return
	}
	rank := make(map[string]int, len(paths))
	for i, p := range paths {
		if _, dup := rank[p]; !dup {
			rank[p] = i
		}
	}
	p := b.p
	p.mu.Lock()
	defer p.mu.Unlock()
	front := make([]*job, len(paths))
	rest := make([]*job, 0, len(p.queue))
	for _, j := range p.queue {
		if r, ok := rank[j.path]; ok && j.b == b && front[r] == nil {
			front[r] = j
			continue
		}
		rest = append(rest, j)
	}
	q := make([]*job, 0, len(p.queue))
	for _, j := range front {
		if j != nil {
			q = append(q, j)
		}
	}
	p.queue = append(q, rest...)
}

// Pending returns the number of queued jobs across all batches.
func (p *Pool) Pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

// Close drops all queued jobs, waits for running ones to finish and stops
// the workers. The pool cannot be used afterwards.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	q := p.queue
	p.queue = nil
	p.mu.Unlock()
	p.cond.Broadcast()
	for _, j := range q {
		j.b.cancel()
		j.b.wg.Done()
	}
	p.wg.Wait()
}
