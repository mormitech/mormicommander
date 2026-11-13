//go:build !linux && !windows && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

// Fallback for platforms without a native backend (for example Solaris, Plan 9 or js/wasm). Starting a native watch
// fails with a clear error, and the watcher then polls the folder instead, so automatic refresh still works, only
// with up to PollInterval of delay.
package fswatch

import "errors"

func startNativeBackend(string, Options, func(raw)) (backend, error) {
	return nil, errors.New("no native change notifications on this platform")
}
