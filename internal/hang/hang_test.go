// Tests of the hang helpers with calls that block on a channel, standing in for a system call stuck on a dead share.
package hang

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoReturnsAnswer(t *testing.T) {
	v, err := Do(context.Background(), func(context.Context) (int, error) { return 7, nil })
	if v != 7 || err != nil {
		t.Fatalf("Do = %v, %v", v, err)
	}
}

func TestDoStopsWaitingWhenContextEnds(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := Do(ctx, func(context.Context) (int, error) { <-block; return 1, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("Do waited for the stuck call")
	}
}

func TestWithTimeout(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	_, err := WithTimeout(30*time.Millisecond, func() (int, error) { <-block; return 1, nil })
	if !errors.Is(err, ErrNotResponding) {
		t.Fatalf("err = %v, want ErrNotResponding", err)
	}
	v, err := WithTimeout(time.Second, func() (int, error) { return 3, nil })
	if v != 3 || err != nil {
		t.Fatalf("fast call = %v, %v", v, err)
	}
	want := errors.New("boom")
	if _, err := WithTimeout(time.Second, func() (int, error) { return 0, want }); err != want {
		t.Fatalf("error not passed through: %v", err)
	}
}

func TestGuardLimitsStuckCalls(t *testing.T) {
	g := &Guard{Limit: 2}
	block := make(chan struct{})
	stuck := func(context.Context) (int, error) { <-block; return 1, nil }
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		if _, err := Guarded(g, ctx, "/net/share", stuck); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("call %d: %v", i, err)
		}
		cancel()
	}
	if n := g.Running("/net/share"); n != 2 {
		t.Fatalf("running = %d, want 2", n)
	}
	// a third call for the same path is refused at once
	if _, err := Guarded(g, context.Background(), "/net/share", stuck); !errors.Is(err, ErrNotResponding) {
		t.Fatalf("third call: %v", err)
	}
	// another path is not affected
	if v, err := Guarded(g, context.Background(), "/other", func(context.Context) (int, error) { return 5, nil }); v != 5 || err != nil {
		t.Fatalf("other path: %v, %v", v, err)
	}
	// once the stuck calls finish, the path is free again
	close(block)
	deadline := time.Now().Add(2 * time.Second)
	for g.Running("/net/share") > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if v, err := Guarded(g, context.Background(), "/net/share", func(context.Context) (int, error) { return 9, nil }); v != 9 || err != nil {
		t.Fatalf("after recovery: %v, %v", v, err)
	}
}

func TestDoGraceKeepsPartialAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// a call that notices the end of its context and returns what it has gets through
	v, err := DoGrace(ctx, time.Second, func(c context.Context) (string, error) { <-c.Done(); return "partial", c.Err() })
	if v != "partial" || !errors.Is(err, context.Canceled) {
		t.Fatalf("DoGrace = %q, %v", v, err)
	}
	// a call stuck for good is given up after the grace time
	block := make(chan struct{})
	defer close(block)
	start := time.Now()
	if _, err := DoGrace(ctx, 30*time.Millisecond, func(context.Context) (string, error) { <-block; return "", nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("stuck call: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("DoGrace waited for the stuck call")
	}
}
