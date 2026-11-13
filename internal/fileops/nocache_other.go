//go:build !darwin && !linux

package fileops

// On Windows and the other systems the copy doesn't change caching: Windows would need unbuffered I/O with aligned
// buffers (FILE_FLAG_NO_BUFFERING), which isn't worth its complexity here. The large buffer of TC's "big file" mode
// still applies, only the cache hint is left out.

import "os"

func noCacheStart(f *os.File) {}
func noCacheEnd(f *os.File)   {}
