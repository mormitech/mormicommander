// Tests for the background pool: delivery of every result, cancellation of
// pending work, prioritising visible items, and shutting down.

package thumbs

import (
	"context"
	"fmt"
	"image"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestPoolDeliversAll(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 20; i++ {
		data := encodeAs(t, "png", halves(60+i, 30))
		if i%5 == 4 {
			data = []byte("not an image")
		}
		paths = append(paths, writeFile(t, dir, fmt.Sprintf("f%02d.png", i), data))
	}
	tn := newTestThumbnailer(t, Options{Width: 32, Height: 32})
	p := NewPool(tn, 4)
	defer p.Close()
	var mu sync.Mutex
	got := map[int]Result{}
	b := p.Submit(context.Background(), paths, func(r Result) {
		mu.Lock()
		got[r.Index] = r
		mu.Unlock()
	})
	b.Wait()
	if len(got) != len(paths) {
		t.Fatalf("got %d results, want %d", len(got), len(paths))
	}
	for i, r := range got {
		if r.Path != paths[i] {
			t.Errorf("index %d path %s", i, r.Path)
		}
		if (i%5 == 4) != (r.Err != nil) || (r.Err == nil) != (r.Thumb != nil) {
			t.Errorf("index %d: thumb %v err %v", i, r.Thumb != nil, r.Err)
		}
	}
	select {
	case <-b.Done():
	default:
		t.Error("Done not closed after Wait")
	}
	if p.Thumbnailer() != tn {
		t.Error("Thumbnailer accessor")
	}
}

// gatedThumbnailer makes every file go through a fake system thumbnailer
// that records the order of calls and blocks until released.
func gatedThumbnailer(t *testing.T) (*Thumbnailer, chan struct{}, func() []string) {
	tn := newTestThumbnailer(t, Options{UseSystem: true})
	gate := make(chan struct{})
	var mu sync.Mutex
	var order []string
	tn.SetSystem(func(ctx context.Context, path string, w, h int) (image.Image, error) {
		mu.Lock()
		order = append(order, path)
		mu.Unlock()
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return halves(4, 4), nil
	})
	return tn, gate, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), order...)
	}
}

func fakeFiles(t *testing.T, n int) []string {
	dir := t.TempDir()
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, writeFile(t, dir, fmt.Sprintf("v%d.mov", i), []byte{byte(i)}))
	}
	return out
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPoolCancel(t *testing.T) {
	tn, gate, order := gatedThumbnailer(t)
	p := NewPool(tn, 1)
	defer p.Close()
	paths := fakeFiles(t, 10)
	var mu sync.Mutex
	var delivered int
	b := p.Submit(context.Background(), paths, func(Result) {
		mu.Lock()
		delivered++
		mu.Unlock()
	})
	waitFor(t, func() bool { return len(order()) == 1 })
	b.Cancel()
	b.Wait()
	close(gate)
	if n := len(order()); n != 1 {
		t.Errorf("%d jobs started, want 1", n)
	}
	if delivered != 0 {
		t.Errorf("%d results delivered after cancel", delivered)
	}
	if p.Pending() != 0 {
		t.Errorf("pending %d", p.Pending())
	}
}

func TestPoolParentContextCancel(t *testing.T) {
	tn, gate, order := gatedThumbnailer(t)
	defer close(gate)
	p := NewPool(tn, 1)
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	b := p.Submit(ctx, fakeFiles(t, 5), nil)
	waitFor(t, func() bool { return len(order()) == 1 })
	cancel()
	select {
	case <-b.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("batch not done after parent cancel")
	}
}

func TestPoolPrioritize(t *testing.T) {
	tn, gate, order := gatedThumbnailer(t)
	p := NewPool(tn, 1)
	defer p.Close()
	paths := fakeFiles(t, 10)
	b := p.Submit(context.Background(), paths, nil)
	waitFor(t, func() bool { return len(order()) == 1 })
	b.Prioritize(paths[9], paths[5], paths[0], "/not/queued", paths[9])
	close(gate)
	b.Wait()
	got := order()
	want := []string{paths[0], paths[9], paths[5], paths[1], paths[2]}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("order %v, want prefix %v", got, want)
		}
	}
	if len(got) != 10 {
		t.Errorf("%d jobs ran", len(got))
	}
	b.Prioritize() // no-op
}

func TestPoolTwoBatches(t *testing.T) {
	// Cancelling one folder's batch must not disturb another's.
	tn, gate, _ := gatedThumbnailer(t)
	p := NewPool(tn, 2)
	defer p.Close()
	a := p.Submit(context.Background(), fakeFiles(t, 6), nil)
	var mu sync.Mutex
	var idx []int
	bPaths := fakeFiles(t, 6)
	b := p.Submit(context.Background(), bPaths, func(r Result) {
		mu.Lock()
		idx = append(idx, r.Index)
		mu.Unlock()
	})
	a.Cancel()
	close(gate)
	a.Wait()
	b.Wait()
	sort.Ints(idx)
	if len(idx) != 6 {
		t.Errorf("batch b got %v", idx)
	}
}

func TestPoolClose(t *testing.T) {
	tn, gate, order := gatedThumbnailer(t)
	p := NewPool(tn, 1)
	b := p.Submit(context.Background(), fakeFiles(t, 5), nil)
	waitFor(t, func() bool { return len(order()) == 1 })
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(gate)
	}()
	p.Close()
	b.Wait()
	if n := len(order()); n != 1 {
		t.Errorf("%d jobs ran, want 1", n)
	}
	p.Close() // idempotent
	late := p.Submit(context.Background(), fakeFiles(t, 2), func(Result) { t.Error("callback on closed pool") })
	select {
	case <-late.Done():
	default:
		t.Error("batch on closed pool not done")
	}
}

func TestNewPoolDefaultWorkers(t *testing.T) {
	p := NewPool(newTestThumbnailer(t, Options{}), 0)
	b := p.Submit(context.Background(), nil, nil)
	b.Wait()
	p.Close()
}
