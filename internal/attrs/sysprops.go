// Opening the operating system's own properties window for a file (F-682).
// mormiCommander has its own cross-platform Properties dialog, but users
// sometimes need the native one: Explorer's Properties sheet with its
// Security and Previous Versions tabs on Windows, Finder's Get Info window on
// macOS, and the file manager's properties dialog on Linux desktops. The
// platform files provide openSystemProperties; this file holds the shared
// entry point and the command builders, which are plain functions so they
// can be tested without starting anything.
package attrs

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenSystemProperties shows the native properties window for path. It
// returns as soon as the window has been asked for.
func OpenSystemProperties(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return openSystemProperties(abs)
}

// finderInfoCommand builds the osascript call that opens Finder's Get Info
// window. The path is passed as an argument rather than pasted into the
// script, so quotes or backslashes in file names cannot break it.
func finderInfoCommand(path string) *exec.Cmd {
	return exec.Command("osascript",
		"-e", "on run argv",
		"-e", "tell application \"Finder\"",
		"-e", "activate",
		"-e", "open information window of (POSIX file (item 1 of argv) as alias)",
		"-e", "end tell",
		"-e", "end run",
		path)
}

// fileManagerPropertiesCommand builds the freedesktop.org FileManager1 D-Bus
// call that Nautilus, Dolphin, Nemo, Caja and Thunar understand. dbus-send
// splits array items on commas, so commas in the URI are percent-encoded.
func fileManagerPropertiesCommand(path string) *exec.Cmd {
	u := (&url.URL{Scheme: "file", Path: path}).String()
	u = strings.ReplaceAll(u, ",", "%2C")
	return exec.Command("dbus-send", "--session", "--print-reply",
		"--dest=org.freedesktop.FileManager1", "--type=method_call",
		"/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1.ShowItemProperties",
		"array:string:"+u, "string:")
}

// startDetached starts a helper and reaps it in the background so it never
// lingers as a zombie.
func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
