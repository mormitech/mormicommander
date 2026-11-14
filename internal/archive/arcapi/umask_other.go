//go:build !unix

// Windows (and any other system without Unix permissions) has no umask; the
// mode bits stored in archives only decide the read-only flag there, so
// nothing is masked. See umask_unix.go for the Unix side.
package arcapi

import "io/fs"

var processUmask fs.FileMode
