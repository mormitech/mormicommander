// On Unix-like systems media fields open files with O_NONBLOCK. For a regular
// file the flag changes nothing, but if a named pipe slipped past the type
// check (it was swapped in after the check), the open returns at once instead
// of waiting forever for a writer, and the type check after opening rejects it.

//go:build unix

package listing

import "syscall"

const openNonblock = syscall.O_NONBLOCK
