// This file ties generation, the disk cache and the operating system's
// thumbnailer together into the one call the thumbnails view needs: "give me
// the preview for this path". It looks in the cache first, decodes natively
// supported images itself, asks the system thumbnailer for everything else
// (HEIC, PDF, video on macOS), stores results and permanent failures in the
// cache, and makes sure two requests for the same file at the same time
// (say, the panel and a prefetch) only do the work once.

package thumbs

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
)

// SystemFunc produces a preview image for a file this package cannot decode,
// at roughly w x h pixels. It returns ErrUnsupported when it has nothing.
type SystemFunc func(ctx context.Context, path string, w, h int) (image.Image, error)

// DefaultSystem is the platform's own system thumbnailer (QuickLook on macOS,
// the freedesktop cache on Linux), or nil where there is none.
func DefaultSystem() SystemFunc { return systemThumbnailer() }

// Chain tries each non-nil SystemFunc in turn and returns the first picture.
// A cancelled context stops the chain; when all fail, the last error is
// returned (ErrUnsupported when there was nothing to try).
func Chain(fs ...SystemFunc) SystemFunc {
	var list []SystemFunc
	for _, f := range fs {
		if f != nil {
			list = append(list, f)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return func(ctx context.Context, path string, w, h int) (image.Image, error) {
		err := error(ErrUnsupported)
		for _, f := range list {
			var img image.Image
			if img, err = f(ctx, path, w, h); err == nil && img != nil {
				return img, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		if err == nil {
			err = ErrUnsupported
		}
		return nil, err
	}
}

// Thumbnailer produces thumbnails with caching. It is safe for concurrent
// use; the Pool in pool.go runs many Thumbnail calls in parallel.
type Thumbnailer struct {
	mu    sync.Mutex
	opt   Options
	cache *Cache
	sys   SystemFunc

	inflight map[Key]*call
}

type call struct {
	done chan struct{}
	t    *Thumb
	err  error
}

// New returns a Thumbnailer with the given options. cache may be nil to
// disable disk caching. The platform's system thumbnailer is used when
// opt.UseSystem is set.
func New(opt Options, cache *Cache) *Thumbnailer {
	return &Thumbnailer{opt: opt.normalized(), cache: cache, sys: systemThumbnailer(), inflight: map[Key]*call{}}
}

// Options returns the current options (normalized).
func (t *Thumbnailer) Options() Options {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.opt
}

// SetOptions changes the options, e.g. when the user moves the thumbnail
// size slider. Cached thumbnails of other sizes stay valid for later.
func (t *Thumbnailer) SetOptions(opt Options) {
	t.mu.Lock()
	t.opt = opt.normalized()
	t.mu.Unlock()
}

// SetSize is a shortcut for changing only the box size.
func (t *Thumbnailer) SetSize(w, h int) {
	t.mu.Lock()
	t.opt.Width, t.opt.Height = ClampSize(w), ClampSize(h)
	t.mu.Unlock()
}

// SetSystem replaces the system thumbnailer (nil disables it). Tests use it
// to inject a fake; the UI never needs it.
func (t *Thumbnailer) SetSystem(f SystemFunc) {
	t.mu.Lock()
	t.sys = f
	t.mu.Unlock()
}

// Cache returns the disk cache, or nil.
func (t *Thumbnailer) Cache() *Cache { return t.cache }

// Thumbnail returns the thumbnail for path, from the cache when possible.
// Directories return ErrIsDir; files without a preview return an error that
// matches ErrUnsupported, ErrDecode or ErrTooLarge, and the UI then shows
// IconFor(path) instead.
func (t *Thumbnailer) Thumbnail(ctx context.Context, path string) (*Thumb, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, ErrIsDir
	}
	t.mu.Lock()
	opt, sys := t.opt, t.sys
	t.mu.Unlock()
	k := keyFromInfo(abs, fi, opt)

	for {
		t.mu.Lock()
		if c, ok := t.inflight[k]; ok {
			t.mu.Unlock()
			select {
			case <-c.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			// The leader may have been cancelled while we are still wanted:
			// in that case loop and do the work ourselves.
			if isCtxErr(c.err) && ctx.Err() == nil {
				continue
			}
			return c.t, c.err
		}
		c := &call{done: make(chan struct{})}
		t.inflight[k] = c
		t.mu.Unlock()

		c.t, c.err = t.produce(ctx, k, abs, opt, sys)

		t.mu.Lock()
		delete(t.inflight, k)
		t.mu.Unlock()
		close(c.done)
		return c.t, c.err
	}
}

func isCtxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// produce does the actual cache lookup / generation for one key.
func (t *Thumbnailer) produce(ctx context.Context, k Key, abs string, opt Options, sys SystemFunc) (*Thumb, error) {
	if t.cache != nil {
		if th, ok, err := t.cache.Get(k); ok {
			return th, err
		}
	}
	th, err := Generate(ctx, abs, opt)
	if errors.Is(err, ErrUnsupported) && opt.UseSystem && sys != nil {
		th, err = t.fromSystem(ctx, abs, opt, sys)
		if errors.Is(err, ErrUnsupported) && t.cache != nil && !isCtxErr(err) {
			// The system thumbnailer is expensive (it may launch a helper
			// process), so remember that it had nothing for this file.
			_ = t.cache.PutFailure(k, err)
		}
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		if t.cache != nil && !errors.Is(err, ErrUnsupported) {
			// Unsupported files are cheap to re-sniff; broken or huge
			// images are not, so only those failures are remembered.
			_ = t.cache.PutFailure(k, err)
		}
		return nil, err
	}
	if t.cache != nil {
		_ = t.cache.Put(k, th)
	}
	return th, nil
}

func (t *Thumbnailer) fromSystem(ctx context.Context, abs string, opt Options, sys SystemFunc) (*Thumb, error) {
	img, err := sys(ctx, abs, opt.Width, opt.Height)
	if err != nil {
		if isCtxErr(err) {
			return nil, err
		}
		if !errors.Is(err, ErrUnsupported) {
			err = fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return nil, err
	}
	return makeThumb(img, Meta{Format: "system", Orientation: 1}, opt)
}
