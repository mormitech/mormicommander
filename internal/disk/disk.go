// Package disk lists the drives and volumes for the drive bar and the drive popup (Alt+F1 / Alt+F2), and tells the
// free space of the volume a folder is on (the drive info line above each panel). It has no UI dependencies, so it
// cross-compiles and tests on its own. Platform parts live in volumes_<os>.go and usage_*.go.
package disk

import (
	"context"
	"errors"
	"sync"
	"time"

	"mormicommander/internal/hang"
)

// Volume is one drive or mounted volume.
type Volume struct {
	Name  string `json:"name"`            // what the user calls it: "Macintosh HD", "USB stick", "C:"
	Path  string `json:"path"`            // where it is mounted: "/", "/Volumes/USB stick", `C:\`
	Label string `json:"label"`           // the volume label where the system has one (Windows), else ""
	Kind  string `json:"kind"`            // "fixed", "removable", "network", "cdrom" or "home"
	Total uint64 `json:"total"`           // bytes
	Free  uint64 `json:"free"`            // bytes the user can still write
	Stale bool   `json:"stale,omitempty"` // the volume did not answer in time (a share whose server is gone): no sizes
}

// Usage is the size and free space of the volume a path lives on.
type Usage struct {
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

// probeTimeout bounds how long the drive list and the drive info line wait for one volume. A network share whose server
// is gone, or a disk that was pulled out without ejecting, can block statfs for minutes; such a volume is listed as
// Stale (no sizes) instead of holding up the drive bar. Windows has its own copy of this rule in volumes_windows.go.
const probeTimeout = 1500 * time.Millisecond

// probes keeps at most a few calls per path stuck at once (see internal/hang): asking a dead share again while the
// last question is still hanging answers "not responding" at once.
var probes hang.Guard

// probe runs fn for the volume at key, giving up after probeTimeout with hang.ErrNotResponding.
func probe[T any](key string, fn func() (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	v, err := hang.Guarded(&probes, ctx, key, func(context.Context) (T, error) { return fn() })
	if errors.Is(err, context.DeadlineExceeded) {
		err = hang.ErrNotResponding
	}
	return v, err
}

// UsageOfQuick is UsageOf that never waits longer than probeTimeout: for a volume that does not answer it returns
// hang.ErrNotResponding.
func UsageOfQuick(path string) (Usage, error) {
	return probe(path, func() (Usage, error) { return UsageOf(path) })
}

// probeEach runs fn for every key at the same time, each bounded by probe, so a list of mounts takes at most about
// probeTimeout however many of them are dead. The answers come back in the order of keys.
func probeEach[T any](keys []string, fn func(string) (T, error)) ([]T, []error) {
	vals := make([]T, len(keys))
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vals[i], errs[i] = probe(k, func() (T, error) { return fn(k) })
		}()
	}
	wg.Wait()
	return vals, errs
}
