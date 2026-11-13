package robust

import (
	"errors"
	"syscall"
)

// Windows error codes that mean "someone else has the file open right now".
const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
)

func isTransient(err error) bool {
	var e syscall.Errno
	if !errors.As(err, &e) {
		return false
	}
	return e == errorAccessDenied || e == errorSharingViolation || e == errorLockViolation
}
