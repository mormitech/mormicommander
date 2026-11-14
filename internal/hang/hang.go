// Package hang keeps the app responsive when a file system stops answering. A folder on an SMB or NFS share whose
// server was unplugged, or a removable disk that went away without being ejected, can block a system call (stat,
// readdir, statfs) for minutes, and Go cannot interrupt a goroutine that is stuck in the kernel. So instead of waiting,
// callers run the risky call in a goroutine of its own and stop waiting for it: Do returns as soon as the caller's
// context ends (Esc, a newer listing), and WithTimeout returns after a fixed time. The stuck goroutine is left behind
// and its late answer is thrown away.
//
// Leaving goroutines behind is only safe if they cannot pile up. A Guard counts the calls still stuck per key (a path)
// and refuses to start more than a few at once for the same key, answering ErrNotResponding at once instead.
package hang

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotResponding is the answer when a call did not finish in time, or when too many calls for the same path are
// still stuck.
var ErrNotResponding = errors.New("not responding")

// Do runs fn in its own goroutine and returns its answer, or ctx.Err() as soon as ctx ends, whichever comes first.
// fn gets the same ctx, so a call that does watch the context (a listing that checks it between entries) stops too.
func Do[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	type answer struct {
		v   T
		err error
	}
	done := make(chan answer, 1) // buffered: a late answer never blocks the abandoned goroutine
	go func() {
		v, err := fn(ctx)
		done <- answer{v, err}
	}()
	select {
	case a := <-done:
		return a.v, a.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// DoGrace is Do for calls that return a useful partial answer when their context ends (a count that was stopped): once
// ctx ends it still waits up to grace for fn's own answer, and only then gives up with ctx.Err().
func DoGrace[T any](ctx context.Context, grace time.Duration, fn func(context.Context) (T, error)) (T, error) {
	type answer struct {
		v   T
		err error
	}
	done := make(chan answer, 1)
	go func() {
		v, err := fn(ctx)
		done <- answer{v, err}
	}()
	select {
	case a := <-done:
		return a.v, a.err
	case <-ctx.Done():
	}
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case a := <-done:
		return a.v, a.err
	case <-t.C:
		var zero T
		return zero, ctx.Err()
	}
}

// WithTimeout runs fn and gives up after d, answering ErrNotResponding.
func WithTimeout[T any](d time.Duration, fn func() (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	v, err := Do(ctx, func(context.Context) (T, error) { return fn() })
	if errors.Is(err, context.DeadlineExceeded) {
		return v, ErrNotResponding
	}
	return v, err
}

// Guard limits how many calls for the same key may be running at once. The zero value is ready to use and allows
// DefaultLimit calls per key.
type Guard struct {
	Limit   int // 0 means DefaultLimit
	mu      sync.Mutex
	running map[string]int
}

// DefaultLimit is how many calls for one key a Guard lets run at the same time.
const DefaultLimit = 4

// Guarded is Do for one key of g: when Limit calls for key are still running (stuck on a dead share), it answers
// ErrNotResponding at once instead of starting another goroutine that would get stuck too.
func Guarded[T any](g *Guard, ctx context.Context, key string, fn func(context.Context) (T, error)) (T, error) {
	if !g.enter(key) {
		var zero T
		return zero, ErrNotResponding
	}
	return Do(ctx, func(c context.Context) (T, error) {
		defer g.leave(key) // runs when fn really finishes, not when the caller stops waiting
		return fn(c)
	})
}

// Running is how many calls for key have started and not finished yet.
func (g *Guard) Running(key string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running[key]
}

func (g *Guard) enter(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	limit := g.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if g.running[key] >= limit {
		return false
	}
	if g.running == nil {
		g.running = map[string]int{}
	}
	g.running[key]++
	return true
}

func (g *Guard) leave(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[key] <= 1 {
		delete(g.running, key)
	} else {
		g.running[key]--
	}
}
