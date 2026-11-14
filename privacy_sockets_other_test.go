//go:build !darwin && !linux

// The socket sampler of the run-time zero-network test (privacy_session_test.go) on systems where it is not built
// (Windows): there the test relies on the recording HTTP transport and resolver, plus privacy_test.go's reading of the
// source.
package main

// socketsWatched says that this system does not sample sockets.
const socketsWatched = false

// watchSockets does nothing here.
func watchSockets(w *netWatch) func() { return func() {} }
