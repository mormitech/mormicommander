// Tests of waitUntil, the bounded wait control mode uses for things the system does a moment after it is asked (the
// About window appearing after the "about" command), so a command answers once the result can be seen.
package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitUntilHoldsLater(t *testing.T) {
	var n atomic.Int32
	start := time.Now()
	ok := waitUntil(func() bool { return n.Add(1) >= 3 }, 10*time.Millisecond, 2*time.Second, 5*time.Millisecond)
	if !ok || n.Load() != 3 {
		t.Fatalf("ok=%v after %d checks", ok, n.Load())
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Fatal("answered before the minimum wait")
	}
}

func TestWaitUntilGivesUp(t *testing.T) {
	start := time.Now()
	if waitUntil(func() bool { return false }, 0, 40*time.Millisecond, 5*time.Millisecond) {
		t.Fatal("a condition that never holds was reported as held")
	}
	if d := time.Since(start); d < 40*time.Millisecond || d > time.Second {
		t.Fatalf("gave up after %v", d)
	}
}

func TestWaitUntilMinimumEvenWhenTrue(t *testing.T) {
	start := time.Now()
	if !waitUntil(func() bool { return true }, 30*time.Millisecond, 30*time.Millisecond, 5*time.Millisecond) {
		t.Fatal("a true condition was reported as not held")
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatal("the minimum wait was skipped")
	}
}
