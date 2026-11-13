// Tests for the Thumbnailer: cache use, invalidation on change, the system
// thumbnailer fallback (with a fake), remembered failures and the
// de-duplication of concurrent requests for the same file.

package thumbs

import (
	"context"
	"errors"
	"image"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestThumbnailer(t *testing.T, opt Options) *Thumbnailer {
	t.Helper()
	c, err := OpenCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	th := New(opt, c)
	th.SetSystem(nil)
	return th
}

func TestThumbnailerCaches(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.png", encodeAs(t, "png", halves(200, 100)))
	tn := newTestThumbnailer(t, Options{Width: 50, Height: 50})
	ctx := context.Background()
	a, err := tn.Thumbnail(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if a.FromCache {
		t.Error("first call from cache")
	}
	b, err := tn.Thumbnail(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !b.FromCache || b.Width != a.Width || b.Meta != a.Meta {
		t.Errorf("second call: %+v", b)
	}
	// Changing the file invalidates the entry.
	if err := os.WriteFile(p, encodeAs(t, "png", halves(100, 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	os.Chtimes(p, future, future)
	c, err := tn.Thumbnail(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if c.FromCache || c.Width != 25 || c.Height != 50 {
		t.Errorf("after change: fromCache=%v %dx%d", c.FromCache, c.Width, c.Height)
	}
	// A different size is a different entry.
	tn.SetSize(100, 100)
	d, err := tn.Thumbnail(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if d.FromCache || d.Height != 100 {
		t.Errorf("after resize: fromCache=%v %dx%d", d.FromCache, d.Width, d.Height)
	}
	if tn.Options().Width != 100 || tn.Cache() == nil {
		t.Error("options/cache accessors")
	}
}

func TestThumbnailerNoCache(t *testing.T) {
	p := writeFile(t, t.TempDir(), "a.bmp", encodeAs(t, "bmp", halves(40, 40)))
	tn := New(Options{}, nil)
	tn.SetSystem(nil)
	for i := 0; i < 2; i++ {
		th, err := tn.Thumbnail(context.Background(), p)
		if err != nil || th.FromCache {
			t.Fatalf("call %d: %v %v", i, th, err)
		}
	}
}

func TestThumbnailerErrors(t *testing.T) {
	dir := t.TempDir()
	tn := newTestThumbnailer(t, Options{UseSystem: true})
	ctx := context.Background()
	if _, err := tn.Thumbnail(ctx, dir); !errors.Is(err, ErrIsDir) {
		t.Errorf("dir: %v", err)
	}
	txt := writeFile(t, dir, "a.txt", []byte("hello"))
	if _, err := tn.Thumbnail(ctx, txt); !errors.Is(err, ErrUnsupported) {
		t.Errorf("txt: %v", err)
	}
	broken := writeFile(t, dir, "b.png", encodeAs(t, "png", halves(50, 50))[:45])
	_, err := tn.Thumbnail(ctx, broken)
	if !errors.Is(err, ErrDecode) || IsCachedFailure(err) {
		t.Errorf("broken first: %v", err)
	}
	_, err = tn.Thumbnail(ctx, broken)
	if !errors.Is(err, ErrDecode) || !IsCachedFailure(err) {
		t.Errorf("broken second should be cached: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := tn.Thumbnail(cctx, txt); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

func TestThumbnailerSystemFallback(t *testing.T) {
	dir := t.TempDir()
	heic := writeFile(t, dir, "photo.heic", []byte("not really heic"))
	video := writeFile(t, dir, "clip.mov", []byte("not really a movie"))
	var calls atomic.Int32
	tn := newTestThumbnailer(t, Options{Width: 64, Height: 64, UseSystem: true})
	tn.SetSystem(func(ctx context.Context, path string, w, h int) (image.Image, error) {
		calls.Add(1)
		if path == video {
			return nil, errors.New("no plug-in")
		}
		if w != 64 || h != 64 {
			t.Errorf("system asked for %dx%d", w, h)
		}
		return halves(128, 64), nil
	})
	ctx := context.Background()
	th, err := tn.Thumbnail(ctx, heic)
	if err != nil {
		t.Fatal(err)
	}
	if th.Meta.Format != "system" || th.Width != 64 || th.Height != 32 || th.Meta.Width != 0 {
		t.Errorf("system thumb %+v", th)
	}
	if th2, err := tn.Thumbnail(ctx, heic); err != nil || !th2.FromCache {
		t.Errorf("system thumb not cached: %v %v", th2, err)
	}
	for i := 0; i < 2; i++ {
		_, err := tn.Thumbnail(ctx, video)
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("video %d: %v", i, err)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("system called %d times, want 2 (one per file)", n)
	}
	// With UseSystem off the fake is never consulted.
	tn.SetOptions(Options{Width: 64, Height: 64})
	calls.Store(0)
	other := writeFile(t, dir, "other.heic", []byte("x"))
	if _, err := tn.Thumbnail(ctx, other); !errors.Is(err, ErrUnsupported) || calls.Load() != 0 {
		t.Errorf("UseSystem=false: %v, calls %d", err, calls.Load())
	}
}

func TestThumbnailerDeduplicates(t *testing.T) {
	p := writeFile(t, t.TempDir(), "x.heic", []byte("x"))
	tn := newTestThumbnailer(t, Options{UseSystem: true})
	var calls atomic.Int32
	release := make(chan struct{})
	tn.SetSystem(func(ctx context.Context, path string, w, h int) (image.Image, error) {
		calls.Add(1)
		<-release
		return halves(10, 10), nil
	})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tn.Thumbnail(context.Background(), p)
			errs <- err
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("generated %d times, want 1", n)
	}
}

func TestThumbnailerLeaderCancelled(t *testing.T) {
	p := writeFile(t, t.TempDir(), "x.heic", []byte("x"))
	tn := newTestThumbnailer(t, Options{UseSystem: true})
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	tn.SetSystem(func(ctx context.Context, path string, w, h int) (image.Image, error) {
		if calls.Add(1) == 1 {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return halves(10, 10), nil
	})
	lctx, cancel := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := tn.Thumbnail(lctx, p)
		leaderErr <- err
	}()
	<-started
	followerErr := make(chan error, 1)
	go func() {
		_, err := tn.Thumbnail(context.Background(), p)
		followerErr <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Errorf("leader: %v", err)
	}
	if err := <-followerErr; err != nil {
		t.Errorf("follower should retry and succeed: %v", err)
	}
}
