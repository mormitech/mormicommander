// This file defines the errors the FTP client returns. Servers answer every
// command with a three-digit code, and the UI needs that code to decide what
// to tell the user (a 4xx reply is worth retrying, a 5xx one is not, 530
// means the login was wrong), so failed commands come back as *Error with
// the code and the server's own text. A few situations that are not server
// replies get sentinel errors that callers can test with errors.Is.
package ftp

import (
	"errors"
	"fmt"
	"strings"
)

// Error is a negative or unexpected reply from the server.
type Error struct {
	Cmd  string // the command that failed, with passwords masked
	Code int    // the reply code, e.g. 550
	Msg  string // the server's reply text
}

func (e *Error) Error() string {
	return fmt.Sprintf("ftp: %s: %d %s", e.Cmd, e.Code, strings.TrimSpace(e.Msg))
}

// Temporary reports whether the reply was a transient (4xx) failure.
func (e *Error) Temporary() bool { return e.Code >= 400 && e.Code < 500 }

// Sentinel errors.
var (
	ErrClosed         = errors.New("ftp: connection closed")
	ErrNotSupported   = errors.New("ftp: not supported by the server")
	ErrActiveViaProxy = errors.New("ftp: active mode is not possible through a SOCKS or HTTP proxy")
	ErrResumeASCII    = errors.New("ftp: resuming is not possible in ASCII mode")
	// ErrResumeTooLarge means a resume was asked for but the existing
	// target is already larger than the source, so it cannot be a partial
	// copy of it. The target is left untouched.
	ErrResumeTooLarge  = errors.New("ftp: cannot resume: target is larger than the source")
	ErrLoginFailed     = errors.New("ftp: login failed")
	ErrTLSRefused      = errors.New("ftp: server refused TLS (AUTH TLS)")
	ErrBadLine         = errors.New("ftp: command or path contains a line break")
	ErrWrongMasterPass = errors.New("ftp: wrong master password")
	ErrLocked          = errors.New("ftp: passwords are locked; enter the master password")
)

// ReplyCode returns the FTP reply code carried by err, or 0.
func ReplyCode(err error) int {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return 0
}

// maskCmd hides the argument of commands that carry secrets so logs and
// error messages never show a password.
func maskCmd(line string) string {
	up := strings.ToUpper(line)
	for _, p := range []string{"PASS ", "ACCT "} {
		if strings.HasPrefix(up, p) {
			return line[:len(p)] + "****"
		}
	}
	return line
}

func protoErr(cmd string, r Reply) error {
	return &Error{Cmd: maskCmd(cmd), Code: r.Code, Msg: r.Msg}
}
