// Windows process start-up. Windows programs receive one raw command line instead of an argument list, so the
// command line is built here from the program path and the raw parameter text, and handed over verbatim through
// SysProcAttr.CmdLine; Go's own argument escaping would break programs such as rundll32, explorer and cmd that parse
// their command line in their own way. HideWindow keeps the console of helper programs from flashing on the screen,
// and CREATE_NEW_CONSOLE gives console programs started from the command line a window of their own, as Total
// Commander does. Minimized, maximized and "run as administrator" starts go through ShellExecute, the only API that
// can request them.
package launch

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNewConsole      = 0x00000010
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// Hide keeps the window of a helper program (PowerShell, cmd, rundll32) from appearing. Code outside this package
// that starts its own helper processes should call it too.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func applyPlatform(cmd *exec.Cmd, c Command, detached bool) {
	attr := &syscall.SysProcAttr{}
	if c.Params != "" {
		attr.CmdLine = QuoteWindowsArg(c.Path) + " " + c.Params
	}
	switch {
	case c.Show == ShowHidden:
		attr.HideWindow = true
		attr.CreationFlags |= createNoWindow
	case c.NewConsole:
		attr.CreationFlags |= createNewConsole
	}
	if detached {
		attr.CreationFlags |= createNewProcessGroup
	}
	cmd.SysProcAttr = attr
}

// shellExecute handles the starts plain CreateProcess cannot do: a window state other than normal or hidden, and a
// verb such as "runas".
func shellExecute(c Command) (bool, error) {
	if c.Verb == "" && c.Show != ShowMinimized && c.Show != ShowMaximized {
		return false, nil
	}
	show := int32(windows.SW_SHOWNORMAL)
	switch c.Show {
	case ShowMinimized:
		show = windows.SW_SHOWMINNOACTIVE
	case ShowMaximized:
		show = windows.SW_SHOWMAXIMIZED
	case ShowHidden:
		show = windows.SW_HIDE
	}
	verb := c.Verb
	if verb == "" {
		verb = "open"
	}
	ptr := func(s string) *uint16 {
		if s == "" {
			return nil
		}
		p, _ := windows.UTF16PtrFromString(s)
		return p
	}
	return true, windows.ShellExecute(0, ptr(verb), ptr(c.Path), ptr(c.Params), ptr(c.Dir), show)
}

// shortName returns the old 8.3 form of a path (Total Commander's lowercase placeholders such as %n), or the path
// itself when the volume has no short names.
func shortName(path string) string {
	long, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return path
	}
	return windows.UTF16ToString(buf[:n])
}
