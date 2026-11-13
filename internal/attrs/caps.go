// What this platform can show and change. The attributes dialog is one dialog
// on three systems, and rather than hard-coding "if Windows" in the UI it asks
// Caps once and enables exactly the controls that will work: the Windows
// attribute checkboxes, the Unix permission grid, the owner and group boxes,
// the creation date field and the macOS hidden flag. The same answer drives
// the attribute column format (rahs on Windows, rwxr-xr-x elsewhere).
package attrs

import (
	"errors"
	"runtime"
)

// ErrUnsupported is returned when a change is not possible on this platform,
// for example setting the creation date on Linux or permissions on Windows.
var ErrUnsupported = errors.New("not supported on this platform")

// Capabilities tells the UI which controls of the attributes dialog work.
type Capabilities struct {
	// SettableAttrs are the Windows-style attributes that can be changed.
	// On Unix read-only maps to the write bits and, on macOS, hidden maps to
	// the Finder hidden flag.
	SettableAttrs WinAttr
	// ReportedAttrs are the attributes that are shown (read only or not).
	ReportedAttrs WinAttr
	// NativeAttrs is true on Windows, where attributes are the real thing.
	NativeAttrs bool
	UnixPerms   bool // rwx grid, octal field and special bits
	Owner       bool // owner and group can be shown and changed
	CreatedTime bool // the creation date can be set
	ReadCreated bool // the creation date can at least be shown
	HiddenFlag  bool // macOS UF_HIDDEN flag
}

// Caps returns the capabilities of the platform the program runs on.
func Caps() Capabilities { return platformCaps() }

// ColumnString returns the text for the panel's attribute column: TC's
// four-letter form on Windows ("-a--"), the ls form elsewhere ("drwxr-xr-x").
func ColumnString(info Info) string {
	if info.WinNative {
		return info.Win.String()
	}
	if runtime.GOOS == "windows" {
		return info.Win.String()
	}
	return FormatMode(info.Mode)
}
