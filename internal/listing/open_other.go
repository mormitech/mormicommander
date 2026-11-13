// On systems without Unix named-pipe semantics (Windows and others) there is
// no non-blocking open flag to add; the regular-file checks around the open
// are enough there, because opening a file never waits for another process.

//go:build !unix

package listing

const openNonblock = 0
